//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type teamLinkedErrorRepo struct {
	mockAccountRepoForPlatform
	setErrors map[int64]string
}

func (r *teamLinkedErrorRepo) SetError(_ context.Context, id int64, message string) error {
	if r.setErrors == nil {
		r.setErrors = make(map[int64]string)
	}
	r.setErrors[id] = message
	return nil
}

type teamLinkedBlocker struct {
	blocked []int64
}

func teamLinkedInt64Ptr(value int64) *int64 { return &value }

func (b *teamLinkedBlocker) BlockAccountScheduling(account *Account, _ time.Time, _ string) {
	if account != nil {
		b.blocked = append(b.blocked, account.ID)
	}
}

func (b *teamLinkedBlocker) ClearAccountSchedulingBlock(int64) {}

func TestOpenAITeamLinkedErrorFansOutOncePerTeam(t *testing.T) {
	repo := &teamLinkedErrorRepo{mockAccountRepoForPlatform: mockAccountRepoForPlatform{accounts: []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "team-1"}},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "team-1"}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"chatgpt_account_id": "team-2"}},
		{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, ParentAccountID: teamLinkedInt64Ptr(1), Credentials: map[string]any{"chatgpt_account_id": "team-1"}},
	}}}
	blocker := &teamLinkedBlocker{}
	rateLimits := NewRateLimitService(repo, nil, nil, nil, nil)
	rateLimits.SetAccountRuntimeBlocker(blocker)

	trigger := &repo.accounts[0]
	body := []byte(`{"detail":{"code":"deactivated_workspace"}}`)
	rateLimits.maybeHandleOpenAITeamLinkedError(context.Background(), trigger, 402, body)
	rateLimits.maybeHandleOpenAITeamLinkedError(context.Background(), trigger, 402, body)

	require.Contains(t, repo.setErrors, int64(2))
	require.NotContains(t, repo.setErrors, int64(3))
	require.NotContains(t, repo.setErrors, int64(4))
	require.Equal(t, []int64{2}, blocker.blocked)
}
