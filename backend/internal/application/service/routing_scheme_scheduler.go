package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

// Error classes understood by routing rule triggers. They map to the
// provider-owned failures recorded in ops_error_logs.
const (
	RoutingErrorClassTimeout          = "timeout"
	RoutingErrorClassHTTP429          = "http_429"
	RoutingErrorClassHTTP5xx          = "http_5xx"
	RoutingErrorClassAuth             = "auth"
	RoutingErrorClassModelUnavailable = "model_unavailable"
)

const (
	// routingMetricWindow is the rolling window used for TTFT and failure
	// signals. A short window keeps the score reactive without amplifying a
	// single bad request.
	routingMetricWindow = 15 * time.Minute
	// routingMetricCacheTTL coalesces the per-key auth misses that happen in
	// the same burst into one aggregation query.
	routingMetricCacheTTL = 5 * time.Second
	// routingColdStartSamples is the sample count below which a group keeps a
	// neutral score so an unmeasured group is never ranked as the healthiest.
	routingColdStartSamples = 5
	// routingTTFTFloorMs/routingTTFTCeilingMs map p95 TTFT to a 100..0 score.
	routingTTFTFloorMs   = 1000
	routingTTFTCeilingMs = 8000
	// routingFailureRateCeiling maps failure rate 0..10% to a 100..0 score.
	routingFailureRateCeiling = 0.10
	// routingFailoverPenalty deprioritizes a group whose failover rule fired
	// but which is still allowed to serve as a last resort.
	routingFailoverPenalty = 30.0
	routingNeutralScore    = 50.0
)

// RoutingSchemeGroupMetric is the rolling signal set for one group.
type RoutingSchemeGroupMetric struct {
	GroupID          int64
	SuccessCount     int64
	FailureCount     int64
	P95TTFTMs        float64
	ErrorClassCounts map[string]int64
	LastErrorAt      *time.Time
}

// RoutingSchemeMetricsRepository reads rolling group signals. Implementations
// must exclude user-side and billing failures from FailureCount.
type RoutingSchemeMetricsRepository interface {
	GetGroupMetrics(ctx context.Context, groupIDs []int64, since time.Time) (map[int64]RoutingSchemeGroupMetric, error)
	// GetSchemeGroupUsage aggregates traffic attributed to one scheme over a
	// window. Attribution uses api_keys.routing_scheme_id, not group ownership.
	GetSchemeGroupUsage(ctx context.Context, schemeID int64, since time.Time) (map[int64]RoutingSchemeGroupUsage, error)
}

// RoutingSchemeGroupUsage is the analytics row for one scheme group.
type RoutingSchemeGroupUsage struct {
	Requests     int64
	Successes    int64
	Failures     int64
	P95TTFTMs    float64
	InputTokens  int64
	OutputTokens int64
	Cost         float64
}

// RoutingGroupScore is the backend-computed composite score. Admins never edit
// it; it is derived from recent traffic so a manually inflated weight cannot
// hide an unhealthy group.
type RoutingGroupScore struct {
	Overall      float64
	TTFT         float64
	Error        float64
	P95TTFTMs    float64
	FailureRate  float64
	FailureCount int64
	SampleCount  int64
	ColdStart    bool
}

type routingMetricsCacheEntry struct {
	metric    RoutingSchemeGroupMetric
	fetchedAt time.Time
}

