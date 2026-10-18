package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type customStreamingHTTPUpstream struct {
	endpoint *url.URL
	client   *http.Client
}

func (u *customStreamingHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	req.URL.Scheme, req.URL.Host = u.endpoint.Scheme, u.endpoint.Host
	return u.client.Do(req)
}

func (u *customStreamingHTTPUpstream) DoWithTLS(req *http.Request, proxy string, account int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, account, concurrency)
}

// The mock withholds all done events until the real HTTP client receives the
// complete input. A buffering regression therefore times out without needing
// an expensive model request or executing any tool.
func TestResponsesCustomInputHTTPBeforeUpstreamDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"native", "grok", "chat_fallback"} {
		t.Run(mode, func(t *testing.T) {
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseDone := func() { releaseOnce.Do(func() { close(release) }) }
			var requests atomic.Int32
			requestBody := make(chan []byte, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				select {
				case requestBody <- data:
				default:
					t.Error("unexpected extra upstream request")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				flusher, ok := w.(http.Flusher)
				if !ok {
					t.Error("mock upstream does not support flushing")
					return
				}
				write := func(payload string) {
					if _, err := io.WriteString(w, passthroughSSEData(payload)); err != nil {
						t.Error(err)
					}
					flusher.Flush()
				}
				if mode == "chat_fallback" {
					write(`{"id":"cc_custom","model":"gpt-5.4","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_exec","type":"function","function":{"name":"exec","arguments":"{\"input\":\"hello"}}]}}]}`)
					write(`{"id":"cc_custom","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" world\"}"}}]}}]}`)
				} else {
					itemType, itemID := "function_call", "fc_exec"
					if mode == "native" {
						itemType, itemID = "custom_tool_call", "ctc_exec"
					}
					write(fmt.Sprintf(`{"type":"response.output_item.added","sequence_number":10,"item":{"type":%q,"id":%q,"call_id":"call_exec","name":"exec","status":"in_progress"}}`, itemType, itemID))
					if mode == "native" {
						write(`{"type":"response.custom_tool_call_input.delta","sequence_number":11,"item_id":"ctc_exec","delta":"hello"}`)
						write(`{"type":"response.custom_tool_call_input.delta","sequence_number":12,"item_id":"ctc_exec","delta":" world"}`)
					} else {
						write(functionArgsDeltaJSON(0, itemID, "call_exec", "exec", `{"input":"hello`))
						write(functionArgsDeltaJSON(0, itemID, "call_exec", "exec", ` world"}`))
					}
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				if mode == "chat_fallback" {
					write(`{"id":"cc_custom","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`)
				} else {
					item := `{"type":"custom_tool_call","id":"ctc_exec","call_id":"call_exec","name":"exec","input":"hello world","status":"completed"}`
					if mode == "native" {
						write(`{"type":"response.custom_tool_call_input.done","sequence_number":13,"item_id":"ctc_exec","input":"hello world"}`)
					} else {
						write(functionArgsDoneJSON(0, "fc_exec", "call_exec", "exec", `{"input":"hello world"}`))
						item = `{"type":"function_call","id":"fc_exec","call_id":"call_exec","name":"exec","arguments":"{\"input\":\"hello world\"}","status":"completed"}`
					}
					write(`{"type":"response.output_item.done","sequence_number":14,"item":` + item + `}`)
					write(`{"type":"response.completed","sequence_number":15,"response":{"id":"resp_http","created_at":1,"output":[` + item + `],"usage":{"input_tokens":3,"output_tokens":4}}}`)
				}
				write(`[DONE]`)
			}))
			defer upstream.Close()
			defer releaseDone()
			endpoint, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			account := passthroughArgsFallbackAccount()
			account.Extra = map[string]any{"openai_passthrough": true,
				openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceResponses)}
			if mode == "grok" {
				account.Platform = PlatformGrok
			} else if mode == "chat_fallback" {
				account.Extra[openai_compat.ExtraKeyResponsesMode] = string(openai_compat.ResponsesSupportModeForceChatCompletions)
			}
			svc := &OpenAIGatewayService{cfg: passthroughArgsTestConfig(), httpUpstream: &customStreamingHTTPUpstream{endpoint: endpoint, client: upstream.Client()}}
			type forwardResult struct {
				result *OpenAIForwardResult
				err    error
			}
			finished := make(chan forwardResult, 1)
			router := gin.New()
			router.POST("/v1/responses", func(c *gin.Context) {
				body, err := io.ReadAll(c.Request.Body)
				if err != nil {
					finished <- forwardResult{err: err}
					return
				}
				result, err := svc.Forward(c.Request.Context(), c, account, body)
				finished <- forwardResult{result: result, err: err}
			})
			gateway := httptest.NewServer(router)
			defer gateway.Close()
			// Release the upstream before closing the gateway on any failure.
			defer releaseDone()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			body := []byte(`{"model":"gpt-5.4","stream":true,"input":"test custom tool","tools":[{"type":"custom","name":"exec"}]}`)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+"/v1/responses", bytes.NewReader(body))
			require.NoError(t, err)
			resp, err := gateway.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
			var input strings.Builder
			scanner := bufio.NewScanner(resp.Body)
			for scanner.Scan() {
				data := strings.TrimPrefix(scanner.Text(), "data: ")
				if gjson.Get(data, "type").String() == "response.custom_tool_call_input.delta" {
					_, _ = input.WriteString(gjson.Get(data, "delta").String())
				}
				if input.String() == "hello world" {
					break
				}
			}
			require.NoError(t, scanner.Err())
			require.Equal(t, "hello world", input.String(), "HTTP client must receive all input before upstream done")
			t.Logf("mode=%s before_done=%q", mode, input.String())
			releaseDone()
			for scanner.Scan() {
				data := strings.TrimPrefix(scanner.Text(), "data: ")
				if gjson.Get(data, "type").String() == "response.custom_tool_call_input.delta" {
					t.Error("completion repeated tool input")
				}
			}
			require.NoError(t, scanner.Err())
			var outcome forwardResult
			select {
			case outcome = <-finished:
			case <-ctx.Done():
				t.Fatal("gateway did not finish after upstream done")
			}
			require.NoError(t, outcome.err)
			require.NotNil(t, outcome.result)
			require.Equal(t, 3, outcome.result.Usage.InputTokens)
			require.Equal(t, 4, outcome.result.Usage.OutputTokens)
			require.Equal(t, int32(1), requests.Load())
			toolType := "function"
			if mode == "native" {
				toolType = "custom"
			}
			require.Equal(t, toolType, gjson.GetBytes(<-requestBody, "tools.0.type").String())
		})
	}
}
