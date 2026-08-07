package repository

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildUpstreamTransportSetsDialTimeout(t *testing.T) {
	transport, err := buildUpstreamTransport(defaultPoolSettings(nil), nil, upstreamProtocolModeDefault)
	require.NoError(t, err)
	require.NotNil(t, transport.DialContext)
	require.Equal(t, defaultUpstreamTLSHandshakeTimeout, transport.TLSHandshakeTimeout)
}

func TestNewUpstreamDialerHasBoundedTimeout(t *testing.T) {
	dialer := newUpstreamDialer()
	require.Equal(t, defaultUpstreamDialTimeout, dialer.Timeout)
	require.Equal(t, defaultUpstreamDialKeepAlive, dialer.KeepAlive)
	require.Greater(t, dialer.Timeout, time.Duration(0))
}

func TestBuildUpstreamTransportKeepsTimeoutAcrossProxies(t *testing.T) {
	for _, rawURL := range []string{"http://127.0.0.1:1080", "socks5h://127.0.0.1:1080"} {
		t.Run(rawURL, func(t *testing.T) {
			proxyURL, err := url.Parse(rawURL)
			require.NoError(t, err)
			transport, err := buildUpstreamTransport(defaultPoolSettings(nil), proxyURL, upstreamProtocolModeDefault)
			require.NoError(t, err)
			require.NotNil(t, transport.DialContext)
		})
	}
}

func TestUpstreamDialerRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	conn, err := newUpstreamDialer().DialContext(ctx, "tcp", "127.0.0.1:1")
	if conn != nil {
		_ = conn.Close()
	}
	require.Error(t, err)
}
