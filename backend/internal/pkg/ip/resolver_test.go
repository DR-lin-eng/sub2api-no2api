//go:build unit

package ip

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newResolverForTest(t testing.TB, mode string, trusted []string) *Resolver {
	t.Helper()
	resolver, err := NewResolver(nil)
	require.NoError(t, err)
	require.NoError(t, resolver.Configure(mode, trusted))
	return resolver
}

func resolveForTest(t testing.TB, resolver *Resolver, remoteAddr string, headers map[string]string) ClientIPResult {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return resolver.ResolveRequest(req)
}

func TestResolverResolutionMatrix(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		trusted []string
		remote  string
		headers map[string]string
		wantIP  string
		source  ClientIPSource
	}{
		{
			name: "direct public peer",
			mode: ResolutionModeAutoCompat, remote: "198.51.100.10:443",
			wantIP: "198.51.100.10", source: ClientIPSourceDirect,
		},
		{
			name: "nginx loopback",
			mode: ResolutionModeAutoCompat, remote: "127.0.0.1:8080",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.42"},
			wantIP:  "203.0.113.42", source: ClientIPSourceForwardedFor,
		},
		{
			name: "docker bridge",
			mode: ResolutionModeAutoCompat, remote: "172.18.0.1:8080",
			headers: map[string]string{"X-Real-IP": "203.0.113.43"},
			wantIP:  "203.0.113.43", source: ClientIPSourceRealIP,
		},
		{
			name: "Cloudflare orange cloud",
			mode: ResolutionModeAutoCompat, remote: "173.245.48.10:443",
			headers: map[string]string{"CF-Connecting-IP": "203.0.113.44"},
			wantIP:  "203.0.113.44", source: ClientIPSourceCloudflare,
		},
		{
			name: "Cloudflare tunnel",
			mode: ResolutionModeAutoCompat, remote: "[::1]:443",
			headers: map[string]string{"CF-Connecting-IP": "203.0.113.45"},
			wantIP:  "203.0.113.45", source: ClientIPSourceCloudflare,
		},
		{
			name: "multi proxy chain",
			mode: ResolutionModeAutoCompat, remote: "127.0.0.1:8080",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.8, 10.0.0.4, 173.245.48.20"},
			wantIP:  "198.51.100.8", source: ClientIPSourceForwardedFor,
		},
		{
			name: "private final client is preserved",
			mode: ResolutionModeAutoCompat, remote: "172.18.0.1:8080",
			headers: map[string]string{"X-Forwarded-For": "192.168.10.20, 10.0.0.4"},
			wantIP:  "192.168.10.20", source: ClientIPSourceForwardedFor,
		},
		{
			name: "public spoof ignored",
			mode: ResolutionModeAutoCompat, remote: "198.51.100.11:443",
			headers: map[string]string{
				"X-Forwarded-For":  "203.0.113.50",
				"X-Real-IP":        "203.0.113.51",
				"CF-Connecting-IP": "203.0.113.52",
			},
			wantIP: "198.51.100.11", source: ClientIPSourceDirect,
		},
		{
			name: "strict mode rejects private peer",
			mode: ResolutionModeTrustedProxy, remote: "172.18.0.1:8080",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.53"},
			wantIP:  "172.18.0.1", source: ClientIPSourceDirect,
		},
		{
			name: "strict mode accepts explicit peer",
			mode: ResolutionModeTrustedProxy, trusted: []string{"172.18.0.0/16"}, remote: "172.18.0.1:8080",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.54"},
			wantIP:  "203.0.113.54", source: ClientIPSourceForwardedFor,
		},
		{
			name: "direct mode ignores loopback headers",
			mode: ResolutionModeDirect, remote: "127.0.0.1:8080",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.55"},
			wantIP:  "127.0.0.1", source: ClientIPSourceDirect,
		},
		{
			name: "IPv6 ULA proxy",
			mode: ResolutionModeAutoCompat, remote: "[fd00::10]:8080",
			headers: map[string]string{"X-Forwarded-For": "2001:db8::20"},
			wantIP:  "2001:db8::20", source: ClientIPSourceForwardedFor,
		},
		{
			name: "mapped IPv4 peer",
			mode: ResolutionModeAutoCompat, remote: "[::ffff:127.0.0.1]:8080",
			headers: map[string]string{"X-Forwarded-For": "203.0.113.56"},
			wantIP:  "203.0.113.56", source: ClientIPSourceForwardedFor,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := newResolverForTest(t, test.mode, test.trusted)
			result := resolveForTest(t, resolver, test.remote, test.headers)
			require.Equal(t, test.wantIP, result.IP)
			require.Equal(t, test.source, result.Source)
		})
	}
}

