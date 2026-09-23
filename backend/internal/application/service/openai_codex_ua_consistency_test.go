//go:build unit

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

const desktopUARegression = "Codex Desktop/0.153.4 (Windows 10.0.26200; x86_64) unknown (Codex Desktop; 26.903.71938)"

func TestCodexUAConfiguredDesktopPreservesBothVersions(t *testing.T) {
	repo := newCodexVersionSettingRepo()
	repo.settings[SettingKeyOpenAICodexUserAgent] = &Setting{Value: desktopUARegression}
	repo.settings[SettingKeyOpenAICodexClientVersionSynced] = &Setting{Value: "0.200.0"}
	svc := NewSettingService(repo, nil)
	SetCodexCanonicalUserAgentResolver(func() string {
		return svc.GetOpenAICodexCanonicalUserAgent(context.Background())
	})
	t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })

	identity := resolveCodexOutboundIdentity("")
	t.Logf("user-agent=%s; originator=%s; version=%s", identity.userAgent, identity.originator, identity.version)
	require.Equal(t, desktopUARegression, identity.userAgent)
	require.Equal(t, "Codex Desktop", identity.originator)
	require.Equal(t, "0.153.4", identity.version)

	// Auth requests use the same pair without an inference-only version header.
	headers := make(http.Header)
	ApplyCodexCanonicalAuthIdentity(headers)
	require.Equal(t, desktopUARegression, headers.Get("User-Agent"))
	require.Equal(t, "Codex Desktop", headers.Get("originator"))
	require.Empty(t, headers.Get("version"))
}

func TestCodexUAClearSettingRestoresSynchronizedDefault(t *testing.T) {
	repo := newCodexVersionSettingRepo()
	repo.settings[SettingKeyOpenAICodexClientVersionSynced] = &Setting{Value: "0.200.0"}
	svc := NewSettingService(repo, nil)
	svc.refreshCachedSettings(&SystemSettings{OpenAICodexUserAgent: desktopUARegression})
	require.Equal(t, desktopUARegression, svc.GetOpenAICodexCanonicalUserAgent(context.Background()))
	svc.refreshCachedSettings(&SystemSettings{})
	require.Equal(t, "codex_cli_rs/0.200.0"+codexCLIUserAgentSuffix, svc.GetOpenAICodexCanonicalUserAgent(context.Background()))
}

func TestCodexUADisabledEnforcementStillPairsVersion(t *testing.T) {
	SetCodexIdentityEnforcementEnabled(false)
	t.Cleanup(func() { SetCodexIdentityEnforcementEnabled(true) })
	h := make(http.Header)
	h.Set("User-Agent", desktopUARegression)
	h.Set("originator", "codex_cli_rs")
	h.Set("version", "0.999.0")
	enforceCodexIdentityHeaders(h)
	require.Equal(t, desktopUARegression, h.Get("User-Agent"))
	require.Equal(t, "Codex Desktop", h.Get("originator"))
	require.Equal(t, "0.153.4", h.Get("version"))
}

func TestCodexUAInvalidConfigurationUsesPairedDefault(t *testing.T) {
	for _, ua := range []string{"Codex Desktop/invalid", "Codex Desktop/", "Mozilla/5.0", "custom-client/1.2.3"} {
		t.Run(ua, func(t *testing.T) {
			identity := resolveCodexOutboundIdentity(ua)
			require.Equal(t, codexCLIUserAgent, identity.userAgent)
			require.Equal(t, codexCLIVersion, identity.version)
			require.Equal(t, "codex_cli_rs", identity.originator)
		})
	}
}

