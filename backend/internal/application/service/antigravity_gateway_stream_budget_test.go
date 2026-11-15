package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type antigravityBudgetTestAdapter struct {
	emitted int
	errors  int
}

func (a *antigravityBudgetTestAdapter) Emit(*apicompat.AnthropicStreamEvent, *antigravityClientWriter) {
	a.emitted++
}

func (a *antigravityBudgetTestAdapter) Finalize(*antigravityClientWriter) {}

func (a *antigravityBudgetTestAdapter) WriteError(writer *antigravityClientWriter, reason string) {
	a.errors++
	writer.Fprintf("event: error\ndata: %q\n\n", reason)
}

func TestAntigravityCompatStreamSessionBoundsPrelude(t *testing.T) {
	adapter := &antigravityBudgetTestAdapter{}
	session := newAntigravityCompatStreamSessionWithLimits(
		"gemini-test",
		time.Now(),
		adapter,
		nil,
		antigravityStreamLimits{maxPendingEvents: 1, maxPendingBytes: 1 << 20},
	)
	prelude := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"gemini-test\",\"usage\":{}}}\n\n")

	require.NoError(t, session.consumeClaudeEvents(prelude))
	err := session.consumeClaudeEvents(prelude)
	var limitErr *antigravityStreamLimitError
	require.ErrorAs(t, err, &limitErr)
	require.Equal(t, "pending_event_count", limitErr.kind)
	require.False(t, session.hasMeaningfulData())
	require.Len(t, session.pendingEvents, 1)
	require.Zero(t, adapter.emitted)
}

func TestAntigravityCompatStreamSessionBoundsToolArgumentsBeforeOutput(t *testing.T) {
	adapter := &antigravityBudgetTestAdapter{}
	session := newAntigravityCompatStreamSessionWithLimits(
		"gemini-test",
		time.Now(),
		adapter,
		nil,
		antigravityStreamLimits{maxToolArgumentBytes: 4},
	)
	event := []byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"content_block\":{\"type\":\"tool_use\",\"name\":\"lookup\",\"input\":{\"query\":\"large\"}}}\n\n")

	err := session.consumeClaudeEvents(event)
	var limitErr *antigravityStreamLimitError
	require.ErrorAs(t, err, &limitErr)
	require.Equal(t, "tool_argument_bytes", limitErr.kind)
	require.False(t, session.hasMeaningfulData())
	require.Zero(t, adapter.emitted)
}

func TestAntigravityCompatLimitBeforeOutputFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	adapter := &antigravityBudgetTestAdapter{}
	writer := newAntigravityClientWriter(c.Writer, c.Writer, "budget test")
	session := newAntigravityCompatStreamSession("gemini-test", time.Now(), adapter, writer)
	limitErr := &antigravityStreamLimitError{kind: "pending_payload_bytes", value: 9, limit: 8}

	result, err := (&AntigravityGatewayService{}).handleAntigravityCompatLimitError(c, session, limitErr, "budget test")
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.Empty(t, recorder.Body.String())
	require.Zero(t, adapter.errors)
}

func TestAntigravityCompatLimitAfterOutputWritesOneProtocolError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	adapter := &antigravityBudgetTestAdapter{}
	writer := newAntigravityClientWriter(c.Writer, c.Writer, "budget test")
	session := newAntigravityCompatStreamSession("gemini-test", time.Now(), adapter, writer)
	session.meaningfulData = true
	limitErr := &antigravityStreamLimitError{kind: "payload_bytes", value: 9, limit: 8}

	result, err := (&AntigravityGatewayService{}).handleAntigravityCompatLimitError(c, session, limitErr, "budget test")
	require.Nil(t, result)
	require.ErrorIs(t, err, limitErr)
	require.True(t, IsResponseCommitted(c))
	require.Equal(t, 1, adapter.errors)
	require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: error"))
}

func TestCollectClaudeStreamResponseBoundsAccumulatedBody(t *testing.T) {
	body := strings.Join([]string{
		`data: {"response":{"responseId":"resp_1","candidates":[{"content":{"parts":[{"text":"first"}]}}]}}`,
		"",
		`data: {"response":{"responseId":"resp_1","candidates":[{"content":{"parts":[{"text":"second"}]},"finishReason":"STOP"}]}}`,
		"",
	}, "\n")
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	limits := defaultAntigravityStreamLimits()
	limits.maxEvents = 1

	response, result, err := (&AntigravityGatewayService{}).collectClaudeStreamResponseWithLimits(resp, time.Now(), "gemini-test", limits)
	require.Nil(t, response)
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.RetryableOnSameAccount)
}

func BenchmarkAntigravityCompatPreludeBodyBounds(b *testing.B) {
	for _, size := range []int{2 * 1024, 256 * 1024} {
		b.Run(fmt.Sprintf("%dKiB", size/1024), func(b *testing.B) {
			prefix := []byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"gemini-test\",\"usage\":{}},\"padding\":\"")
			suffix := []byte("\"}\n\n")
			data := make([]byte, 0, size)
			data = append(data, prefix...)
			data = append(data, bytes.Repeat([]byte("x"), size-len(prefix)-len(suffix))...)
			data = append(data, suffix...)
			adapter := &antigravityBudgetTestAdapter{}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				session := newAntigravityCompatStreamSession("gemini-test", time.Now(), adapter, nil)
				if err := session.consumeClaudeEvents(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAntigravityCompatProcessorPath(b *testing.B) {
	for _, size := range []int{2 * 1024, 256 * 1024} {
		line := antigravityCompatGeminiTextLine(size)
		b.Run(fmt.Sprintf("legacy_bytes_%dKiB", size/1024), func(b *testing.B) {
			adapter := &antigravityBudgetTestAdapter{}
			b.SetBytes(int64(len(line)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				session := newAntigravityCompatStreamSession("gemini-test", time.Now(), adapter, nil)
				claudeEvents := session.processor.ProcessLine(line)
				if err := session.consumeClaudeEvents(claudeEvents); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("typed_%dKiB", size/1024), func(b *testing.B) {
			adapter := &antigravityBudgetTestAdapter{}
			b.SetBytes(int64(len(line)))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				session := newAntigravityCompatStreamSession("gemini-test", time.Now(), adapter, nil)
				if err := session.consume(line); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func antigravityCompatGeminiTextLine(size int) string {
	prefix := []byte(`data: {"response":{"responseId":"resp_bench","candidates":[{"content":{"parts":[{"text":"`)
	suffix := []byte(`"}]}}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3}}}`)
	if size < len(prefix)+len(suffix) {
		size = len(prefix) + len(suffix)
	}
	line := make([]byte, 0, size)
	line = append(line, prefix...)
	line = append(line, bytes.Repeat([]byte("x"), size-len(prefix)-len(suffix))...)
	line = append(line, suffix...)
	return string(line)
}
