//go:build unit

package handler

import (
	"bufio"
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
)

type streamPoolRecoveryUpstream struct {
	service.HTTPUpstream
	mu             sync.Mutex
	accounts       []int64
	releaseFailure <-chan struct{}
	releaseSuccess <-chan struct{}
	exhausted      bool
	timeout        bool
}

func (u *streamPoolRecoveryUpstream) Do(req *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.mu.Lock()
	u.accounts = append(u.accounts, accountID)
	first := len(u.accounts) == 1
	u.mu.Unlock()
	if accountID == 5 && !u.exhausted {
		select {
		case <-u.releaseSuccess:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"recovered\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_recovered\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":1}}}\n\n"))}, nil
	}
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if _, err := io.WriteString(writer, "data: {\"type\":\"codex.rate_limits\",\"rate_limits\":{\"allowed\":true}}\n\n"); err != nil {
			return
		}
		if u.timeout {
			select {
			case <-req.Context().Done():
			case <-time.After(2500 * time.Millisecond):
			}
			return
		}
		if first {
			select {
			case <-u.releaseFailure:
			case <-req.Context().Done():
				return
			}
		}
		_, _ = io.WriteString(writer, "data: {\"type\":\"error\",\"error\":{\"type\":\"upstream_error\",\"code\":\"server_error\",\"message\":\"Transport error: network error: error decoding response body\"}}\n\n")
	}()
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}, nil
}

func TestOpenAIStreamPoolRecoveryContinuesPastTwoFirstOutputTimeouts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%v", passthrough), func(t *testing.T) {
			gate := make(chan struct{})
			close(gate)
			upstream := &streamPoolRecoveryUpstream{timeout: true, releaseFailure: gate, releaseSuccess: gate}
			h := newOpenAIResponsesFailoverTestHandler(t, upstream, func(accounts *[]service.Account, cfg *config.Config) {
				cfg.Gateway.StreamKeepaliveInterval = 1
				cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
				last := (*accounts)[0]
				last.ID, last.Priority = 5, 5
				*accounts = append(*accounts, last)
				for i := range *accounts {
					(*accounts)[i].Extra = map[string]any{"openai_passthrough": passthrough}
				}
			})
			h.maxAccountSwitches = 1
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			c, recorder := newOpenAIResponsesFailoverTestContext(t, ctx)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5.6-sol","stream":true,"store":false,"input":"hello","instructions":"fixture"}`)).WithContext(ctx)
			h.Responses(c)
			require.Contains(t, recorder.Body.String(), "recovered")
			require.NotContains(t, recorder.Body.String(), "response.failed")
			require.NotContains(t, recorder.Body.String(), `"type":"error"`)
			upstream.mu.Lock()
			calls := append([]int64(nil), upstream.accounts...)
			upstream.mu.Unlock()
			require.Equal(t, []int64{1, 2, 5}, calls)
		})
	}
}

func TestOpenAIStreamPoolRecoveryKeepsOneConnectionUntilPoolExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, exhausted := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%v/exhausted=%v", passthrough, exhausted), func(t *testing.T) {
				failureGate := make(chan struct{})
				successGate := make(chan struct{})
				var failureOnce, successOnce sync.Once
				releaseFailure := func() { failureOnce.Do(func() { close(failureGate) }) }
				releaseSuccess := func() { successOnce.Do(func() { close(successGate) }) }
				upstream := &streamPoolRecoveryUpstream{releaseFailure: failureGate, releaseSuccess: successGate, exhausted: exhausted}
				h := newOpenAIResponsesFailoverTestHandler(t, upstream, func(accounts *[]service.Account, cfg *config.Config) {
					cfg.Gateway.StreamKeepaliveInterval = 1
					cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 10
					for id := int64(3); id <= 5; id++ {
						account := (*accounts)[0]
						account.ID, account.Priority = id, int(id)
						*accounts = append(*accounts, account)
					}
					for i := range *accounts {
						(*accounts)[i].Extra = map[string]any{"openai_passthrough": passthrough}
					}
				})
				h.maxAccountSwitches = 1
				router := gin.New()
				router.POST("/v1/responses", func(c *gin.Context) {
					fixture, _ := newOpenAIResponsesFailoverTestContext(t, c.Request.Context())
					for key, value := range fixture.Keys {
						c.Set(key, value)
					}
					h.Responses(c)
				})
				server := httptest.NewTLSServer(router)
				defer server.Close()
				defer releaseFailure()
				defer releaseSuccess()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/responses", strings.NewReader(`{"model":"gpt-5.6-sol","stream":true,"store":false,"input":"hello","instructions":"fixture"}`))
				require.NoError(t, err)
				req.Header.Set("Content-Type", "application/json")
				resp, err := server.Client().Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				scanner := bufio.NewScanner(resp.Body)
				var wire strings.Builder
				heartbeats := 0
				for scanner.Scan() {
					line := scanner.Text()
					wire.WriteString(line + "\n")
					if strings.HasPrefix(line, ":") {
						heartbeats++
						if heartbeats == 1 {
							releaseFailure()
						}
						if heartbeats >= 2 {
							releaseSuccess()
						}
					}
				}
				require.NoError(t, scanner.Err())
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.GreaterOrEqual(t, heartbeats, 1)
				upstream.mu.Lock()
				calls := append([]int64(nil), upstream.accounts...)
				upstream.mu.Unlock()
				require.Equal(t, []int64{1, 2, 3, 4, 5}, calls, "try each eligible account before exposing failure")
				require.NotContains(t, wire.String(), `"type":"error"`)
				require.NotContains(t, wire.String(), "error decoding response body")
				if exhausted {
					require.Equal(t, 1, strings.Count(wire.String(), `"type":"response.failed"`))
					require.NotContains(t, wire.String(), "response.completed")
				} else {
					require.GreaterOrEqual(t, heartbeats, 2, "keepalive must cover recovery response-header waits")
					require.Contains(t, wire.String(), "recovered")
					require.Equal(t, 1, strings.Count(wire.String(), `"type":"response.completed"`))
					require.NotContains(t, wire.String(), "response.failed")
				}
			})
		}
	}
}
