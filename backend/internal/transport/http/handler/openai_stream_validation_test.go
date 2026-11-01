package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAICompatibleHandlersRejectInvalidStreamFieldType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body string
		run  func(*gin.Context)
	}{
		{
			name: "gateway_responses_string_stream",
			path: "/v1/responses",
			body: `{"model":"gpt-5","stream":"true","input":"hello"}`,
			run: func(c *gin.Context) {
				(&GatewayHandler{}).Responses(c)
			},
		},
		{
			name: "gateway_responses_number_stream",
			path: "/v1/responses",
			body: `{"model":"gpt-5","stream":1,"input":"hello"}`,
			run: func(c *gin.Context) {
				(&GatewayHandler{}).Responses(c)
			},
		},
		{
			name: "gateway_chat_completions_string_stream",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5","stream":"true","messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				(&GatewayHandler{}).ChatCompletions(c)
			},
		},
		{
			name: "gateway_chat_completions_number_stream",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5","stream":1,"messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				(&GatewayHandler{}).ChatCompletions(c)
			},
		},
		{
			name: "openai_chat_completions_string_stream",
			path: "/openai/v1/chat/completions",
			body: `{"model":"gpt-5","stream":"true","messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				newOpenAIHandlerForPreviousResponseIDValidation(t, nil).ChatCompletions(c)
			},
		},
		{
			name: "openai_chat_completions_number_stream",
			path: "/openai/v1/chat/completions",
			body: `{"model":"gpt-5","stream":1,"messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				newOpenAIHandlerForPreviousResponseIDValidation(t, nil).ChatCompletions(c)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := newOpenAICompatibleStreamValidationContext(tt.path, tt.body, false)

			tt.run(c)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, invalidStreamFieldTypeMessage, gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
			require.Contains(t, rec.Body.String(), "invalid_request_error")
		})
	}
}

func TestValidateChatCompletionsOutputTokenLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		message string
		valid   bool
	}{
		{name: "negative max_tokens", body: `{"max_tokens":-1}`, message: "max_tokens must be greater than or equal to 0"},
		{name: "negative max_completion_tokens takes precedence", body: `{"max_tokens":100,"max_completion_tokens":-1}`, message: "max_completion_tokens must be greater than or equal to 0"},
		{name: "positive max_completion_tokens overrides negative max_tokens", body: `{"max_tokens":-1,"max_completion_tokens":64}`, valid: true},
		{name: "zero max_completion_tokens overrides negative max_tokens", body: `{"max_tokens":-1,"max_completion_tokens":0}`, valid: true},
		{name: "null max_completion_tokens falls back to max_tokens", body: `{"max_tokens":-1,"max_completion_tokens":null}`, message: "max_tokens must be greater than or equal to 0"},
		{name: "zero is valid", body: `{"max_tokens":0}`, valid: true},
		{name: "omitted is valid", body: `{}`, valid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, valid := validateChatCompletionsOutputTokenLimits([]byte(tt.body))
			require.Equal(t, tt.valid, valid)
			if tt.valid {
				require.Empty(t, message)
			} else {
				require.Equal(t, tt.message, message)
			}
		})
	}
}

func TestChatCompletionsHandlersRejectNegativeOutputTokenLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handlers := []struct {
		name string
		run  func(*gin.Context)
	}{
		{name: "gateway", run: (&GatewayHandler{}).ChatCompletions},
		{name: "openai", run: newOpenAIHandlerForPreviousResponseIDValidation(t, nil).ChatCompletions},
	}
	for _, handler := range handlers {
		for _, field := range []string{"max_tokens", "max_completion_tokens"} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", handler.name, field, stream), func(t *testing.T) {
					body := fmt.Sprintf(`{"model":"gpt-5","messages":[{"role":"user","content":"hi"}],"%s":-1,"stream":%t}`, field, stream)
					c, rec := newOpenAICompatibleStreamValidationContext("/v1/chat/completions", body, false)
					// The services have no routing or upstream dependencies. Reaching
					// them would fail instead of producing this validation response.
					handler.run(c)
					require.Equal(t, http.StatusBadRequest, rec.Code)
					require.Equal(t, "invalid_request_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
					require.Equal(t, field+" must be greater than or equal to 0", gjson.GetBytes(rec.Body.Bytes(), "error.message").String())
					require.NotContains(t, rec.Header().Get("Content-Type"), "text/event-stream")
				})
			}
		}
	}
}

func TestGatewayOpenAICompatibleHandlersAllowBooleanStreamToContinue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		path string
		body string
		run  func(*gin.Context)
	}{
		{
			name: "responses_false",
			path: "/v1/responses",
			body: `{"model":"gpt-5","stream":false,"input":"hello"}`,
			run: func(c *gin.Context) {
				(&GatewayHandler{gatewayService: &service.GatewayService{}}).Responses(c)
			},
		},
		{
			name: "chat_completions_true",
			path: "/v1/chat/completions",
			body: `{"model":"gpt-5","stream":true,"messages":[{"role":"user","content":"hello"}]}`,
			run: func(c *gin.Context) {
				(&GatewayHandler{gatewayService: &service.GatewayService{}}).ChatCompletions(c)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, rec := newOpenAICompatibleStreamValidationContext(tt.path, tt.body, true)

			tt.run(c)

			require.Equal(t, http.StatusForbidden, rec.Code)
			require.Contains(t, rec.Body.String(), "This group is restricted to Claude Code clients")
		})
	}
}

func newOpenAICompatibleStreamValidationContext(path, body string, claudeCodeOnly bool) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	groupID := int64(7)
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      11,
		GroupID: &groupID,
		Group:   &service.Group{ID: groupID, ClaudeCodeOnly: claudeCodeOnly},
		User:    &service.User{ID: 13},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 13, Concurrency: 1})

	return c, rec
}
