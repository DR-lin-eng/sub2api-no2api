//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiStreamInternalCancellationWithLiveClientStaysError(t *testing.T) {
	for _, route := range []string{"native", "messages", "chat"} {
		t.Run(route, func(t *testing.T) {
			body := []byte(`{"model":"gemini-2.5-flash","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`)
			svc, c, _ := newGeminiAbortingStreamService(t, "/", body, geminiAbortFinalChunk, false)
			svc.httpUpstream.(*geminiCompatHTTPUpstreamStub).response.Body.(*geminiAbortingStreamBody).abortErr = context.Canceled
			var result *ForwardResult
			var err error
			switch route {
			case "native":
				result, err = svc.ForwardNative(context.Background(), c, geminiCompatThinkingTestAccount(), "gemini-2.5-flash", "streamGenerateContent", true, []byte(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
			case "messages":
				result, err = svc.Forward(context.Background(), c, geminiCompatThinkingTestAccount(), body)
			case "chat":
				result, err = svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
			}
			require.ErrorIs(t, err, context.Canceled)
			require.Nil(t, result)
		})
	}
}

func TestGeminiChatDisconnectPreservesDeliveredImageMarkdown(t *testing.T) {
	for _, usage := range []string{"", `,"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2}`} {
		sse := `data: {"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}],"role":"model"}}]` + usage + "}\n\n"
		body := []byte(`{"model":"gemini-3-pro-image","stream":true,"messages":[{"role":"user","content":"draw"}]}`)
		svc, c, rec := newGeminiAbortingStreamService(t, "/v1/chat/completions", body, sse, true)
		result, err := svc.ForwardAsChatCompletions(context.Background(), c, geminiCompatThinkingTestAccount(), body)
		require.NoError(t, err)
		require.True(t, result.ClientDisconnect)
		require.Equal(t, 1, result.ImageCount)
		require.Contains(t, rec.Body.String(), "data:image/png;base64,aGVsbG8=")
	}
}

func TestGeminiNativeWriteFailureDoesNotChargeUndeliveredImage(t *testing.T) {
	c, _ := gin.CreateTestContext(&geminiFailingResponseWriter{header: http.Header{}})
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	svc, stub := newGeminiClientAbortTestService()
	stub.response = &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(geminiAbortImageChunk))}
	result, err := svc.ForwardNative(context.Background(), c, geminiCompatThinkingTestAccount(), "gemini-3-pro-image", "streamGenerateContent", true, []byte(`{"contents":[{"parts":[{"text":"draw"}]}]}`))
	require.NoError(t, err)
	require.True(t, result.ClientDisconnect)
	require.Zero(t, result.ImageCount)
	require.Equal(t, 1290, result.Usage.OutputTokens)
}

func TestGeminiMessagesWriteFailureStopsBeforeReadingProvider(t *testing.T) {
	body := []byte(`{"model":"gemini-3-pro-image","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"draw"}]}`)
	c, _ := gin.CreateTestContext(&geminiFailingResponseWriter{header: http.Header{}})
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	svc, stub := newGeminiClientAbortTestService()
	reader := bytes.NewReader([]byte(geminiAbortImageChunk))
	stub.response = &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(reader)}
	result, err := svc.Forward(context.Background(), c, geminiCompatThinkingTestAccount(), body)
	require.NoError(t, err)
	require.True(t, result.ClientDisconnect)
	require.Zero(t, result.ImageCount)
	require.Equal(t, len(geminiAbortImageChunk), reader.Len())
}

type geminiStringOnlyWriter struct{}

func (geminiStringOnlyWriter) Write([]byte) (int, error)         { return 0, io.ErrClosedPipe }
func (geminiStringOnlyWriter) WriteString(s string) (int, error) { return len(s), nil }

func TestGeminiStreamWriteTrackerPreservesStringWriter(t *testing.T) {
	writer := &geminiStreamWriteTracker{Writer: geminiStringOnlyWriter{}}
	largeLine := strings.Repeat("data:image/png;base64,QUJD", 1<<16)
	require.Zero(t, testing.AllocsPerRun(100, func() {
		n, err := io.WriteString(writer, largeLine)
		if err != nil || n != len(largeLine) {
			t.Fatalf("string writer bypassed: n=%d err=%v", n, err)
		}
	}))
	_, err := writer.Write([]byte("broken"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	_, err = writer.WriteString("must not retry")
	require.ErrorIs(t, err, io.ErrClosedPipe)
}
