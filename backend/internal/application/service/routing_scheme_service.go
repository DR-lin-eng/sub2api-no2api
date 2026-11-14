package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

var (
	ErrRoutingSchemeNotFound = infraerrors.NotFound("ROUTING_SCHEME_NOT_FOUND", "routing scheme not found")
	ErrRoutingSchemeInvalid  = infraerrors.BadRequest("ROUTING_SCHEME_INVALID", "routing scheme is invalid")
)

const (
	maxRoutingSchemeItems = 20
	maxRoutingSchemeRules = 20
)

// RoutingScheme is the admin-facing scheme aggregate.
type RoutingScheme struct {
	ID          int64
	Name        string
	Description string
	Platform    string
	Status      string
	SortOrder   int
	// TTFTWeight and ErrorWeight trade speed against stability. Only their
	// ratio matters; the scheduler normalizes them into a weighted average.
	TTFTWeight  int
	ErrorWeight int
	Items       []RoutingSchemeItem
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RoutingSchemeItem binds one group to a scheme and carries its failover rules.
type RoutingSchemeItem struct {
	GroupID        int64
	GroupName      string
	Platform       string
	RateMultiplier float64
	Position       int
	Rules          []domain.RoutingSchemeRule
}

// RoutingSchemeInput is the write payload shared by create and update.
type RoutingSchemeInput struct {
	Name        string
	Description string
	Platform    string
	Status      string
	SortOrder   int
	// A zero/zero pair means "use the defaults" so older clients that omit the
	// fields keep the previous 40/60 behavior.
	TTFTWeight  int
	ErrorWeight int
	Items       []RoutingSchemeItemInput
}

type RoutingSchemeItemInput struct {
	GroupID int64
	Rules   []domain.RoutingSchemeRule
}

// AvailableRoutingScheme is the user-facing projection. It must never contain
// scheme groups, weights or rule parameters.
type AvailableRoutingScheme struct {
	ID              int64
	Name            string
	Description     string
	Platform        string
	MinRate         float64
	MaxRate         float64
	TotalGroupCount int
}

// RoutingSchemeRepository persists schemes and their items.
type RoutingSchemeRepository interface {
	List(ctx context.Context, platform string, includeDisabled bool) ([]RoutingScheme, error)
	GetByID(ctx context.Context, id int64) (*RoutingScheme, error)
	Create(ctx context.Context, scheme *RoutingScheme) error
	Update(ctx context.Context, scheme *RoutingScheme) error
	Delete(ctx context.Context, id int64) error
	// ListSchemeIDsByGroupID returns the live schemes that still reference the
	// group, so a group change can refresh the keys routed through them.
	ListSchemeIDsByGroupID(ctx context.Context, groupID int64) ([]int64, error)
}

// RoutingSchemeService owns validation, admin CRUD and the user-facing
// available-scheme projection.
type RoutingSchemeService struct {
	repo      RoutingSchemeRepository
	groupRepo GroupRepository
	metrics   RoutingSchemeMetricsRepository

	metricsMu    sync.Mutex
	metricsCache map[int64]routingMetricsCacheEntry

	cooldownMu    sync.Mutex
	cooldownUntil map[string]time.Time

	authCacheInvalidator RoutingSchemeAuthCacheInvalidator
}

// RoutingSchemeAuthCacheInvalidator drops cached auth snapshots for keys that
// reference a changed scheme.
type RoutingSchemeAuthCacheInvalidator interface {
	InvalidateAuthCacheByRoutingScheme(ctx context.Context, schemeID int64)
}

func NewRoutingSchemeService(repo RoutingSchemeRepository, groupRepo GroupRepository, metrics RoutingSchemeMetricsRepository) *RoutingSchemeService {
	return &RoutingSchemeService{repo: repo, groupRepo: groupRepo, metrics: metrics}
}

// SetAuthCacheInvalidator wires the API key cache invalidator after both
// services exist.
func (s *RoutingSchemeService) SetAuthCacheInvalidator(invalidator RoutingSchemeAuthCacheInvalidator) {
	s.authCacheInvalidator = invalidator
}

func (s *RoutingSchemeService) List(ctx context.Context, platform string, includeDisabled bool) ([]RoutingScheme, error) {
	schemes, err := s.repo.List(ctx, strings.TrimSpace(platform), includeDisabled)
	if err != nil {
		return nil, err
	}
	s.hydrateItemGroups(ctx, schemes)
	return schemes, nil
}

func (s *RoutingSchemeService) Get(ctx context.Context, id int64) (*RoutingScheme, error) {
	if id <= 0 {
		return nil, ErrRoutingSchemeNotFound
	}
	scheme, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	items := []RoutingScheme{*scheme}
	s.hydrateItemGroups(ctx, items)
	*scheme = items[0]
	return scheme, nil
}

// RoutingSchemeAnalyticsGroup is one group's traffic inside a scheme window.
type RoutingSchemeAnalyticsGroup struct {
	GroupID        int64
	GroupName      string
	Platform       string
	RateMultiplier float64
	Requests       int64
	Successes      int64
	// Failures counts provider-owned failed attempts, not failed requests: one
	// request may fail over several times. It is shown separately and must not
	// be used as the denominator of the success rate.
	Failures     int64
	SuccessRate  float64
	P95TTFTMs    float64
	InputTokens  int64
	OutputTokens int64
	Cost         float64
	RequestShare float64
	CostShare    float64
}

// RoutingSchemeAnalytics is the admin panel projection for one scheme.
type RoutingSchemeAnalytics struct {
	SchemeID     int64
	Window       string
	Since        time.Time
	Requests     int64
	Cost         float64
	InputTokens  int64
	OutputTokens int64
	Groups       []RoutingSchemeAnalyticsGroup
}

// Analytics aggregates traffic and spend for the keys that opted into a scheme.
// Only usage attributed to that scheme is counted, so shared groups are not
// inflated by manual keys.
func (s *RoutingSchemeService) Analytics(ctx context.Context, schemeID int64, window string) (*RoutingSchemeAnalytics, error) {
	scheme, err := s.Get(ctx, schemeID)
	if err != nil {
		return nil, err
	}
	duration, windowKey := routingAnalyticsWindow(window)
	since := time.Now().Add(-duration)
	usage := map[int64]RoutingSchemeGroupUsage{}
	if s.metrics != nil {
		usage, err = s.metrics.GetSchemeGroupUsage(ctx, schemeID, since)
		if err != nil {
			return nil, err
		}
	}

	analytics := &RoutingSchemeAnalytics{
		SchemeID: scheme.ID,
		Window:   windowKey,
		Since:    since,
		Groups:   make([]RoutingSchemeAnalyticsGroup, 0, len(scheme.Items)),
	}
	for _, item := range scheme.Items {
		row := usage[item.GroupID]
		analytics.Groups = append(analytics.Groups, RoutingSchemeAnalyticsGroup{
			GroupID:        item.GroupID,
			GroupName:      item.GroupName,
			Platform:       item.Platform,
			RateMultiplier: item.RateMultiplier,
			Requests:       row.Requests,
			Successes:      row.Successes,
			Failures:       row.Failures,
			P95TTFTMs:      row.P95TTFTMs,
			InputTokens:    row.InputTokens,
			OutputTokens:   row.OutputTokens,
			Cost:           row.Cost,
		})
		analytics.Requests += row.Requests
		analytics.Cost += row.Cost
		analytics.InputTokens += row.InputTokens
		analytics.OutputTokens += row.OutputTokens
	}
	for i := range analytics.Groups {
		if analytics.Requests > 0 {
			analytics.Groups[i].RequestShare = float64(analytics.Groups[i].Requests) / float64(analytics.Requests) * 100
		}
		// Success rate uses the request count as the denominator. Provider
		// failures come from attempt-level error logs, so mixing them into the
		// denominator would understate healthy traffic after failovers.
		if analytics.Groups[i].Requests > 0 {
			analytics.Groups[i].SuccessRate = float64(analytics.Groups[i].Successes) / float64(analytics.Groups[i].Requests) * 100
		}
		if analytics.Cost > 0 {
			analytics.Groups[i].CostShare = analytics.Groups[i].Cost / analytics.Cost * 100
		}
	}
	sort.SliceStable(analytics.Groups, func(i, j int) bool {
		if analytics.Groups[i].Requests != analytics.Groups[j].Requests {
			return analytics.Groups[i].Requests > analytics.Groups[j].Requests
		}
		return analytics.Groups[i].Cost > analytics.Groups[j].Cost
	})
	return analytics, nil
}

func routingAnalyticsWindow(window string) (time.Duration, string) {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case "7d":
		return 7 * 24 * time.Hour, "7d"
	case "30d":
		return 30 * 24 * time.Hour, "30d"
	default:
		return 24 * time.Hour, "24h"
	}
}

