//go:build unit

package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type policyForwardingUpstream struct {
	service.HTTPUpstream
	mu                sync.Mutex
	attempts          int
	status            int
	contentType, body string
}

func (u *policyForwardingUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.attempts++
	u.mu.Unlock()
	return &http.Response{StatusCode: u.status, Header: http.Header{
		"Content-Type": {u.contentType}, "X-Request-Id": {"policy-fixture"},
	}, Body: io.NopCloser(strings.NewReader(u.body))}, nil
}

func TestOpenAIPolicyErrorForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const message = "Your request was rejected as a result of our safety system."
	policy := `{"type":"invalid_request_error","code":"content_policy_violation","message":"` + message + `"}`
	cyber := `{"type":"invalid_request_error","code":"cyber_policy","message":"` + message + `"}`
	failed := func(err string) string {
		return `{"type":"response.failed","response":{"id":"resp_policy","object":"response","status":"failed","error":` + err + `,"output":[],"usage":{"input_tokens":7,"output_tokens":0,"total_tokens":7}}}`
	}
	sse := func(payload string) string { return "data: " + payload + "\n\n" }
	cases := []struct {
		name, contentType, body, code string
		status, wantStatus            int
		streamOnly                    bool
	}{
		{"http400_policy", "application/json", `{"error":` + policy + `}`, "content_policy_violation", 400, 400, false},
		{"http403_policy", "application/json", `{"error":` + policy + `}`, "content_policy_violation", 403, 403, false},
		{"http502_policy", "application/json", `{"error":` + policy + `}`, "content_policy_violation", 502, 400, false},
		{"http400_cyber", "application/json", `{"error":` + cyber + `}`, "cyber_policy", 400, 400, false},
		{"http403_cyber", "application/json", `{"error":` + cyber + `}`, "cyber_policy", 403, 403, false},
		{"sse_failed_policy", "text/event-stream", sse(failed(policy)), "content_policy_violation", 200, 400, false},
		{"sse_bare_policy", "text/event-stream", "event: error\n" + sse(`{"type":"error","error":`+policy+`}`), "content_policy_violation", 200, 400, false},
		{"sse_bare_cyber", "text/event-stream", "event: error\n" + sse(`{"type":"error","error":`+cyber+`}`), "cyber_policy", 200, 400, false},
		{"sse_server_policy", "text/event-stream", sse(`{"type":"error","error":{"type":"server_error","code":"content_policy_violation","message":"` + message + `"}}`), "content_policy_violation", 200, 400, false},
		{"sse_message_policy", "text/event-stream", sse(`{"type":"error","error":{"type":"invalid_request_error","message":"` + message + `"}}`), "content_policy_violation", 200, 400, false},
		{"sse_partial_policy", "text/event-stream", sse(`{"type":"response.output_text.delta","delta":"partial"}`) + "event: error\n" + sse(`{"type":"error","error":`+policy+`}`), "content_policy_violation", 200, 400, true},
		{"json_failed_policy", "application/json", failed(policy), "content_policy_violation", 200, 400, false},
	}
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, tc := range cases {
				if tc.streamOnly && !stream || tc.contentType == "application/json" && tc.status == 200 && stream {
					continue
				}
				t.Run(fmt.Sprintf("%s/passthrough=%v/stream=%v", tc.name, passthrough, stream), func(t *testing.T) {
					upstream := &policyForwardingUpstream{status: tc.status, contentType: tc.contentType, body: tc.body}
					var fixtureAccounts []service.Account
					var fixtureConfig *config.Config
					h := newOpenAIResponsesFailoverTestHandler(t, upstream, func(accounts *[]service.Account, cfg *config.Config) {
						cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 0
						for i := range *accounts {
							(*accounts)[i].Extra = map[string]any{"openai_passthrough": passthrough}
							if passthrough && !stream {
								(*accounts)[i].Type = service.AccountTypeAPIKey
								(*accounts)[i].Credentials = map[string]any{"api_key": "fixture-key"}
							}
						}
						fixtureAccounts, fixtureConfig = *accounts, cfg
					})
					h.gatewayService = service.NewOpenAIGatewayService(
						openAIImagesFailoverAccountRepo{accounts: fixtureAccounts}, nil, nil, nil, nil, nil, nil, fixtureConfig,
						nil, nil, service.NewBillingService(fixtureConfig, nil), nil, nil, upstream, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
					)
					router := gin.New()
					router.POST("/v1/responses", func(c *gin.Context) {
						fixture, _ := newOpenAIResponsesFailoverTestContext(t, context.Background())
						for key, value := range fixture.Keys {
							c.Set(key, value)
						}
						h.Responses(c)
					})
					server := httptest.NewServer(router)
					defer server.Close()
					request := fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%v,"store":false,"input":"hello","instructions":"fixture"}`, stream)
					client := &http.Client{Timeout: 5 * time.Second}
					response, err := client.Post(server.URL+"/v1/responses", "application/json", strings.NewReader(request))
					require.NoError(t, err)
					defer func() { _ = response.Body.Close() }()
					wire, readErr := io.ReadAll(response.Body)
					upstream.mu.Lock()
					attempts := upstream.attempts
					upstream.mu.Unlock()
					t.Logf("status=%d attempts=%d read_error=%v body=%s", response.StatusCode, attempts, readErr, wire)
					require.NoError(t, readErr)
					require.Equal(t, 1, attempts, "policy rejection must not replay the request on another account")
					wantStatus := tc.wantStatus
					if stream && tc.status == 200 {
						wantStatus = http.StatusOK
					}
					require.Equal(t, wantStatus, response.StatusCode)
					errorJSON := string(wire)
					if response.StatusCode == http.StatusOK {
						terminalCount := 0
						for _, line := range strings.Split(string(wire), "\n") {
							if !strings.HasPrefix(line, "data:") {
								continue
							}
							data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
							if gjson.Get(data, "type").String() == "response.failed" {
								terminalCount++
								errorJSON = gjson.Get(data, "response").Raw
							}
						}
						require.Equal(t, 1, terminalCount, "stream must contain exactly one Responses terminal failure")
						require.NotContains(t, string(wire), "event: error")
					}
					require.Equal(t, tc.code, gjson.Get(errorJSON, "error.code").String())
					require.Equal(t, message, gjson.Get(errorJSON, "error.message").String())
					require.NotContains(t, string(wire), "Upstream request failed")
					require.NotContains(t, string(wire), "upstream_stream_truncated")
				})
			}
		}
	}
}
