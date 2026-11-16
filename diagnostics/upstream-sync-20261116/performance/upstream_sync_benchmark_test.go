package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/gin-gonic/gin"
)

var syncBenchmarkBody []byte

func BenchmarkUpstreamSyncMetadata(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			item := `{"type":"message","role":"user","content":"opaque data","internal_chat_message_metadata_passthrough":{"key":"value"}}`
			body := []byte(`{"store":false,"stream":true,"input":[` + strings.Repeat(item+",", count-1) + item + `]}`)
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var err error
				syncBenchmarkBody, _, err = normalizeOpenAIPassthroughOAuthBody(body, false)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
func BenchmarkUpstreamSyncCacheControlNoop(b *testing.B) {
	body := []byte(`{"messages":[{"role":"user","content":"` + strings.Repeat("x", 1<<20) + `"}]}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		syncBenchmarkBody = enforceCacheControlLimit(body)
	}
}
func BenchmarkUpstreamSyncAnthropicStream(b *testing.B) {
	gin.SetMode(gin.TestMode)
	var data bytes.Buffer
	data.WriteString("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"fixture\",\"usage\":{\"input_tokens\":500}}}\n\n")
	for range 500 {
		data.WriteString("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"fixture token\"}}\n\n")
	}
	data.WriteString("event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":500}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	raw := data.String()
	svc := &GatewayService{cfg: &config.Config{}}
	account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey}
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}
		result, err := svc.handleStreamingResponse(context.Background(), response, c, account, time.Now(), "fixture", "fixture", false)
		if err != nil || result.usage.OutputTokens != 500 {
			b.Fatalf("stream result=%+v err=%v", result, err)
		}
	}
}