// groupBatchLoader is an optional capability on GroupRepository. The production
// repository implements it, so expanding a scheme costs one query; lightweight
// test doubles keep working through the per-id fallback.
type groupBatchLoader interface {
	GetByIDs(ctx context.Context, ids []int64) ([]Group, error)
}

func loadGroupsByIDs(ctx context.Context, repo GroupRepository, ids []int64) map[int64]*Group {
	out := make(map[int64]*Group, len(ids))
	if len(ids) == 0 {
		return out
	}
	if loader, ok := repo.(groupBatchLoader); ok {
		groups, err := loader.GetByIDs(ctx, ids)
		if err != nil {
			return out
		}
		for i := range groups {
			out[groups[i].ID] = &groups[i]
		}
		return out
	}
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		group, err := repo.GetByID(ctx, id)
		if err != nil || group == nil {
			continue
		}
		out[id] = group
	}
	return out
}

// hydrateItemGroups fills the admin-facing group projection (name, platform
// and rate) that is not persisted on the scheme item itself.
func (s *RoutingSchemeService) hydrateItemGroups(ctx context.Context, schemes []RoutingScheme) {
	if s.groupRepo == nil || len(schemes) == 0 {
		return
	}
	groupIDs := make([]int64, 0, len(schemes))
	for i := range schemes {
		for j := range schemes[i].Items {
			groupIDs = append(groupIDs, schemes[i].Items[j].GroupID)
		}
	}
	groupByID := loadGroupsByIDs(ctx, s.groupRepo, groupIDs)
	for i := range schemes {
		for j := range schemes[i].Items {
			group := groupByID[schemes[i].Items[j].GroupID]
			if group == nil {
				continue
			}
			schemes[i].Items[j].GroupName = group.Name
			schemes[i].Items[j].Platform = group.Platform
			schemes[i].Items[j].RateMultiplier = group.RateMultiplier
		}
	}
}

