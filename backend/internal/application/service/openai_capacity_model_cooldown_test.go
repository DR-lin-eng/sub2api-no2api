//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func openAICapacityShedModelRuleAccount(accountType string) *Account {
	return &Account{
		ID: 101, Platform: PlatformOpenAI, Type: accountType, Status: StatusActive, Schedulable: true,
		Credentials: map[string]any{
			"temp_unschedulable_enabled": true,
			"temp_unschedulable_rules": []any{map[string]any{
				"error_code": http.StatusServiceUnavailable, "keywords": []any{"overloaded"},
				"duration_minutes": 5, "scope": TempUnschedScopeModel,
			}},
			"model_mapping": map[string]any{"alias": "gpt-6.1-sol", "gpt-6.1-sol": "must-not-remap"},
		},
	}
}

func TestOpenAICapacityShedModelRule_StreamReaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	readers := []struct {
		name string
		run  func(*OpenAIGatewayService, *gin.Context, *http.Response, *Account, []byte) error
	}{
		{"stream", func(s *OpenAIGatewayService, c *gin.Context, r *http.Response, a *Account, _ []byte) error {
			_, err := s.handleStreamingResponse(c.Request.Context(), r, c, a, time.Now(), "alias", "gpt-6.1-sol")
			return err
		}},
		{"passthrough", func(s *OpenAIGatewayService, c *gin.Context, r *http.Response, a *Account, _ []byte) error {
			_, err := s.handleStreamingResponsePassthrough(c.Request.Context(), r, c, a, time.Now(), "alias", "gpt-6.1-sol")
			return err
		}},
		{"buffered", func(s *OpenAIGatewayService, c *gin.Context, r *http.Response, a *Account, body []byte) error {
			_, err := s.handleSSEToJSONWithAccount(r, c, body, "alias", "gpt-6.1-sol", a, false)
			return err
		}},
		{"buffered_passthrough", func(s *OpenAIGatewayService, c *gin.Context, r *http.Response, a *Account, body []byte) error {
			_, err := s.handlePassthroughSSEToJSONWithAccount(r, c, body, "alias", "gpt-6.1-sol", a)
			return err
		}},
	}
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey} {
		for _, reader := range readers {
			for _, frame := range []struct{ name, payload string }{
				{"error", `{"type":"error","error":{"code":"server_is_overloaded","type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`},
				{"response.failed", `{"type":"response.failed","response":{"error":{"code":"slow_down","message":"overloaded"}}}`},
			} {
				t.Run(accountType+"/"+reader.name+"/"+frame.name, func(t *testing.T) {
					repo := &modelNotFoundAccountRepoStub{}
					svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
					account := openAICapacityShedModelRuleAccount(accountType)
					rec := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(rec)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
					body := "data: " + frame.payload + "\n\n"
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
						Body: io.NopCloser(strings.NewReader(body))}

					err := reader.run(svc, c, resp, account, []byte(body))

					var failover *UpstreamFailoverError
					require.ErrorAs(t, err, &failover)
					require.Len(t, repo.modelRateLimitCalls, 1)
					require.False(t, failover.RetryableOnSameAccount, "a matching model cooldown must stop same-account replay")
					require.True(t, failover.Scope == GatewayFailureScopeRequest, "do not enable generic account-wide cooldowns")
					call := repo.modelRateLimitCalls[0]
					require.Equal(t, "gpt-6.1-sol", call.scope, "use the already mapped scheduling model")
					require.WithinDuration(t, time.Now().Add(5*time.Minute), call.resetAt, time.Second)
					var state TempUnschedState
					require.NoError(t, json.Unmarshal([]byte(call.reason), &state))
					require.Equal(t, TempUnschedScopeModel, state.Scope)
					require.Equal(t, http.StatusServiceUnavailable, state.StatusCode)
					require.Zero(t, repo.tempCalls)
					require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
					require.False(t, c.Writer.Written())
					require.Empty(t, rec.Body.String())
				})
			}
		}
	}
}

