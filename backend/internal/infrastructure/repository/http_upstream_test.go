package repository

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type httpUpstreamSettingRepo struct {
	mu     sync.RWMutex
	values map[string]string
}

func newHTTPUpstreamSettingRepo() *httpUpstreamSettingRepo {
	return &httpUpstreamSettingRepo{values: make(map[string]string)}
}

func (r *httpUpstreamSettingRepo) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}

func (r *httpUpstreamSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *httpUpstreamSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *httpUpstreamSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (r *httpUpstreamSettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, value := range values {
		r.values[key] = value
	}
	return nil
}

func (r *httpUpstreamSettingRepo) GetAll(context.Context) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	values := make(map[string]string, len(r.values))
	for key, value := range r.values {
		values[key] = value
	}
	return values, nil
}

func (r *httpUpstreamSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

func newHTTPUpstreamWithStreamTimeout(t *testing.T, cfg *config.Config, seconds int) service.HTTPUpstream {
	t.Helper()
	settingService := service.NewSettingService(newHTTPUpstreamSettingRepo(), cfg)
	settings := service.DefaultStreamTimeoutSettings()
	settings.ResponseHeaderTimeoutSeconds = seconds
	require.NoError(t, settingService.SetStreamTimeoutSettings(t.Context(), settings))
	return NewHTTPUpstream(cfg, settingService)
}

func TestHTTPUpstreamDoCanDisableRedirectsPerRequest(t *testing.T) {
	var redirectedCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectedCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	upstream := NewHTTPUpstream(nil, nil)
	req, err := http.NewRequestWithContext(
		service.WithHTTPUpstreamRedirectsDisabled(t.Context()),
		http.MethodGet,
		redirector.URL,
		nil,
	)
	require.NoError(t, err)

	resp, err := upstream.Do(req, "", 1, 1)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Zero(t, redirectedCalls.Load())
}

func TestHTTPUpstreamDoWithTLSPlainHTTPUsesConfiguredHTTPProxy(t *testing.T) {
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusTeapot)
	}))
	t.Cleanup(upstream.Close)
	var proxyCalls atomic.Int64
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(proxy.Close)

	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	client := NewHTTPUpstream(nil, nil)
	resp, err := client.DoWithTLS(req, proxy.URL, 41, 1, &tlsfingerprint.Profile{Name: "unused-for-http"})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int64(1), proxyCalls.Load())
	require.Zero(t, upstreamCalls.Load(), "plain HTTP must not bypass the configured proxy")
}

func TestHTTPUpstreamDoWithTLSPlainHTTPUsesConfiguredSOCKSProxy(t *testing.T) {
	var upstreamCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	proxyURL, proxyCalls := startTestSOCKS5Proxy(t)

	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	client := NewHTTPUpstream(nil, nil)
	resp, err := client.DoWithTLS(req, proxyURL, 42, 1, &tlsfingerprint.Profile{Name: "unused-for-http"})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, int64(1), proxyCalls.Load())
	require.Equal(t, int64(1), upstreamCalls.Load())
}

func TestTLSFingerprintHTTPSProxyPreservesProxyAndFingerprintDialer(t *testing.T) {
	proxyURL, err := url.Parse("https://user:pass@proxy.example:8443")
	require.NoError(t, err)
	transport, err := buildUpstreamTransportWithTLSFingerprint(poolSettings{}, proxyURL, &tlsfingerprint.Profile{Name: "test"})
	require.NoError(t, err)
	require.Nil(t, transport.Proxy)
	require.NotNil(t, transport.DialTLSContext)
}

func startTestSOCKS5Proxy(t *testing.T) (string, *atomic.Int64) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	calls := &atomic.Int64{}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			calls.Add(1)
			go serveTestSOCKS5Conn(conn)
		}
	}()
	return "socks5h://" + listener.Addr().String(), calls
}

