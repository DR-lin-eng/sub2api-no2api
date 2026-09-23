package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListDegradedQualityAccountsIncludesOAuthDegradedAnd401(t *testing.T) {
	checked := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	repo := &qualityRepoStub{accounts: []Account{
		{ID: 101, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{"email": "degraded@example.com"}, Extra: map[string]any{"account_quality_status": "degraded", "account_quality_last_checked_at": checked.Format(time.RFC3339Nano)}},
		{ID: 102, Type: AccountTypeOAuth, Platform: PlatformGemini, Credentials: map[string]any{"email": "unauthorized@example.com"}, ErrorMessage: "Authentication failed (401): token expired"},
		{ID: 103, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{"email": "history-401@example.com"}, Extra: map[string]any{"account_quality_status": "healthy", "account_quality_history": []any{map[string]any{"outcome": "error", "error": "stage1: API returned 401"}}}},
		{ID: 104, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{"email": "healthy@example.com"}, Extra: map[string]any{"account_quality_status": "healthy"}},
		{ID: 105, Type: AccountTypeAPIKey, Platform: PlatformOpenAI, Credentials: map[string]any{"email": "apikey@example.com"}, Extra: map[string]any{"account_quality_status": "degraded"}, ErrorMessage: "API returned 401"},
		{ID: 106, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{}, Extra: map[string]any{"account_quality_status": "degraded"}},
	}}
	svc := &AccountQualityMonitoringService{accountRepo: repo}
	got, err := svc.ListDegradedQualityAccounts(context.Background())
	require.NoError(t, err)
	require.Equal(t, 4, got.Total)
	require.Len(t, got.Items, 4)
	require.Equal(t, int64(101), got.Items[0].AccountID)
	require.Equal(t, "degraded@example.com", got.Items[0].Email)
	require.Equal(t, "degraded", got.Items[0].Reason)
	require.Equal(t, checked, *got.Items[0].LastCheckedAt)
	require.Equal(t, int64(102), got.Items[1].AccountID)
	require.Equal(t, "unauthorized", got.Items[1].Reason)
	require.Equal(t, 401, *got.Items[1].HTTPStatus)
	require.Contains(t, got.Items[1].ErrorMessage, "401")
	require.Equal(t, int64(103), got.Items[2].AccountID)
	require.Equal(t, "unauthorized", got.Items[2].Reason)
	require.Equal(t, int64(106), got.Items[3].AccountID)
	require.Empty(t, got.Items[3].Email, "missing email does not hide the degraded account")
}

func TestListDegradedQualityAccountsRequiresRepository(t *testing.T) {
	svc := &AccountQualityMonitoringService{}
	_, err := svc.ListDegradedQualityAccounts(context.Background())
	require.ErrorIs(t, err, ErrAccountInspectionUnavailable)
}

func TestListDegradedQualityAccountsIncludes401FromQualityState(t *testing.T) {
	observed := time.Date(2026, 9, 14, 2, 0, 0, 0, time.UTC)
	state := AccountQualityRunState{Status: AccountInspectionStatusSucceeded, Results: []AccountInspectionAccountResult{{AccountID: 201, QualityStatus: "error", QualityError: "stage1: API returned 401", QualityCompletedAt: &observed}}}
	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)
	repo := &qualityRepoStub{accounts: []Account{{ID: 201, Type: AccountTypeOAuth, Platform: PlatformOpenAI, Credentials: map[string]any{"email": "state-401@example.com"}}}}
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{accountQualityStateKey: string(stateJSON)}}
	svc := &AccountQualityMonitoringService{accountRepo: repo, settingRepo: settingsRepo}
	got, err := svc.ListDegradedQualityAccounts(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, got.Total)
	require.Equal(t, int64(201), got.Items[0].AccountID)
	require.Equal(t, "unauthorized", got.Items[0].Reason)
	require.Equal(t, 401, *got.Items[0].HTTPStatus)
	require.Equal(t, observed, *got.Items[0].LastCheckedAt)
}
