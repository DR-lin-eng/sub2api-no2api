package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// flushTrackingWriter separates bytes the handler has written from bytes it has
// flushed, which is what a streaming client can actually observe.
type flushTrackingWriter struct {
	gin.ResponseWriter
	mu      sync.Mutex
	pending bytes.Buffer
	visible bytes.Buffer
	flushes int
}

func (w *flushTrackingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.pending.Write(p)
	return w.ResponseWriter.Write(p)
}

func (w *flushTrackingWriter) WriteString(s string) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.pending.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func (w *flushTrackingWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushes++
	_, _ = w.pending.WriteTo(&w.visible)
	w.ResponseWriter.Flush()
}

func (w *flushTrackingWriter) snapshot() (visible string, pending int, flushes int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.visible.String(), w.pending.Len(), w.flushes
}

func newFlushTrackingContext(t *testing.T, path string) (*gin.Context, *flushTrackingWriter) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	tracker := &flushTrackingWriter{ResponseWriter: c.Writer}
	c.Writer = tracker
	return c, tracker
}

type streamFlushCase struct {
	name     string
	path     string
	delta    func(text string) string
	terminal string
	run      func(c *gin.Context, resp *http.Response, performanceMode bool) error
}

func streamFlushTestSettings(performanceMode bool) *SettingService {
	settings := &SettingService{}
	settings.streamModePerformanceEnabled.Store(performanceMode)
	settings.streamModePerformanceLoaded.Store(time.Now().UnixNano())
	return settings
}

func streamFlushCases() []streamFlushCase {
	return []streamFlushCase{
		{
			name: "anthropic messages",
			path: "/v1/messages",
			delta: func(text string) string {
				return "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"" + text + "\"}}\n\n"
			},
			terminal: "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\n" +
				"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			run: func(c *gin.Context, resp *http.Response, performanceMode bool) error {
				svc := &GatewayService{
					cfg:              &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}},
					rateLimitService: &RateLimitService{},
					settingService:   streamFlushTestSettings(performanceMode),
				}
				_, err := svc.handleStreamingResponse(context.Background(), resp, c, &Account{ID: 1}, time.Now(), "model", "model", false)
				return err
			},
		},
		{
			name: "chat completions bridge",
			path: "/v1/chat/completions",
			delta: func(text string) string {
				return "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + text + "\"}\n\n"
			},
			terminal: "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":3,\"total_tokens\":4}}}\n\n",
			run: func(c *gin.Context, resp *http.Response, performanceMode bool) error {
				// A keepalive interval selects the channel-based loop used in production.
				svc := &OpenAIGatewayService{
					cfg:            &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 30}},
					settingService: streamFlushTestSettings(performanceMode),
				}
				_, err := svc.handleChatStreamingResponse(resp, c, &Account{ID: 1, Platform: PlatformOpenAI}, "gpt-5.5", "gpt-5.5", "gpt-5.5", time.Now(), 0)
				return err
			},
		},
	}
}

// An event that arrives on its own must reach the client before the next one is
// produced: coalescing may only merge flushes for events that are already queued.
func TestStreamFlushCoalescing_IsolatedEventIsFlushedWithoutWaiting(t *testing.T) {
	for _, tc := range streamFlushCases() {
		t.Run(tc.name, func(t *testing.T) {
			c, tracker := newFlushTrackingContext(t, tc.path)
			pr, pw := io.Pipe()
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: pr}

			done := make(chan error, 1)
			go func() { done <- tc.run(c, resp, true) }()

			for _, text := range []string{"first-marker", "second-marker"} {
				_, err := pw.Write([]byte(tc.delta(text)))
				require.NoError(t, err)
				require.Eventually(t, func() bool {
					visible, _, _ := tracker.snapshot()
					return strings.Contains(visible, text)
				}, 5*time.Second, time.Millisecond, "%s was written but not flushed while upstream was idle", text)
			}

			_, err := pw.Write([]byte(tc.terminal))
			require.NoError(t, err)
			require.NoError(t, pw.Close())
			require.NoError(t, <-done)
			_, pending, _ := tracker.snapshot()
			require.Zero(t, pending, "handler returned with unflushed output")
		})
	}
}

// A burst delivered in one upstream read keeps every event and its order and
// leaves nothing unflushed in either mode. Performance mode merges the flushes
// of queued events; normal mode still flushes every event.
func TestStreamFlushCoalescing_BurstKeepsEveryEventAndFlushesTail(t *testing.T) {
	const events = 300
	for _, tc := range streamFlushCases() {
		for _, performanceMode := range []bool{true, false} {
			name := tc.name + "/normal mode"
			if performanceMode {
				name = tc.name + "/performance mode"
			}
			t.Run(name, func(t *testing.T) {
				c, tracker := newFlushTrackingContext(t, tc.path)

				var upstream strings.Builder
				markers := make([]string, 0, events)
				for i := range events {
					marker := "m" + strings.Repeat("x", i%7) + "-" + string(rune('a'+i%26)) + "-" + time.Duration(i).String()
					markers = append(markers, marker)
					_, _ = upstream.WriteString(tc.delta(marker))
				}
				_, _ = upstream.WriteString(tc.terminal)
				resp := &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
					Body:       io.NopCloser(strings.NewReader(upstream.String())),
				}

				require.NoError(t, tc.run(c, resp, performanceMode))

				visible, pending, flushes := tracker.snapshot()
				require.Zero(t, pending, "handler returned with unflushed output")
				if performanceMode {
					require.Positive(t, flushes)
					require.LessOrEqual(t, flushes, events+4)
				} else {
					require.GreaterOrEqual(t, flushes, events, "normal mode must flush every event")
				}
				offset := 0
				for _, marker := range markers {
					idx := strings.Index(visible[offset:], marker)
					require.GreaterOrEqual(t, idx, 0, "missing or reordered event %q", marker)
					offset += idx + len(marker)
				}
			})
		}
	}
}