func serveTestSOCKS5Conn(client net.Conn) {
	defer func() { _ = client.Close() }()
	header := make([]byte, 2)
	if _, err := io.ReadFull(client, header); err != nil || header[0] != 5 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(client, methods); err != nil {
		return
	}
	if _, err := client.Write([]byte{5, 0}); err != nil {
		return
	}
	request := make([]byte, 4)
	if _, err := io.ReadFull(client, request); err != nil || request[0] != 5 || request[1] != 1 {
		return
	}
	var host string
	switch request[3] {
	case 1:
		address := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(client, address); err != nil {
			return
		}
		host = net.IP(address).String()
	case 3:
		length := make([]byte, 1)
		if _, err := io.ReadFull(client, length); err != nil {
			return
		}
		address := make([]byte, int(length[0]))
		if _, err := io.ReadFull(client, address); err != nil {
			return
		}
		host = string(address)
	case 4:
		address := make([]byte, net.IPv6len)
		if _, err := io.ReadFull(client, address); err != nil {
			return
		}
		host = net.IP(address).String()
	default:
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(client, portBytes); err != nil {
		return
	}
	target, err := net.Dial("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", binary.BigEndian.Uint16(portBytes))))
	if err != nil {
		_, _ = client.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = target.Close() }()
	if _, err := client.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	go func() { _, _ = io.Copy(target, client); _ = target.Close() }()
	_, _ = io.Copy(client, target)
}

func TestHTTPUpstreamDoAppliesGrokCLIIdentityBeforeOAuthRoundTrip(t *testing.T) {
	t.Setenv("XAI_GROK_CLI_VERSION", "")

	for _, endpoint := range []string{"responses", "chat/completions"} {
		t.Run(endpoint, func(t *testing.T) {
			upstream := NewHTTPUpstream(nil, nil)
			svc, ok := upstream.(*httpUpstreamService)
			require.True(t, ok)

			const accountID int64 = 4084
			isolation := svc.getIsolationMode()
			profile := service.HTTPUpstreamProfileDefault
			proxyKey := directProxyKey
			protocolMode := svc.resolveProtocolMode(profile, proxyKey, nil)
			settings := svc.resolvePoolSettings(isolation, 1)
			settings = svc.applyProfilePoolSettings(settings, profile)
			cacheKey := buildCacheKey(isolation, proxyKey, accountID, protocolMode)

			var capturedHeaders http.Header
			svc.clients[cacheKey] = &upstreamClientEntry{
				client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					capturedHeaders = req.Header.Clone()
					statusCode := http.StatusOK
					if req.Header.Get("X-XAI-Token-Auth") != "xai-grok-cli" {
						statusCode = http.StatusForbidden
					}
					return &http.Response{
						StatusCode: statusCode,
						Header:     make(http.Header),
						Body:       http.NoBody,
						Request:    req,
					}, nil
				})},
				proxyKey:     proxyKey,
				poolKey:      buildPoolKey(settings, protocolMode),
				protocolMode: protocolMode,
			}

			req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/"+endpoint, nil)
			require.NoError(t, err)
			req.Header.Set("User-Agent", "sub2api-grok/1.0")

			resp, err := svc.Do(req, "", accountID, 1)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.NoError(t, resp.Body.Close())

			require.Equal(t, "0.2.114", capturedHeaders.Get("x-grok-client-version"))
			require.Equal(t, "xai-grok-cli", capturedHeaders.Get("X-XAI-Token-Auth"))
			require.Equal(t, "xai-grok-workspace/0.2.114", capturedHeaders.Get("User-Agent"))
		})
	}
}

