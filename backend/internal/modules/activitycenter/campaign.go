package activitycenter

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
)

const (
	CampaignTypeLottery = "lottery"
	CampaignTypeInflate = "inflate"
	CampaignTypeCheckin = "checkin"
	// CampaignTypeRedeem is retained for campaigns created before inflate was introduced.
	CampaignTypeRedeem = "redeem"
	CampaignTypeCustom = "custom"
)

const (
	CampaignStatusDraft    = "draft"
	CampaignStatusActive   = "active"
	CampaignStatusArchived = "archived"
)

var (
	ErrCampaignNotFound         = infraerrors.NotFound("ACTIVITY_CAMPAIGN_NOT_FOUND", "activity campaign not found")
	ErrCampaignNotVisible       = infraerrors.NotFound("ACTIVITY_CAMPAIGN_NOT_VISIBLE", "activity campaign is not available")
	ErrCampaignInputRequired    = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_INPUT_REQUIRED", "activity campaign input is required")
	ErrCampaignTitleInvalid     = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_TITLE_INVALID", "activity campaign title is invalid")
	ErrCampaignSubtitleInvalid  = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_SUBTITLE_INVALID", "activity campaign subtitle is invalid")
	ErrCampaignBannerInvalid    = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_BANNER_INVALID", "activity campaign banner_url is invalid")
	ErrCampaignRefInvalid       = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_REF_INVALID", "activity campaign ref_id is invalid")
	ErrCampaignConfigInvalid    = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_CONFIG_INVALID", "activity campaign config_json is invalid")
	ErrCampaignTypeInvalid      = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_TYPE_INVALID", "activity campaign type is invalid")
	ErrCampaignStatusInvalid    = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_STATUS_INVALID", "activity campaign status is invalid")
	ErrCampaignContentInvalid   = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_CONTENT_INVALID", "activity campaign content is invalid")
	ErrCampaignSortInvalid      = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_SORT_INVALID", "activity campaign sort_order is invalid")
	ErrCampaignScheduleInvalid  = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_TIME_RANGE_INVALID", "starts_at must be before ends_at")
	ErrCampaignNoPrize          = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_NO_PRIZE", "activity campaign has no available prize")
	ErrCampaignPoolNotFound     = infraerrors.NotFound("ACTIVITY_CAMPAIGN_POOL_NOT_FOUND", "activity lottery pool not found")
	ErrCampaignNotEligible      = infraerrors.Forbidden("ACTIVITY_CAMPAIGN_NOT_ELIGIBLE", "user is not eligible for this activity")
	ErrCampaignDailyLimit       = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_DAILY_LIMIT", "daily activity participation limit reached")
	ErrCampaignRewardFailed     = infraerrors.InternalServer("ACTIVITY_CAMPAIGN_REWARD_FAILED", "activity reward could not be granted")
	ErrCampaignAlreadyCheckedIn = infraerrors.BadRequest("ACTIVITY_CAMPAIGN_ALREADY_CHECKED_IN", "already checked in today")
)

type Campaign struct {
	ID         int64
	Title      string
	Subtitle   string
	BannerURL  string
	BannerHTML string
	Type       string
	RefID      string
	ConfigJSON string
	Status     string
	StartsAt   *time.Time
	EndsAt     *time.Time
	SortOrder  int
	Content    string
	CreatedBy  *int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (c *Campaign) IsVisibleAt(now time.Time) bool {
	if c == nil {
		return false
	}
	if c.Status != CampaignStatusActive {
		return false
	}
	if c.StartsAt != nil && now.Before(*c.StartsAt) {
		return false
	}
	if c.EndsAt != nil && !now.Before(*c.EndsAt) {
		return false
	}
	return true
}

type ListFilters struct {
	Status string
	Type   string
	Search string
}

type RecordFilters struct {
	UserID     int64
	CampaignID int64
	Type       string
	Search     string
}

type Repository interface {
	Create(ctx context.Context, campaign *Campaign) error
	GetByID(ctx context.Context, id int64) (*Campaign, error)
	GetVisibleByID(ctx context.Context, id int64, now time.Time) (*Campaign, error)
	Update(ctx context.Context, campaign *Campaign) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, params pagination.PaginationParams, filters ListFilters) ([]Campaign, *pagination.PaginationResult, error)
	ListVisible(ctx context.Context, now time.Time) ([]Campaign, error)
	CreateRecord(ctx context.Context, record *Record) error
	WithParticipationTx(ctx context.Context, campaignID int64, fn func(context.Context) error) error
	CountUserPoolRecordsSince(ctx context.Context, userID, campaignID int64, poolID string, since time.Time) (int64, error)
	CountPrizeRecords(ctx context.Context, campaignID int64, prizeID string) (int64, error)
	ListPrizeIssuedCodes(ctx context.Context, campaignID int64, prizeID string) ([]string, error)
	UserHasAllowedGroup(ctx context.Context, userID int64, groupIDs []int64) (bool, error)
	ListUserAllowedGroupIDs(ctx context.Context, userID int64) ([]int64, error)
	ListRecords(ctx context.Context, params pagination.PaginationParams, filters RecordFilters) ([]Record, *pagination.PaginationResult, error)
	GetCheckinStatus(ctx context.Context, campaignID, userID int64, checkinDate time.Time, cycleDays int) (*CheckinStatus, error)
	ListCheckinLeaderboard(ctx context.Context, campaignID int64, limit int) ([]CheckinLeaderboardEntry, error)
	CreateCheckinRecord(ctx context.Context, record *CheckinRecord) error
}

