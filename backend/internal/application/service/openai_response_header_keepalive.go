package service

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// The response-body loop starts too late to protect a slow first attempt or a
// recovery account waiting for headers. Keep writes on the request goroutine;
// only the HTTP round trip runs concurrently, without touching the Gin context.
func (s *OpenAIGatewayService) doOpenAIResponsesUpstream(ctx context.Context, c *gin.Context, req *http.Request, proxyURL string, account *Account, stream bool) (*http.Response, error) {
	if !stream || account == nil || !account.IsOpenAI() || c == nil || c.Writer == nil {
		return s.doAccountHTTPUpstream(req, proxyURL, account)
	}
	interval := s.openAIStreamKeepaliveIntervalWithContext(ctx)
	if interval <= 0 {
		return s.doAccountHTTPUpstream(req, proxyURL, account)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	upstreamCtx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(upstreamCtx)
	type result struct {
		response *http.Response
		err      error
	}
	results := make(chan result)
	done := make(chan struct{})
	defer close(done)
	go func() {
		resp, err := s.doAccountHTTPUpstream(req, proxyURL, account)
		select {
		case results <- result{resp, err}:
		case <-done:
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
		}
	}()
	timer := time.NewTimer(openAIStreamKeepaliveDelay(c, interval))
	defer timer.Stop()
	for {
		select {
		case result := <-results:
			if result.err == nil && result.response != nil && result.response.Body != nil {
				result.response.Body = &openAIRequestContextReadCloser{ReadCloser: result.response.Body, cleanup: cancel}
			} else {
				cancel()
			}
			return result.response, result.err
		case <-ctx.Done():
			cancel()
			return nil, ctx.Err()
		case <-upstreamCtx.Done():
			cancel()
			return nil, upstreamCtx.Err()
		case <-timer.C:
			if !c.Writer.Written() {
				declareOpenAIStreamResponseMetadataTrailers(c)
				c.Header("Content-Type", "text/event-stream")
				c.Header("Cache-Control", "no-cache")
				c.Header("Connection", "keep-alive")
				c.Header("X-Accel-Buffering", "no")
			}
			n, err := c.Writer.Write([]byte(":\n\n"))
			recordOpenAIStreamKeepaliveBytes(c, n)
			if err != nil {
				cancel()
				return nil, context.Canceled
			}
			c.Writer.Flush()
			timer.Reset(interval)
		}
	}
}

func writeOpenAIResponsesErrorAfterKeepalive(c *gin.Context, status int, code, message string) bool {
	if c == nil || c.Writer == nil || !openAIStreamResponseMetadataTrailersActive(c) || !c.Writer.Written() {
		return false
	}
	StopOpenAICompactSSEKeepaliveCommitted(c)
	MarkResponseCommitted(c)
	writeOpenAICompactSSEFailureMessage(c, status, code, message)
	return true
}
