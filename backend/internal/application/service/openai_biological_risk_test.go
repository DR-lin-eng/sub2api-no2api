//go:build unit

package service

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/tidwall/gjson"
)

const openAIBiologicalRiskMessage = "This content was flagged for possible biological risk. If this seems wrong, try rephrasing your request. We are continuously refining our work in detecting biological risk, and you can read more about our approach in our blog post: [https://openai.com/index/preparing-for-future-ai-capabilities-in-biology](https://openai.com/index/preparing-for-future-ai-capabilities-in-biology)"
const openAIBiologicalRiskBody = `{"error":{"type":"safety_error","code":null,"message":"` + openAIBiologicalRiskMessage + `"}}`

func TestOpenAIBiologicalRiskPolicyDetails(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		policy     bool
	}{
		{"safety_error", openAIBiologicalRiskBody, true},
		{"message_only", `{"error":{"message":"` + openAIBiologicalRiskMessage + `"}}`, true},
		{"nested", `{"response":` + openAIBiologicalRiskBody + `}`, true},
		{"case_insensitive", `{"error":{"message":"THIS CONTENT WAS FLAGGED FOR POSSIBLE BIOLOGICAL RISK."}}`, true},
		{"output_quote", `{"output":[{"content":[{"text":"` + openAIBiologicalRiskMessage + `"}]}]}`, false},
		{"documentation", `{"error":{"message":"Read about biological risk detection in our documentation."}}`, false},
		{"auth_diagnostic", `{"error":{"code":"invalid_api_key","message":"` + openAIBiologicalRiskMessage + `"}}`, false},
		{"quota_diagnostic", `{"error":{"code":"rate_limit_exceeded","message":"` + openAIBiologicalRiskMessage + `"}}`, false},
		{"schema_diagnostic", `{"error":{"code":"invalid_function_parameters","message":"Invalid enum: flagged for possible biological risk"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detail, policy := openAIUpstreamPolicyErrorDetails([]byte(tc.body))
			assert.Equal(t, tc.policy, policy)
			if tc.policy {
				assert.Equal(t, "content_policy_violation", detail.Code)
			}
		})
	}
}

func TestOpenAIBiologicalRisk400Passthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []string{"responses", "passthrough", "chat", "messages", "responses_chat_only", "chat_chat_only", "messages_chat_only"} {
		for _, stream := range []bool{false, true} {
			for _, policy := range []string{"default", "custom_codes", "pool_retry", "temp_unschedulable", "rewrite_rule"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", route, stream, policy), func(t *testing.T) {
					path, body := contextWindowPassthroughRequest(route, stream)
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
					c.Request.Header.Set("Content-Type", "application/json")
					if policy == "rewrite_rule" {
						rules := &ErrorPassthroughService{}
						rules.setLocalCache([]*model.ErrorPassthroughRule{
							newNonFailoverPassthroughRule(http.StatusBadRequest, "biological risk", http.StatusBadGateway, "Upstream request failed"),
						})
						BindErrorPassthroughService(c, rules)
					}
					account := contextWindowPassthroughAccount(route, policy)
					if policy == "temp_unschedulable" {
						account.Credentials["temp_unschedulable_rules"] = []any{map[string]any{
							"error_code": float64(http.StatusBadRequest), "keywords": []any{"biological risk"}, "duration_minutes": float64(10),
						}}
					}
					upstream := &httpUpstreamRecorder{resp: newOpenAIUpstreamErrorResponse(http.StatusBadRequest, openAIBiologicalRiskBody)}
					repo := &modelNotFoundAccountRepoStub{}
					svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream, rateLimitService: &RateLimitService{accountRepo: repo}}
					result, err := forwardContextWindowRequest(svc, c, account, route, body)
					var failoverErr *UpstreamFailoverError
					failover := errors.As(err, &failoverErr)
					_, classified := openAIUpstreamPolicyErrorDetails([]byte(openAIBiologicalRiskBody))
					t.Logf("OBSERVED policy=%t status=%d calls=%d failover=%t pauses=%d body=%s", classified, recorder.Code, len(upstream.requests), failover, repo.tempCalls+len(repo.modelRateLimitCalls), recorder.Body.String())
					assert.True(t, classified)
					assert.Error(t, err)
					assert.Nil(t, result)
					assert.False(t, failover)
					assert.Equal(t, http.StatusBadRequest, recorder.Code)
					assert.Equal(t, openAIBiologicalRiskMessage, gjson.Get(recorder.Body.String(), "error.message").String())
					assert.Equal(t, "safety_error", gjson.Get(recorder.Body.String(), "error.type").String())
					if !strings.HasPrefix(route, "messages") {
						assert.Equal(t, "content_policy_violation", gjson.Get(recorder.Body.String(), "error.code").String())
					}
					assert.Len(t, upstream.requests, 1)
					assert.Zero(t, repo.tempCalls)
					assert.Empty(t, repo.modelRateLimitCalls)
					assert.Nil(t, GetOpsCyberPolicy(c))
					assert.True(t, IsResponseCommitted(c))
				})
			}
		}
	}
}
