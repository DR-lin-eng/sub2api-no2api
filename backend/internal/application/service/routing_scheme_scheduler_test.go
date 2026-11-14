package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestScoringWeightsFallsBackToDefaults(t *testing.T) {
	ttft, errors := scoringWeights(0, 0)
	require.Equal(t, 40, ttft)
	require.Equal(t, 60, errors)

	ttft, errors = scoringWeights(100, 0)
	require.Equal(t, 100, ttft)
	require.Equal(t, 0, errors)

	ttft, errors = scoringWeights(-5, 80)
	require.Equal(t, 0, ttft)
	require.Equal(t, 80, errors)
}

func TestScoreRoutingGroupUsesSchemeWeights(t *testing.T) {
	// Fast request window but every TTFT sample is at the ceiling: the TTFT
	// dimension scores 0 while the error dimension scores 100.
	metric := RoutingSchemeGroupMetric{
		GroupID:      1,
		SuccessCount: 10,
		P95TTFTMs:    routingTTFTCeilingMs,
	}

	balanced := scoreRoutingGroup(metric, 40, 60)
	require.InDelta(t, 60, balanced.Overall, 0.001)

	speedOnly := scoreRoutingGroup(metric, 100, 0)
	require.InDelta(t, 0, speedOnly.Overall, 0.001)

	stabilityOnly := scoreRoutingGroup(metric, 0, 100)
	require.InDelta(t, 100, stabilityOnly.Overall, 0.001)
}

func TestWeightedScoreIgnoresAbsoluteScale(t *testing.T) {
	require.InDelta(t, 60, weightedScore(0, 100, 40, 60), 0.001)
	require.InDelta(t, 60, weightedScore(0, 100, 20, 30), 0.001)
	require.InDelta(t, routingNeutralScore, weightedScore(0, 100, 0, 0), 0.001)
}

func TestEvaluateRoutingRulesTTFTNeedsMinSamples(t *testing.T) {
	rules := []domain.RoutingSchemeRule{{
		Enabled: true,
		Trigger: domain.RoutingSchemeRuleTrigger{
			Kind:             domain.RoutingTriggerTTFT,
			ThresholdSeconds: 3,
			Consecutive:      5,
		},
		Action: domain.RoutingSchemeRuleAction{Kind: domain.RoutingActionCooldown, CooldownSeconds: 120},
	}}

	// Slow enough, but only 2 samples: the window aggregate is not trusted yet.
	metric := RoutingSchemeGroupMetric{GroupID: 1, SuccessCount: 2, P95TTFTMs: 5000}
	action, _ := evaluateRoutingRules(rules, metric, scoreRoutingGroup(metric, 40, 60))
	require.Empty(t, action)

	metric.SuccessCount = 5
	action, cooldown := evaluateRoutingRules(rules, metric, scoreRoutingGroup(metric, 40, 60))
	require.Equal(t, domain.RoutingActionCooldown, action)
	require.Equal(t, 120, cooldown)
}

func TestEvaluateRoutingRulesTTFTIgnoresNonStreamGroups(t *testing.T) {
	rules := []domain.RoutingSchemeRule{{
		Enabled: true,
		Trigger: domain.RoutingSchemeRuleTrigger{Kind: domain.RoutingTriggerTTFT, ThresholdSeconds: 1, Consecutive: 1},
		Action:  domain.RoutingSchemeRuleAction{Kind: domain.RoutingActionFailover},
	}}
	// No TTFT samples at all (non-stream traffic): must not trip the rule.
	metric := RoutingSchemeGroupMetric{GroupID: 1, SuccessCount: 10}
	action, _ := evaluateRoutingRules(rules, metric, scoreRoutingGroup(metric, 40, 60))
	require.Empty(t, action)
}

func TestEvaluateRoutingRulesErrorRateNeedsMinRequests(t *testing.T) {
	rules := []domain.RoutingSchemeRule{{
		Enabled: true,
		Trigger: domain.RoutingSchemeRuleTrigger{
			Kind:           domain.RoutingTriggerErrorRate,
			FailurePercent: 30,
			MinRequests:    10,
		},
		Action: domain.RoutingSchemeRuleAction{Kind: domain.RoutingActionFailover, CooldownSeconds: 60},
	}}

	// 50% failures but only 4 requests: below the minimum sample count.
	metric := RoutingSchemeGroupMetric{GroupID: 1, SuccessCount: 2, FailureCount: 2}
	action, _ := evaluateRoutingRules(rules, metric, scoreRoutingGroup(metric, 40, 60))
	require.Empty(t, action)

	// 30% failures across 10 requests: triggers.
	metric = RoutingSchemeGroupMetric{GroupID: 1, SuccessCount: 7, FailureCount: 3}
	action, _ = evaluateRoutingRules(rules, metric, scoreRoutingGroup(metric, 40, 60))
	require.Equal(t, domain.RoutingActionFailover, action)
}