func TestResolverInvalidOrOversizedForwardedForFallsBack(t *testing.T) {
	resolver := newResolverForTest(t, ResolutionModeAutoCompat, nil)
	tests := []struct {
		name string
		xff  string
	}{
		{name: "malformed", xff: "203.0.113.20, unknown"},
		{name: "too many hops", xff: strings.Repeat("10.0.0.1,", maxForwardedForHops) + "203.0.113.20"},
		{name: "too many bytes", xff: strings.Repeat("1", maxForwardedForBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := resolveForTest(t, resolver, "127.0.0.1:8080", map[string]string{
				"X-Forwarded-For": test.xff,
				"X-Real-IP":       "203.0.113.99",
			})
			require.Equal(t, "203.0.113.99", result.IP)
			require.Equal(t, ClientIPSourceRealIP, result.Source)
		})
	}
}

func TestResolverAcceptsSixteenForwardedForHops(t *testing.T) {
	resolver := newResolverForTest(t, ResolutionModeAutoCompat, nil)
	hops := []string{"203.0.113.80"}
	for index := 1; index < maxForwardedForHops; index++ {
		hops = append(hops, fmt.Sprintf("10.0.0.%d", index))
	}
	result := resolveForTest(t, resolver, "127.0.0.1:8080", map[string]string{
		"X-Forwarded-For": strings.Join(hops, ", "),
	})
	require.Equal(t, "203.0.113.80", result.IP)
}

func TestResolverMergesStaticAndRuntimeTrustedProxies(t *testing.T) {
	resolver, err := NewResolver([]string{"192.0.2.0/24"})
	require.NoError(t, err)
	require.NoError(t, resolver.Configure(ResolutionModeTrustedProxy, []string{"198.51.100.0/24"}))

	staticResult := resolveForTest(t, resolver, "192.0.2.10:8080", map[string]string{
		"X-Forwarded-For": "203.0.113.81",
	})
	runtimeResult := resolveForTest(t, resolver, "198.51.100.10:8080", map[string]string{
		"X-Forwarded-For": "203.0.113.82",
	})
	require.Equal(t, "203.0.113.81", staticResult.IP)
	require.Equal(t, "203.0.113.82", runtimeResult.IP)
	status := resolver.Status()
	require.Equal(t, 1, status.StaticPrefixCount)
	require.Equal(t, 1, status.CustomPrefixCount)
}

func TestResolverMiddlewareParsesExactlyOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := newResolverForTest(t, ResolutionModeAutoCompat, nil)
	router := gin.New()
	router.Use(resolver.Middleware())
	router.GET("/", func(c *gin.Context) {
		for range 20 {
			require.Equal(t, "203.0.113.60", GetClientIP(c))
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.60")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.Equal(t, uint64(1), resolver.Metrics().Resolutions)
}

func TestResolverConfigurationValidationAndCanonicalization(t *testing.T) {
	resolver := newResolverForTest(t, ResolutionModeAutoCompat, nil)
	require.NoError(t, resolver.Configure(ResolutionModeTrustedProxy, []string{
		"192.168.1.1",
		"192.168.1.1/32",
		"2001:db8::1",
	}))
	mode, prefixes := resolver.CurrentConfiguration()
	require.Equal(t, ResolutionModeTrustedProxy, mode)
	require.Equal(t, []string{"192.168.1.1/32", "2001:db8::1/128"}, prefixes)
	require.Error(t, resolver.Configure("invalid", nil))
	require.Error(t, resolver.Configure(ResolutionModeAutoCompat, []string{"not-an-ip"}))
	require.Error(t, resolver.Configure(ResolutionModeAutoCompat, make([]string, MaxTrustedProxyPrefixes+1)))
}

func TestCloudflareRefreshUsesAtomicLastKnownGoodSnapshot(t *testing.T) {
	var mu sync.Mutex
	valid := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		serveValid := valid
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !serveValid {
			_, _ = w.Write([]byte(`{"success":true,"result":{"ipv4_cidrs":[],"ipv6_cidrs":[]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"result":{"ipv4_cidrs":["203.0.113.0/24"],"ipv6_cidrs":["2001:db8::/32"]}}`))
	}))
	defer server.Close()

	resolver := newResolverForTest(t, ResolutionModeAutoCompat, nil)
	resolver.rangesURL = server.URL
	resolver.httpClient = server.Client()
	require.NoError(t, resolver.refreshCloudflareRanges(context.Background()))
	status := resolver.Status()
	require.Equal(t, "refreshed", status.CloudflareRangesSource)
	require.Equal(t, 2, status.CloudflarePrefixCount)
	require.NotNil(t, status.CloudflareLastSuccessAt)

	result := resolveForTest(t, resolver, "203.0.113.10:443", map[string]string{"CF-Connecting-IP": "198.51.100.1"})
	require.Equal(t, "198.51.100.1", result.IP)

	mu.Lock()
	valid = false
	mu.Unlock()
	require.Error(t, resolver.refreshCloudflareRanges(context.Background()))
	result = resolveForTest(t, resolver, "203.0.113.10:443", map[string]string{"CF-Connecting-IP": "198.51.100.2"})
	require.Equal(t, "198.51.100.2", result.IP)
}