func (s *RoutingSchemeService) metricsFor(ctx context.Context, groupIDs []int64) map[int64]RoutingSchemeGroupMetric {
	out := make(map[int64]RoutingSchemeGroupMetric, len(groupIDs))
	if s.metrics == nil || len(groupIDs) == 0 {
		return out
	}
	now := time.Now()
	missing := make([]int64, 0, len(groupIDs))
	s.metricsMu.Lock()
	if s.metricsCache == nil {
		s.metricsCache = make(map[int64]routingMetricsCacheEntry, len(groupIDs))
	}
	for _, id := range groupIDs {
		entry, ok := s.metricsCache[id]
		if ok && now.Sub(entry.fetchedAt) < routingMetricCacheTTL {
			out[id] = entry.metric
			continue
		}
		missing = append(missing, id)
	}
	s.metricsMu.Unlock()
	if len(missing) == 0 {
		return out
	}

	fetched, err := s.metrics.GetGroupMetrics(ctx, missing, now.Add(-routingMetricWindow))
	if err != nil {
		// Fail open: keep the configured order rather than failing auth.
		return out
	}
	s.metricsMu.Lock()
	for _, id := range missing {
		metric, ok := fetched[id]
		if !ok {
			metric = RoutingSchemeGroupMetric{GroupID: id}
		}
		s.metricsCache[id] = routingMetricsCacheEntry{metric: metric, fetchedAt: now}
		out[id] = metric
	}
	if len(s.metricsCache) > 4096 {
		for id, entry := range s.metricsCache {
			if now.Sub(entry.fetchedAt) >= routingMetricCacheTTL {
				delete(s.metricsCache, id)
			}
		}
	}
	s.metricsMu.Unlock()
	return out
}

func scoreRoutingGroup(metric RoutingSchemeGroupMetric, ttftWeight, errorWeight int) RoutingGroupScore {
	samples := metric.SuccessCount + metric.FailureCount
	score := RoutingGroupScore{
		P95TTFTMs:    metric.P95TTFTMs,
		FailureCount: metric.FailureCount,
		SampleCount:  samples,
	}
	if samples > 0 {
		score.FailureRate = float64(metric.FailureCount) / float64(samples)
	}
	if samples < routingColdStartSamples {
		// Cold start and stale data both stay neutral so an unmeasured group is
		// neither punished nor credited as the healthiest.
		score.ColdStart = true
		score.TTFT = routingNeutralScore
		score.Error = routingNeutralScore
		score.Overall = routingNeutralScore
		return score
	}
	if metric.P95TTFTMs > 0 {
		score.TTFT = routingTTFTScore(metric.P95TTFTMs)
	} else {
		// No stream samples in the window (e.g. non-stream traffic only): do not
		// credit the group as the fastest.
		score.TTFT = routingNeutralScore
	}
	score.Error = routingErrorScore(score.FailureRate)
	score.Overall = weightedScore(score.TTFT, score.Error, ttftWeight, errorWeight)
	if score.Overall > 100 {
		score.Overall = 100
	}
	return score
}

// weightedScore blends the dimension scores with the scheme's weights. Only the
// ratio matters, so 40/60, 20/30 and 2/3 all produce the same distribution.
func weightedScore(ttftScore, errorScore float64, ttftWeight, errorWeight int) float64 {
	total := ttftWeight + errorWeight
	if total <= 0 {
		return routingNeutralScore
	}
	return (ttftScore*float64(ttftWeight) + errorScore*float64(errorWeight)) / float64(total)
}

func routingTTFTScore(p95ms float64) float64 {
	if p95ms <= routingTTFTFloorMs {
		return 100
	}
	if p95ms >= routingTTFTCeilingMs {
		return 0
	}
	return (routingTTFTCeilingMs - p95ms) / (routingTTFTCeilingMs - routingTTFTFloorMs) * 100
}

func routingErrorScore(rate float64) float64 {
	if rate <= 0 {
		return 100
	}
	if rate >= routingFailureRateCeiling {
		return 0
	}
	return (routingFailureRateCeiling - rate) / routingFailureRateCeiling * 100
}

type rankedRoutingItem struct {
	item  RoutingSchemeItem
	score RoutingGroupScore
}

// OrderedGroupWeights returns the request-visible candidate order together with
// score-derived distribution weights. Rules only affect future requests: a
// triggered cooldown removes the group from this order until it expires, and at
// least one group is always kept. Weights come from the same composite score the
// admin UI displays as the share, so a healthier group attracts more traffic
// without starving the others. A single metrics snapshot is used for both
// outputs.
func (s *RoutingSchemeService) OrderedGroupWeights(ctx context.Context, scheme *RoutingScheme) ([]int64, map[int64]int) {
	ranked := s.orderedRoutingItems(ctx, scheme)
	ids := make([]int64, 0, len(ranked))
	weights := make(map[int64]int, len(ranked))
	for _, entry := range ranked {
		ids = append(ids, entry.item.GroupID)
		weight := int(math.Round(entry.score.Overall))
		if weight < 1 {
			weight = 1
		}
		weights[entry.item.GroupID] = weight
	}
	return ids, weights
}

