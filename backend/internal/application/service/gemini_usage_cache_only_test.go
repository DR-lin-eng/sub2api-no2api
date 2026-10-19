//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiCacheOnlyUsageIsNonNegative(t *testing.T) {
	for _, metadata := range []string{
		`{"cachedContentTokenCount":30}`,
		`{"promptTokenCount":10,"cachedContentTokenCount":30}`,
	} {
		body := `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":` + metadata + `}`
		usage := extractGeminiUsage([]byte(body))
		require.Zero(t, usage.InputTokens)
		require.Equal(t, 30, usage.CacheReadInputTokens)
		for _, stream := range []bool{false, true} {
			upstreamBody := body
			if stream {
				upstreamBody = "data: " + body + "\n\n"
			}
			stub := &geminiCompatHTTPUpstreamStub{response: &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(upstreamBody))}}
			svc := &GeminiMessagesCompatService{httpUpstream: stub, cfg: &config.Config{}}
			c := newGeminiImageTestContext(t)
			result, err := svc.ForwardNative(context.Background(), c, geminiCompatThinkingTestAccount(), "gemini-2.5-flash", "generateContent", stream, []byte(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
			require.NoError(t, err)
			require.Zero(t, result.Usage.InputTokens)
			require.Equal(t, 30, result.Usage.CacheReadInputTokens)
		}
	}
}
