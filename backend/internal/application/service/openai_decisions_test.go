package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBuildOpenAIDecisionsURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "bare domain", base: "https://api.openai.com", want: "https://api.openai.com/v1/decisions"},
		{name: "bare v1", base: "https://api.openai.com/v1", want: "https://api.openai.com/v1/decisions"},
		{name: "already decisions", base: "https://api.openai.com/v1/decisions", want: "https://api.openai.com/v1/decisions"},
		{name: "versioned relay", base: "https://relay.example/api/v4", want: "https://relay.example/api/v4/decisions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, buildOpenAIDecisionsURL(tt.base))
		})
	}
}

func TestForwardDecisionsAPIKeyPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reqBody := []byte(`{"model":"gpt-6-luna","input":"I was charged twice.","questions":[{"type":"choice","name":"department","instructions":"Which department?","choices":[{"value":"billing"}]}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/decisions", bytes.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"decision-rid"},
		},
		Body: io.NopCloser(strings.NewReader(`{"answers":[{"type":"choice","name":"department","choice":"billing"}],"usage":{"input_tokens":17}}`)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{
		ID:       42,
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "https://relay.example/v1",
			"model_mapping": map[string]any{
				"gpt-6-luna": "gpt-6-luna-relay",
			},
		},
	}

	result, err := svc.ForwardDecisions(context.Background(), c, account, reqBody)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"answers":[{"type":"choice","name":"department","choice":"billing"}],"usage":{"input_tokens":17}}`, rec.Body.String())
	require.NotNil(t, result)
	require.Equal(t, "decision-rid", result.RequestID)
	require.Equal(t, "gpt-6-luna", result.Model)
	require.Equal(t, "gpt-6-luna-relay", result.UpstreamModel)
	require.Equal(t, 17, result.Usage.InputTokens)
	require.Equal(t, "/v1/decisions", result.UpstreamEndpoint)
	require.Equal(t, "https://relay.example/v1/decisions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer sk-test", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "gpt-6-luna-relay", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "choice", gjson.GetBytes(upstream.lastBody, "questions.0.type").String())
}

func TestAccountSupportsOpenAIDecisionsOnlyForAPIKeysAndHonorsExplicitCapabilities(t *testing.T) {
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.True(t, apiKey.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisions))

	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, oauth.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisions))

	restricted := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"openai_capabilities": []any{"chat_completions"},
		},
	}
	require.False(t, restricted.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityDecisions))
}