func TestHTTPUpstreamDoFallsBackToOfficialGrokAPIOnCLIAccessDenied(t *testing.T) {
	upstream := NewHTTPUpstream(nil, nil)
	svc, ok := upstream.(*httpUpstreamService)
	require.True(t, ok)

	const accountID int64 = 4421
	isolation := svc.getIsolationMode()
	profile := service.HTTPUpstreamProfileDefault
	proxyKey := directProxyKey
	protocolMode := svc.resolveProtocolMode(profile, proxyKey, nil)
	settings := svc.resolvePoolSettings(isolation, 1)
	settings = svc.applyProfilePoolSettings(settings, profile)
	cacheKey := buildCacheKey(isolation, proxyKey, accountID, protocolMode)

	payload := []byte(`{"model":"grok-4.5","input":"hello"}`)
	var calls int
	var fallbackBody []byte
	var fallbackHeaders http.Header
	svc.clients[cacheKey] = &upstreamClientEntry{
		client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			if calls == 1 {
				require.Equal(t, grokCLIProxyHost, req.URL.Hostname())
				require.Equal(t, "xai-grok-cli", req.Header.Get("X-XAI-Token-Auth"))
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":"Access denied"}`)),
					Request:    req,
				}, nil
			}

			fallbackBody = body
			fallbackHeaders = req.Header.Clone()
			require.Equal(t, grokOfficialAPIHost, req.URL.Hostname())
			require.Equal(t, "/v1/responses", req.URL.Path)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"id":"response-ok"}`)),
				Request:    req,
			}, nil
		})},
		proxyKey:     proxyKey,
		poolKey:      buildPoolKey(settings, protocolMode),
		protocolMode: protocolMode,
	}

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer oauth-token")

	resp, err := svc.Do(req, "", accountID, 1)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	responseBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.JSONEq(t, `{"id":"response-ok"}`, string(responseBody))
	require.Equal(t, 2, calls)
	require.Equal(t, payload, fallbackBody)
	require.Equal(t, "Bearer oauth-token", fallbackHeaders.Get("Authorization"))
	require.Empty(t, fallbackHeaders.Get("X-XAI-Token-Auth"))
	require.Empty(t, fallbackHeaders.Get("x-grok-client-version"))
	require.Empty(t, fallbackHeaders.Get("User-Agent"))
}

func TestGrokAccessDeniedFallbackRecognizesChatEndpointPermissionDenied(t *testing.T) {
	var hosts []string
	transport := &grokAccessDeniedFallbackTransport{
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			hosts = append(hosts, req.URL.Hostname())
			if req.URL.Hostname() == grokCLIProxyHost {
				return &http.Response{
					StatusCode: http.StatusForbidden,
					Header:     make(http.Header),
					Body: io.NopCloser(strings.NewReader(
						`{"code":"permission_denied","error":"Access to the chat endpoint is denied. Please ensure you're using the correct credentials. If you believe this is a mistake, please contact support."}`,
					)),
					Request: req,
				}, nil
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"id":"response-ok"}`)),
				Request:    req,
			}, nil
		}),
	}

	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", strings.NewReader(`{"model":"grok-4.5"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer oauth-token")
	req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, []string{grokCLIProxyHost, grokOfficialAPIHost}, hosts)
}

func TestIsGrokCLICompatibilityAccessDenied(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "legacy compatibility wording", body: `{"error":"Access denied"}`, want: true},
		{
			name: "observed chat endpoint permission denial",
			body: `{"code":"permission_denied","error":"Access to the chat endpoint is denied. Please ensure you're using the correct credentials. If you believe this is a mistake, please contact support."}`,
			want: true,
		},
		{
			name: "entitlement denial using the same broad terms",
			body: `{"code":"permission_denied","error":"Access to the chat endpoint is denied because a subscription is required"}`,
			want: false,
		},
		{
			name: "different permission denied endpoint",
			body: `{"code":"permission_denied","error":"Access to the billing endpoint is denied."}`,
			want: false,
		},
		{
			name: "wrong structured error code",
			body: `{"code":"subscription_required","error":"Access to the chat endpoint is denied. Please ensure you're using the correct credentials. If you believe this is a mistake, please contact support."}`,
			want: false,
		},
		{name: "malformed response", body: `permission_denied: chat endpoint denied`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isGrokCLICompatibilityAccessDenied([]byte(tt.body)))
		})
	}
}

