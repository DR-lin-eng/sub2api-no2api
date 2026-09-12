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
