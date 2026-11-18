//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestRateLimitService_WorkspaceDeactivatedCleanup(t *testing.T) {
	for _, tt := range []struct {
		name, platform, accountType, body string
		status                            int
		wantDelete                        bool
	}{
		{"error_code", PlatformOpenAI, AccountTypeOAuth, `{"error":{"code":"deactivated_workspace","message":"Payment Required","type":"upstream_error"}}`, 402, true},
		{"detail_code", PlatformOpenAI, AccountTypeOAuth, `{"detail":{"code":"deactivated_workspace"}}`, 402, true},
		{"response_error_code", PlatformOpenAI, AccountTypeOAuth, `{"response":{"error":{"code":"deactivated_workspace"}}}`, 402, true},
		{"wrapped_error", PlatformOpenAI, AccountTypeOAuth, `{"error":{"message":"{\"error\":{\"code\":\"deactivated_workspace\"}}"}}`, 402, true},
		{"ordinary_payment", PlatformOpenAI, AccountTypeOAuth, `{"error":{"message":"Payment Required"}}`, 402, false},
		{"insufficient_quota", PlatformOpenAI, AccountTypeOAuth, `{"error":{"code":"insufficient_quota"}}`, 402, false},
		{"message_only", PlatformOpenAI, AccountTypeOAuth, `{"error":{"message":"deactivated_workspace"}}`, 402, false},
		{"api_key", PlatformOpenAI, AccountTypeAPIKey, `{"error":{"code":"deactivated_workspace"}}`, 402, false},
		{"other_platform", PlatformAnthropic, AccountTypeOAuth, `{"error":{"code":"deactivated_workspace"}}`, 402, false},
		{"server_error", PlatformOpenAI, AccountTypeOAuth, `{"error":{"code":"deactivated_workspace"}}`, 502, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{deleteIDs: []int64{910}, deleteApplied: true}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			svc.SetSettingService(oauth401AutoDeleteSettingService(t, true))
			account := &Account{ID: 910, Platform: tt.platform, Type: tt.accountType, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"refresh_token": "rt-910"}}

			disabled := svc.HandleUpstreamError(t.Context(), account, tt.status, http.Header{}, []byte(tt.body))

			if tt.wantDelete {
				require.True(t, disabled)
				require.Equal(t, 1, repo.deleteCalls)
				require.Equal(t, account.Credentials, repo.deleteExpected)
				require.Zero(t, repo.setErrorCalls)
				require.False(t, account.Schedulable)
			} else {
				require.Zero(t, repo.deleteCalls)
			}
		})
	}
}

func TestOpenAIGatewayService_WorkspaceDeactivatedCleanupPolicy(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		enabled, deleted      bool
		deleteErr             error
		wantDelete, wantError int
	}{
		{name: "enabled", enabled: true, deleted: true, wantDelete: 1},
		{name: "disabled", wantError: 1},
		{name: "reauthorized", enabled: true, wantDelete: 1},
		{name: "delete_failed", enabled: true, deleteErr: errors.New("delete unavailable"), wantDelete: 1, wantError: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &rateLimitAccountRepoStub{deleteIDs: []int64{911}, deleteApplied: tt.deleted, deleteErr: tt.deleteErr}
			invalidator := &tokenCacheInvalidatorRecorder{}
			cleaner := &oauth401RuntimeCleanerRecorder{}
			rateLimits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			rateLimits.SetSettingService(oauth401AutoDeleteSettingService(t, tt.enabled))
			rateLimits.SetTokenCacheInvalidator(invalidator)
			rateLimits.SetAccountRuntimeStateCleaner(cleaner)
			gateway := &OpenAIGatewayService{rateLimitService: rateLimits}
			rateLimits.SetAccountRuntimeBlocker(gateway)
			account := &Account{ID: 911, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"refresh_token": "rt-911"}}

			disabled := gateway.handleOpenAIAccountUpstreamError(context.Background(), account, 402, http.Header{}, []byte(`{"error":{"code":"deactivated_workspace","message":"Payment Required"}}`), "gpt-image-2.6-sunburst")

			require.True(t, disabled)
			require.Equal(t, tt.wantDelete, repo.deleteCalls, "the gateway and generic policy must attempt deletion at most once")
			require.Equal(t, tt.wantError, repo.setErrorCalls)
			require.Zero(t, repo.tempCalls, "workspace deactivation cannot recover by refreshing a token")
			if tt.deleted {
				require.Len(t, invalidator.accounts, 1)
				require.Equal(t, []int64{911}, cleaner.deletedIDs)
				reason, ok := gateway.openaiAccountRuntimeBlockReason.Load(account.ID)
				require.True(t, ok)
				require.Equal(t, "oauth_401_deleted", reason)
			} else if tt.wantError == 0 {
				require.False(t, gateway.isOpenAIAccountRuntimeBlocked(account), "a stale request must not block a reauthorized account")
				require.True(t, account.Schedulable)
				require.Empty(t, cleaner.deletedIDs)
				require.Empty(t, invalidator.accounts)
			} else {
				require.Contains(t, repo.lastErrorMsg, "Workspace deactivated (402)")
				require.True(t, gateway.isOpenAIAccountRuntimeBlocked(account))
			}
		})
	}
}
