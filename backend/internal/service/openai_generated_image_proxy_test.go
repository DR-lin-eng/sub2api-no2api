package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type generatedImageStoreStub struct {
	urls map[string]string
	ttls map[string]time.Duration
}

func (s *generatedImageStoreStub) GetSessionAccountID(context.Context, int64, string) (int64, error) {
	return 0, nil
}

func (s *generatedImageStoreStub) SetSessionAccountID(context.Context, int64, string, int64, time.Duration) error {
	return nil
}

func (s *generatedImageStoreStub) RefreshSessionTTL(context.Context, int64, string, time.Duration) error {
	return nil
}

func (s *generatedImageStoreStub) DeleteSessionAccountID(context.Context, int64, string) error {
	return nil
}

func (s *generatedImageStoreStub) SetGeneratedImageURL(_ context.Context, hash, rawURL string, ttl time.Duration) error {
	if s.urls == nil {
		s.urls = make(map[string]string)
	}
	if s.ttls == nil {
		s.ttls = make(map[string]time.Duration)
	}
	s.urls[hash] = rawURL
	s.ttls[hash] = ttl
	return nil
}

func (s *generatedImageStoreStub) GetGeneratedImageURL(_ context.Context, hash string) (string, bool, error) {
	value, ok := s.urls[hash]
	return value, ok, nil
}

type generatedImageHTTPUpstreamStub struct {
	request *http.Request
	resp    *http.Response
}

func (s *generatedImageHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.request = req
	return s.resp, nil
}

func (s *generatedImageHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestOpenAIGeneratedImage_RewritesURLAndStoresThirtyMinuteMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &generatedImageStoreStub{}
	svc := &OpenAIGatewayService{cache: store}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "https://internal/v1/images/generations", nil)
	c.Request.Host = "internal:8080"
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", "images.local.example")

	rawURL := "https://cdn.vendor.example/generated/result.png?signature=abc"
	body := []byte(`{"created":1,"data":[{"url":"` + rawURL + `","revised_prompt":"cat"}]}`)
	rewritten := svc.rewriteOpenAIImagesResponseURLs(c, body)

	sum := sha256.Sum256([]byte(rawURL))
	hash := hex.EncodeToString(sum[:])
	require.Equal(t, "https://images.local.example/generated/"+hash+".png", gjson.GetBytes(rewritten, "data.0.url").String())
	require.Equal(t, rawURL, store.urls[hash])
	require.Equal(t, 30*time.Minute, store.ttls[hash])
	require.NotContains(t, string(rewritten), "cdn.vendor.example")
}

func TestOpenAIGeneratedImage_NormalizesNamedSSEKeepalive(t *testing.T) {
	svc := &OpenAIGatewayService{}
	require.Equal(t, "\n", string(svc.rewriteOpenAIImagesSSELine(nil, []byte(": keep-alive\n"))))
	require.Equal(t, "\r\n", string(svc.rewriteOpenAIImagesSSELine(nil, []byte(":keepalive\r\n"))))
}

func TestOpenAIGeneratedImage_RewritesSSEDataURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &generatedImageStoreStub{}
	svc := &OpenAIGatewayService{cache: store}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "https://relay.example/v1/images/generations", nil)
	rawURL := "https://cdn.vendor.example/generated/result.webp"

	line := svc.rewriteOpenAIImagesSSELine(c, []byte(`data: {"type":"image_generation.completed","url":"`+rawURL+`"}`+"\n"))

	require.NotContains(t, string(line), rawURL)
	payload := strings.TrimSpace(strings.TrimPrefix(string(line), "data:"))
	rewrittenURL := gjson.Get(payload, "url").String()
	require.Contains(t, rewrittenURL, "https://relay.example/generated/")
	require.True(t, strings.HasSuffix(rewrittenURL, ".webp"))
}

func TestOpenAIGeneratedImage_OpensMappedURLWithoutAuthorization(t *testing.T) {
	rawURL := "https://cdn.vendor.example/generated/result.png?signature=abc"
	sum := sha256.Sum256([]byte(rawURL))
	hash := hex.EncodeToString(sum[:])
	store := &generatedImageStoreStub{urls: map[string]string{hash: rawURL}}
	upstream := &generatedImageHTTPUpstreamStub{resp: &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(strings.NewReader("png-bytes")),
		ContentLength: 9,
	}}
	svc := &OpenAIGatewayService{cache: store, httpUpstream: upstream}
	headers := http.Header{
		"Authorization": []string{"Bearer must-not-leak"},
		"Range":         []string{"bytes=0-3"},
	}

	resp, err := svc.OpenGeneratedImage(context.Background(), hash+".png", headers)

	require.NoError(t, err)
	require.Same(t, upstream.resp, resp)
	require.Equal(t, rawURL, upstream.request.URL.String())
	require.Empty(t, upstream.request.Header.Get("Authorization"))
	require.Equal(t, "bytes=0-3", upstream.request.Header.Get("Range"))
}

func TestOpenAIGeneratedImage_RejectsNonHashFilename(t *testing.T) {
	svc := &OpenAIGatewayService{cache: &generatedImageStoreStub{}}
	_, err := svc.OpenGeneratedImage(context.Background(), "https://example.com/image.png", nil)
	require.ErrorIs(t, err, ErrGeneratedImageNotFound)
}