func TestIsGrokCLIAccessDeniedFallbackCandidateRequiresAuthenticatedReplayableCLI403(t *testing.T) {
	newRequest := func() *http.Request {
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", strings.NewReader(`{"model":"grok-4.5"}`))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer oauth-token")
		req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")
		return req
	}
	newResponse := func() *http.Response { return &http.Response{StatusCode: http.StatusForbidden} }

	t.Run("valid candidate", func(t *testing.T) {
		require.True(t, isGrokCLIAccessDeniedFallbackCandidate(newRequest(), newResponse()))
	})
	t.Run("non CLI host", func(t *testing.T) {
		req := newRequest()
		req.URL.Host = "api.x.ai"
		require.False(t, isGrokCLIAccessDeniedFallbackCandidate(req, newResponse()))
	})
	t.Run("missing CLI identity", func(t *testing.T) {
		req := newRequest()
		req.Header.Del("X-XAI-Token-Auth")
		require.False(t, isGrokCLIAccessDeniedFallbackCandidate(req, newResponse()))
	})
	t.Run("missing bearer authentication", func(t *testing.T) {
		req := newRequest()
		req.Header.Del("Authorization")
		require.False(t, isGrokCLIAccessDeniedFallbackCandidate(req, newResponse()))
	})
	t.Run("non forbidden response", func(t *testing.T) {
		resp := newResponse()
		resp.StatusCode = http.StatusUnauthorized
		require.False(t, isGrokCLIAccessDeniedFallbackCandidate(newRequest(), resp))
	})
	t.Run("non replayable request", func(t *testing.T) {
		req := newRequest()
		req.GetBody = nil
		require.False(t, isGrokCLIAccessDeniedFallbackCandidate(req, newResponse()))
	})
}

func TestHTTPUpstreamDoDoesNotFallbackForGrokEntitlementDenial(t *testing.T) {
	transport := &grokAccessDeniedFallbackTransport{
		base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"subscription required"}`)),
				Request:    req,
			}, nil
		}),
	}
	req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", strings.NewReader(`{"model":"grok-4.5"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer oauth-token")
	req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.JSONEq(t, `{"error":"subscription required"}`, string(body))
}

func TestUpstreamClientEntryCachesGrokFallbackClient(t *testing.T) {
	baseClient := &http.Client{Transport: http.DefaultTransport}
	entry := &upstreamClientEntry{client: baseClient}

	nonGrokReq, err := http.NewRequest(http.MethodPost, "https://api.example.com/v1/responses", strings.NewReader(`{"model":"test"}`))
	require.NoError(t, err)
	require.Same(t, baseClient, entry.clientForRequest(nonGrokReq))

	grokReq, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", strings.NewReader(`{"model":"grok-4.5"}`))
	require.NoError(t, err)
	grokReq.Header.Set("Authorization", "Bearer oauth-token")
	grokReq.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")

	const callers = 32
	clients := make(chan *http.Client, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clients <- entry.clientForRequest(grokReq)
		}()
	}
	wg.Wait()
	close(clients)

	var fallbackClient *http.Client
	for client := range clients {
		if fallbackClient == nil {
			fallbackClient = client
			continue
		}
		require.Same(t, fallbackClient, client)
	}
	require.NotSame(t, baseClient, fallbackClient)
	_, ok := fallbackClient.Transport.(*grokAccessDeniedFallbackTransport)
	require.True(t, ok)
}

