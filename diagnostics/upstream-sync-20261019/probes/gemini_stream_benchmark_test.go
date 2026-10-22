//go:build unit

package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type syncBenchmarkWriter struct{ header http.Header }

func (w *syncBenchmarkWriter) Header() http.Header         { return w.header }
func (w *syncBenchmarkWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *syncBenchmarkWriter) WriteString(s string) (int, error) { return len(s), nil }
func (w *syncBenchmarkWriter) WriteHeader(int)            {}
func (w *syncBenchmarkWriter) Flush()                     {}

func BenchmarkUpstreamSyncGeminiStream(b *testing.B) {
	gin.SetMode(gin.TestMode)
	chunk := "data: " + `{"candidates":[{"content":{"parts":[{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}` + "\n\n"
	final := "data: " + `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}` + "\n\n"
	for _, route := range []string{"native", "messages", "chat", "native-8MiB-image"} {
		b.Run(route, func(b *testing.B) {
			body := strings.Repeat(chunk, 64) + final
			if route == "native-8MiB-image" {
				body = "data: " + `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + strings.Repeat("QUJD", 2<<20) + `"}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}` + "\n\n"
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for b.Loop() {
				c, _ := gin.CreateTestContext(&syncBenchmarkWriter{header: http.Header{}})
				c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
				beginGeminiImageOutputObservation(c)
				resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
				svc := &GeminiMessagesCompatService{}
				var err error
				switch route {
				case "native", "native-8MiB-image":
					_, err = svc.handleNativeStreamingResponse(c, resp, time.Now(), false)
				case "messages":
					_, err = svc.handleStreamingResponse(c, resp, time.Now(), "gemini-2.5-flash")
				case "chat":
					_, err = svc.handleChatCompletionsStreamingResponseFromGemini(c, resp, time.Now(), "gemini-2.5-flash", false, true)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