func TestResolverConcurrentSnapshotUpdates(t *testing.T) {
	resolver := newResolverForTest(t, ResolutionModeAutoCompat, nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:8080"
	req.Header.Set("X-Forwarded-For", "203.0.113.70")

	var wg sync.WaitGroup
	var wrongResult atomic.Bool
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				result := resolver.ResolveRequest(req)
				if result.IP != "203.0.113.70" {
					wrongResult.Store(true)
				}
			}
		}()
	}
	for index := range 200 {
		mode := ResolutionModeAutoCompat
		if index%2 == 1 {
			mode = ResolutionModeTrustedProxy
		}
		require.NoError(t, resolver.Configure(mode, []string{"127.0.0.0/8"}))
	}
	wg.Wait()
	require.False(t, wrongResult.Load())
}

func BenchmarkResolver(b *testing.B) {
	resolver := newResolverForTest(b, ResolutionModeAutoCompat, nil)
	maxHops := make([]string, 0, maxForwardedForHops)
	maxHops = append(maxHops, "203.0.113.1")
	for index := 1; index < maxForwardedForHops; index++ {
		maxHops = append(maxHops, fmt.Sprintf("10.0.0.%d", index))
	}
	tests := []struct {
		name    string
		remote  string
		headers map[string]string
	}{
		{name: "direct", remote: "198.51.100.1:443"},
		{name: "one_hop", remote: "127.0.0.1:8080", headers: map[string]string{"X-Forwarded-For": "203.0.113.1"}},
		{name: "four_hops", remote: "127.0.0.1:8080", headers: map[string]string{"X-Forwarded-For": "203.0.113.1, 10.0.0.1, 172.18.0.1, 173.245.48.10"}},
		{name: "sixteen_hops", remote: "127.0.0.1:8080", headers: map[string]string{"X-Forwarded-For": strings.Join(maxHops, ", ")}},
		{name: "public_spoof", remote: "198.51.100.2:443", headers: map[string]string{"X-Forwarded-For": "203.0.113.2"}},
	}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = test.remote
			for key, value := range test.headers {
				req.Header.Set(key, value)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = resolver.ResolveRequest(req)
			}
		})
	}
}

func BenchmarkGetClientIPCached(b *testing.B) {
	gin.SetMode(gin.TestMode)
	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	requestContext := context.WithValue(ginContext.Request.Context(), clientIPResultContextKey{}, &ClientIPResult{IP: "203.0.113.1"})
	ginContext.Request = ginContext.Request.WithContext(requestContext)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = GetClientIP(ginContext)
		}
	})
}

func BenchmarkGinClientIPV0161(b *testing.B) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name    string
		trusted []string
		remote  string
		xff     string
	}{
		{name: "direct", remote: "198.51.100.1:443"},
		{name: "one_hop", trusted: []string{"127.0.0.1"}, remote: "127.0.0.1:8080", xff: "203.0.113.1"},
	} {
		b.Run(test.name, func(b *testing.B) {
			ginContext, engine := gin.CreateTestContext(httptest.NewRecorder())
			require.NoError(b, engine.SetTrustedProxies(test.trusted))
			ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			ginContext.Request.RemoteAddr = test.remote
			if test.xff != "" {
				ginContext.Request.Header.Set("X-Forwarded-For", test.xff)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				_ = ginContext.ClientIP()
			}
		})
	}
}

func BenchmarkTenClientIPConsumers(b *testing.B) {
	gin.SetMode(gin.TestMode)
	b.Run("v0.1.161_gin", func(b *testing.B) {
		ginContext, engine := gin.CreateTestContext(httptest.NewRecorder())
		require.NoError(b, engine.SetTrustedProxies([]string{"127.0.0.1"}))
		ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		ginContext.Request.RemoteAddr = "127.0.0.1:8080"
		ginContext.Request.Header.Set("X-Forwarded-For", "203.0.113.1")
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			for range 10 {
				_ = ginContext.ClientIP()
			}
		}
	})
	b.Run("request_cached", func(b *testing.B) {
		ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginContext.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		requestContext := context.WithValue(ginContext.Request.Context(), clientIPResultContextKey{}, &ClientIPResult{IP: "203.0.113.1"})
		ginContext.Request = ginContext.Request.WithContext(requestContext)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			for range 10 {
				_ = GetClientIP(ginContext)
			}
		}
	})
}