type Service struct {
	repo          Repository
	rewardGranter RewardGranter
}

type CheckinLeaderboardEntry struct {
	Rank         int
	UserName     string
	UserEmail    string
	StreakDays   int
	CheckinCount int
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) SetRewardGranter(granter RewardGranter) {
	s.rewardGranter = granter
}

type CreateInput struct {
	Title      string
	Subtitle   string
	BannerURL  string
	BannerHTML string
	Type       string
	RefID      string
	ConfigJSON string
	Status     string
	StartsAt   *time.Time
	EndsAt     *time.Time
	SortOrder  int
	Content    string
	ActorID    *int64
}

type UpdateInput struct {
	Title      *string
	Subtitle   *string
	BannerURL  *string
	BannerHTML *string
	Type       *string
	RefID      *string
	ConfigJSON *string
	Status     *string
	StartsAt   **time.Time
	EndsAt     **time.Time
	SortOrder  *int
	Content    *string
}

type Record struct {
	ID                int64
	CampaignID        int64
	CampaignTitle     string
	CampaignType      string
	UserID            int64
	UserEmail         string
	UserName          string
	PoolID            string
	PoolName          string
	PrizeID           string
	PrizeLabel        string
	PrizeType         string
	PrizeColor        string
	ResultStatus      string
	RewardStatus      string
	RewardPayloadJSON string
	CreatedAt         time.Time
}

type CheckinReward struct {
	Day           int    `json:"day"`
	RewardType    string `json:"reward_type"`
	Value         string `json:"value"`
	RewardGroupID int64  `json:"reward_group_id,omitempty"`
	Label         string `json:"label,omitempty"`
}

type CheckinConfig struct {
	Timezone         string          `json:"timezone"`
	CycleType        string          `json:"cycle_type"`
	RequiredGroupIDs []int64         `json:"required_group_ids"`
	DailyRewards     []CheckinReward `json:"daily_rewards"`
	StreakMode       string          `json:"streak_mode"`
}

type CheckinRecord struct {
	ID                int64
	CampaignID        int64
	UserID            int64
	CheckinDate       time.Time
	CycleNo           int
	CycleDay          int
	StreakDays        int
	RewardType        string
	RewardValue       string
	RewardStatus      string
	RewardPayloadJSON string
	CreatedAt         time.Time
}

type CheckinStatus struct {
	CheckedToday    bool
	StreakDays      int
	CycleDay        int
	LastCheckinDate *time.Time
	Records         []CheckinRecord
}

type ParticipateInput struct {
	UserID int64
	PoolID string
}

type RewardGrant struct {
	PrizeType   string
	ValueAmount string
	GroupID     int64
	Code        string
	Description string
}

type PrizeStockStat struct {
	PoolID         string
	PrizeID        string
	IssuedCount    int64
	AvailableCount *int64
	RemainingCount *int64
}

type RewardGranter interface {
	Grant(ctx context.Context, userID int64, grant RewardGrant) error
	AfterCommit(ctx context.Context, userID int64, grant RewardGrant)
}

type ActivityConfig struct {
	Lottery *LotteryConfig `json:"lottery"`
	Inflate *InflateConfig `json:"inflate"`
	Redeem  *InflateConfig `json:"redeem"`
	Checkin *CheckinConfig `json:"checkin"`
}

type InflateConfig struct {
	MinValue         string  `json:"min_value"`
	MaxValue         string  `json:"max_value"`
	RequiredGroupIDs []int64 `json:"required_group_ids"`
	MinInflatePct    string  `json:"min_inflate_pct"`
	MaxInflatePct    string  `json:"max_inflate_pct"`
	Priority         int     `json:"priority"`
}

type LotteryConfig struct {
	Pools []LotteryPool `json:"pools"`
}

