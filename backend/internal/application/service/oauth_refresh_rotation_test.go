//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/openai"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

type openAIRotationClientStub struct {
	openaiOAuthClientRefreshStub
	rotationCalls int
}

func (c *openAIRotationClientStub) RefreshTokenWithClientID(context.Context, string, string, string) (*openai.TokenResponse, error) {
	c.rotationCalls++
	return &openai.TokenResponse{
		AccessToken: "rotated-access", RefreshToken: "rotated-refresh", ExpiresIn: 3600,
	}, nil
}

func TestRefreshIfNeeded_OpenAIEnrichmentTimeoutKeepsRotatedCredentials(t *testing.T) {
	account := openAIRefreshTestAccount()
	account.Credentials["expires_at"] = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	repo := &refreshAPIAccountRepo{account: account, respectReadContext: true}
	client := &openAIRotationClientStub{}
	oauthService := NewOpenAIOAuthService(nil, client)
	defer oauthService.Stop()
	privacyCalls := 0
	oauthService.SetPrivacyClientFactory(func(context.Context, string) (*req.Client, error) {
		return req.C().WrapRoundTripFunc(func(req.RoundTripper) req.RoundTripFunc {
			return func(request *req.Request) (*req.Response, error) {
				privacyCalls++
				<-request.Context().Done()
				return nil, request.Context().Err()
			}
		}), nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	executor := NewOpenAITokenRefresher(oauthService, repo)

	result, err := NewOAuthRefreshAPI(repo, nil).RefreshIfNeeded(ctx, account, executor, time.Hour)

	require.NoError(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	require.Positive(t, privacyCalls, "optional enrichment must be the operation that exhausts the context")
	require.Equal(t, 1, client.rotationCalls)
	require.True(t, result.Refreshed)
	require.Equal(t, "rotated-refresh", result.Account.GetOpenAIRefreshToken())
	require.Equal(t, "rotated-refresh", repo.account.GetOpenAIRefreshToken())
	require.Equal(t, 1, repo.openAISuccessCASCalls)
	require.NoError(t, repo.openAIPersistCtxErr)
}

func TestRefreshIfNeeded_OpenAILockLeaseReservesPersistenceBudget(t *testing.T) {
	account := openAIRefreshTestAccount()
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	var executorBudget time.Duration
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "rotated-access", "refresh_token": "rotated-refresh"},
		onRefreshContext: func(ctx context.Context) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			executorBudget = time.Until(deadline)
		},
	}

	_, err := NewOAuthRefreshAPI(repo, cache).RefreshIfNeeded(context.Background(), account, executor, time.Hour)

	require.NoError(t, err)
	require.Positive(t, executorBudget)
	require.LessOrEqual(t, executorBudget, 41*time.Second, "60-second lease must reserve 19 seconds for persistence and cleanup")
	require.Equal(t, 1, executor.refreshCalls)
	require.Equal(t, 1, cache.releaseCalls)
}

func TestRefreshIfNeeded_OpenAIShortLockLeaseDoesNotConsumeRefreshToken(t *testing.T) {
	for _, lease := range []time.Duration{time.Second, 19 * time.Second} {
		t.Run(lease.String(), func(t *testing.T) {
			account := openAIRefreshTestAccount()
			repo := &refreshAPIAccountRepo{account: account}
			cache := &refreshAPICacheStub{lockResult: true}
			executor := &refreshAPIExecutorStub{
				needsRefresh: true,
				credentials:  map[string]any{"access_token": "rotated-access", "refresh_token": "rotated-refresh"},
			}

			_, err := NewOAuthRefreshAPI(repo, cache, lease).RefreshIfNeeded(context.Background(), account, executor, time.Hour)

			var configuration *providerConfigurationRefreshError
			require.ErrorAs(t, err, &configuration)
			require.Zero(t, executor.refreshCalls)
			require.Zero(t, repo.openAISuccessCASCalls)
			require.Equal(t, "old-refresh", repo.account.GetOpenAIRefreshToken())
			require.Equal(t, 1, cache.releaseCalls)
		})
	}
}

