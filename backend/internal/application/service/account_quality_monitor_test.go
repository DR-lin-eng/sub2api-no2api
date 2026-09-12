package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type qualityRepoStub struct {
	AccountRepository
	groups map[int64][]int64
	extra  map[int64]map[string]any
}

func (r *qualityRepoStub) BindGroups(_ context.Context, accountID int64, ids []int64) error {
	r.groups[accountID] = append([]int64(nil), ids...)
	return nil
}
func (r *qualityRepoStub) UpdateExtra(_ context.Context, accountID int64, updates map[string]any) error {
	if r.extra[accountID] == nil {
		r.extra[accountID] = map[string]any{}
	}
	for key, value := range updates {
		r.extra[accountID][key] = value
	}
	return nil
}

type qualityGroupRepoStub struct {
	GroupRepository
	group *Group
}

func (r qualityGroupRepoStub) GetByID(context.Context, int64) (*Group, error) { return r.group, nil }

func TestQualityGroupSwitchPreservesOriginalAndRestoresAfterRecovery(t *testing.T) {
	targetID := int64(20)
	repo := &qualityRepoStub{groups: map[int64][]int64{}, extra: map[int64]map[string]any{}}
	svc := &AccountQualityMonitoringService{accountRepo: repo, groupRepo: qualityGroupRepoStub{group: &Group{ID: targetID, Platform: PlatformOpenAI, Status: StatusActive}}}
	account := &Account{ID: 7, Platform: PlatformOpenAI, GroupIDs: []int64{10, 11}, Extra: map[string]any{}}
	result := &AccountInspectionAccountResult{}
	require.NoError(t, svc.switchQualityGroup(context.Background(), account, &targetID, result))
	require.Equal(t, "switched_group", result.QualityAction)
	require.Equal(t, []int64{20}, repo.groups[7])
	require.Equal(t, []int64{10, 11}, repo.extra[7][AccountQualityOriginalGroupsExtraKey])

	account.GroupIDs = []int64{20}
	account.Extra = map[string]any{AccountQualityOriginalGroupsExtraKey: []any{float64(10), float64(11)}, AccountQualityRoutingGroupExtraKey: float64(20)}
	result = &AccountInspectionAccountResult{}
	require.NoError(t, svc.restoreQualityGroups(context.Background(), account, result))
	require.Equal(t, "restored_group", result.QualityAction)
	require.Equal(t, []int64{10, 11}, repo.groups[7])
}

func TestQualityGroupRestorePreservesManualGroupChange(t *testing.T) {
	repo := &qualityRepoStub{groups: map[int64][]int64{}, extra: map[int64]map[string]any{}}
	svc := &AccountQualityMonitoringService{accountRepo: repo}
	account := &Account{ID: 8, GroupIDs: []int64{30}, Extra: map[string]any{AccountQualityOriginalGroupsExtraKey: []any{float64(10)}, AccountQualityRoutingGroupExtraKey: float64(20)}}
	result := &AccountInspectionAccountResult{}
	require.NoError(t, svc.restoreQualityGroups(context.Background(), account, result))
	require.Equal(t, "manual_group_change_preserved", result.QualityAction)
	require.Empty(t, repo.groups)
}

func TestQualityExtraUpdateKeepsBoundedHistory(t *testing.T) {
	updated := map[string]any{}
	for i := 0; i < 30; i++ {
		updated = qualityExtraUpdate(updated, "healthy", 0, i+1, time.Unix(int64(i), 0), "passed", "")
	}
	history, ok := updated[accountQualityHistoryExtraKey].([]map[string]any)
	require.True(t, ok)
	require.Len(t, history, 24)
}

func TestQualityDefaultPromptIsDeterministic(t *testing.T) {
	settings := DefaultAccountQualitySettings()
	require.Contains(t, settings.Prompt, "SVG")
	require.Contains(t, settings.Prompt, "鹈鹕骑自行车")
}