func (s *RoutingSchemeService) Create(ctx context.Context, input RoutingSchemeInput) (*RoutingScheme, error) {
	scheme, err := s.validateInput(ctx, 0, input)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func (s *RoutingSchemeService) Update(ctx context.Context, id int64, input RoutingSchemeInput) (*RoutingScheme, error) {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// Platform is immutable after creation: changing it would silently move
	// keys to unrelated groups.
	input.Platform = existing.Platform
	scheme, err := s.validateInput(ctx, id, input)
	if err != nil {
		return nil, err
	}
	scheme.CreatedAt = existing.CreatedAt
	if err := s.repo.Update(ctx, scheme); err != nil {
		return nil, err
	}
	s.ClearSchemeRuntimeState(id)
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByRoutingScheme(ctx, id)
	}
	return scheme, nil
}

func (s *RoutingSchemeService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrRoutingSchemeNotFound
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.ClearSchemeRuntimeState(id)
	if s.authCacheInvalidator != nil {
		s.authCacheInvalidator.InvalidateAuthCacheByRoutingScheme(ctx, id)
	}
	return nil
}

// SchemeIDsByGroupID returns the live schemes that contain the group. Group
// edits use it to refresh keys that are routed through a scheme instead of a
// direct group binding.
func (s *RoutingSchemeService) SchemeIDsByGroupID(ctx context.Context, groupID int64) ([]int64, error) {
	if s == nil || s.repo == nil || groupID <= 0 {
		return nil, nil
	}
	return s.repo.ListSchemeIDsByGroupID(ctx, groupID)
}

// AvailableForUser returns scheme metadata for the key editor. Groups that the
// user cannot bind are dropped before computing the rate range; schemes with no
// bindable group are not returned.
func (s *RoutingSchemeService) AvailableForUser(ctx context.Context, user *User, platform string) ([]AvailableRoutingScheme, error) {
	schemes, err := s.repo.List(ctx, strings.TrimSpace(platform), false)
	if err != nil {
		return nil, err
	}
	// Batch-load every referenced group once. Both the key list and the key
	// editor expand all schemes on each load, so a per-item lookup made the
	// request cost grow with schemes x items.
	groupIDs := make([]int64, 0, len(schemes))
	for _, scheme := range schemes {
		for _, item := range scheme.Items {
			groupIDs = append(groupIDs, item.GroupID)
		}
	}
	groupByID := loadGroupsByIDs(ctx, s.groupRepo, groupIDs)
	out := make([]AvailableRoutingScheme, 0, len(schemes))
	for _, scheme := range schemes {
		if scheme.Status != domain.RoutingSchemeStatusActive {
			continue
		}
		rates := make([]float64, 0, len(scheme.Items))
		for _, item := range scheme.Items {
			group := groupByID[item.GroupID]
			if group == nil || !group.IsActive() {
				continue
			}
			if user != nil && !group.IsSubscriptionType() && !user.CanBindGroup(group.ID, group.IsExclusive) {
				continue
			}
			rates = append(rates, group.RateMultiplier)
		}
		if len(rates) == 0 {
			continue
		}
		minRate, maxRate := rates[0], rates[0]
		for _, rate := range rates[1:] {
			if rate < minRate {
				minRate = rate
			}
			if rate > maxRate {
				maxRate = rate
			}
		}
		out = append(out, AvailableRoutingScheme{
			ID:              scheme.ID,
			Name:            scheme.Name,
			Description:     scheme.Description,
			Platform:        scheme.Platform,
			MinRate:         minRate,
			MaxRate:         maxRate,
			TotalGroupCount: len(scheme.Items),
		})
	}
	return out, nil
}