func TestEvaluateRoutingRulesErrorClassCountsOnlySelectedClasses(t *testing.T) {
	rules := []domain.RoutingSchemeRule{{
		Enabled: true,
		Trigger: domain.RoutingSchemeRuleTrigger{
			Kind:        domain.RoutingTriggerErrorClass,
			Classes:     []string{RoutingErrorClassTimeout, RoutingErrorClassHTTP5xx},
			Consecutive: 2,
		},
		Action: domain.RoutingSchemeRuleAction{Kind: domain.RoutingActionCooldown, CooldownSeconds: 90},
	}}
	metric := RoutingSchemeGroupMetric{
		GroupID:      1,
		SuccessCount: 10,
		ErrorClassCounts: map[string]int64{
			RoutingErrorClassTimeout: 1,
			RoutingErrorClassHTTP429: 5,
		},
	}
	action, _ := evaluateRoutingRules(rules, metric, scoreRoutingGroup(metric, 40, 60))
	require.Empty(t, action)

	metric.ErrorClassCounts[RoutingErrorClassHTTP5xx] = 1
	action, cooldown := evaluateRoutingRules(rules, metric, scoreRoutingGroup(metric, 40, 60))
	require.Equal(t, domain.RoutingActionCooldown, action)
	require.Equal(t, 90, cooldown)
}

func TestEvaluateRoutingRulesCooldownBeatsFailover(t *testing.T) {
	trigger := domain.RoutingSchemeRuleTrigger{Kind: domain.RoutingTriggerTTFT, ThresholdSeconds: 1, Consecutive: 1}
	rules := []domain.RoutingSchemeRule{
		{
			Enabled: true,
			Trigger: trigger,
			Action:  domain.RoutingSchemeRuleAction{Kind: domain.RoutingActionFailover, CooldownSeconds: 30},
		},
		{
			Enabled: true,
			Trigger: trigger,
			Action:  domain.RoutingSchemeRuleAction{Kind: domain.RoutingActionCooldown, CooldownSeconds: 300},
		},
	}
	metric := RoutingSchemeGroupMetric{GroupID: 1, SuccessCount: 10, P95TTFTMs: 4000}

	action, cooldown := evaluateRoutingRules(rules, metric, scoreRoutingGroup(metric, 40, 60))
	require.Equal(t, domain.RoutingActionCooldown, action)
	require.Equal(t, 300, cooldown)
}

func TestOrderedRoutingItemsSkipsCoolingGroupsButNeverStarves(t *testing.T) {
	scheme := &RoutingScheme{
		ID:          9,
		TTFTWeight:  40,
		ErrorWeight: 60,
		Items: []RoutingSchemeItem{
			{GroupID: 1, Position: 0},
			{GroupID: 2, Position: 1},
		},
	}
	svc := &RoutingSchemeService{}
	svc.setCooldown(scheme.ID, 1, time.Now().Add(time.Minute))

	ranked := svc.orderedRoutingItems(context.Background(), scheme)
	require.Len(t, ranked, 1)
	require.Equal(t, int64(2), ranked[0].item.GroupID)

	// Every group cooling down: the request still gets exactly one candidate.
	svc.setCooldown(scheme.ID, 2, time.Now().Add(time.Minute))
	ranked = svc.orderedRoutingItems(context.Background(), scheme)
	require.Len(t, ranked, 1)
}

type groupBatchLoaderStub struct {
	GroupRepository
	groups     map[int64]*Group
	batchCalls int
	perIDCalls int
}

func (s *groupBatchLoaderStub) GetByIDs(ctx context.Context, ids []int64) ([]Group, error) {
	s.batchCalls++
	out := make([]Group, 0, len(ids))
	for _, id := range ids {
		if group, ok := s.groups[id]; ok && group != nil {
			out = append(out, *group)
		}
	}
	return out, nil
}

func (s *groupBatchLoaderStub) GetByID(ctx context.Context, id int64) (*Group, error) {
	s.perIDCalls++
	if group, ok := s.groups[id]; ok {
		return group, nil
	}
	return nil, ErrGroupNotFound
}

func TestLoadGroupsByIDsUsesBatchCapability(t *testing.T) {
	repo := &groupBatchLoaderStub{groups: map[int64]*Group{
		1: activeRoutingGroup(1),
		2: activeRoutingGroup(2),
	}}

	groups := loadGroupsByIDs(context.Background(), repo, []int64{1, 2, 3})

	require.Len(t, groups, 2)
	require.Equal(t, 1, repo.batchCalls)
	require.Zero(t, repo.perIDCalls)
}

func TestLoadGroupsByIDsFallsBackForPlainRepositories(t *testing.T) {
	repo := apiKeyGroupRoutingGroupRepo{groups: map[int64]*Group{
		1: activeRoutingGroup(1),
	}}

	groups := loadGroupsByIDs(context.Background(), repo, []int64{1, 2})

	require.Len(t, groups, 1)
	require.NotNil(t, groups[1])
	require.Nil(t, groups[2])
}
