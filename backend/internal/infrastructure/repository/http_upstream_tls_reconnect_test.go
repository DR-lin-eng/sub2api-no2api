package repository

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestCodexExperimentalHTTP2ClientReconnectsWithFreshKeyShares(t *testing.T) {
	codexsimulation.SetCLevelEnabled(true)
	codexsimulation.SetExperimentalTransportEnabled(true)
	t.Cleanup(func() {
		codexsimulation.SetCLevelEnabled(false)
		codexsimulation.SetExperimentalTransportEnabled(false)
	})

	for _, http2 := range []bool{false, true} {
		name := "http1"
		if http2 {
			name = "http2"
		}
		t.Run(name, func(t *testing.T) {
			var handshakes atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "ok")
			}))
			server.EnableHTTP2 = http2
			server.TLS = &tls.Config{
				MinVersion: tls.VersionTLS13,
				GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
					handshakes.Add(1)
					return nil, nil
				},
			}
			server.StartTLS()
			defer server.Close()

			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			profile := tlsfingerprint.BuiltInCodexRustlsProfile().WithCodexExperimentalTransport()
			client := buildCodexExperimentalHTTP2Client(profile, platformegress.DirectRoute(false), nil)
			require.NotNil(t, client)
			client.SetTLSClientConfig(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
			defer client.GetClient().CloseIdleConnections()

			for attempt := 1; attempt <= 3; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				response, err := client.GetClient().Do(request)
				if err != nil {
					cancel()
					t.Fatalf("TLS connection %d failed after %d server handshakes: %v", attempt, handshakes.Load(), err)
				}
				body, readErr := io.ReadAll(response.Body)
				closeErr := response.Body.Close()
				cancel()
				require.NoError(t, readErr)
				require.NoError(t, closeErr)
				require.Equal(t, "ok", string(body))
				require.NotNil(t, response.TLS)
				require.NotEmpty(t, response.TLS.VerifiedChains)
				if http2 {
					require.Equal(t, 2, response.ProtoMajor)
				} else {
					require.Equal(t, 1, response.ProtoMajor)
				}
				// A healthy first request must not hide a broken later handshake.
				server.CloseClientConnections()
				client.GetClient().CloseIdleConnections()
			}
			require.Equal(t, int32(3), handshakes.Load())
		})
	}
}

func TestCodexExperimentalTLSHandshakeConcurrentConnections1600(t *testing.T) {
	codexsimulation.SetCLevelEnabled(true)
	codexsimulation.SetExperimentalTransportEnabled(true)
	t.Cleanup(func() {
		codexsimulation.SetCLevelEnabled(false)
		codexsimulation.SetExperimentalTransportEnabled(false)
	})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	profile := tlsfingerprint.BuiltInCodexRustlsProfile().WithCodexExperimentalTransport()
	profileKey := tlsfingerprint.FingerprintKey(profile)
	client := buildCodexExperimentalHTTP2Client(profile, platformegress.DirectRoute(false), nil)
	require.NotNil(t, client)
	client.SetTLSClientConfig(&tls.Config{RootCAs: roots})

	const connections = 1600
	type result struct {
		state *tls.ConnectionState
		err   error
	}
	results := make(chan result, connections)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < connections; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			plain, err := (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			if err != nil {
				results <- result{err: err}
				return
			}
			defer plain.Close()
			conn, state, err := client.GetTransport().TLSHandshakeContext(ctx, server.Listener.Addr().String(), plain)
			if conn != nil {
				_ = conn.Close()
			}
			results <- result{state: state, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	for r := range results {
		require.NoError(t, r.err)
		require.NotNil(t, r.state)
		require.True(t, r.state.HandshakeComplete)
		require.Equal(t, "h2", r.state.NegotiatedProtocol)
		require.NotEmpty(t, r.state.VerifiedChains)
	}
	require.Equal(t, profileKey, tlsfingerprint.FingerprintKey(profile))
}

func TestCodexExperimentalTLSHandshakeRejectsUntrustedCertificate(t *testing.T) {
	codexsimulation.SetCLevelEnabled(true)
	codexsimulation.SetExperimentalTransportEnabled(true)
	t.Cleanup(func() {
		codexsimulation.SetCLevelEnabled(false)
		codexsimulation.SetExperimentalTransportEnabled(false)
	})
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	profile := tlsfingerprint.BuiltInCodexRustlsProfile().WithCodexExperimentalTransport()
	client := buildCodexExperimentalHTTP2Client(profile, platformegress.DirectRoute(false), nil)
	require.NotNil(t, client)
	defer client.GetClient().CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	response, err := client.GetClient().Do(request)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	var unknownAuthority x509.UnknownAuthorityError
	require.ErrorAs(t, err, &unknownAuthority)
}
