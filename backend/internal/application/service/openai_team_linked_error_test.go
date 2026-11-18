//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type teamLinkedErrorRepo struct {
	rateLimitAccountRepoStub
	workspaceIDs []string
	revokedIDs   []int64
	revokeErr    error
	order        []string
}

func (r *teamLinkedErrorRepo) RevokeOpenAIWorkspaceAccounts(_ context.Context, workspaceID, _ string) ([]int64, error) {
	r.workspaceIDs = append(r.workspaceIDs, workspaceID)
	r.order = append(r.order, "revoke")
	return r.revokedIDs, r.revokeErr
}

func (r *teamLinkedErrorRepo) DeleteOAuthAccountIfCredentialsUnchanged(ctx context.Context, id int64, credentials map[string]any) ([]int64, bool, error) {
	r.order = append(r.order, "delete")
	return r.rateLimitAccountRepoStub.DeleteOAuthAccountIfCredentialsUnchanged(ctx, id, credentials)
}

type teamLinkedBlocker struct{ blocked []int64 }

func (b *teamLinkedBlocker) BlockAccountScheduling(account *Account, _ time.Time, _ string) {
	b.blocked = append(b.blocked, account.ID)
}
func (*teamLinkedBlocker) ClearAccountSchedulingBlock(int64) {}

func workspaceRevocationSettings(t *testing.T, cleanup, revoke bool) *SettingService {
	t.Helper()
	settings := oauth401AutoDeleteSettingService(t, cleanup)
	require.NoError(t, settings.SetOAuth401CleanupSettings(t.Context(), &OAuth401CleanupSettings{Enabled: cleanup, RevokeSameWorkspace: revoke}))
	return settings
}

func TestOpenAITeamLinkedErrorFansOutOncePerTeam(t *testing.T) {
	for _, body := range []string{
		`{"detail":{"code":"deactivated_workspace"}}`,
		`{"error":{"code":"deactivated_workspace","message":"Payment Required"}}`,
		`{"response":{"error":{"code":"deactivated_workspace"}}}`,
	} {
		t.Run(body, func(t *testing.T) {
			repo := &teamLinkedErrorRepo{revokedIDs: []int64{1, 2, 4}}
			blocker := &teamLinkedBlocker{}
			limits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			limits.SetSettingService(workspaceRevocationSettings(t, false, true))
			limits.SetAccountRuntimeBlocker(blocker)
			trigger := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": " team-1 "}}
			limits.maybeHandleOpenAITeamLinkedError(t.Context(), trigger, 402, []byte(body))
			limits.maybeHandleOpenAITeamLinkedError(t.Context(), trigger, 402, []byte(body))
			require.Equal(t, []string{"team-1"}, repo.workspaceIDs)
			require.Equal(t, []int64{1, 2, 4}, blocker.blocked, "every mutated owner and shadow must be blocked immediately")
		})
	}
}

func TestOpenAITeamLinkedErrorRequiresEnabledPolicyAndConfirmedWorkspace(t *testing.T) {
	for _, tt := range []struct {
		name, platform, accountType, workspace, body string
		status                                       int
		enabled                                      bool
	}{
		{"disabled", PlatformOpenAI, AccountTypeOAuth, "team-1", workspaceDeactivatedTestBody, 402, false},
		{"missing_workspace", PlatformOpenAI, AccountTypeOAuth, "", workspaceDeactivatedTestBody, 402, true},
		{"api_key", PlatformOpenAI, AccountTypeAPIKey, "team-1", workspaceDeactivatedTestBody, 402, true},
		{"other_platform", PlatformAnthropic, AccountTypeOAuth, "team-1", workspaceDeactivatedTestBody, 402, true},
		{"token_401", PlatformOpenAI, AccountTypeOAuth, "team-1", `{"error":{"code":"token_expired"}}`, 401, true},
		{"ordinary_402", PlatformOpenAI, AccountTypeOAuth, "team-1", `{"error":{"message":"Payment Required"}}`, 402, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &teamLinkedErrorRepo{}
			limits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			limits.SetSettingService(workspaceRevocationSettings(t, false, tt.enabled))
			account := &Account{ID: 1, Platform: tt.platform, Type: tt.accountType, Credentials: map[string]any{"chatgpt_account_id": tt.workspace}}
			limits.maybeHandleOpenAITeamLinkedError(t.Context(), account, tt.status, []byte(tt.body))
			require.Empty(t, repo.workspaceIDs)
		})
	}
	repo := newMockSettingRepo()
	repo.data[SettingKeyOAuth401CleanupSettings] = `{"enabled":true}`
	value, err := NewSettingService(repo, &config.Config{}).GetOAuth401CleanupSettings(t.Context())
	require.NoError(t, err)
	require.False(t, value.RevokeSameWorkspace, "old settings must keep bulk revocation disabled")
}

func TestOpenAITeamLinkedErrorFailureCanRetry(t *testing.T) {
	repo := &teamLinkedErrorRepo{revokeErr: errors.New("database unavailable"), revokedIDs: []int64{2}}
	limits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	limits.SetSettingService(workspaceRevocationSettings(t, false, true))
	blocker := &teamLinkedBlocker{}
	limits.SetAccountRuntimeBlocker(blocker)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "team-1"}}
	limits.maybeHandleOpenAITeamLinkedError(t.Context(), account, 402, []byte(workspaceDeactivatedTestBody))
	require.Empty(t, blocker.blocked, "a failed atomic mutation must not block unmodified accounts")
	repo.revokeErr = nil
	limits.maybeHandleOpenAITeamLinkedError(t.Context(), account, 402, []byte(workspaceDeactivatedTestBody))
	require.Len(t, repo.workspaceIDs, 2, "database failures must not poison the success deduplication window")
	require.Equal(t, []int64{2}, blocker.blocked)
}

func TestOpenAITeamLinkedErrorRunsBeforeOAuthCleanup(t *testing.T) {
	for _, entry := range []string{"gateway", "ratelimit"} {
		t.Run(entry, func(t *testing.T) {
			repo := &teamLinkedErrorRepo{revokedIDs: []int64{1, 2, 4}}
			repo.deleteApplied, repo.deleteIDs = true, []int64{1}
			limits := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			limits.SetSettingService(workspaceRevocationSettings(t, true, true))
			gateway := &OpenAIGatewayService{rateLimitService: limits}
			limits.SetAccountRuntimeBlocker(gateway)
			account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"chatgpt_account_id": "team-1", "refresh_token": "rt-1"}}
			var disabled bool
			if entry == "gateway" {
				disabled = gateway.handleOpenAIAccountUpstreamError(t.Context(), account, http.StatusPaymentRequired, http.Header{}, []byte(workspaceDeactivatedTestBody))
			} else {
				disabled = limits.HandleUpstreamError(t.Context(), account, http.StatusPaymentRequired, http.Header{}, []byte(workspaceDeactivatedTestBody))
			}
			require.True(t, disabled)
			require.Equal(t, []string{"revoke", "delete"}, repo.order)
			require.Equal(t, 1, repo.deleteCalls)
			for _, id := range []int64{2, 4} {
				require.True(t, gateway.isOpenAIAccountRuntimeBlocked(&Account{ID: id, Platform: PlatformOpenAI}))
			}
		})
	}
}
