package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardAsAnthropic_PreservesMaxForFinalGPT56ResponsesModel(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		account       *Account
		model         string
		defaultMapped string
		effort        string
		wantModel     string
		wantEffort    string
	}{
		{
			name:          "API Key mapping wins and keeps Luna max",
			account:       rawGPT56ResponsesAPIKeyAccount("luna", "gpt-5.6-luna"),
			model:         "luna",
			defaultMapped: "gpt-5.6-sol",
			effort:        "max",
			wantModel:     "gpt-5.6-luna",
			wantEffort:    "max",
		},
		{
			name:          "OAuth mapping wins and keeps Sol max",
			account:       rawGPT56ResponsesOAuthAccount("sol", "gpt-5.6-sol"),
			model:         "sol",
			defaultMapped: "gpt-5.6-terra",
			effort:        "max",
			wantModel:     "gpt-5.6-sol",
			wantEffort:    "max",
		},
		{
			name:       "old model still maps max to xhigh",
			account:    rawGPT56ResponsesAPIKeyAccount("gpt-5.5", "gpt-5.5"),
			model:      "gpt-5.5",
			effort:     "max",
			wantModel:  "gpt-5.5",
			wantEffort: "xhigh",
		},
		{
			name:       "GPT56 default remains medium",
			account:    rawGPT56ResponsesAPIKeyAccount("gpt-5.6-sol", "gpt-5.6-sol"),
			model:      "gpt-5.6-sol",
			wantModel:  "gpt-5.6-sol",
			wantEffort: "medium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			body := `{"model":"` + tt.model + `","max_tokens":16,"messages":[{"role":"user","content":"hello"}`
			if tt.effort != "" {
				body += `],"output_config":{"effort":"` + tt.effort + `"},"stream":false}`
			} else {
				body += `],"stream":false}`
			}
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_gpt56_"+tt.name, tt.wantModel)}
			svc := &OpenAIGatewayService{
				httpUpstream: upstream,
				cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
			}

			result, err := svc.ForwardAsAnthropic(context.Background(), c, tt.account, []byte(body), "", tt.defaultMapped)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.wantModel, result.UpstreamModel)
			require.Equal(t, tt.wantModel, gjson.GetBytes(upstream.lastBody, "model").String())
			require.Equal(t, tt.wantEffort, gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
			require.NotNil(t, result.ReasoningEffort)
			require.Equal(t, tt.wantEffort, *result.ReasoningEffort)
		})
	}
}

func rawGPT56ResponsesAPIKeyAccount(requestedModel, mappedModel string) *Account {
	return &Account{
		ID:          501,
		Name:        "gpt56-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":       "sk-test",
			"base_url":      "https://api.example.com/v1",
			"model_mapping": map[string]any{requestedModel: mappedModel},
		},
		Extra: map[string]any{"use_responses_api": true},
	}
}

func rawGPT56ResponsesOAuthAccount(requestedModel, mappedModel string) *Account {
	return &Account{
		ID:          502,
		Name:        "gpt56-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-acc",
			"model_mapping":      map[string]any{requestedModel: mappedModel},
		},
	}
}
