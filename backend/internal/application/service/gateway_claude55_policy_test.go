package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClaude55NativePolicyMatrix(t *testing.T) {
	cases := []struct {
		model, body string
		valid       bool
	}{
		{"opus", `{}`, true}, {"opus", `{"thinking":{"type":"enabled"}}`, false}, {"opus", `{"thinking":{"type":"disabled"}}`, false},
		{"opus", `{"tool_choice":{"type":"any"}}`, false}, {"opus", `{"output_config":{"effort":"unknown"}}`, false},
		{"sonnet", `{"thinking":{"type":"between_tools"},"output_config":{"effort":"high"}}`, true},
		{"sonnet", `{"thinking":{"type":"between_tools","budget_tokens":10}}`, false},
		{"sonnet", `{"thinking":{"type":"between_tools"},"output_config":{"effort":"max"}}`, false},
		{"sonnet", `{"temperature":1,"top_p":0.99}`, true}, {"sonnet", `{"temperature":0.5}`, false},
		{"sonnet", `{"top_k":1}`, false}, {"sonnet", `{"tool_choice":"required"}`, false},
		{"haiku", `{"thinking":{"type":"disabled"},"output_config":{"effort":"high"}}`, true},
		{"haiku", `{"thinking":{"type":"disabled"},"output_config":{"effort":"xhigh"}}`, false},
		{"haiku", `{"thinking":{"type":"enabled"}}`, false}, {"haiku", `{"thinking":{"type":"between_tools"}}`, false},
		{"haiku", `{"tool_choice":{"type":"tool","name":"read"}}`, true},
		{"haiku", `{"temperature":1,"top_p":0.99}`, false}, {"haiku", `{"top_p":1}`, false}, {"haiku", `{"top_p":0.99}`, true},
	}
	for _, c := range cases {
		t.Run(c.model+c.body, func(t *testing.T) {
			err := validateClaude55Request([]byte(c.body), "claude-"+c.model+"-5-5")
			if c.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	require.NoError(t, validateClaude55Request([]byte(`{"thinking":{"type":"enabled"}}`), "claude-sonnet-4-6"))
}

func TestClaude55AliasesValidateBeforeCredentialsAcrossEntrypoints(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"alias": "claude-opus-5-5"}}}
	for _, route := range []string{"messages", "count", "responses", "chat", "native-responses", "native-chat"} {
		t.Run(route, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			gateway := &GatewayService{}
			openai := &OpenAIGatewayService{cfg: &config.Config{}}
			var err error
			if route == "messages" || route == "count" {
				body := []byte(`{"model":"alias","messages":[{"role":"user","content":"test"}],"thinking":{"type":"enabled"}}`)
				parsed, e := ParseGatewayRequest(NewRequestBodyRef(body), PlatformAnthropic)
				require.NoError(t, e)
				if route == "messages" {
					_, err = gateway.Forward(context.Background(), c, account, parsed)
				} else {
					err = gateway.ForwardCountTokens(context.Background(), c, account, parsed)
				}
			} else {
				input := "input"
				if strings.Contains(route, "chat") {
					input = "messages"
				}
				choice := `"required"`
				body := []byte(`{"model":"alias","` + input + `":[{"role":"user","content":"test"}],"tools":[{"type":"function","name":"read","function":{"name":"read","parameters":{"type":"object"}}}],"tool_choice":` + choice + `}`)
				switch route {
				case "responses":
					_, err = gateway.ForwardAsResponses(context.Background(), c, account, body, nil)
				case "chat":
					_, err = gateway.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
				case "native-responses":
					_, err = openai.forwardResponsesViaNativeAnthropic(context.Background(), c, account, body, "")
				case "native-chat":
					_, err = openai.forwardChatCompletionsViaNativeAnthropic(context.Background(), c, account, body, "")
				}
			}
			require.Error(t, err)
			require.Equal(t, 400, rec.Code)
			require.Contains(t, err.Error(), "claude-opus-5-5")
		})
	}
}

func TestClaude55ResponsesReasoningModes(t *testing.T) {
	for _, family := range []string{"opus", "sonnet", "haiku"} {
		for _, effort := range []string{"", "none", "low", "medium", "high", "xhigh", "max", "invalid"} {
			input := &apicompat.ResponsesRequest{Model: "claude-" + family + "-5-5", Input: json.RawMessage(`"hello"`), Reasoning: &apicompat.ResponsesReasoning{Effort: effort}}
			out, err := apicompat.ResponsesToAnthropicRequest(input)
			if effort == "invalid" || family == "opus" && effort == "none" {
				require.Error(t, err)
				continue
			}
			require.NoError(t, err)
			mode := "adaptive"
			if effort == "none" {
				mode = "disabled"
				if family == "sonnet" {
					mode = "between_tools"
				}
			}
			require.Equal(t, mode, out.Thinking.Type)
			body, _ := json.Marshal(out)
			require.NoError(t, validateClaude55Request(body, out.Model))
		}
	}
}

func BenchmarkClaude55ValidationLargeHistory(b *testing.B) {
	body := []byte(`{"model":"claude-opus-5-5","messages":[{"role":"user","content":"` + strings.Repeat("x", 1<<20) + `"}],"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for i := 0; i < b.N; i++ {
		if err := validateClaude55Request(body, "claude-opus-5-5"); err != nil {
			b.Fatal(err)
		}
	}
}
