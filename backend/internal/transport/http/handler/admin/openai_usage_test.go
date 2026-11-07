//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type usageHandlerStub struct {
	openAIQuotaWorkflowStub
	calls    atomic.Int32
	released atomic.Int32
}

func (s *usageHandlerStub) QueryUsageDetails(_ context.Context, id int64, query service.OpenAIUsageDetailsRequest) (*service.OpenAIUsageDetails, error) {
	s.calls.Add(1)
	return &service.OpenAIUsageDetails{Kind: query.Kind, Data: json.RawMessage(`{"data":[]}`), FetchedAt: id, RefreshAfterSeconds: 60}, nil
}
func (s *usageHandlerStub) SubscribeUsage(context.Context, int64) (<-chan service.OpenAIUsagePushEvent, func(), error) {
	s.calls.Add(1)
	events := make(chan service.OpenAIUsagePushEvent, 2)
	events <- service.OpenAIUsagePushEvent{Type: "quota", Transport: "websocket", Quota: &service.OpenAIQuotaUsage{FetchedAt: 41}}
	events <- service.OpenAIUsagePushEvent{Type: "fallback", Transport: "polling"}
	return events, func() { s.released.Add(1) }, nil
}

func TestOpenAIUsageHandlersValidateAndStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &usageHandlerStub{}
	handler := &OpenAIOAuthHandler{quotaService: stub}
	router := gin.New()
	router.POST("/accounts/:id/usage/query", handler.QueryUsageDetails)
	router.GET("/accounts/:id/usage/ws", handler.StreamUsage)
	for _, request := range []struct {
		path, body string
		status     int
	}{
		{"/accounts/0/usage/query", `{"kind":"daily"}`, 400},
		{"/accounts/-1/usage/query", `{"kind":"daily"}`, 400},
		{"/accounts/41/usage/query", `{`, 400},
		{"/accounts/41/usage/query", `{"kind":"daily"}`, 200},
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		require.Equal(t, request.status, recorder.Code)
	}
	require.EqualValues(t, 1, stub.calls.Load())
	server := httptest.NewServer(router)
	defer server.Close()
	dialer := websocket.Dialer{Subprotocols: []string{"sub2api-admin", "jwt.fixture-token"}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/accounts/41/usage/ws", nil)
	require.NoError(t, err)
	defer conn.Close()
	require.Equal(t, "sub2api-admin", conn.Subprotocol(), "authentication material is never echoed as the selected protocol")
	var event service.OpenAIUsagePushEvent
	require.NoError(t, conn.ReadJSON(&event))
	require.Equal(t, "websocket", event.Transport)
	require.EqualValues(t, 41, event.Quota.FetchedAt)
	require.NoError(t, conn.ReadJSON(&event))
	require.Equal(t, "fallback", event.Type)
	_, _, err = conn.ReadMessage()
	require.Error(t, err)
	require.EqualValues(t, 1, stub.released.Load())
	_, resp, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/accounts/41/usage/ws", http.Header{"Origin": []string{"https://other.example"}})
	require.Error(t, err)
	require.Equal(t, 403, resp.StatusCode)
	require.EqualValues(t, 2, stub.calls.Load(), "reject foreign origins before starting an upstream subscription")
}
