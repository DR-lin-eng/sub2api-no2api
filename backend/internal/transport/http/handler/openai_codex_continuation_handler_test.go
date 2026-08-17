//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIGatewayHandlerCodexContinuationMismatchDoesNotFailOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(4501)
	repo := &codexContinuationHandlerAccountRepo{
		first: codexContinuationHandlerAccount(101, 1, "principal-a"),
		other: codexContinuationHandlerAccount(102, 0, "principal-b"),
	}
	upstream := &codexContinuationHandlerUpstream{}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Gateway.MaxAccountSwitches = 3
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{
		IdentitySecret:        "handler-codex-continuation-secret-32",
		ContinuationMode:      "enforce",
		StateTTLSeconds:       7 * 24 * 60 * 60,
		FullSimulationEnabled: false,
	}
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	handler := NewOpenAIGatewayHandler(
		gateway,
		service.NewConcurrencyService(nil),
		billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil, nil, nil, nil, cfg,
	)
	handler.maxAccountSwitches = 3

	firstBody := []byte(`{"model":"gpt-5.4","stream":false,"input":"hello"}`)
	firstContext, firstRecorder := newCodexContinuationHandlerContext(groupID, firstBody)
	handler.Responses(firstContext)
	require.Equal(t, http.StatusOK, firstRecorder.Code, firstRecorder.Body.String())
	require.Equal(t, "resp_owner", gjson.GetBytes(firstRecorder.Body.Bytes(), "id").String())
	require.Equal(t, []int64{101}, upstream.accountHits())
	require.Empty(t, upstream.projectHeaders()[0], "ingress-only project header must not reach upstream")

	repo.useOtherFirst()
	secondBody := []byte(`{"model":"gpt-5.4","stream":false,"input":[{"type":"item_reference","id":"unresolved-item"}]}`)
	secondContext, secondRecorder := newCodexContinuationHandlerContext(groupID, secondBody)
	handler.Responses(secondContext)

	require.Equal(t, http.StatusConflict, secondRecorder.Code, secondRecorder.Body.String())
	require.Equal(t, "invalid_request_error", gjson.GetBytes(secondRecorder.Body.Bytes(), "error.type").String())
	require.Contains(t, gjson.GetBytes(secondRecorder.Body.Bytes(), "error.message").String(), "cannot migrate")
	require.Equal(t, []int64{101}, upstream.accountHits(), "terminal mismatch must not touch principal B or fail over to principal A")
}

func codexContinuationHandlerAccount(id int64, priority int, principal string) service.Account {
	return service.Account{
		ID:          id,
		Name:        principal,
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Priority:    priority,
		Credentials: map[string]any{
			"access_token":       "token-" + principal,
			"chatgpt_account_id": principal,
		},
	}
}

type codexContinuationHandlerAccountRepo struct {
	service.AccountRepository
	mu         sync.RWMutex
	otherFirst bool
	first      service.Account
	other      service.Account
}

func (r *codexContinuationHandlerAccountRepo) useOtherFirst() {
	r.mu.Lock()
	r.otherFirst = true
	r.mu.Unlock()
}

func (r *codexContinuationHandlerAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, account := range []service.Account{r.first, r.other} {
		if account.ID == id {
			copyAccount := account
			return &copyAccount, nil
		}
	}
	return nil, service.ErrNoAvailableAccounts
}

func (r *codexContinuationHandlerAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]service.Account, error) {
	return r.accounts(platform), nil
}

func (r *codexContinuationHandlerAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.accounts(platform), nil
}

func (r *codexContinuationHandlerAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.accounts(platform), nil
}

func (r *codexContinuationHandlerAccountRepo) accounts(platform string) []service.Account {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.first.Platform != platform {
		return nil
	}
	if !r.otherFirst {
		return []service.Account{r.first}
	}
	return []service.Account{r.other, r.first}
}

type codexContinuationHandlerUpstream struct {
	service.HTTPUpstream
	mu       sync.Mutex
	hits     []int64
	projects []string
}

func (u *codexContinuationHandlerUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.hits = append(u.hits, accountID)
	u.projects = append(u.projects, req.Header.Get(service.CodexProjectIDHeader))
	u.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(bytes.NewBufferString(
			`{"id":"resp_owner","object":"response","model":"gpt-5.4","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
		)),
	}, nil
}

func (u *codexContinuationHandlerUpstream) accountHits() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.hits...)
}

func (u *codexContinuationHandlerUpstream) projectHeaders() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.projects...)
}

func newCodexContinuationHandlerContext(groupID int64, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("thread-id", "handler-thread")
	c.Request.Header.Set(service.CodexProjectIDHeader, "handler-project")
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      4502,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive},
		User:    &service.User{ID: 4503, Status: service.StatusActive},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 4503, Concurrency: 0})
	return c, recorder
}
