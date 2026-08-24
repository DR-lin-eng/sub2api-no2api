package service

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestCoderOpenAIWSClientDialerReadsIPv6SwitchAtDialTime(t *testing.T) {
	cfg := &config.Config{IPv6Egress: config.IPv6EgressConfig{Enabled: true, FreeBind: true}}
	dialer, ok := newConfiguredOpenAIWSClientDialer(cfg).(*coderOpenAIWSClientDialer)
	require.True(t, ok)
	route := platformegress.IPv6PoolRoute("2001:db8::10", 1, 1, false)
	require.True(t, dialer.currentEgressPolicy().IPv6Enabled)
	cfg.IPv6Egress.SetRuntimeEnabled(false)
	_, err := platformegress.ApplyPolicy(route, dialer.currentEgressPolicy())
	require.ErrorIs(t, err, platformegress.ErrIPv6Disabled)
}

func TestCoderOpenAIWSClientDialer_ProxyHTTPClientReuse(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	c1, err := impl.proxyHTTPClient("http://127.0.0.1:8080")
	require.NoError(t, err)
	c2, err := impl.proxyHTTPClient("http://127.0.0.1:8080")
	require.NoError(t, err)
	require.Same(t, c1, c2, "同一代理地址应复用同一个 HTTP 客户端")

	c3, err := impl.proxyHTTPClient("http://127.0.0.1:8081")
	require.NoError(t, err)
	require.NotSame(t, c1, c3, "不同代理地址应分离客户端")
}

func TestCoderOpenAIWSClientDialer_ProxyHTTPClientInvalidURL(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	_, err := impl.proxyHTTPClient("://bad")
	require.Error(t, err)
}

func TestCoderOpenAIWSClientDialer_TransportMetricsSnapshot(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	_, err := impl.proxyHTTPClient("http://127.0.0.1:18080")
	require.NoError(t, err)
	_, err = impl.proxyHTTPClient("http://127.0.0.1:18080")
	require.NoError(t, err)
	_, err = impl.proxyHTTPClient("http://127.0.0.1:18081")
	require.NoError(t, err)

	snapshot := impl.SnapshotTransportMetrics()
	require.Equal(t, int64(1), snapshot.ProxyClientCacheHits)
	require.Equal(t, int64(2), snapshot.ProxyClientCacheMisses)
	require.InDelta(t, 1.0/3.0, snapshot.TransportReuseRatio, 0.0001)
}

func TestCoderOpenAIWSClientDialer_ProxyClientCacheCapacity(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	total := openAIWSProxyClientCacheMaxEntries + 32
	for i := 0; i < total; i++ {
		_, err := impl.proxyHTTPClient(fmt.Sprintf("http://127.0.0.1:%d", 20000+i))
		require.NoError(t, err)
	}

	impl.proxyMu.Lock()
	cacheSize := len(impl.proxyClients)
	impl.proxyMu.Unlock()

	require.LessOrEqual(t, cacheSize, openAIWSProxyClientCacheMaxEntries, "代理客户端缓存应受容量上限约束")
}

func TestCoderOpenAIWSClientDialer_ProxyClientCacheIdleTTL(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	oldProxy := "http://127.0.0.1:28080"
	_, err := impl.proxyHTTPClient(oldProxy)
	require.NoError(t, err)

	impl.proxyMu.Lock()
	oldEntry := impl.proxyClients[oldProxy]
	require.NotNil(t, oldEntry)
	oldEntry.lastUsedUnixNano = time.Now().Add(-openAIWSProxyClientCacheIdleTTL - time.Minute).UnixNano()
	impl.proxyMu.Unlock()

	// 触发一次新的代理获取，驱动 TTL 清理。
	_, err = impl.proxyHTTPClient("http://127.0.0.1:28081")
	require.NoError(t, err)

	impl.proxyMu.Lock()
	_, exists := impl.proxyClients[oldProxy]
	impl.proxyMu.Unlock()

	require.False(t, exists, "超过空闲 TTL 的代理客户端应被回收")
}

func TestCoderOpenAIWSClientDialer_ProxyTransportTLSHandshakeTimeout(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	client, err := impl.proxyHTTPClient("http://127.0.0.1:38080")
	require.NoError(t, err)
	require.NotNil(t, client)

	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport)
	require.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout)
}

func TestCoderOpenAIWSClientDialer_ProfileClientsStayIsolated(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	profileA := &tlsfingerprint.Profile{Name: "profile-a", CipherSuites: []uint16{0x1301}}
	profileB := &tlsfingerprint.Profile{Name: "profile-b", CipherSuites: []uint16{0x1302}}
	clientA, err := impl.routeHTTPClientWithProfile(platformegress.DirectRoute(false), profileA)
	require.NoError(t, err)
	sameA, err := impl.routeHTTPClientWithProfile(platformegress.DirectRoute(false), profileA)
	require.NoError(t, err)
	clientB, err := impl.routeHTTPClientWithProfile(platformegress.DirectRoute(false), profileB)
	require.NoError(t, err)

	require.Same(t, clientA, sameA)
	require.NotSame(t, clientA, clientB)
	transport, ok := clientA.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.DialTLSContext)
}

func TestCoderOpenAIWSClientDialer_ProfileHTTPProxyUsesFingerprintDialer(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	client, err := impl.routeHTTPClientWithProfile(
		platformegress.ExternalProxyRoute("http://127.0.0.1:18080"),
		&tlsfingerprint.Profile{Name: "profile"},
	)
	require.NoError(t, err)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.DialTLSContext)
	require.Nil(t, transport.Proxy, "custom CONNECT dialer owns the HTTP proxy tunnel")
}

func TestCoderOpenAIWSClientDialer_ProfileHTTPSProxyUsesFingerprintDialer(t *testing.T) {
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)

	client, err := impl.routeHTTPClientWithProfile(
		platformegress.ExternalProxyRoute("https://127.0.0.1:18443"),
		&tlsfingerprint.Profile{Name: "profile"},
	)
	require.NoError(t, err)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	require.NotNil(t, transport.DialTLSContext)
	require.Nil(t, transport.Proxy)
}

func TestCoderOpenAIWSClientDialer_DirectClientCarriesCookieJar(t *testing.T) {
	codexsimulation.SetCLevelEnabled(true)
	t.Cleanup(func() { codexsimulation.SetCLevelEnabled(false) })
	dialer := newDefaultOpenAIWSClientDialer()
	impl, ok := dialer.(*coderOpenAIWSClientDialer)
	require.True(t, ok)
	client, err := impl.routeHTTPClientWithProfile(platformegress.DirectRoute(false), nil)
	require.NoError(t, err)
	require.NotNil(t, client.Jar, "direct WS handshakes must use the shared Cloudflare-only cookie jar")
}

func TestOpenAIWSAcquireCompatibilityIncludesTLSProfile(t *testing.T) {
	first := openAIWSAcquireCompatibility(openAIWSAcquireRequest{
		TLSProfile: &tlsfingerprint.Profile{Name: "profile-a", CipherSuites: []uint16{0x1301}},
	})
	second := openAIWSAcquireCompatibility(openAIWSAcquireRequest{
		TLSProfile: &tlsfingerprint.Profile{Name: "profile-b", CipherSuites: []uint16{0x1302}},
	})

	require.NotEmpty(t, first.tlsProfileKey)
	require.NotEqual(t, first.tlsProfileKey, second.tlsProfileKey)
}

func TestCoderOpenAIWSClientConn_DoesNotSupportIdlePingWithoutReader(t *testing.T) {
	require.False(t, (&coderOpenAIWSClientConn{}).SupportsIdlePingWithoutReader())
}