func TestApplyGrokCLIProxyHeaders(t *testing.T) {
	t.Run("uses pinned stable version for the CLI proxy", func(t *testing.T) {
		t.Setenv("XAI_GROK_CLI_VERSION", "")
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "sub2api-grok/1.0")

		applyGrokCLIProxyHeaders(req)

		require.Equal(t, "0.2.114", req.Header.Get("x-grok-client-version"))
		require.Equal(t, "xai-grok-cli", req.Header.Get("X-XAI-Token-Auth"))
		require.Equal(t, "xai-grok-workspace/0.2.114", req.Header.Get("User-Agent"))
	})

	t.Run("accepts a valid operator override", func(t *testing.T) {
		t.Setenv("XAI_GROK_CLI_VERSION", "0.2.115-alpha.1")
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/chat/completions", nil)
		require.NoError(t, err)

		applyGrokCLIProxyHeaders(req)

		require.Equal(t, "0.2.115-alpha.1", req.Header.Get("x-grok-client-version"))
		require.Equal(t, "xai-grok-workspace/0.2.115-alpha.1", req.Header.Get("User-Agent"))
	})

	t.Run("rejects an unsafe override", func(t *testing.T) {
		t.Setenv("XAI_GROK_CLI_VERSION", "0.2.115\r\nX-Injected: true")
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
		require.NoError(t, err)

		applyGrokCLIProxyHeaders(req)

		require.Equal(t, "0.2.114", req.Header.Get("x-grok-client-version"))
		require.Empty(t, req.Header.Get("X-Injected"))
	})

	t.Run("rejects an override below the supported minimum", func(t *testing.T) {
		t.Setenv("XAI_GROK_CLI_VERSION", "0.2.113")
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
		require.NoError(t, err)

		applyGrokCLIProxyHeaders(req)

		require.Equal(t, "0.2.114", req.Header.Get("x-grok-client-version"))
		require.Equal(t, "xai-grok-workspace/0.2.114", req.Header.Get("User-Agent"))
	})

	t.Run("rejects a prerelease override at the minimum version", func(t *testing.T) {
		t.Setenv("XAI_GROK_CLI_VERSION", "0.2.114-beta.1")
		req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
		require.NoError(t, err)

		applyGrokCLIProxyHeaders(req)

		require.Equal(t, "0.2.114", req.Header.Get("x-grok-client-version"))
		require.Equal(t, "xai-grok-workspace/0.2.114", req.Header.Get("User-Agent"))
	})

	for _, version := range []string{
		"0.2.0115",
		"0.2.115-alpha..1",
		"0.3",
		"1",
		"0.2.115+build.1",
	} {
		t.Run("rejects invalid semver "+version, func(t *testing.T) {
			t.Setenv("XAI_GROK_CLI_VERSION", version)
			req, err := http.NewRequest(http.MethodPost, "https://cli-chat-proxy.grok.com/v1/responses", nil)
			require.NoError(t, err)

			applyGrokCLIProxyHeaders(req)

			require.Equal(t, "0.2.114", req.Header.Get("x-grok-client-version"))
			require.Equal(t, "xai-grok-workspace/0.2.114", req.Header.Get("User-Agent"))
		})
	}

	t.Run("leaves direct xAI API requests unchanged", func(t *testing.T) {
		t.Setenv("XAI_GROK_CLI_VERSION", "0.2.95")
		req, err := http.NewRequest(http.MethodPost, "https://api.x.ai/v1/responses", nil)
		require.NoError(t, err)
		req.Header.Set("User-Agent", "sub2api-grok/1.0")

		applyGrokCLIProxyHeaders(req)

		require.Empty(t, req.Header.Get("x-grok-client-version"))
		require.Empty(t, req.Header.Get("X-XAI-Token-Auth"))
		require.Equal(t, "sub2api-grok/1.0", req.Header.Get("User-Agent"))
	})
}

// HTTPUpstreamSuite HTTP 上游服务测试套件
// 使用 testify/suite 组织测试，支持 SetupTest 初始化
type HTTPUpstreamSuite struct {
	suite.Suite
	cfg *config.Config // 测试用配置
}

// SetupTest 每个测试用例执行前的初始化
// 创建空配置，各测试用例可按需覆盖
