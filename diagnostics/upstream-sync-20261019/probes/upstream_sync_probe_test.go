//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
)

type syncProbeBody struct {
	*bytes.Reader
	cancel context.CancelFunc
}

func (b *syncProbeBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.cancel()
		return n, context.Canceled
	}
	return n, err
}
func (b *syncProbeBody) Close() error { return nil }

func TestUpstreamSync20261019Probe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	u := extractGeminiUsage([]byte(`{"usageMetadata":{"cachedContentTokenCount":30}}`))
	fmt.Printf("CACHE_ONLY input=%d cache_read=%d\n", u.InputTokens, u.CacheReadInputTokens)
	for _, route := range []string{"native", "messages", "chat"} {
		ctx, cancel := context.WithCancel(context.Background())
		body := []byte(`{"model":"gemini-2.5-flash","stream":true,"max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)).WithContext(ctx)
		sse := "data: " + `{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}` + "\n\n"
		upstream := &geminiCompatHTTPUpstreamStub{response: &http.Response{
			StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: &syncProbeBody{bytes.NewReader([]byte(sse)), cancel},
		}}
		svc := &GeminiMessagesCompatService{httpUpstream: upstream, cfg: &config.Config{}}
		account := &Account{ID: 611, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}, Concurrency: 1}
		var result *ForwardResult
		var err error
		switch route {
		case "native":
			result, err = svc.ForwardNative(context.Background(), c, account, "gemini-2.5-flash", "streamGenerateContent", true, []byte(`{"contents":[{"parts":[{"text":"hi"}]}]}`))
		case "messages":
			result, err = svc.Forward(context.Background(), c, account, body)
		case "chat":
			result, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body)
		}
		if result == nil {
			fmt.Printf("ABORT %s result=nil error=%v\n", route, err)
		} else {
			fmt.Printf("ABORT %s input=%d output=%d client_disconnect=%t error=%v\n", route, result.Usage.InputTokens, result.Usage.OutputTokens, result.ClientDisconnect, err)
		}
		cancel()
	}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"internal":true}`)), Request: req}, nil
	})}
	storage := &fakeImageStorage{}
	_, err := NewImageResultUploader(storage, "images/", 0, imageStorageTestUpstream{client}).Rewrite(context.Background(), "fixture", json.RawMessage(`{"data":[{"url":"https://cdn.example.test/result"}]}`))
	fmt.Printf("NON_IMAGE stored=%d rejected=%t\n", len(storage.saved), err != nil)
}