func TestQualityProbeEligibleUsesSourceGroupAndKeepsReroutedAccount(t *testing.T) {
	source := int64(10)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, GroupIDs: []int64{10}}
	require.True(t, qualityProbeEligible(account, &source))
	account.GroupIDs = []int64{20}
	account.Extra = map[string]any{AccountQualityRoutingGroupExtraKey: float64(20)}
	require.True(t, qualityProbeEligible(account, &source))
	account.GroupIDs = []int64{30}
	require.False(t, qualityProbeEligible(account, &source))
}

type qualityStageProbeStub struct {
	responses []string
	results   []*ScheduledTestResult
	prompts   []string
}

func (p *qualityStageProbeStub) RunQualityTestBackground(_ context.Context, _ int64, _, prompt, _ string) (*ScheduledTestResult, error) {
	p.prompts = append(p.prompts, prompt)
	if len(p.results) > 0 {
		result := p.results[0]
		p.results = p.results[1:]
		return result, nil
	}
	response := ""
	if len(p.responses) > 0 {
		response, p.responses = p.responses[0], p.responses[1:]
	}
	return &ScheduledTestResult{Status: "success", ResponseText: response}, nil
}

type qualityStageProcessorStub struct{ calls int }

func (p *qualityStageProcessorStub) Process(context.Context, string) (*QualityArtifact, error) {
	p.calls++
	return &QualityArtifact{Label: "normal", Confidence: 0.99}, nil
}

