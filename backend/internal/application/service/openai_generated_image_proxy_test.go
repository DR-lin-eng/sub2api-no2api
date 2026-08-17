package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type generatedImageStoreStub struct {
	urls       map[string]string
	ttls       map[string]time.Duration
	routes     map[string]platformegress.Route
	accountIDs map[string]int64
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

func (s *generatedImageStoreStub) SetGeneratedImageRoute(_ context.Context, hash string, route platformegress.Route, accountID int64, _ time.Duration) error {
	if s.routes == nil {
		s.routes = make(map[string]platformegress.Route)
	}
	if s.accountIDs == nil {
		s.accountIDs = make(map[string]int64)
	}
	s.routes[hash] = route
	s.accountIDs[hash] = accountID
	return nil
}

func (s *generatedImageStoreStub) GetGeneratedImageRoute(_ context.Context, hash string) (platformegress.Route, int64, bool, error) {
	route, ok := s.routes[hash]
	return route, s.accountIDs[hash], ok, nil
}

type generatedImageHTTPUpstreamStub struct {
	request   *http.Request
	proxyURL  string
	accountID int64
	resp      *http.Response
}

type generatedImageRoutedUpstreamStub struct {
	generatedImageHTTPUpstreamStub
	route     platformegress.Route
	accountID int64
}

type generatedImageConcurrentUpstreamStub struct {
	mu      sync.Mutex
	current int
	max     int
	release <-chan struct{}
}

func (s *generatedImageConcurrentUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.mu.Lock()
	s.current++
	if s.current > s.max {
		s.max = s.current
	}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.current--
		s.mu.Unlock()
	}()
	select {
	case <-s.release:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(strings.NewReader("png-bytes")),
		ContentLength: 9,
	}, nil
}

func (s *generatedImageConcurrentUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *generatedImageConcurrentUpstreamStub) snapshot() (current, max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current, s.max
}

func (s *generatedImageHTTPUpstreamStub) Do(req *http.Request, proxyURL string, accountID int64, _ int) (*http.Response, error) {
	s.request = req
	s.proxyURL = proxyURL
	s.accountID = accountID
	return s.resp, nil
}

func (s *generatedImageHTTPUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s *generatedImageRoutedUpstreamStub) DoRoute(req *http.Request, route platformegress.Route, accountID int64, _ int) (*http.Response, error) {
	s.route = route
	s.accountID = accountID
	return s.Do(req, route.ProxyURL, accountID, 0)
}

func (s *generatedImageRoutedUpstreamStub) DoWithTLSRoute(req *http.Request, route platformegress.Route, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.DoRoute(req, route, accountID, accountConcurrency)
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

func TestOpenAIGeneratedImage_StrictURLModeNeverLeaksUpstreamWithoutRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "https://api.funai.works/v1/images/generations", nil)
	rawURL := "https://cdn.vendor.example/generated/result.png"

	response, err := svc.rewriteOpenAIImagesResponseURLsStrict(c, []byte(`{"data":[{"url":"`+rawURL+`"}]}`))

	require.ErrorIs(t, err, ErrGeneratedImageUnavailable)
	require.Empty(t, response)
}

func TestOpenAIGeneratedImage_UnexpectedURLBecomesBase64ByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "https://api.funai.works/v1/images/generations", nil)
	rawURL := "https://cdn.vendor.example/generated/result.png"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"url":"` + rawURL + `"}]}`)),
	}
	upstream := &generatedImageHTTPUpstreamStub{resp: &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(strings.NewReader("png-bytes")),
		ContentLength: 9,
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	_, _, _, err := svc.handleOpenAIImagesNonStreamingResponse(resp, c)

	require.NoError(t, err)
	require.Equal(t, "cG5nLWJ5dGVz", gjson.Get(rec.Body.String(), "data.0.b64_json").String())
	require.False(t, gjson.Get(rec.Body.String(), "data.0.url").Exists())
	require.NotContains(t, rec.Body.String(), rawURL)
}

func TestOpenAIGeneratedImage_UnexpectedURLNeverLeaksWhenDownloadFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "https://api.funai.works/v1/images/generations", nil)
	rawURL := "https://cdn.vendor.example/generated/result.png"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"url":"` + rawURL + `"}]}`)),
	}
	svc := &OpenAIGatewayService{}

	_, _, _, err := svc.handleOpenAIImagesNonStreamingResponse(resp, c)

	require.ErrorIs(t, err, ErrGeneratedImageUnavailable)
	require.NotContains(t, rec.Body.String(), rawURL)
}