func TestRefreshIfNeeded_OpenAILateSuccessPersistsWithBoundedContext(t *testing.T) {
	for _, cancellation := range []string{"deadline", "parent cancellation"} {
		t.Run(cancellation, func(t *testing.T) {
			account := openAIRefreshTestAccount()
			expected := shallowCopyMap(account.Credentials)
			repo := &refreshAPIAccountRepo{account: account, respectReadContext: true}
			cache := &refreshAPICacheStub{lockResult: true}
			ctx, cancel := context.WithCancel(context.Background())
			if cancellation == "deadline" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
			}
			defer cancel()
			executor := &refreshAPIExecutorStub{
				needsRefresh: true,
				credentials:  map[string]any{"access_token": "rotated-access", "refresh_token": "rotated-refresh"},
				onRefresh: func() {
					if cancellation == "parent cancellation" {
						cancel()
					}
					<-ctx.Done()
				},
			}

			result, err := NewOAuthRefreshAPI(repo, cache).RefreshIfNeeded(ctx, account, executor, time.Hour)

			require.NoError(t, err)
			require.True(t, result.Refreshed)
			require.Equal(t, "rotated-refresh", result.Account.GetOpenAIRefreshToken())
			require.Equal(t, "rotated-refresh", repo.account.GetOpenAIRefreshToken())
			require.Equal(t, expected, repo.lastExpectedCredentials)
			require.Equal(t, 1, executor.refreshCalls)
			require.Equal(t, 1, repo.openAISuccessCASCalls)
			require.NoError(t, repo.openAIPersistCtxErr)
			require.False(t, repo.openAIPersistDeadline.IsZero(), "detached persistence must remain bounded")
			require.LessOrEqual(t, time.Until(repo.openAIPersistDeadline), 10*time.Second)
			require.Equal(t, 1, cache.releaseCalls)
			require.NoError(t, cache.releaseCtxErr)
		})
	}
}

func openAIRefreshTestAccount() *Account {
	return &Account{
		ID: 154, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{
			"access_token": "old-access", "refresh_token": "old-refresh", "_token_version": int64(1),
		},
	}
}

func TestRefreshIfNeeded_OpenAICASKeepsConcurrentReauthorization(t *testing.T) {
	account := openAIRefreshTestAccount()
	expected := shallowCopyMap(account.Credentials)
	repo := &refreshAPIAccountRepo{account: account}
	repo.beforeSuccessCAS = func(repo *refreshAPIAccountRepo) {
		repo.account.Credentials = map[string]any{
			"access_token": "reauthorized-access", "refresh_token": "reauthorized-refresh", "_token_version": int64(2),
		}
	}
	executor := &refreshAPIExecutorStub{
		needsRefresh: true,
		credentials:  map[string]any{"access_token": "rotated-access", "refresh_token": "rotated-refresh"},
	}

	result, err := NewOAuthRefreshAPI(repo, nil).RefreshIfNeeded(context.Background(), account, executor, time.Hour)

	require.NoError(t, err)
	require.False(t, result.Refreshed)
	require.Equal(t, "reauthorized-refresh", result.Account.GetOpenAIRefreshToken())
	require.Equal(t, expected, repo.lastExpectedCredentials)
	require.Zero(t, repo.updateCalls)
}

func TestRefreshIfNeeded_OpenAIPersistenceFailureContainsProvider(t *testing.T) {
	for _, failure := range []string{"CAS write", "durable reread"} {
		t.Run(failure, func(t *testing.T) {
			account := openAIRefreshTestAccount()
			repo := &refreshAPIAccountRepo{account: account}
			if failure == "CAS write" {
				repo.updateErr = errors.New("database unavailable")
			} else {
				repo.getByIDErrAfterCall = 2
				repo.getByIDErrAfterCallErr = errors.New("database reread unavailable")
			}
			executor := &refreshAPIExecutorStub{
				needsRefresh: true,
				credentials:  map[string]any{"access_token": "rotated-access", "refresh_token": "rotated-refresh"},
			}

			_, err := NewOAuthRefreshAPI(repo, nil).RefreshIfNeeded(context.Background(), account, executor, time.Hour)

			var containment *providerCycleContainmentRefreshError
			require.ErrorAs(t, err, &containment)
			require.Equal(t, 1, executor.refreshCalls)
			require.Equal(t, 1, repo.openAISuccessCASCalls)
			if failure == "CAS write" {
				require.Equal(t, "old-refresh", account.GetOpenAIRefreshToken(), "failed persistence must not publish in-memory credentials")
			}
		})
	}
}