func TestQualityStagesCanRunIndependently(t *testing.T) {
	account := Account{ID: 9, Name: "quality", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	results := func() []AccountInspectionAccountResult {
		return []AccountInspectionAccountResult{neutralAccountInspectionResult(&account, time.Now().UTC())}
	}
	stage1Probe := &qualityStageProbeStub{responses: []string{"21"}}
	stage1Svc := &AccountQualityMonitoringService{accountRepo: &qualityRepoStub{extra: map[int64]map[string]any{}}, accountTestSvc: stage1Probe}
	stage1Settings := DefaultAccountQualitySettings()
	stage1Settings.Stage2Enabled = false
	stage1Results := results()
	require.NoError(t, stage1Svc.runQualityMonitoring(context.Background(), []Account{account}, stage1Results, nil, stage1Settings, time.Now().UTC()))
	require.Len(t, stage1Probe.prompts, 1)
	require.Equal(t, "passed", stage1Results[0].QualityStage1Status)
	require.Equal(t, "disabled", stage1Results[0].QualityStage2Status)

	stage2Probe := &qualityStageProbeStub{responses: []string{"<html/>"}}
	stage2Processor := &qualityStageProcessorStub{}
	stage2Svc := &AccountQualityMonitoringService{accountRepo: &qualityRepoStub{extra: map[int64]map[string]any{}}, accountTestSvc: stage2Probe, qualityProcessor: stage2Processor}
	stage2Settings := DefaultAccountQualitySettings()
	stage2Settings.Stage1Enabled = false
	stage2Results := results()
	require.NoError(t, stage2Svc.runQualityMonitoring(context.Background(), []Account{account}, stage2Results, nil, stage2Settings, time.Now().UTC()))
	require.Len(t, stage2Probe.prompts, 1)
	require.Equal(t, 1, stage2Processor.calls)
	require.Equal(t, "disabled", stage2Results[0].QualityStage1Status)
	require.Equal(t, "passed", stage2Results[0].QualityStage2Status)
}

func TestQualityStagesBothDisabledSkipProbe(t *testing.T) {
	account := Account{ID: 13, Name: "quality", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	probe := &qualityStageProbeStub{}
	svc := &AccountQualityMonitoringService{accountRepo: &qualityRepoStub{extra: map[int64]map[string]any{}}, accountTestSvc: probe}
	settings := DefaultAccountQualitySettings()
	settings.Stage1Enabled = false
	settings.Stage2Enabled = false
	results := []AccountInspectionAccountResult{neutralAccountInspectionResult(&account, time.Now().UTC())}
	require.NoError(t, svc.runQualityMonitoring(context.Background(), []Account{account}, results, nil, settings, time.Now().UTC()))
	require.Empty(t, probe.prompts)
	require.Equal(t, "disabled", results[0].QualityStatus)
	require.Equal(t, "disabled", results[0].QualityStage1Status)
	require.Equal(t, "disabled", results[0].QualityStage2Status)
}

func TestQualityStageOneNonTwentyOneIsDegraded(t *testing.T) {
	probe := &qualityStageProbeStub{responses: []string{"20"}}
	svc := &AccountQualityMonitoringService{accountRepo: &qualityRepoStub{extra: map[int64]map[string]any{}}, accountTestSvc: probe}
	settings := DefaultAccountQualitySettings()
	settings.Stage2Enabled = false
	settings.FailureThreshold = 1
	account := Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
	results := []AccountInspectionAccountResult{neutralAccountInspectionResult(&account, time.Now().UTC())}
	require.NoError(t, svc.runQualityMonitoring(context.Background(), []Account{account}, results, nil, settings, time.Now().UTC()))
	require.Equal(t, "wrong", results[0].QualityStage1Status)
	require.Equal(t, "degraded", results[0].QualityStatus)
}

func TestQualityStageOneRequiresExactConfiguredAnswer(t *testing.T) {
	for _, answer := range []string{"答案：21", "21。", "最终答案是 21", "210"} {
		t.Run(answer, func(t *testing.T) {
			probe := &qualityStageProbeStub{responses: []string{answer}}
			svc := &AccountQualityMonitoringService{accountRepo: &qualityRepoStub{extra: map[int64]map[string]any{}}, accountTestSvc: probe}
			settings := DefaultAccountQualitySettings()
			settings.Stage2Enabled = false
			settings.FailureThreshold = 1
			account := Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}
			results := []AccountInspectionAccountResult{neutralAccountInspectionResult(&account, time.Now().UTC())}
			require.NoError(t, svc.runQualityMonitoring(context.Background(), []Account{account}, results, nil, settings, time.Now().UTC()))
			require.Equal(t, "wrong", results[0].QualityStage1Status)
			require.Equal(t, "degraded", results[0].QualityStatus)
		})
	}
}

func TestQualityReasoningTokenThresholdUsesStrictLessThan(t *testing.T) {
	for _, test := range []struct {
		name   string
		tokens *int64
		want   string
	}{{"below", func() *int64 { v := int64(99); return &v }(), "degraded"}, {"equal", func() *int64 { v := int64(100); return &v }(), "healthy"}, {"unknown", nil, "uncertain"}} {
		t.Run(test.name, func(t *testing.T) {
			probe := &qualityStageProbeStub{results: []*ScheduledTestResult{{Status: "success", ResponseText: "21", ReasoningTokens: test.tokens}}}
			svc := &AccountQualityMonitoringService{accountRepo: &qualityRepoStub{extra: map[int64]map[string]any{}}, accountTestSvc: probe}
			settings := DefaultAccountQualitySettings()
			settings.Stage2Enabled = false
			settings.MinReasoningTokens = 100
			account := Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}
			results := []AccountInspectionAccountResult{neutralAccountInspectionResult(&account, time.Now().UTC())}
			require.NoError(t, svc.runQualityMonitoring(context.Background(), []Account{account}, results, nil, settings, time.Now().UTC()))
			require.Equal(t, test.want, results[0].QualityStatus)
			require.Equal(t, test.tokens, results[0].QualityReasoningTokens)
		})
	}
}

func TestQualityReasoningDistributionIncludesZeroAndUnknown(t *testing.T) {
	zero, high := int64(0), int64(1200)
	got := summarizeReasoningTokenDistribution([]AccountInspectionAccountResult{{QualityStage1Status: "passed", QualityReasoningTokens: &zero}, {QualityStage1Status: "passed", QualityReasoningTokens: &high}, {QualityStage1Status: "uncertain"}, {QualityStage1Status: "disabled", QualityReasoningTokens: &high}})
	require.Equal(t, 2, got.MeasuredAccounts)
	require.Equal(t, 1, got.UnknownAccounts)
	require.Equal(t, 1, got.Buckets[0].Count)
	require.Equal(t, 1, got.Buckets[5].Count)
	require.InDelta(t, 600, *got.AverageTokens, 0.001)
}