func TestOpenAIGeneratedImage_Base64DownloadUsesAccountIPv6Route(t *testing.T) {
	route := platformegress.IPv6PoolRoute("2001:db8::44", 4, 7, false)
	ctx := platformegress.WithContextAccountRoute(context.Background(), route, platformegress.Policy{IPv6Enabled: true}, 44)
	upstream := &generatedImageRoutedUpstreamStub{generatedImageHTTPUpstreamStub: generatedImageHTTPUpstreamStub{resp: &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(strings.NewReader("png-bytes")),
		ContentLength: 9,
	}}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	encoded, err := svc.downloadGeneratedImageBase64(ctx, "https://cdn.vendor.example/generated/result.png")

	require.NoError(t, err)
	require.Equal(t, "cG5nLWJ5dGVz", encoded)
	require.Equal(t, route, upstream.route)
	require.Equal(t, int64(44), upstream.accountID)
}

func TestOpenAIGeneratedImage_Base64DownloadFailsClosedWithoutRoutedIPv6Support(t *testing.T) {
	route := platformegress.IPv6PoolRoute("2001:db8::45", 4, 8, false)
	ctx := platformegress.WithContextAccountRoute(context.Background(), route, platformegress.Policy{IPv6Enabled: true}, 45)
	upstream := &generatedImageHTTPUpstreamStub{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"image/png"}},
		Body:       io.NopCloser(strings.NewReader("must-not-be-used")),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := svc.downloadGeneratedImageBase64(ctx, "https://cdn.vendor.example/generated/result.png")

	require.ErrorIs(t, err, platformegress.ErrIPv6Unsupported)
	require.Nil(t, upstream.request)
}

func TestOpenAIGeneratedImage_Base64DownloadPreservesExternalProxyContext(t *testing.T) {
	route := platformegress.ExternalProxyRoute("http://proxy.example:8080")
	ctx := platformegress.WithContextAccountRoute(context.Background(), route, platformegress.Policy{}, 46)
	upstream := &generatedImageHTTPUpstreamStub{resp: &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(strings.NewReader("png-bytes")),
		ContentLength: 9,
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}

	_, err := svc.downloadGeneratedImageBase64(ctx, "https://cdn.vendor.example/generated/result.png")

	require.NoError(t, err)
	require.Equal(t, "http://proxy.example:8080", upstream.proxyURL)
	require.Equal(t, int64(46), upstream.accountID)
}

func TestOpenAIGeneratedImage_BackfillConcurrencyIsProcessWide(t *testing.T) {
	gin.SetMode(gin.TestMode)
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	upstream := &generatedImageConcurrentUpstreamStub{release: release}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	errs := make(chan error, 3)

	for requestIndex := 0; requestIndex < 3; requestIndex++ {
		requestIndex := requestIndex
		go func() {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "https://api.funai.works/v1/images/generations", nil)
			body := fmt.Sprintf(`{"data":[
				{"url":"https://cdn.vendor.example/generated/%d-0.png"},
				{"url":"https://cdn.vendor.example/generated/%d-1.png"},
				{"url":"https://cdn.vendor.example/generated/%d-2.png"},
				{"url":"https://cdn.vendor.example/generated/%d-3.png"}
			]}`, requestIndex, requestIndex, requestIndex, requestIndex)
			_, err := svc.rewriteOpenAIImagesResponseURLsAsBase64(c, []byte(body))
			errs <- err
		}()
	}

	require.Eventually(t, func() bool {
		current, _ := upstream.snapshot()
		return current == generatedImageDownloadConcurrency
	}, time.Second, time.Millisecond)
	close(release)
	released = true
	for i := 0; i < 3; i++ {
		require.NoError(t, <-errs)
	}
	current, max := upstream.snapshot()
	require.Zero(t, current)
	require.Equal(t, generatedImageDownloadConcurrency, max)
}

func TestOpenAIGeneratedImage_NormalizesNamedSSEKeepalive(t *testing.T) {
	svc := &OpenAIGatewayService{}
	require.Equal(t, "data: {}\n", string(svc.rewriteOpenAIImagesSSELine(nil, []byte(": keep-alive\n"))))
	require.Equal(t, "data: {}\r\n", string(svc.rewriteOpenAIImagesSSELine(nil, []byte(":keepalive\r\n"))))
}

func TestOpenAIGeneratedImage_NormalizesEmptyDataSSEKeepalive(t *testing.T) {
	svc := &OpenAIGatewayService{}
	require.Equal(t, "data: {}\n", string(svc.rewriteOpenAIImagesSSELine(nil, []byte("data: {}\n"))))
	require.Equal(t, "data: {}\r\n", string(svc.rewriteOpenAIImagesSSELine(nil, []byte("data: { }\r\n"))))
	require.Equal(t, "data: {\"status\":\"working\"}\n", string(svc.rewriteOpenAIImagesSSELine(nil, []byte("data: {\"status\":\"working\"}\n"))))
}