type LotteryPool struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	RequiredGroupIDs []int64        `json:"required_group_ids"`
	Enabled          *bool          `json:"enabled"`
	CanDraw          bool           `json:"can_draw"`
	DailyLimit       int            `json:"daily_limit"`
	Prizes           []LotteryPrize `json:"prizes"`
}

type LotteryPrize struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	PrizeType      string   `json:"prize_type"`
	ValueAmount    string   `json:"value_amount"`
	RewardGroupID  *int64   `json:"reward_group_id"`
	Value          string   `json:"value"`
	Weight         int      `json:"weight"`
	IsFallback     bool     `json:"is_fallback"`
	Color          string   `json:"color"`
	AvailableCount *int     `json:"available_count"`
	Codes          []string `json:"codes"`
}

func (s *Service) Create(ctx context.Context, input *CreateInput) (*Campaign, error) {
	if input == nil {
		return nil, ErrCampaignInputRequired
	}

	campaign := &Campaign{
		Title:      input.Title,
		Subtitle:   input.Subtitle,
		BannerURL:  input.BannerURL,
		BannerHTML: input.BannerHTML,
		Type:       input.Type,
		RefID:      input.RefID,
		ConfigJSON: input.ConfigJSON,
		Status:     input.Status,
		StartsAt:   input.StartsAt,
		EndsAt:     input.EndsAt,
		SortOrder:  input.SortOrder,
		Content:    input.Content,
	}
	if input.ActorID != nil && *input.ActorID > 0 {
		campaign.CreatedBy = input.ActorID
	}
	if err := normalizeCampaign(campaign); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, campaign); err != nil {
		return nil, err
	}
	return campaign, nil
}

func (s *Service) Update(ctx context.Context, id int64, input *UpdateInput) (*Campaign, error) {
	if input == nil {
		return nil, ErrCampaignInputRequired
	}

	campaign, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Title != nil {
		campaign.Title = *input.Title
	}
	if input.Subtitle != nil {
		campaign.Subtitle = *input.Subtitle
	}
	if input.BannerURL != nil {
		campaign.BannerURL = *input.BannerURL
	}
	if input.BannerHTML != nil {
		campaign.BannerHTML = *input.BannerHTML
	}
	if input.Type != nil {
		campaign.Type = *input.Type
	}
	if input.RefID != nil {
		campaign.RefID = *input.RefID
	}
	if input.ConfigJSON != nil {
		campaign.ConfigJSON = *input.ConfigJSON
	}
	if input.Status != nil {
		campaign.Status = *input.Status
	}
	if input.StartsAt != nil {
		campaign.StartsAt = *input.StartsAt
	}
	if input.EndsAt != nil {
		campaign.EndsAt = *input.EndsAt
	}
	if input.SortOrder != nil {
		campaign.SortOrder = *input.SortOrder
	}
	if input.Content != nil {
		campaign.Content = *input.Content
	}

	if err := normalizeCampaign(campaign); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, campaign); err != nil {
		return nil, err
	}
	return campaign, nil
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}

func (s *Service) Get(ctx context.Context, id int64) (*Campaign, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetVisible(ctx context.Context, id int64) (*Campaign, error) {
	item, err := s.repo.GetVisibleByID(ctx, id, time.Now())
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) GetVisibleForUser(ctx context.Context, id, userID int64) (*Campaign, error) {
	now := time.Now()
	item, err := s.repo.GetVisibleByID(ctx, id, now)
	if err != nil {
		return nil, err
	}
	groupIDs, err := s.repo.ListUserAllowedGroupIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !filterCampaignPoolsForUser(item, groupIDs) {
		return nil, ErrCampaignNotVisible
	}
	return item, nil
}

func (s *Service) List(ctx context.Context, params pagination.PaginationParams, filters ListFilters) ([]Campaign, *pagination.PaginationResult, error) {
	filters.Status = strings.TrimSpace(filters.Status)
	filters.Type = strings.TrimSpace(filters.Type)
	filters.Search = strings.TrimSpace(filters.Search)
	if len(filters.Search) > 200 {
		filters.Search = filters.Search[:200]
	}
	return s.repo.List(ctx, params, filters)
}

func (s *Service) ListVisible(ctx context.Context) ([]Campaign, error) {
	return s.repo.ListVisible(ctx, time.Now())
}

func (s *Service) ListVisibleForUser(ctx context.Context, userID int64) ([]Campaign, error) {
	now := time.Now()
	items, err := s.repo.ListVisible(ctx, now)
	if err != nil {
		return nil, err
	}
	groupIDs, err := s.repo.ListUserAllowedGroupIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Campaign, 0, len(items))
	for i := range items {
		if filterCampaignPoolsForUser(&items[i], groupIDs) {
			out = append(out, items[i])
		}
	}
	return out, nil
}