// APIKeyRoutingSchemeSummary is the read-only projection attached to an API
// Key. It intentionally omits groups, weights and rules.
type APIKeyRoutingSchemeSummary struct {
	ID          int64
	Name        string
	Description string
	Platform    string
	MinRate     float64
	MaxRate     float64
}

// SummariesForUser resolves the schemes referenced by a user's keys. Schemes
// the user can no longer bind are omitted so the key list degrades gracefully.
func (s *RoutingSchemeService) SummariesForUser(ctx context.Context, user *User, ids []int64) map[int64]APIKeyRoutingSchemeSummary {
	if len(ids) == 0 {
		return nil
	}
	wanted := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			wanted[id] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	available, err := s.AvailableForUser(ctx, user, "")
	if err != nil {
		return nil
	}
	out := make(map[int64]APIKeyRoutingSchemeSummary, len(available))
	for _, scheme := range available {
		if _, ok := wanted[scheme.ID]; !ok {
			continue
		}
		out[scheme.ID] = APIKeyRoutingSchemeSummary{
			ID:          scheme.ID,
			Name:        scheme.Name,
			Description: scheme.Description,
			Platform:    scheme.Platform,
			MinRate:     scheme.MinRate,
			MaxRate:     scheme.MaxRate,
		}
	}
	return out
}

func (s *RoutingSchemeService) validateInput(ctx context.Context, id int64, input RoutingSchemeInput) (*RoutingScheme, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len([]rune(name)) > 100 {
		return nil, fmt.Errorf("%w: name must be 1-100 characters", ErrRoutingSchemeInvalid)
	}
	platform := strings.TrimSpace(input.Platform)
	if platform == "" || platform == PlatformComposite || platform == PlatformMultimodal {
		return nil, fmt.Errorf("%w: unsupported platform", ErrRoutingSchemeInvalid)
	}
	status := input.Status
	if status == "" {
		status = domain.RoutingSchemeStatusActive
	}
	if status != domain.RoutingSchemeStatusActive && status != domain.RoutingSchemeStatusDisabled {
		return nil, fmt.Errorf("%w: unsupported status", ErrRoutingSchemeInvalid)
	}
	ttftWeight, errorWeight := input.TTFTWeight, input.ErrorWeight
	if ttftWeight < 0 || ttftWeight > domain.RoutingSchemeMaxScoreWeight {
		return nil, fmt.Errorf("%w: ttft_weight must be within [0, %d]", ErrRoutingSchemeInvalid, domain.RoutingSchemeMaxScoreWeight)
	}
	if errorWeight < 0 || errorWeight > domain.RoutingSchemeMaxScoreWeight {
		return nil, fmt.Errorf("%w: error_weight must be within [0, %d]", ErrRoutingSchemeInvalid, domain.RoutingSchemeMaxScoreWeight)
	}
	ttftWeight, errorWeight = scoringWeights(ttftWeight, errorWeight)
	if len(input.Items) == 0 {
		return nil, fmt.Errorf("%w: at least one group is required", ErrRoutingSchemeInvalid)
	}
	if len(input.Items) > maxRoutingSchemeItems {
		return nil, fmt.Errorf("%w: at most %d groups are allowed", ErrRoutingSchemeInvalid, maxRoutingSchemeItems)
	}

	seenGroups := make(map[int64]struct{}, len(input.Items))
	seenRules := make(map[string]struct{})
	items := make([]RoutingSchemeItem, 0, len(input.Items))
	for position, item := range input.Items {
		if item.GroupID <= 0 {
			return nil, fmt.Errorf("%w: group_id must be positive", ErrRoutingSchemeInvalid)
		}
		if _, exists := seenGroups[item.GroupID]; exists {
			return nil, fmt.Errorf("%w: duplicate group %d", ErrRoutingSchemeInvalid, item.GroupID)
		}
		seenGroups[item.GroupID] = struct{}{}
		if len(item.Rules) > maxRoutingSchemeRules {
			return nil, fmt.Errorf("%w: at most %d rules per group are allowed", ErrRoutingSchemeInvalid, maxRoutingSchemeRules)
		}
		group, err := s.groupRepo.GetByID(ctx, item.GroupID)
		if err != nil || group == nil {
			return nil, fmt.Errorf("%w: group %d not found", ErrRoutingSchemeInvalid, item.GroupID)
		}
		if group.Platform != platform {
			return nil, fmt.Errorf("%w: group %d belongs to %s", ErrRoutingSchemeInvalid, item.GroupID, group.Platform)
		}
		if !group.IsActive() || group.IsSubscriptionType() {
			return nil, fmt.Errorf("%w: group %d is not a selectable standard group", ErrRoutingSchemeInvalid, item.GroupID)
		}
		for _, rule := range item.Rules {
			if rule.ID == "" {
				return nil, fmt.Errorf("%w: rule id is required", ErrRoutingSchemeInvalid)
			}
			if _, exists := seenRules[rule.ID]; exists {
				return nil, fmt.Errorf("%w: duplicate rule id %s", ErrRoutingSchemeInvalid, rule.ID)
			}
			seenRules[rule.ID] = struct{}{}
			if err := validateRoutingRule(rule); err != nil {
				return nil, err
			}
		}
		items = append(items, RoutingSchemeItem{
			GroupID:        item.GroupID,
			GroupName:      group.Name,
			Platform:       group.Platform,
			RateMultiplier: group.RateMultiplier,
			Position:       position,
			Rules:          item.Rules,
		})
	}

	return &RoutingScheme{
		ID:          id,
		Name:        name,
		Description: strings.TrimSpace(input.Description),
		Platform:    platform,
		Status:      status,
		SortOrder:   input.SortOrder,
		TTFTWeight:  ttftWeight,
		ErrorWeight: errorWeight,
		Items:       items,
	}, nil
}

