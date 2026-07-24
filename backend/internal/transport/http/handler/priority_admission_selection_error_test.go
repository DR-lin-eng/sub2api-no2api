package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type priorityAdmissionSelectionAccountRepo struct {
	service.AccountRepository
	account service.Account
}

func (r *priorityAdmissionSelectionAccountRepo) ListSchedulableByPlatform(context.Context, string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

func (r *priorityAdmissionSelectionAccountRepo) ListSchedulableByGroupIDAndPlatform(context.Context, int64, string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

func (r *priorityAdmissionSelectionAccountRepo) ListSchedulableUngroupedByPlatform(context.Context, string) ([]service.Account, error) {
	return []service.Account{r.account}, nil
}

func (r *priorityAdmissionSelectionAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if id != r.account.ID {
		return nil, nil
	}
	account := r.account
	return &account, nil
}

type priorityAdmissionStreamingCache struct {
	concurrencyCacheMock
	userWaitAttempts atomic.Int32
}

func (c *priorityAdmissionStreamingCache) AcquirePriorityAccountSlot(context.Context, service.PriorityAccountAdmissionRequest) (service.PriorityAccountAdmissionStatus, error) {
	return service.PriorityAccountAdmissionRejected, errors.New("redis unavailable")
}

func (c *priorityAdmissionStreamingCache) CancelPriorityAccountWait(context.Context, int64, string) error {
	return nil
}

func (c *priorityAdmissionStreamingCache) AcquirePriorityUserSlot(_ context.Context, request service.PriorityUserAdmissionRequest) (service.PriorityAccountAdmissionStatus, error) {
	if !request.Register {
		return service.PriorityAccountAdmissionWaiting, nil
	}
	if c.userWaitAttempts.Add(1) == 1 {
		return service.PriorityAccountAdmissionWaiting, nil
	}
	return service.PriorityAccountAdmissionAcquired, nil
}

func (c *priorityAdmissionStreamingCache) CancelPriorityUserWait(context.Context, int64, string) error {
	return nil
}

func newPriorityAdmissionSelectionHandler(
	t *testing.T,
	cache service.ConcurrencyCache,
	pingFormat SSEPingFormat,
	pingInterval time.Duration,
) (*OpenAIGatewayHandler, *service.APIKey, middleware.AuthSubject) {
	t.Helper()

	groupID := int64(2)
	account := service.Account{
		ID:          9902,
		Name:        "priority-admission-test",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://api.example.test",
		},
		Extra: map[string]any{
			"openai_passthrough":                            true,
			"openai_apikey_responses_websockets_v2_enabled": true,
			"openai_apikey_responses_websockets_v2_mode":    service.OpenAIWSIngressModePassthrough,
		},
	}

	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true

	concurrency := service.NewConcurrencyService(cache)
	concurrency.SetPriorityAdmissionRuntimeConfig(service.PriorityAdmissionRuntimeConfig{
		Enabled:                 true,
		PendingLimitPerInstance: 256,
		PendingBytesPerInstance: 256 << 20,
	})
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingCache.Stop)
	gateway := service.NewOpenAIGatewayService(
		&priorityAdmissionSelectionAccountRepo{account: account},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		nil,
		concurrency,
		service.NewBillingService(cfg, nil),
		nil,
		billingCache,
		nil,
		&service.DeferredService{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	h := NewOpenAIGatewayHandler(
		gateway,
		concurrency,
		billingCache,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg,
	)
	h.concurrencyHelper = NewConcurrencyHelper(concurrency, pingFormat, pingInterval)

	user := &service.User{ID: 1704, Status: service.StatusActive}
	group := &service.Group{ID: groupID, Platform: service.PlatformOpenAI, Status: service.StatusActive}
	apiKey := &service.APIKey{ID: 1804, GroupID: &groupID, User: user, Group: group}
	subject := middleware.AuthSubject{
		UserID:         user.ID,
		SchedulingTier: service.RequestSchedulingTierNormal,
	}
	return h, apiKey, subject
}

func newPriorityAdmissionResponsesContext(
	t *testing.T,
	h *OpenAIGatewayHandler,
	apiKey *service.APIKey,
	subject middleware.AuthSubject,
	stream bool,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := `{"model":"gpt-5.2","input":"hello","stream":false}`
	if stream {
		body = `{"model":"gpt-5.2","input":"hello","stream":true}`
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(string(middleware.ContextKeyAPIKey), apiKey)
	c.Set(string(middleware.ContextKeyUser), subject)
	h.Responses(c)
	return c, recorder
}

func TestOpenAIResponsesPriorityAdmissionUnavailableReturnsGeneric503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, apiKey, subject := newPriorityAdmissionSelectionHandler(t, nil, SSEPingFormatNone, time.Second)
	c, recorder := newPriorityAdmissionResponsesContext(t, h, apiKey, subject, false)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Equal(t, "api_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
	require.Equal(t, "Service temporarily unavailable, please retry later", gjson.GetBytes(recorder.Body.Bytes(), "error.message").String())
	require.False(t, isOpsRoutingCapacityLimited(c))
}

func TestOpenAIResponsesPriorityAdmissionUnavailableAfterSSEStartKeepsIntended503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &priorityAdmissionStreamingCache{}
	h, apiKey, subject := newPriorityAdmissionSelectionHandler(t, cache, SSEPingFormatComment, time.Millisecond)
	subject.Concurrency = 1
	c, recorder := newPriorityAdmissionResponsesContext(t, h, apiKey, subject, true)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "event: response.failed\n")
	streamErr, ok := service.GetOpsStreamError(c)
	require.True(t, ok)
	require.Equal(t, http.StatusServiceUnavailable, streamErr.IntendedStatus)
	require.Equal(t, "api_error", streamErr.ErrType)
	require.False(t, isOpsRoutingCapacityLimited(c))
}

func TestOpenAIResponsesWebSocketPriorityAdmissionUnavailableCloses1011(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _, subject := newPriorityAdmissionSelectionHandler(t, nil, SSEPingFormatNone, time.Second)
	server := newOpenAIWSHandlerTestServer(t, h, subject)
	defer server.Close()

	dialCtx, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	client, _, err := websocket.Dial(dialCtx, "ws"+strings.TrimPrefix(server.URL, "http")+"/openai/v1/responses", nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()

	writeCtx, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = client.Write(writeCtx, websocket.MessageText, []byte(`{"type":"response.create","model":"gpt-5.2","stream":true}`))
	cancelWrite()
	require.NoError(t, err)

	readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, _, err = client.Read(readCtx)
	cancelRead()
	var closeErr websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.StatusInternalError, closeErr.Code)
	require.NotEqual(t, websocket.StatusTryAgainLater, closeErr.Code)
	require.Equal(t, "service temporarily unavailable", closeErr.Reason)
}