func (s *RoutingSchemeService) orderedRoutingItems(ctx context.Context, scheme *RoutingScheme) []rankedRoutingItem {
	if scheme == nil || len(scheme.Items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(scheme.Items))
	for _, item := range scheme.Items {
		if item.GroupID > 0 {
			ids = append(ids, item.GroupID)
		}
	}
	metrics := s.metricsFor(ctx, ids)
	now := time.Now()
	ttftWeight, errorWeight := scoringWeights(scheme.TTFTWeight, scheme.ErrorWeight)

	ranked := make([]rankedRoutingItem, 0, len(scheme.Items))
	fallback := make([]rankedRoutingItem, 0, len(scheme.Items))
	cooldownCreated := false
	for _, item := range scheme.Items {
		if item.GroupID <= 0 {
			continue
		}
		metric := metrics[item.GroupID]
		score := scoreRoutingGroup(metric, ttftWeight, errorWeight)
		entry := rankedRoutingItem{item: item, score: score}
		fallback = append(fallback, entry)
		if s.inCooldown(scheme.ID, item.GroupID, now) {
			continue
		}
		if action, cooldown := evaluateRoutingRules(item.Rules, metric, score); action != "" {
			if action == domain.RoutingActionCooldown {
				if cooldown <= 0 {
					cooldown = 60
				}
				if s.setCooldown(scheme.ID, item.GroupID, now.Add(time.Duration(cooldown)*time.Second)) {
					cooldownCreated = true
				}
				continue
			}
			// failover: keep the group but test it after the healthy ones.
			entry.score.Overall -= routingFailoverPenalty
			if entry.score.Overall < 0 {
				entry.score.Overall = 0
			}
		}
		ranked = append(ranked, entry)
	}
	if cooldownCreated && s.authCacheInvalidator != nil {
		// Keys holding an older snapshot would keep hitting the cooled group
		// until the L2 TTL expired; dropping them makes the next request rebuild.
		s.authCacheInvalidator.InvalidateAuthCacheByRoutingScheme(ctx, scheme.ID)
	}
	if len(ranked) == 0 {
		// Never starve the key: fall back to the best-scoring group even when
		// every group is cooling down.
		sort.SliceStable(fallback, func(i, j int) bool {
			return fallback[i].score.Overall > fallback[j].score.Overall
		})
		ranked = fallback[:1]
	}
	// Candidate order is only a stable display/fallback order; the request-time
	// weighted rendezvous decides which group actually serves.
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score.Overall != ranked[j].score.Overall {
			return ranked[i].score.Overall > ranked[j].score.Overall
		}
		return ranked[i].item.Position < ranked[j].item.Position
	})
	return ranked
}

// evaluateRoutingRules returns the highest-priority action triggered by an
// item's enabled rules.
func evaluateRoutingRules(rules []domain.RoutingSchemeRule, metric RoutingSchemeGroupMetric, score RoutingGroupScore) (string, int) {
	action := ""
	cooldown := 0
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !routingRuleTriggered(rule.Trigger, metric, score) {
			continue
		}
		// Cooldown is stronger than a soft failover.
		if action == domain.RoutingActionCooldown {
			continue
		}
		action = rule.Action.Kind
		cooldown = rule.Action.CooldownSeconds
	}
	return action, cooldown
}

func routingRuleTriggered(trigger domain.RoutingSchemeRuleTrigger, metric RoutingSchemeGroupMetric, score RoutingGroupScore) bool {
	switch trigger.Kind {
	case domain.RoutingTriggerTTFT:
		// Consecutive is the minimum sample count in the rolling window; the
		// metrics are aggregates, so an exact "N slow requests in a row" cannot
		// be observed here.
		minSamples := trigger.Consecutive
		if minSamples < 1 {
			minSamples = 1
		}
		if score.SampleCount < int64(minSamples) || metric.P95TTFTMs <= 0 {
			return false
		}
		return metric.P95TTFTMs > trigger.ThresholdSeconds*1000
	case domain.RoutingTriggerErrorClass:
		if len(trigger.Classes) == 0 {
			return false
		}
		total := int64(0)
		for _, class := range trigger.Classes {
			total += metric.ErrorClassCounts[class]
		}
		minHits := int64(trigger.Consecutive)
		if minHits < 1 {
			minHits = 1
		}
		return total >= minHits
	case domain.RoutingTriggerErrorRate:
		minRequests := int64(trigger.MinRequests)
		if minRequests < 1 {
			minRequests = 1
		}
		if score.SampleCount < minRequests {
			return false
		}
		return score.FailureRate*100 >= trigger.FailurePercent
	}
	return false
}