func TestRefreshIfNeeded_OpenAICanceledWithoutSuccessfulCredentialsDoesNotPersist(t *testing.T) {
	for _, refreshErr := range []error{nil, errors.New("token endpoint failed")} {
		t.Run(fmt.Sprint(refreshErr), func(t *testing.T) {
			account := openAIRefreshTestAccount()
			repo := &refreshAPIAccountRepo{account: account}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			executor := &refreshAPIExecutorStub{needsRefresh: true, err: refreshErr, onRefresh: cancel}

			_, err := NewOAuthRefreshAPI(repo, nil).RefreshIfNeeded(ctx, account, executor, time.Hour)

			require.Error(t, err)
			require.Zero(t, repo.openAISuccessCASCalls)
			require.Zero(t, repo.updateCalls)
		})
	}
}

func (r *refreshAPIAccountRepo) UpdateOpenAIOAuthCredentialsIfUnchanged(
	ctx context.Context,
	id int64,
	expectedCredentials map[string]any,
	expectedProxyID *int64,
	credentials map[string]any,
) (bool, error) {
	r.openAISuccessCASCalls++
	r.openAIPersistCtxErr = ctx.Err()
	r.openAIPersistDeadline, _ = ctx.Deadline()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.lastExpectedCredentials = shallowCopyMap(expectedCredentials)
	r.lastExpectedProxyID = expectedProxyID
	if r.beforeSuccessCAS != nil {
		r.beforeSuccessCAS(r)
	}
	if r.updateErr != nil {
		return false, r.updateErr
	}
	if r.account == nil || r.account.ID != id || r.account.Platform != PlatformOpenAI ||
		r.account.Type != AccountTypeOAuth || r.account.IsCredentialShadow() ||
		!reflect.DeepEqual(r.account.Credentials, expectedCredentials) ||
		!reflect.DeepEqual(r.account.ProxyID, expectedProxyID) {
		return false, nil
	}
	r.updateCalls++
	r.updateCredentialsCalls++
	r.account.Credentials = shallowCopyMap(credentials)
	return true, nil
}

func TestOAuthRefreshAPI_RefreshNowForcesFutureExpiryRefresh(t *testing.T) {
	account := &Account{
		ID:       901,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "old-access-token",
			"refresh_token": "old-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &refreshAPIAccountRepo{account: account}
	executor := &refreshAPIExecutorStub{
		needsRefresh: false,
		credentials: map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}

	result, err := NewOAuthRefreshAPI(repo, nil).RefreshNow(context.Background(), account, executor)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Refreshed)
	require.Equal(t, 1, executor.refreshCalls)
	require.Equal(t, 1, repo.updateCredentialsCalls)
	require.Equal(t, "new-access-token", result.Account.GetCredential("access_token"))
}

func TestOpenAITokenProvider_RefreshAfterUnauthorizedForcesRefresh(t *testing.T) {
	account := &Account{
		ID:       902,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Status:   StatusActive,
		Credentials: map[string]any{
			"access_token":  "old-access-token",
			"refresh_token": "old-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	repo := &refreshAPIAccountRepo{account: account}
	cache := &refreshAPICacheStub{lockResult: true}
	executor := &refreshAPIExecutorStub{
		needsRefresh: false,
		credentials: map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_at":    time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		},
	}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), executor)

	err := provider.RefreshAfterUnauthorized(context.Background(), account)

	require.NoError(t, err)
	require.Equal(t, "new-access-token", account.GetOpenAIAccessToken())
	require.Equal(t, 1, executor.refreshCalls)
	require.Equal(t, 2, cache.deleteCalls, "invalidate before using the rejected token, then after the durable rotation")
}