func TestOpenAICapacityModelRuleHTTPAndOutputIsolation(t *testing.T) {
	for _, keyword := range []string{"overloaded", "output-only"} {
		repo := &modelNotFoundAccountRepoStub{}
		svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
		account := openAICapacityShedModelRuleAccount(AccountTypeOAuth)
		rules := account.Credentials["temp_unschedulable_rules"].([]any)
		rules[0].(map[string]any)["keywords"] = []any{keyword}
		body := []byte(`{"error":{"code":"server_is_overloaded","message":"overloaded"},"output":[{"text":"output-only"}]}`)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		failure := svc.failoverOpenAIUpstreamHTTPError(context.Background(), c, account, &http.Response{StatusCode: 503, Header: http.Header{}}, body, "overloaded", "gpt-6.1-sol")
		require.NotNil(t, failure)
		if keyword == "overloaded" {
			require.Len(t, repo.modelRateLimitCalls, 1)
			require.False(t, failure.RetryableOnSameAccount)
		} else {
			require.Empty(t, repo.modelRateLimitCalls)
			require.True(t, failure.RetryableOnSameAccount)
		}
		require.Zero(t, repo.tempCalls)
	}
}

func TestOpenAICapacityModelRulePreservesDefaultScopeAndWriteFailureIsolation(t *testing.T) {
	for _, name := range []string{"default_scope", "account_scope", "grok", "disabled", "missing_model", "write_failed"} {
		t.Run(name, func(t *testing.T) {
			repo := &modelNotFoundAccountRepoStub{}
			svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
			account := openAICapacityShedModelRuleAccount(AccountTypeOAuth)
			rule := account.Credentials["temp_unschedulable_rules"].([]any)[0].(map[string]any)
			model := "gpt-6.1-sol"
			switch name {
			case "default_scope":
				delete(rule, "scope")
			case "account_scope":
				rule["scope"] = TempUnschedScopeAccount
			case "grok":
				account.Platform = PlatformGrok
			case "disabled":
				account.Credentials["temp_unschedulable_enabled"] = false
			case "missing_model":
				model = ""
			case "write_failed":
				repo.modelRateLimitErr = errors.New("fixture write failed")
			}
			payload := []byte(`{"error":{"code":"server_is_overloaded","message":"overloaded"}}`)
			matched := svc.applyOpenAICapacityModelCooldown(context.Background(), account, 503, payload, model)
			require.Equal(t, name == "write_failed", matched, "the existing rule owner fails over this attempt even when model persistence fails")
			require.Zero(t, repo.tempCalls)
			require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
			if name != "write_failed" {
				require.Empty(t, repo.modelRateLimitCalls)
			}
		})
	}
}

func TestOpenAICapacityModelRuleJSONAfterOutputCoolsModelWithoutReplay(t *testing.T) {
	repo := &modelNotFoundAccountRepoStub{}
	svc := &OpenAIGatewayService{rateLimitService: &RateLimitService{accountRepo: repo}}
	account := openAICapacityShedModelRuleAccount(AccountTypeOAuth)
	for _, output := range []string{`[]`, `[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"already produced"}]}]`} {
		repo.modelRateLimitCalls = nil
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		body := []byte(`{"status":"failed","error":{"code":"server_is_overloaded","message":"overloaded"},"output":` + output + `}`)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}
		_, err := svc.handleNonStreamingResponse(context.Background(), resp, c, account, "alias", "gpt-6.1-sol")
		require.Error(t, err)
		require.Len(t, repo.modelRateLimitCalls, 1)
		require.Equal(t, "gpt-6.1-sol", repo.modelRateLimitCalls[0].scope)
		var failover *UpstreamFailoverError
		if output == `[]` {
			require.ErrorAs(t, err, &failover)
			require.False(t, failover.RetryableOnSameAccount)
		} else {
			require.False(t, errors.As(err, &failover), "output must never be replayed")
			require.Equal(t, http.StatusBadGateway, rec.Code)
		}
	}
}
