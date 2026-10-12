package service

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAntigravityCompatControlOnlyStreamRemainsRetryable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, payload := range []string{
		`{"response":{"candidates":[{"content":{"parts":[]},"finishReason":"MALFORMED_FUNCTION_CALL"}]}}`,
		`{"response":{"candidates":[{"content":{"parts":[{"thought":true,"thoughtSignature":"signature-only"}]},"finishReason":"STOP"}]}}`,
	} {
		for _, chat := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/chat=%t", payload, chat), func(t *testing.T) {
				svc := newAntigravityCompatService(config.GatewayConfig{MaxLineSize: defaultMaxLineSize}, nil)
				c, rec := newAntigravityCompatContext(http.MethodPost, "/", nil)
				resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n"))}
				var result *antigravityStreamResult
				var err error
				if chat {
					result, err = svc.handleChatCompletionsStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high", true)
				} else {
					result, err = svc.handleResponsesStreamingFromAntigravity(c, resp, time.Now(), "gemini-3.1-pro-high")
				}
				require.Nil(t, result)
				var failover *UpstreamFailoverError
				require.ErrorAs(t, err, &failover)
				require.Empty(t, rec.Body.String())
				require.False(t, c.Writer.Written())
			})
		}
	}
}