func (s *Service) ResolveInflatedBalance(ctx context.Context, userID int64, amount float64) (float64, int64, string, string, float64, error) {
	if amount <= 0 {
		return amount, 0, "", "", 0, nil
	}
	items, err := s.repo.ListVisible(ctx, time.Now())
	if err != nil {
		return 0, 0, "", "", 0, err
	}
	groups, err := s.repo.ListUserAllowedGroupIDs(ctx, userID)
	if err != nil {
		return 0, 0, "", "", 0, err
	}
	groupSet := make(map[int64]struct{}, len(groups))
	for _, groupID := range groups {
		groupSet[groupID] = struct{}{}
	}
	matched := false
	bestPriority := -1
	minPct, maxPct := 0.0, 0.0
	var campaignID int64
	var campaignTitle, campaignType string
	for _, campaign := range items {
		if campaign.Type != CampaignTypeInflate && campaign.Type != CampaignTypeRedeem {
			continue
		}
		config, err := parseActivityConfig(campaign.ConfigJSON)
		if err != nil {
			continue
		}
		rule := config.Inflate
		if rule == nil {
			rule = config.Redeem
		}
		if rule == nil {
			continue
		}
		minValue, err1 := strconv.ParseFloat(rule.MinValue, 64)
		maxValue, err2 := strconv.ParseFloat(rule.MaxValue, 64)
		low, err3 := strconv.ParseFloat(rule.MinInflatePct, 64)
		high, err4 := strconv.ParseFloat(rule.MaxInflatePct, 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || amount < minValue || amount > maxValue {
			continue
		}
		eligible := len(rule.RequiredGroupIDs) == 0
		for _, groupID := range rule.RequiredGroupIDs {
			if _, ok := groupSet[groupID]; ok {
				eligible = true
				break
			}
		}
		if !eligible || (matched && rule.Priority <= bestPriority) {
			continue
		}
		matched, bestPriority, minPct, maxPct = true, rule.Priority, low, high
		campaignID, campaignTitle, campaignType = campaign.ID, campaign.Title, campaign.Type
	}
	if !matched || maxPct <= 0 {
		return amount, 0, "", "", 0, nil
	}
	ratio, err := cryptoRandomInt(1000001)
	if err != nil {
		return 0, 0, "", "", 0, err
	}
	pct := minPct + (maxPct-minPct)*float64(ratio)/1000000
	// Balances are stored with eight decimal places; round once before the
	// transaction applies the adjustment so binary float noise is not credited.
	credited := math.Round((amount+amount*pct/100)*1e8) / 1e8
	return credited, campaignID, campaignTitle, campaignType, pct, nil
}

func (s *Service) RecordInflation(ctx context.Context, userID, campaignID int64, title, campaignType string, originalAmount, creditedAmount, inflatePct float64) error {
	payload, err := json.Marshal(map[string]any{
		"original_amount": originalAmount,
		"credited_amount": creditedAmount,
		"inflate_pct":     inflatePct,
		"value":           strconv.FormatFloat(creditedAmount, 'f', -1, 64),
	})
	if err != nil {
		return err
	}
	return s.repo.CreateRecord(ctx, &Record{
		CampaignID:        campaignID,
		CampaignTitle:     title,
		CampaignType:      campaignType,
		UserID:            userID,
		ResultStatus:      "inflated",
		RewardStatus:      "granted",
		RewardPayloadJSON: string(payload),
	})
}

func (s *Service) Participate(ctx context.Context, campaignID int64, input ParticipateInput) (*Record, error) {
	if input.UserID <= 0 {
		return nil, ErrCampaignInputRequired
	}
	var record *Record
	var granted *RewardGrant
	err := s.repo.WithParticipationTx(ctx, campaignID, func(txCtx context.Context) error {
		now := time.Now()
		campaign, err := s.repo.GetVisibleByID(txCtx, campaignID, now)
		if err != nil {
			return err
		}
		if campaign.Type != CampaignTypeLottery {
			record = &Record{
				CampaignID:    campaign.ID,
				CampaignTitle: campaign.Title,
				CampaignType:  campaign.Type,
				UserID:        input.UserID,
				ResultStatus:  "recorded",
				RewardStatus:  "none",
			}
			return s.repo.CreateRecord(txCtx, record)
		}

		pool, err := s.selectLotteryPool(txCtx, campaign, input.PoolID, input.UserID, now)
		if err != nil {
			return err
		}
		prize, code, err := s.drawPrize(txCtx, campaign.ID, pool)
		if err != nil {
			return err
		}
		grant := rewardGrantForPrize(prize, code)
		if prize.PrizeType != "none" {
			if s.rewardGranter == nil {
				return ErrCampaignRewardFailed
			}
			if err := s.rewardGranter.Grant(txCtx, input.UserID, grant); err != nil {
				return err
			}
			granted = &grant
		}
		record = &Record{
			CampaignID:        campaign.ID,
			CampaignTitle:     campaign.Title,
			CampaignType:      campaign.Type,
			UserID:            input.UserID,
			PoolID:            pool.ID,
			PoolName:          pool.Name,
			PrizeID:           prize.ID,
			PrizeLabel:        prize.Label,
			PrizeType:         prize.PrizeType,
			PrizeColor:        prize.Color,
			ResultStatus:      "won",
			RewardStatus:      "granted",
			RewardPayloadJSON: buildRewardPayloadJSON(grant),
		}
		if prize.PrizeType == "none" {
			record.ResultStatus = "none"
			record.RewardStatus = "none"
		}
		return s.repo.CreateRecord(txCtx, record)
	})
	if err != nil {
		return nil, err
	}
	if granted != nil {
		s.rewardGranter.AfterCommit(ctx, input.UserID, *granted)
	}
	return record, nil
}

func (s *Service) ListUserRecords(ctx context.Context, userID int64, params pagination.PaginationParams) ([]Record, *pagination.PaginationResult, error) {
	return s.repo.ListRecords(ctx, params, RecordFilters{UserID: userID})
}

func (s *Service) ListRecords(ctx context.Context, params pagination.PaginationParams, filters RecordFilters) ([]Record, *pagination.PaginationResult, error) {
	filters.Type = strings.TrimSpace(filters.Type)
	filters.Search = strings.TrimSpace(filters.Search)
	if len(filters.Search) > 200 {
		filters.Search = filters.Search[:200]
	}
	return s.repo.ListRecords(ctx, params, filters)
}

func (s *Service) PrizeStockStats(ctx context.Context, campaign *Campaign) ([]PrizeStockStat, error) {
	if campaign == nil || campaign.Type != CampaignTypeLottery {
		return nil, nil
	}
	config, err := parseActivityConfig(campaign.ConfigJSON)
	if err != nil || config.Lottery == nil {
		return nil, ErrCampaignConfigInvalid
	}

	stats := make([]PrizeStockStat, 0)
	for _, pool := range config.Lottery.Pools {
		for _, prize := range pool.Prizes {
			prizeID := strings.TrimSpace(prize.ID)
			if prizeID == "" {
				continue
			}
			issued, err := s.repo.CountPrizeRecords(ctx, campaign.ID, prizeID)
			if err != nil {
				return nil, err
			}
			stat := PrizeStockStat{
				PoolID:      pool.ID,
				PrizeID:     prizeID,
				IssuedCount: issued,
			}
			if prize.PrizeType == "card" {
				total := int64(len(normalizedPrizeCodes(prize.Codes)))
				remaining := total - issued
				if remaining < 0 {
					remaining = 0
				}
				stat.AvailableCount = &total
				stat.RemainingCount = &remaining
			} else if prize.AvailableCount != nil {
				stock := int64(*prize.AvailableCount)
				remaining := stock - issued
				if remaining < 0 {
					remaining = 0
				}
				stat.AvailableCount = &stock
				stat.RemainingCount = &remaining
			}
			stats = append(stats, stat)
		}
	}
	return stats, nil
}

func (s *Service) selectLotteryPool(ctx context.Context, campaign *Campaign, poolID string, userID int64, now time.Time) (*LotteryPool, error) {
	config, err := parseActivityConfig(campaign.ConfigJSON)
	if err != nil || config.Lottery == nil {
		return nil, ErrCampaignConfigInvalid
	}
	ineligibleMatchedPool := false
	for i := range config.Lottery.Pools {
		pool := &config.Lottery.Pools[i]
		if pool.ID == "" || pool.Enabled != nil && !*pool.Enabled {
			continue
		}
		if poolID != "" && pool.ID != poolID {
			continue
		}
		if len(pool.RequiredGroupIDs) > 0 {
			ok, err := s.repo.UserHasAllowedGroup(ctx, userID, pool.RequiredGroupIDs)
			if err != nil {
				return nil, err
			}
			if !ok {
				ineligibleMatchedPool = true
				continue
			}
		}
		if pool.DailyLimit > 0 {
			since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			count, err := s.repo.CountUserPoolRecordsSince(ctx, userID, campaign.ID, pool.ID, since)
			if err != nil {
				return nil, err
			}
			if count >= int64(pool.DailyLimit) {
				return nil, ErrCampaignDailyLimit
			}
		}
		return pool, nil
	}
	if ineligibleMatchedPool {
		return nil, ErrCampaignNotEligible
	}
	if poolID != "" {
		return nil, ErrCampaignPoolNotFound
	}
	return nil, ErrCampaignNotEligible
}

func (s *Service) drawPrize(ctx context.Context, campaignID int64, pool *LotteryPool) (*LotteryPrize, string, error) {
	available := make([]LotteryPrize, 0, len(pool.Prizes))
	availableCodes := make([]string, 0, len(pool.Prizes))
	totalWeight := int64(0)
	for _, prize := range pool.Prizes {
		if prize.ID == "" || prize.Weight <= 0 {
			continue
		}
		used, err := s.repo.CountPrizeRecords(ctx, campaignID, prize.ID)
		if err != nil {
			return nil, "", err
		}
		stock, limited := effectivePrizeStock(prize)
		candidateCode := ""
		if prize.PrizeType == "card" {
			issued, err := s.repo.ListPrizeIssuedCodes(ctx, campaignID, prize.ID)
			if err != nil {
				return nil, "", err
			}
			issuedSet := make(map[string]struct{}, len(issued))
			for _, code := range issued {
				issuedSet[code] = struct{}{}
			}
			codes := normalizedPrizeCodes(prize.Codes)
			for _, code := range codes {
				if _, exists := issuedSet[code]; !exists {
					candidateCode = code
					break
				}
			}
			if candidateCode == "" {
				continue
			}
		}
		if limited && used >= stock {
			continue
		}
		available = append(available, prize)
		availableCodes = append(availableCodes, candidateCode)
		totalWeight += int64(prize.Weight)
	}
	if len(available) == 0 || totalWeight <= 0 {
		return nil, "", ErrCampaignNoPrize
	}
	picked, err := cryptoRandomInt(totalWeight)
	if err != nil {
		return nil, "", err
	}
	acc := int64(0)
	for i := range available {
		acc += int64(available[i].Weight)
		if picked < acc {
			return &available[i], availableCodes[i], nil
		}
	}
	last := len(available) - 1
	return &available[last], availableCodes[last], nil
}

func effectivePrizeStock(prize LotteryPrize) (int64, bool) {
	if prize.PrizeType == "card" {
		return int64(len(normalizedPrizeCodes(prize.Codes))), true
	}
	if prize.AvailableCount == nil {
		return 0, false
	}
	return int64(*prize.AvailableCount), true
}

func parseActivityConfig(raw string) (*ActivityConfig, error) {
	var config ActivityConfig
	if raw == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func cryptoRandomInt(max int64) (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0, err
	}
	return n.Int64(), nil
}

func rewardGrantForPrize(prize *LotteryPrize, code string) RewardGrant {
	grant := RewardGrant{}
	if prize == nil {
		return grant
	}
	grant.PrizeType = prize.PrizeType
	grant.ValueAmount = strings.TrimSpace(prize.ValueAmount)
	grant.Description = strings.TrimSpace(prize.Value)
	if prize.RewardGroupID != nil {
		grant.GroupID = *prize.RewardGroupID
	}
	if prize.PrizeType == "card" {
		grant.Code = code
	}
	return grant
}

func buildRewardPayloadJSON(grant RewardGrant) string {
	if grant.PrizeType == "" || grant.PrizeType == "none" {
		return "{}"
	}
	payload := map[string]any{
		"prize_type": grant.PrizeType,
	}
	if grant.ValueAmount != "" {
		payload["value_amount"] = grant.ValueAmount
	}
	if grant.GroupID > 0 {
		payload["reward_group_id"] = grant.GroupID
	}
	if grant.Description != "" {
		payload["value"] = grant.Description
	}
	if grant.Code != "" {
		payload["code"] = grant.Code
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func normalizeCampaign(c *Campaign) error {
	if c == nil {
		return ErrCampaignInputRequired
	}

	c.Title = strings.TrimSpace(c.Title)
	c.Subtitle = strings.TrimSpace(c.Subtitle)
	c.BannerURL = strings.TrimSpace(c.BannerURL)
	c.BannerHTML = strings.TrimSpace(c.BannerHTML)
	c.Type = strings.TrimSpace(c.Type)
	c.RefID = strings.TrimSpace(c.RefID)
	c.ConfigJSON = strings.TrimSpace(c.ConfigJSON)
	c.Status = strings.TrimSpace(c.Status)
	c.Content = strings.TrimSpace(c.Content)

	if c.Title == "" || len(c.Title) > 200 {
		return ErrCampaignTitleInvalid
	}
	if len(c.Subtitle) > 500 {
		return ErrCampaignSubtitleInvalid
	}
	if len(c.BannerURL) > 500 {
		return ErrCampaignBannerInvalid
	}
	if len(c.BannerHTML) > 20000 {
		return ErrCampaignBannerInvalid
	}
	if len(c.RefID) > 200 {
		return ErrCampaignRefInvalid
	}
	if c.ConfigJSON == "" {
		c.ConfigJSON = "{}"
	}
	if len(c.ConfigJSON) > 100000 || !json.Valid([]byte(c.ConfigJSON)) {
		return ErrCampaignConfigInvalid
	}
	if c.Type == "" {
		c.Type = CampaignTypeCustom
	}
	if !isValidCampaignType(c.Type) {
		return ErrCampaignTypeInvalid
	}
	if c.Status == "" {
		c.Status = CampaignStatusDraft
	}
	if !isValidCampaignStatus(c.Status) {
		return ErrCampaignStatusInvalid
	}
	if len(c.Content) > 50000 {
		return ErrCampaignContentInvalid
	}
	if c.SortOrder < 0 || c.SortOrder > 10000 {
		return ErrCampaignSortInvalid
	}
	if c.StartsAt != nil && c.EndsAt != nil && !c.StartsAt.Before(*c.EndsAt) {
		return ErrCampaignScheduleInvalid
	}
	if c.Type == CampaignTypeLottery {
		config, err := parseActivityConfig(c.ConfigJSON)
		if err != nil || validateLotteryConfig(config) != nil {
			return ErrCampaignConfigInvalid
		}
	}
	if c.Type == CampaignTypeInflate {
		config, err := parseActivityConfig(c.ConfigJSON)
		if err != nil || validateInflateConfig(config) != nil {
			return ErrCampaignConfigInvalid
		}
	}
	if c.Type == CampaignTypeCheckin {
		config, err := parseActivityConfig(c.ConfigJSON)
		if err != nil || validateCheckinConfig(config) != nil {
			return ErrCampaignConfigInvalid
		}
	}
	return nil
}

func validateInflateConfig(config *ActivityConfig) error {
	if config == nil {
		return ErrCampaignConfigInvalid
	}
	rule := config.Inflate
	if rule == nil {
		rule = config.Redeem
	}
	if rule == nil {
		return ErrCampaignConfigInvalid
	}
	minValue, err := strconv.ParseFloat(strings.TrimSpace(rule.MinValue), 64)
	if err != nil || minValue < 0 {
		return ErrCampaignConfigInvalid
	}
	maxValue, err := strconv.ParseFloat(strings.TrimSpace(rule.MaxValue), 64)
	if err != nil || maxValue < minValue {
		return ErrCampaignConfigInvalid
	}
	minPct, err := strconv.ParseFloat(strings.TrimSpace(rule.MinInflatePct), 64)
	if err != nil || minPct < 0 || minPct > 100 {
		return ErrCampaignConfigInvalid
	}
	maxPct, err := strconv.ParseFloat(strings.TrimSpace(rule.MaxInflatePct), 64)
	if err != nil || maxPct < minPct || maxPct > 100 {
		return ErrCampaignConfigInvalid
	}
	if rule.Priority < 0 {
		return ErrCampaignConfigInvalid
	}
	for _, groupID := range rule.RequiredGroupIDs {
		if groupID <= 0 {
			return ErrCampaignConfigInvalid
		}
	}
	return nil
}

func validateLotteryConfig(config *ActivityConfig) error {
	if config == nil || config.Lottery == nil || len(config.Lottery.Pools) == 0 {
		return ErrCampaignConfigInvalid
	}
	poolIDs := make(map[string]struct{}, len(config.Lottery.Pools))
	prizeIDs := make(map[string]struct{})
	allCodes := make(map[string]struct{})
	for _, pool := range config.Lottery.Pools {
		poolID := strings.TrimSpace(pool.ID)
		if poolID == "" || pool.DailyLimit < 0 {
			return ErrCampaignConfigInvalid
		}
		if _, exists := poolIDs[poolID]; exists {
			return ErrCampaignConfigInvalid
		}
		poolIDs[poolID] = struct{}{}
		if len(pool.Prizes) == 0 {
			return ErrCampaignConfigInvalid
		}
		hasWeightedPrize := false
		for _, prize := range pool.Prizes {
			prizeID := strings.TrimSpace(prize.ID)
			if prizeID == "" || strings.TrimSpace(prize.Label) == "" || prize.Weight < 0 {
				return ErrCampaignConfigInvalid
			}
			if _, exists := prizeIDs[prizeID]; exists {
				return ErrCampaignConfigInvalid
			}
			prizeIDs[prizeID] = struct{}{}
			if prize.AvailableCount != nil && *prize.AvailableCount < 0 {
				return ErrCampaignConfigInvalid
			}
			if prize.Weight > 0 {
				hasWeightedPrize = true
			}
			amount := strings.TrimSpace(prize.ValueAmount)
			switch prize.PrizeType {
			case "none":
			case "card":
				codes := normalizedPrizeCodes(prize.Codes)
				if len(codes) == 0 {
					return ErrCampaignConfigInvalid
				}
				for _, code := range codes {
					if _, exists := allCodes[code]; exists {
						return ErrCampaignConfigInvalid
					}
					allCodes[code] = struct{}{}
				}
			case "balance":
				value, err := strconv.ParseFloat(amount, 64)
				if err != nil || value <= 0 {
					return ErrCampaignConfigInvalid
				}
			case "concurrency":
				value, err := strconv.Atoi(amount)
				if err != nil || value <= 0 {
					return ErrCampaignConfigInvalid
				}
			case "subscription":
				days, err := strconv.Atoi(amount)
				if err != nil || days <= 0 || prize.RewardGroupID == nil || *prize.RewardGroupID <= 0 {
					return ErrCampaignConfigInvalid
				}
			default:
				return ErrCampaignConfigInvalid
			}
		}
		if !hasWeightedPrize {
			return ErrCampaignConfigInvalid
		}
	}
	return nil
}

func normalizedPrizeCodes(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		code := strings.TrimSpace(value)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}
	return out
}

func filterCampaignPoolsForUser(campaign *Campaign, groupIDs []int64) bool {
	if campaign == nil || campaign.Type != CampaignTypeLottery {
		return campaign != nil
	}
	config, err := parseActivityConfig(campaign.ConfigJSON)
	if err != nil || config.Lottery == nil {
		return false
	}
	groups := make(map[int64]struct{}, len(groupIDs))
	for _, groupID := range groupIDs {
		groups[groupID] = struct{}{}
	}
	filtered := make([]LotteryPool, 0, len(config.Lottery.Pools))
	for _, pool := range config.Lottery.Pools {
		if pool.Enabled != nil && !*pool.Enabled {
			continue
		}
		pool.CanDraw = len(pool.RequiredGroupIDs) == 0
		for _, required := range pool.RequiredGroupIDs {
			if _, ok := groups[required]; ok {
				pool.CanDraw = true
				break
			}
		}
		filtered = append(filtered, pool)
	}
	if len(filtered) == 0 {
		return false
	}
	config.Lottery.Pools = filtered
	raw, err := json.Marshal(config)
	if err != nil {
		return false
	}
	campaign.ConfigJSON = string(raw)
	return true
}

func isValidCampaignType(value string) bool {
	switch value {
	case CampaignTypeLottery, CampaignTypeInflate, CampaignTypeCheckin, CampaignTypeRedeem, CampaignTypeCustom:
		return true
	default:
		return false
	}
}

func validateCheckinConfig(config *ActivityConfig) error {
	if config == nil || config.Checkin == nil || len(config.Checkin.DailyRewards) == 0 {
		return ErrCampaignConfigInvalid
	}
	cycleDays := map[string]int{"weekly": 7, "biweekly": 14, "monthly": 30}
	if config.Checkin.CycleType == "" {
		config.Checkin.CycleType = "weekly"
	}
	maxDay, ok := cycleDays[config.Checkin.CycleType]
	if !ok || (config.Checkin.StreakMode != "" && config.Checkin.StreakMode != "reset_on_miss") {
		return ErrCampaignConfigInvalid
	}
	seen := make(map[int]struct{}, len(config.Checkin.DailyRewards))
	for _, reward := range config.Checkin.DailyRewards {
		if reward.Day < 1 || reward.Day > maxDay || reward.Value == "" {
			return ErrCampaignConfigInvalid
		}
		if _, exists := seen[reward.Day]; exists {
			return ErrCampaignConfigInvalid
		}
		seen[reward.Day] = struct{}{}
		switch reward.RewardType {
		case "balance":
			if value, err := strconv.ParseFloat(reward.Value, 64); err != nil || value <= 0 {
				return ErrCampaignConfigInvalid
			}
		case "concurrency":
			if value, err := strconv.Atoi(reward.Value); err != nil || value <= 0 {
				return ErrCampaignConfigInvalid
			}
		case "subscription":
			if reward.RewardGroupID <= 0 {
				return ErrCampaignConfigInvalid
			}
		default:
			return ErrCampaignConfigInvalid
		}
	}
	return nil
}

func isValidCampaignStatus(value string) bool {
	switch value {
	case CampaignStatusDraft, CampaignStatusActive, CampaignStatusArchived:
		return true
	default:
		return false
	}
}