// Scores returns the read-only composite score per group for admin display.
func (s *RoutingSchemeService) Scores(ctx context.Context, scheme *RoutingScheme) map[int64]RoutingGroupScore {
	out := make(map[int64]RoutingGroupScore)
	if scheme == nil || len(scheme.Items) == 0 {
		return out
	}
	ids := make([]int64, 0, len(scheme.Items))
	for _, item := range scheme.Items {
		if item.GroupID > 0 {
			ids = append(ids, item.GroupID)
		}
	}
	metrics := s.metricsFor(ctx, ids)
	ttftWeight, errorWeight := scoringWeights(scheme.TTFTWeight, scheme.ErrorWeight)
	for _, item := range scheme.Items {
		if item.GroupID <= 0 {
			continue
		}
		out[item.GroupID] = scoreRoutingGroup(metrics[item.GroupID], ttftWeight, errorWeight)
	}
	return out
}

// GroupScores returns read-only scores for arbitrary groups. The admin editor
// uses it before a scheme is saved, so even the cold-start value and the
// weighting come from the backend instead of a frontend placeholder.
func (s *RoutingSchemeService) GroupScores(ctx context.Context, groupIDs []int64, ttftWeight, errorWeight int) map[int64]RoutingGroupScore {
	out := make(map[int64]RoutingGroupScore)
	ids := make([]int64, 0, len(groupIDs))
	seen := make(map[int64]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out
	}
	ttftWeight, errorWeight = scoringWeights(ttftWeight, errorWeight)
	metrics := s.metricsFor(ctx, ids)
	for _, id := range ids {
		out[id] = scoreRoutingGroup(metrics[id], ttftWeight, errorWeight)
	}
	return out
}

func (s *RoutingSchemeService) cooldownKey(schemeID, groupID int64) string {
	return fmt.Sprintf("%d:%d", schemeID, groupID)
}

func (s *RoutingSchemeService) inCooldown(schemeID, groupID int64, now time.Time) bool {
	s.cooldownMu.Lock()
	defer s.cooldownMu.Unlock()
	until, ok := s.cooldownUntil[s.cooldownKey(schemeID, groupID)]
	if !ok {
		return false
	}
	if !now.Before(until) {
		delete(s.cooldownUntil, s.cooldownKey(schemeID, groupID))
		return false
	}
	return true
}

// setCooldown records a cooldown and reports whether it was newly created.
func (s *RoutingSchemeService) setCooldown(schemeID, groupID int64, until time.Time) bool {
	s.cooldownMu.Lock()
	defer s.cooldownMu.Unlock()
	if s.cooldownUntil == nil {
		s.cooldownUntil = make(map[string]time.Time)
	}
	key := s.cooldownKey(schemeID, groupID)
	if existing, ok := s.cooldownUntil[key]; ok && existing.After(until) {
		return false
	}
	s.cooldownUntil[key] = until
	return true
}

// ClearSchemeRuntimeState drops cached metrics and cooldown state after the
// admin edits a scheme so the next auth does not serve stale order.
func (s *RoutingSchemeService) ClearSchemeRuntimeState(schemeID int64) {
	if schemeID <= 0 {
		return
	}
	prefix := fmt.Sprintf("%d:", schemeID)
	s.cooldownMu.Lock()
	for key := range s.cooldownUntil {
		if strings.HasPrefix(key, prefix) {
			delete(s.cooldownUntil, key)
		}
	}
	s.cooldownMu.Unlock()
	s.metricsMu.Lock()
	s.metricsCache = nil
	s.metricsMu.Unlock()
}