// scoringWeights resolves a scheme's score weights. A zero/zero pair means the
// caller omitted them, so it falls back to the defaults; negatives are clamped.
func scoringWeights(ttftWeight, errorWeight int) (int, int) {
	if ttftWeight <= 0 && errorWeight <= 0 {
		return domain.RoutingSchemeDefaultTTFTWeight, domain.RoutingSchemeDefaultErrorWeight
	}
	if ttftWeight < 0 {
		ttftWeight = 0
	}
	if errorWeight < 0 {
		errorWeight = 0
	}
	return ttftWeight, errorWeight
}

func validateRoutingRule(rule domain.RoutingSchemeRule) error {
	switch rule.Trigger.Kind {
	case domain.RoutingTriggerTTFT:
		if rule.Trigger.ThresholdSeconds <= 0 || rule.Trigger.ThresholdSeconds > 600 {
			return fmt.Errorf("%w: ttft threshold must be within (0, 600] seconds", ErrRoutingSchemeInvalid)
		}
	case domain.RoutingTriggerErrorClass:
		if len(rule.Trigger.Classes) == 0 {
			return fmt.Errorf("%w: error_class requires at least one class", ErrRoutingSchemeInvalid)
		}
		for _, class := range rule.Trigger.Classes {
			switch class {
			case RoutingErrorClassTimeout, RoutingErrorClassHTTP429, RoutingErrorClassHTTP5xx,
				RoutingErrorClassAuth, RoutingErrorClassModelUnavailable:
			default:
				return fmt.Errorf("%w: unsupported error class %s", ErrRoutingSchemeInvalid, class)
			}
		}
	case domain.RoutingTriggerErrorRate:
		if rule.Trigger.FailurePercent <= 0 || rule.Trigger.FailurePercent > 100 {
			return fmt.Errorf("%w: error_rate failure_percent must be within (0, 100]", ErrRoutingSchemeInvalid)
		}
	default:
		return fmt.Errorf("%w: unsupported trigger kind %s", ErrRoutingSchemeInvalid, rule.Trigger.Kind)
	}
	switch rule.Action.Kind {
	case domain.RoutingActionFailover, domain.RoutingActionCooldown:
	default:
		return fmt.Errorf("%w: unsupported action kind %s", ErrRoutingSchemeInvalid, rule.Action.Kind)
	}
	if rule.Action.CooldownSeconds < 0 || rule.Action.CooldownSeconds > 86400 {
		return fmt.Errorf("%w: cooldown_seconds must be within [0, 86400]", ErrRoutingSchemeInvalid)
	}
	return nil
}
