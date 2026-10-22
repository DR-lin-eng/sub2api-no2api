package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCloudflareBotBlockJSON(t *testing.T) {
	for _, body := range []string{`{"cloudflare_error":true,"code":1010}`, `{"error":{"error_name":"browser_signature_banned"}}`, `{"data":[{"cloudflare_error":true,"error_code":"1010"}]}`, `<html>Error code: 1010</html>`} {
		for _, platform := range []string{PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformAnthropic} {
			require.False(t, (&RateLimitService{}).handle403(context.Background(), &Account{Platform: platform}, "", []byte(body)))
		}
	}
	for _, body := range []string{`{"code":1010}`, `{"cloudflare_error":false,"code":1010}`, `{"cloudflare_error":true,"code":1020}`, `{"error":{"message":"account banned"}}`} {
		require.False(t, isCloudflareBotBlockResponse([]byte(body)))
	}
}
