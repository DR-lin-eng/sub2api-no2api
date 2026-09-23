//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/infrastructure/repository"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Exercise both production transports against one local, certificate-verified
// TLS server. The CONNECT proxy routes the fixed OAuth hostname locally, so no
// request or credential leaves the test process.
func TestCodexUATLSHTTPAndWebSocketWire(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("run in Docker: SSL_CERT_FILE configures the isolated Linux trust store")
	}
	const ua = "Codex Desktop/0.153.4 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.903.71938)"
	const completed = `{"type":"response.completed","response":{"id":"resp_wire","status":"completed","model":"gpt-5.5","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`
	service.SetCodexCanonicalUserAgentResolver(func() string { return ua })
	t.Cleanup(func() { service.SetCodexCanonicalUserAgentResolver(nil) })
	gin.SetMode(gin.TestMode)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{"chatgpt.com"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	rootFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(rootFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0600))
	t.Setenv("SSL_CERT_FILE", rootFile)

	type helloSnapshot struct {
		Ciphers    []uint16
		Curves     []tls.CurveID
		Signatures []tls.SignatureScheme
		Versions   []uint16
		ALPN       []string
		ServerName string
	}
	hellos := make(chan helloSnapshot, 8)
	headers := make(chan http.Header, 8)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Proto != "HTTP/1.1" || r.TLS == nil || r.TLS.NegotiatedProtocol != "http/1.1" {
			t.Errorf("unexpected wire protocol: %s / %+v", r.Proto, r.TLS)
		}
		headers <- r.Header.Clone()
		if r.Header.Get("Upgrade") != "websocket" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ok"}`)
			return
		}
		conn, acceptErr := coderws.Accept(w, r, nil)
		if acceptErr != nil {
			t.Error(acceptErr)
			return
		}
		defer conn.CloseNow()
		_, _, readErr := conn.Read(r.Context())
		if readErr != nil {
			t.Error(readErr)
			return
		}
		if writeErr := conn.Write(r.Context(), coderws.MessageText, []byte(completed)); writeErr != nil {
			t.Error(writeErr)
			return
		}
		_, _, _ = conn.Read(r.Context())
	}))
	upstream.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{"http/1.1"},
		GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			hellos <- helloSnapshot{
				Ciphers: append([]uint16(nil), h.CipherSuites...), Curves: append([]tls.CurveID(nil), h.SupportedCurves...),
				Signatures: append([]tls.SignatureScheme(nil), h.SignatureSchemes...), Versions: append([]uint16(nil), h.SupportedVersions...),
				ALPN: append([]string(nil), h.SupportedProtos...), ServerName: h.ServerName,
			}
			return nil, nil
		},
	}
	upstream.StartTLS()
	defer upstream.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != "chatgpt.com:443" {
			http.Error(w, "unexpected CONNECT target", 400)
			return
		}
		target, dialErr := net.DialTimeout("tcp", upstream.Listener.Addr().String(), time.Second)
		if dialErr != nil {
			t.Error(dialErr)
			return
		}
		defer target.Close()
		client, buffered, hijackErr := w.(http.Hijacker).Hijack()
		if hijackErr != nil {
			t.Error(hijackErr)
			return
		}
		defer client.Close()
		_, _ = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buffered.Flush()
		go func() { _, _ = io.Copy(target, buffered); _ = target.Close() }()
		_, _ = io.Copy(client, target)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(proxyURL.Port())
	require.NoError(t, err)
	proxyID := int64(1)
	account := &service.Account{
		ID: 601, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Concurrency: 1,
		Credentials: map[string]any{"chatgpt_account_id": "wire-fixture", "access_token": "fixture-token", "user_agent": ua},
		Extra:       map[string]any{"enable_tls_fingerprint": true, "codex_fingerprint_mode": "full", "responses_websockets_v2_enabled": true},
		ProxyID:     &proxyID, Proxy: &service.Proxy{Protocol: "http", Host: proxyURL.Hostname(), Port: port},
	}
	profiles := &service.TLSFingerprintProfileService{}
	profile := profiles.ResolveTLSProfile(account)
	require.NotNil(t, profile)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{FullSimulationEnabled: true, IdentitySecret: "local-wire-test-secret-32-bytes", StateTTLSeconds: 3600}
	httpUpstream := repository.NewHTTPUpstream(cfg, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", strings.NewReader(`{"input":"hello"}`))
	require.NoError(t, err)
	service.ApplyCodexCanonicalAuthIdentity(req.Header)
	req.Header.Set("version", service.CodexCanonicalClientVersion())
	resp, err := httpUpstream.DoWithTLS(req, proxy.URL, account.ID, 1, profile)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	httpHeaders, httpHello := <-headers, <-hellos

	gw := service.NewOpenAIGatewayService(nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, httpUpstream, nil, nil, nil, nil, nil, nil, nil, nil)
	gw.SetTLSFingerprintProfileService(profiles)
	defer gw.CloseOpenAIWSPool()
	gatewayDone := make(chan error, 1)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, acceptErr := coderws.Accept(w, r, nil)
		if acceptErr != nil {
			gatewayDone <- acceptErr
			return
		}
		defer conn.CloseNow()
		_, first, readErr := conn.Read(r.Context())
		if readErr != nil {
			gatewayDone <- readErr
			return
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		gatewayDone <- gw.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "fixture-token", first, nil)
	}))
	defer gateway.Close()
	client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(gateway.URL, "http"), nil)
	require.NoError(t, err)
	defer client.CloseNow()
	err = client.Write(ctx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.5","input":"hello","stream":true}`))
	require.NoError(t, err)
	_, event, err := client.Read(ctx)
	require.NoError(t, err)
	require.Contains(t, string(event), "response.completed")
	wsHeaders, wsHello := <-headers, <-hellos
	for _, name := range []string{"User-Agent", "originator", "version"} {
		require.Equal(t, httpHeaders.Get(name), wsHeaders.Get(name), name)
	}
	require.Equal(t, ua, wsHeaders.Get("User-Agent"))
	require.Equal(t, httpHello, wsHello, "HTTP and WS must offer the same TLS parameters")
	require.Equal(t, []string{"http/1.1"}, wsHello.ALPN)
	t.Logf("HTTP=200 WS=101 response.completed; user-agent=%s; originator=%s; version=%s; ClientHello parameters=equal ALPN=%v", wsHeaders.Get("User-Agent"), wsHeaders.Get("originator"), wsHeaders.Get("version"), wsHello.ALPN)
	_ = client.CloseNow()
	select {
	case gatewayErr := <-gatewayDone:
		require.NoError(t, gatewayErr)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