func TestCodexUAModelsProbeUsesAccountVersion(t *testing.T) {
	account := newCodexModelsTestAccount()
	account.Credentials["user_agent"] = desktopUARegression
	var captured *http.Request
	svc := &OpenAIGatewayService{httpUpstream: &codexModelsHTTPUpstreamStub{do: func(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
		captured = req
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"models":[]}`))}, nil
	}}}
	for _, queryVersion := range []string{"", "0.190.0"} {
		_, err := svc.FetchCodexModelsManifest(context.Background(), account, queryVersion, "")
		require.NoError(t, err)
		require.NotNil(t, captured)
		require.Equal(t, desktopUARegression, captured.Header.Get("User-Agent"))
		require.Equal(t, "0.153.4", captured.Header.Get("version"))
		if queryVersion == "" {
			queryVersion = "0.153.4"
		}
		require.Equal(t, queryVersion, captured.URL.Query().Get("client_version"))
	}
}

func TestCodexUAHTTPAndWSConsistency(t *testing.T) {
	for _, mode := range []codexFingerprintMode{codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull} {
		for _, source := range []string{"global", "account"} {
			t.Run(string(mode)+"/"+source, func(t *testing.T) {
				SetCodexCanonicalUserAgentResolver(func() string {
					if source == "global" {
						return desktopUARegression
					}
					return codexCLIUserAgent
				})
				t.Cleanup(func() { SetCodexCanonicalUserAgentResolver(nil) })
				svc := newCodexSimulationTestService(true, codexContinuationOff)
				svc.tlsFPProfileService = &TLSFingerprintProfileService{}
				account := openAIFingerprintAccount(53, map[string]any{
					codexFingerprintModeExtraKey: string(mode),
					"enable_tls_fingerprint":     true,
				})
				account.Credentials = map[string]any{"chatgpt_account_id": "ua-fixture-principal"}
				if source == "account" {
					account.Credentials["user_agent"] = desktopUARegression
				}
				for _, stream := range []bool{false, true} {
					for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
						c := newCodexSimulationTestContext(path)
						c.Request.Header.Set("User-Agent", "codex_vscode/0.100.0")
						c.Request.Header.Set("thread-id", "ua-fixture-thread")
						body := []byte(`{"model":"gpt-5.5","input":"hello","prompt_cache_key":"ua-fixture-thread"}`)
						_, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
						require.NoError(t, err)
						ids := resolveCodexFingerprintIDsFromGinContext(account, c)
						request, err := svc.buildUpstreamRequestWithFingerprint(context.Background(), c, account, body, "fixture-token", stream, "ua-fixture-thread", true, ids)
						require.NoError(t, err)
						passthrough, err := svc.buildUpstreamRequestOpenAIPassthroughWithFingerprint(context.Background(), c, account, body, "fixture-token", ids)
						require.NoError(t, err)
						ws, _, err := svc.buildOpenAIWSHeaders(context.Background(), c, account, "fixture-token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "ua-fixture-thread", "gpt-5.5", "")
						require.NoError(t, err)
						applyCodexFingerprintWSHeaders(ws, ids)
						for _, h := range []http.Header{request.Header, passthrough.Header, ws} {
							require.Equal(t, desktopUARegression, h.Get("user-agent"), "mode=%s source=%s stream=%t path=%s", mode, source, stream, path)
							require.Equal(t, "Codex Desktop", h.Get("originator"))
							require.Equal(t, "0.153.4", h.Get("version"))
						}
						if mode == codexFingerprintFull || (mode == codexFingerprintSession && path == "/v1/responses") {
							for _, name := range []string{"session-id", "thread-id", "x-client-request-id", "x-codex-window-id", "x-codex-turn-metadata"} {
								require.Equal(t, request.Header.Get(name), ws.Get(name), name)
								require.Equal(t, request.Header.Get(name), passthrough.Header.Get(name), name)
							}
						}
						httpProfile := HTTPUpstreamTLSProfileFromContext(request.Context())
						require.NotNil(t, httpProfile)
						require.Equal(t, tlsfingerprint.FingerprintKey(httpProfile), tlsfingerprint.FingerprintKey(svc.resolveTLSProfile(account)))
						require.Equal(t, tlsfingerprint.FingerprintKey(httpProfile), tlsfingerprint.FingerprintKey(HTTPUpstreamTLSProfileFromContext(passthrough.Context())))
					}
				}
			})
		}
	}
}

func TestCodexUARequestFingerprintMatchesHTTPAndWS(t *testing.T) {
	for _, mode := range []codexFingerprintMode{codexFingerprintSession, codexFingerprintFull} {
		t.Run(string(mode), func(t *testing.T) {
			ids := resolveCodexFingerprintIDs(openAIFingerprintAccount(55, nil), "client-thread", mode)
			httpHeaders, wsHeaders := make(http.Header), make(http.Header)
			httpHeaders.Set("x-client-request-id", "incoming-request")
			wsHeaders.Set("x-client-request-id", "incoming-request")
			applyCodexFingerprintHeaders(httpHeaders, ids)
			applyCodexFingerprintWSHeaders(wsHeaders, ids)
			require.Equal(t, ids.turnID, httpHeaders.Get("x-client-request-id"))
			require.Equal(t, httpHeaders.Get("x-client-request-id"), wsHeaders.Get("x-client-request-id"))
		})
	}
}

func TestCodexUAPoolRotatesChangedIdentity(t *testing.T) {
	for _, field := range []string{"User-Agent", "originator", "version"} {
		t.Run(field, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(&openAIWSFakeDialer{})
			t.Cleanup(pool.Close)
			headers := make(http.Header)
			headers.Set("User-Agent", desktopUARegression)
			headers.Set("originator", "Codex Desktop")
			headers.Set("version", "0.153.4")
			req := openAIWSAcquireRequest{Account: openAIFingerprintAccount(54, nil), WSURL: "wss://example.com/responses", Headers: headers}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			first, err := pool.Acquire(ctx, req)
			require.NoError(t, err)
			firstID := first.ConnID()
			first.Release()
			reused, err := pool.Acquire(ctx, req)
			require.NoError(t, err)
			require.Equal(t, firstID, reused.ConnID(), "unchanged identity must retain connection affinity")
			reused.Release()
			req.Headers = headers.Clone()
			req.Headers.Set(field, "changed-identity")
			changed, err := pool.Acquire(ctx, req)
			require.NoError(t, err)
			defer changed.Release()
			t.Logf("%s changed: reused=%t", field, changed.Reused())
			require.NotEqual(t, firstID, changed.ConnID(), "a handshake cannot adopt new identity headers")
		})
	}
}