func TestOpenAIImagesPseudoStreamingResponse_WrapsFinalJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "https://relay.example/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"created":1710000000,"data":[{"b64_json":"aW1hZ2U="}]}`)),
	}
	svc := &OpenAIGatewayService{}

	_, imageCount, _, err := svc.handleOpenAIImagesPseudoStreamingResponse(resp, c)

	require.NoError(t, err)
	require.Equal(t, 1, imageCount)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), `data: {"created":1710000000,"data":[{"b64_json":"aW1hZ2U="}]}`)
	require.True(t, strings.HasSuffix(rec.Body.String(), "data: [DONE]\n\n"))
}

func TestOpenAIImagesNonStreamingResponse_StripsEmptyDataKeepalives(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "https://relay.example/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {}\n\ndata: { }\n\n" +
				`{"created":1710000000,"data":[{"b64_json":"aW1hZ2U="}]}`,
		)),
	}
	svc := &OpenAIGatewayService{}

	_, imageCount, _, err := svc.handleOpenAIImagesNonStreamingResponse(resp, c)

	require.NoError(t, err)
	require.Equal(t, 1, imageCount)
	require.NotContains(t, rec.Body.String(), "data:")
	require.JSONEq(t, `{"created":1710000000,"data":[{"b64_json":"aW1hZ2U="}]}`, rec.Body.String())
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

func TestOpenAIGeneratedImage_OpensMappedURLWithRequestAccountRoute(t *testing.T) {
	rawURL := "https://cdn.vendor.example/generated/result.png?signature=route"
	sum := sha256.Sum256([]byte(rawURL))
	hash := hex.EncodeToString(sum[:])
	store := &generatedImageStoreStub{urls: map[string]string{hash: rawURL}}
	upstream := &generatedImageRoutedUpstreamStub{generatedImageHTTPUpstreamStub: generatedImageHTTPUpstreamStub{resp: &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(strings.NewReader("png-bytes")),
		ContentLength: 9,
	}}}
	svc := &OpenAIGatewayService{cache: store, httpUpstream: upstream}
	route := platformegress.IPv6PoolRoute("2001:db8::47", 4, 9, false)
	ctx := platformegress.WithContextAccountRoute(context.Background(), route, platformegress.Policy{IPv6Enabled: true}, 47)

	resp, err := svc.OpenGeneratedImage(ctx, hash+".png", nil)

	require.NoError(t, err)
	require.Same(t, upstream.resp, resp)
	require.Equal(t, route, upstream.route)
	require.Equal(t, int64(47), upstream.accountID)
}

func TestOpenAIGeneratedImage_PersistedMappingRestoresAccountRoute(t *testing.T) {
	rawURL := "https://cdn.vendor.example/generated/result.png?signature=persisted-route"
	store := &generatedImageStoreStub{}
	upstream := &generatedImageRoutedUpstreamStub{generatedImageHTTPUpstreamStub: generatedImageHTTPUpstreamStub{resp: &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(strings.NewReader("png-bytes")),
		ContentLength: 9,
	}}}
	svc := &OpenAIGatewayService{cache: store, httpUpstream: upstream, cfg: &config.Config{IPv6Egress: config.IPv6EgressConfig{Enabled: true, FreeBind: true}}}
	route := platformegress.IPv6PoolRoute("2001:db8::48", 4, 10, false)
	mapCtx := platformegress.WithContextAccountRoute(context.Background(), route, platformegress.Policy{IPv6Enabled: true, FreeBind: true}, 48)

	localURL := svc.mapGeneratedImageURL(mapCtx, store, "https://images.local.example", rawURL)
	filename := path.Base(localURL)
	resp, err := svc.OpenGeneratedImage(context.Background(), filename, nil)

	require.NoError(t, err)
	require.Same(t, upstream.resp, resp)
	require.Equal(t, route, upstream.route)
	require.Equal(t, int64(48), upstream.accountID)
	directHash := sha256.Sum256([]byte(rawURL))
	require.NotEqual(t, hex.EncodeToString(directHash[:]), strings.TrimSuffix(filename, path.Ext(filename)))
}

func TestOpenAIGeneratedImage_RejectsNonHashFilename(t *testing.T) {
	svc := &OpenAIGatewayService{cache: &generatedImageStoreStub{}}
	_, err := svc.OpenGeneratedImage(context.Background(), "https://example.com/image.png", nil)
	require.ErrorIs(t, err, ErrGeneratedImageNotFound)
}
