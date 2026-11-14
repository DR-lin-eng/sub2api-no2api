//go:build unit

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/chatgptimages"
	"github.com/stretchr/testify/require"
)

type chatGPTImageCurrentRepo struct {
	AccountRepository
	mu      sync.Mutex
	account *Account
}

func (r *chatGPTImageCurrentRepo) GetByID(context.Context, int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return snapshotOAuthRefreshAccount(r.account), nil
}

func TestChatGPTAppImagesRejectStalePrepare(t *testing.T) {
	for _, stage := range []string{"before_prepare", "after_prepare"} {
		t.Run(stage, func(t *testing.T) {
			account := chatGPTAppImageTestAccount()
			repo := &chatGPTImageCurrentRepo{account: snapshotOAuthRefreshAccount(account)}
			changePrincipal := func() {
				repo.mu.Lock()
				defer repo.mu.Unlock()
				repo.account.Credentials["chatgpt_account_id"] = "another-workspace"
			}
			svc, upstream := newChatGPTAppImageTestServiceWithPrepare(t, func(w http.ResponseWriter, r *http.Request) {
				require.Contains(t, r.UserAgent(), "Codex Desktop/26.1106.51428")
				if stage == "after_prepare" {
					changePrincipal()
				}
				fmt.Fprint(w, `{"token":"fixture-token"}`)
			}, func(w http.ResponseWriter, _ *http.Request) {
				t.Error("stale generation reached the upstream")
				w.WriteHeader(500)
			})
			c := newOpenAIImagesContext(t, []byte(`{"model":"gpt-image-2","prompt":"draw"}`))
			session, err := svc.newChatGPTImageSession(t.Context(), c, account)
			require.NoError(t, err)
			svc.accountRepo = repo
			if stage == "before_prepare" {
				changePrincipal()
			}
			err = session.generate(t.Context(), map[string]any{}, &chatgptimages.Decoder{})
			require.True(t, isOpenAIOAuthPrincipalChangedError(err), "%v", err)
			require.Empty(t, upstream.imageCalls())
			require.False(t, session.generationAccepted)
			upstream.mu.Lock()
			defer upstream.mu.Unlock()
			want := 0
			if stage == "after_prepare" {
				want = 1
			}
			require.Len(t, upstream.calls, want)
		})
	}
}

func TestChatGPTAppImagesCurrentCheckPreservesPrincipal(t *testing.T) {
	for _, scenario := range []string{"token_refresh", "plan_upgrade", "user_changed", "plan_category_changed", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			expected := chatGPTAppImageTestAccount()
			expected.Credentials["chatgpt_user_id"] = "fixture-user"
			current := snapshotOAuthRefreshAccount(expected)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch scenario {
			case "token_refresh":
				current.Credentials["access_token"] = "new-same-principal-token"
			case "plan_upgrade":
				current.Credentials["plan_type"] = "enterprise"
			case "user_changed":
				current.Credentials["chatgpt_user_id"] = "another-user"
			case "plan_category_changed":
				current.Credentials["plan_type"] = "plus"
			case "canceled":
				cancel()
			}
			session := &chatGPTImageSession{gateway: &OpenAIGatewayService{}, account: current, credential: expected, token: expected.GetOpenAIAccessToken()}
			err := session.assertCurrent(ctx)
			switch scenario {
			case "canceled":
				require.ErrorIs(t, err, context.Canceled)
			case "user_changed", "plan_category_changed":
				require.True(t, isOpenAIOAuthPrincipalChangedError(err))
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestOpenAIRequestCurrentCheckRunsAtSend(t *testing.T) {
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	expected := errors.New("request became stale")
	checked := false
	ctx := withOpenAIRequestCurrentCheck(t.Context(), func(context.Context) error {
		checked = true
		return expected
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid/f/conversation/prepare", nil)
	require.NoError(t, err)
	_, err = svc.doAccountHTTPUpstream(req, "", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey})
	// The callback is evaluated at the common send boundary, even for a port
	// implementation that would otherwise ignore the request's context.
	require.ErrorIs(t, err, expected)
	require.True(t, checked)
	require.Empty(t, upstream.requests)
}
