package repository

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type publicDestinationResolverFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f publicDestinationResolverFunc) LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestResolvePublicDestinationRejectsAllUnsafeDNSAnswers(t *testing.T) {
	for _, ips := range [][]netip.Addr{
		{netip.MustParseAddr("10.0.0.1")},
		{netip.MustParseAddr("169.254.169.254")},
		{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("::ffff:127.0.0.1")},
		{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("fc00::1")},
		nil,
	} {
		resolver := publicDestinationResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) { return ips, nil })
		_, err := resolvePublicDestination(t.Context(), resolver, "images.example.com")
		require.Error(t, err)
	}
	resolver := publicDestinationResolverFunc(func(ctx context.Context, network, host string) ([]netip.Addr, error) {
		require.Equal(t, "ip", network)
		require.Equal(t, "images.example.com", host)
		_, ok := ctx.Deadline()
		require.True(t, ok)
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("2606:4700:4700::1111")}, nil
	})
	ips, err := resolvePublicDestination(t.Context(), resolver, "images.example.com")
	require.NoError(t, err)
	require.Len(t, ips, 2)
	_, err = resolvePublicDestination(t.Context(), nil, "127.0.0.1")
	require.Error(t, err)
}

func TestPublicDestinationPinsIPAndPreservesHostAndTLS(t *testing.T) {
	for _, raw := range []string{"https://cdn.example.com/a.png?sig=123", "http://cdn.example.com:8080/a.png"} {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		require.NoError(t, err)
		pinned := pinPublicDestination(req, netip.MustParseAddr("8.8.8.8"))
		require.Equal(t, req.URL.Host, pinned.Host)
		require.Equal(t, req.URL.Path, pinned.URL.Path)
		require.Equal(t, req.URL.RawQuery, pinned.URL.RawQuery)
		require.Equal(t, "cdn.example.com", req.URL.Hostname())
		require.Equal(t, "8.8.8.8", pinned.URL.Hostname())
	}
	tpt := &publicDestinationTransport{route: platformegress.DirectRoute(false)}
	transport, err := tpt.transportForIP("cdn.example.com")
	require.NoError(t, err)
	require.Equal(t, "cdn.example.com", transport.TLSClientConfig.ServerName)
	require.False(t, transport.TLSClientConfig.InsecureSkipVerify)
	require.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
	require.True(t, transport.DisableKeepAlives)
	require.Nil(t, transport.Proxy)
}

func TestPublicDestinationPinsHTTPAndSOCKSProxyRequests(t *testing.T) {
	var proxyHits atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		require.Equal(t, http.MethodConnect, r.Method)
		require.Equal(t, "8.8.8.8:80", r.Host)
		conn, reader, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		_, err = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
		require.NoError(t, err)
		tunneled, err := http.ReadRequest(reader.Reader)
		require.NoError(t, err)
		require.Equal(t, "cdn.example.com", tunneled.Host)
		require.Equal(t, "/a.png", tunneled.URL.Path)
		_, err = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 14\r\n\r\npublic fixture")
		require.NoError(t, err)
	}))
	t.Cleanup(proxy.Close)
	parsedProxy, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	resolver := publicDestinationResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})
	tpt := &publicDestinationTransport{resolver: resolver, allowHTTP: true, route: platformegress.ExternalProxyRoute(proxy.URL), proxyURL: parsedProxy}
	req, err := http.NewRequest(http.MethodGet, "http://cdn.example.com/a.png", nil)
	require.NoError(t, err)
	resp, err := tpt.RoundTrip(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.EqualValues(t, 1, proxyHits.Load())
	require.Equal(t, req.URL, resp.Request.URL)

	// A second DNS answer is private: it must be rejected before contacting
	// even an administrator-configured proxy.
	tpt.resolver = publicDestinationResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	})
	_, err = tpt.RoundTrip(req)
	require.Error(t, err)
	require.EqualValues(t, 1, proxyHits.Load())

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	destination := make(chan string, 1)
	go observePublicSOCKSConnect(listener, destination)
	socksURL, err := url.Parse("socks5h://" + listener.Addr().String())
	require.NoError(t, err)
	tpt.proxyURL = socksURL
	tpt.route = platformegress.ExternalProxyRoute(socksURL.String())
	tpt.resolver = resolver
	_, err = tpt.RoundTrip(req)
	require.Error(t, err) // The fixture records the connect target, then closes.
	select {
	case target := <-destination:
		require.Equal(t, "8.8.8.8:80", target)
	case <-time.After(5 * time.Second):
		t.Fatal("SOCKS proxy did not receive the pinned destination")
	}
}

func observePublicSOCKSConnect(listener net.Listener, destination chan<- string) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	if _, err := conn.Write([]byte{5, 0}); err != nil {
		return
	}
	request := make([]byte, 10)
	if _, err := io.ReadFull(conn, request); err != nil || request[3] != 1 {
		return
	}
	destination <- net.JoinHostPort(net.IP(request[4:8]).String(), fmt.Sprint(binary.BigEndian.Uint16(request[8:10])))
	_, _ = conn.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
}

func TestPublicDestinationPolicyDoesNotUsePrivateUpstreamExceptions(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowPrivateHosts: true, AllowInsecureHTTP: true}}}
	upstream := NewHTTPUpstream(cfg, nil)
	req, err := http.NewRequestWithContext(service.WithHTTPUpstreamPublicDestination(t.Context()), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	_, err = upstream.Do(req, "", 0, 0)
	require.Error(t, err)
	_, err = upstream.DoWithTLS(req, "", 0, 0, tlsfingerprint.BuiltInCodexRustlsProfile())
	require.Error(t, err)
	require.Zero(t, hits.Load())

	trusted, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := upstream.Do(trusted, "", 0, 0)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.EqualValues(t, 1, hits.Load())
}

func TestPublicDestinationHTTPSVerifiesOriginalHostname(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "example.com", r.Host)
		require.Equal(t, "example.com", r.TLS.ServerName)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	tpt := &publicDestinationTransport{route: platformegress.DirectRoute(false)}
	for _, host := range []string{"example.com", "wrong.test"} {
		transport, err := tpt.transportForIP(host)
		require.NoError(t, err)
		t.Cleanup(transport.CloseIdleConnections)
		transport.TLSClientConfig.RootCAs = roots
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			require.Equal(t, "8.8.8.8:443", address)
			return newUpstreamDialer().DialContext(ctx, network, server.Listener.Addr().String())
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+host+"/image.png", nil)
		require.NoError(t, err)
		resp, err := transport.RoundTrip(pinPublicDestination(req, netip.MustParseAddr("8.8.8.8")))
		if host == "example.com" {
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
		} else {
			require.ErrorContains(t, err, "certificate")
		}
	}
}

func TestPublicDestinationFailsClosedForCancelledDNSAndIPv6Routes(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	resolver := publicDestinationResolverFunc(func(ctx context.Context, _, _ string) ([]netip.Addr, error) { return nil, ctx.Err() })
	_, err := resolvePublicDestination(ctx, resolver, "cdn.example.com")
	require.ErrorIs(t, err, context.Canceled)
	tpt := &publicDestinationTransport{
		resolver: publicDestinationResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
		}),
		route: platformegress.IPv6PoolRoute("2606:4700::1", 1, 1, false),
	}
	req, err := http.NewRequest(http.MethodGet, "https://cdn.example.com/image.png", nil)
	require.NoError(t, err)
	_, err = tpt.RoundTrip(req)
	require.True(t, errors.Is(err, platformegress.ErrIPv6Destination))
}
