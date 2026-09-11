package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIDistillationSessionHeadersAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	groupID := int64(980001)
	c.Set("api_key", &APIKey{ID: 41, GroupID: &groupID, Group: &Group{ID: groupID, Platform: PlatformOpenAI, IsDistillationGroup: true}})
	c.Request, _ = http.NewRequest(http.MethodPost, "/v1/responses", nil)
	account := &Account{ID: 990001, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	counter := &distillationCounterStub{}
	svc := &OpenAIGatewayService{distillationCounterSource: counter}

	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"client-cache","client_metadata":{"session_id":"raw-session","thread_id":"raw-thread"},"input":[{"type":"input_text","text":"hello","cache_control":{"type":"ephemeral"}}]}`)
	cleaned := stripDistillationCacheFields(body)
	require.False(t, gjson.GetBytes(cleaned, "prompt_cache_key").Exists())
	require.Equal(t, "raw-session", gjson.GetBytes(cleaned, "client_metadata.session_id").String())
	require.False(t, gjson.GetBytes(cleaned, "input.0.cache_control").Exists())

	sessionID, ok := svc.DistillationSessionID(context.Background(), c, account)
	require.True(t, ok)
	headers := make(http.Header)
	applyCodexOutboundSessionHeaders(c, account, cleaned, "", headers, nil)
	require.Equal(t, sessionID, headers.Get("session-id"))
	require.Equal(t, sessionID, headers.Get("thread-id"))
	require.Equal(t, sessionID, headers.Get("x-client-request-id"))
	require.NotEmpty(t, headers.Get("session_id"))
}
