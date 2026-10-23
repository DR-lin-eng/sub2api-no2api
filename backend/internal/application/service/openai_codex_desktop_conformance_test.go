package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// These are distinct desktop surfaces, not inference request requirements.
var codexDesktopForeignSurfaceHeaders = []string{
	"X-OpenAI-Account-Routing-Override", "X-OpenAI-Attach-Integrity-State",
	"X-OAI-IS", "X-OAI-IS-Pending-Updates", "X-OAI-IS-Client-Observation",
	"X-OAI-Attestation", "X-Sentinel-DC", "X-Codex-Client-Session-Token",
	"X-Codex-Protocol-Version", "X-Codex-Subscribe-Cursor",
	"X-OpenAI-Allow-Environmentless-Fallback", "X-OpenAI-Product-Sku",
}

func TestCodexDesktopHTTPWireConformance(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("passthrough=%t/stream=%t/compact=%t", passthrough, stream, compact), func(t *testing.T) {
					path := "/v1/responses"
					if compact {
						path += "/compact"
					}
					c := newCodexSessionHeaderTestContext(t, path)
					c.Request.Header.Set("session-id", "fixture-session")
					c.Request.Header.Set("x-codex-installation-id", "foreign-installation")
					c.Request.Header.Set("x-codex-window-id", "foreign-window")
					for _, name := range codexDesktopForeignSurfaceHeaders {
						c.Request.Header.Set(name, "foreign-surface")
					}
					account := openAIFingerprintAccount(704, nil)
					account.Credentials = map[string]any{"chatgpt_account_id": "fixture-principal"}
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"input":[{"type":"reasoning","encrypted_content":"opaque-reasoning+/=="},{"type":"function_call","call_id":"call_fixture","name":"echo","arguments":"{\"text\":\"hello\"}","encrypted_function_args":"opaque-arguments+/=="}],"client_metadata":{"session_id":"fixture-session","custom_counter":9007199254740993,"x-codex-turn-metadata":"{\"thread_source\":\"user\"}"}}`, stream))
					svc := &OpenAIGatewayService{}
					var req *http.Request
					var err error
					if passthrough {
						req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "fixture-token")
					} else {
						req, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "fixture-token", stream, "", true)
					}
					require.NoError(t, err)
					type capture struct {
						headers http.Header
						body    []byte
						path    string
					}
					captured := make(chan capture, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						data, readErr := io.ReadAll(r.Body)
						if readErr != nil {
							http.Error(w, readErr.Error(), 500)
							return
						}
						captured <- capture{r.Header.Clone(), data, r.URL.Path}
						if stream && !compact {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprint(w, "data: {\"type\":\"response.completed\"}\n\n")
						} else {
							w.Header().Set("Content-Type", "application/json")
							fmt.Fprint(w, `{ "id": "resp_fixture" }`)
						}
					}))
					defer server.Close()
					localURL, err := url.Parse(server.URL)
					require.NoError(t, err)
					req.URL.Scheme, req.URL.Host = localURL.Scheme, localURL.Host
					response, err := server.Client().Do(req)
					require.NoError(t, err)
					defer func() { _ = response.Body.Close() }()
					_, err = io.Copy(io.Discard, response.Body)
					require.NoError(t, err)
					wire := <-captured
					if wire.headers.Get("Content-Encoding") == "zstd" {
						wire.body = decodeZstdBody(t, wire.body)
					}
					bodyInstallation := gjson.GetBytes(wire.body, "client_metadata.x-codex-installation-id").String()
					t.Logf("installation_matches_body=%t foreign_window=%t metadata_counter=%s encrypted_history_preserved=%t", wire.headers.Get("x-codex-installation-id") == bodyInstallation, wire.headers.Get("x-codex-window-id") != "", gjson.GetBytes(wire.body, "client_metadata.custom_counter").Raw, gjson.GetBytes(wire.body, "input").Raw == gjson.GetBytes(body, "input").Raw)
					require.Equal(t, "/backend-api/codex/responses"+strings.TrimPrefix(path, "/v1/responses"), wire.path)
					require.Equal(t, "Bearer fixture-token", wire.headers.Get("Authorization"))
					require.Equal(t, "fixture-principal", wire.headers.Get("chatgpt-account-id"))
					for _, name := range codexDesktopForeignSurfaceHeaders {
						require.Empty(t, wire.headers.Get(name), name)
					}
					if compact {
						require.Equal(t, "application/json", wire.headers.Get("Accept"))
					} else {
						require.Equal(t, "text/event-stream", wire.headers.Get("Accept"))
					}
					require.Equal(t, gjson.GetBytes(body, "input").Raw, gjson.GetBytes(wire.body, "input").Raw)
					require.Equal(t, "9007199254740993", gjson.GetBytes(wire.body, "client_metadata.custom_counter").Raw)
					require.Equal(t, bodyInstallation, wire.headers.Get("x-codex-installation-id"))
					require.Empty(t, wire.headers.Get("x-codex-window-id"))
				})
			}
		}
	}
}

func TestCodexDesktopWSWireConformance(t *testing.T) {
	c := newCodexSessionHeaderTestContext(t, "/v1/responses")
	c.Request.Header.Set("session-id", "fixture-session")
	c.Request.Header.Set("x-codex-installation-id", "foreign-installation")
	c.Request.Header.Set("x-codex-window-id", "foreign-window")
	for _, name := range codexDesktopForeignSurfaceHeaders {
		c.Request.Header.Set(name, "foreign-surface")
	}
	account := openAIFingerprintAccount(705, nil)
	body := []byte(`{"client_metadata":{"session_id":"fixture-session","x-codex-turn-metadata":"{\"thread_source\":\"user\"}"}}`)
	ids := resolveCodexOutboundSessionIDs(c, account, body, "")
	prepared, err := rewriteCodexOutboundSessionMetadata(body, account, ids, c)
	require.NoError(t, err)
	stageCodexOutboundSessionBody(c, prepared)
	headers, _, err := (&OpenAIGatewayService{}).buildOpenAIWSHeaders(context.Background(), c, account, "fixture-token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "", "gpt-5.5", "")
	require.NoError(t, err)
	captured := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, acceptErr := coderws.Accept(w, r, nil)
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		captured <- r.Header.Clone()
		_ = conn.Write(r.Context(), coderws.MessageText, []byte(`{"type":"ready"}`))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &coderws.DialOptions{HTTPHeader: headers})
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	_, _, err = conn.Read(ctx)
	require.NoError(t, err)
	wire := <-captured
	installation := gjson.GetBytes(prepared, "client_metadata.x-codex-installation-id").String()
	t.Logf("ws_installation_matches_body=%t foreign_window=%t", wire.Get("x-codex-installation-id") == installation, wire.Get("x-codex-window-id") != "")
	for _, name := range codexDesktopForeignSurfaceHeaders {
		require.Empty(t, wire.Get(name), name)
	}
	require.Equal(t, installation, wire.Get("x-codex-installation-id"))
	require.Empty(t, wire.Get("x-codex-window-id"))
}

func TestCodexDesktopMetadataHeaderBudget(t *testing.T) {
	for _, mode := range []codexFingerprintMode{codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull} {
		t.Run(string(mode), func(t *testing.T) {
			c := newCodexSessionHeaderTestContext(t, "/v1/responses")
			account := openAIFingerprintAccount(706, map[string]any{codexFingerprintModeExtraKey: string(mode)})
			if mode != codexFingerprintOff {
				stageCodexFingerprintIDs(c, resolveCodexFingerprintIDs(account, "fixture-session", mode))
			}
			for _, field := range []string{"tool_namespaces_info", "workspaces"} {
				raw, err := json.Marshal(map[string]any{field: []any{map[string]any{"description": strings.Repeat("large-tool-inventory", 900)}}})
				require.NoError(t, err)
				headers := make(http.Header)
				headers.Set(openAIWSTurnMetadataHeader, string(raw))
				applyOpenAICodexSemanticRequestHeaders(headers, c, account, nil)
				header := headers.Get(openAIWSTurnMetadataHeader)
				t.Logf("mode=%s field=%s header_bytes=%d inventory_in_header=%t", mode, field, len(header), gjson.Get(header, "tool_namespaces_info").Exists())
				require.LessOrEqual(t, len(header), codexWorkspaceMetadataMaxBytes)
				require.False(t, gjson.Get(header, "tool_namespaces_info").Exists())
			}
		})
	}
}

func TestCodexDesktopPoolIdentityChanges(t *testing.T) {
	for _, name := range []string{"x-codex-installation-id", "x-codex-window-id"} {
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(&openAIWSFakeDialer{})
			defer pool.Close()
			headers := make(http.Header)
			headers.Set("Authorization", "Bearer fixture-token")
			headers.Set(name, "identity-a")
			req := openAIWSAcquireRequest{Account: openAIFingerprintAccount(707, nil), WSURL: "wss://fixture.example/responses", Headers: headers}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			first, err := pool.Acquire(ctx, req)
			require.NoError(t, err)
			firstID := first.ConnID()
			first.Release()
			unchanged, err := pool.Acquire(ctx, req)
			require.NoError(t, err)
			require.Equal(t, firstID, unchanged.ConnID())
			unchanged.Release()
			req.Headers = headers.Clone()
			req.Headers.Set(name, "identity-b")
			changed, err := pool.Acquire(ctx, req)
			require.NoError(t, err)
			defer changed.Release()
			t.Logf("field=%s changed_identity_reused=%t", name, changed.Reused())
			require.NotEqual(t, firstID, changed.ConnID())
		})
	}
}

func TestCodexDesktopClearedAttemptDoesNotRevivePreviousPrincipal(t *testing.T) {
	c := newCodexSessionHeaderTestContext(t, "/v1/responses")
	c.Request.Header.Set("x-codex-installation-id", "foreign-installation")
	c.Request.Header.Set("x-codex-window-id", "foreign-window")
	svc := newCodexSimulationTestService(true, codexContinuationOff)
	previous := openAIFingerprintAccount(708, map[string]any{codexFingerprintModeExtraKey: "full"})
	body := []byte(`{"model":"gpt-5.5","input":"hello","client_metadata":{"session_id":"fixture-session"}}`)
	_, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, previous, body)
	require.NoError(t, err)
	require.NotNil(t, resolveCodexFingerprintIDsFromGinContext(previous, c))
	current := openAIFingerprintAccount(709, nil)
	req, err := svc.buildUpstreamRequest(context.Background(), c, current, body, "fixture-token", false, "", true)
	require.NoError(t, err)
	t.Logf("cleared_attempt_revived=%t current_installation_bound=%t", resolveCodexFingerprintIDsFromGinContext(current, c) != nil, req.Header.Get("x-codex-installation-id") == resolveConvergedInstallationID(current))
	require.Nil(t, resolveCodexFingerprintIDsFromGinContext(current, c))
	require.Equal(t, resolveConvergedInstallationID(current), req.Header.Get("x-codex-installation-id"))
	require.Empty(t, req.Header.Get("x-codex-window-id"))
}

func TestCodexDesktopMetadataNumbersAndJSONBoundary(t *testing.T) {
	for _, raw := range []string{`{} {}`, `{} trailing`, `[]`, `"text"`} {
		_, err := decodeCodexMetadataObject(raw)
		require.Error(t, err, raw)
	}
	for _, mode := range []codexFingerprintMode{codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull} {
		t.Run(string(mode), func(t *testing.T) {
			ids := resolveCodexFingerprintIDs(openAIFingerprintAccount(710, nil), "fixture-session", mode)
			ids.fullSimulation = mode == codexFingerprintFull
			raw := `{"forked_from_ordinal_exclusive":9007199254740993,"history_ingest_requested":true,"analytics_enabled":false}`
			metadata := rewriteCodexTurnMetadataValue(raw, ids)
			header := codexProjectedTurnMetadataHeader(metadata)
			require.Equal(t, "9007199254740993", gjson.Get(header, "forked_from_ordinal_exclusive").Raw)
			require.Equal(t, gjson.True, gjson.Get(header, "history_ingest_requested").Type)
			require.Equal(t, gjson.False, gjson.Get(header, "analytics_enabled").Type)
		})
	}
}

func TestCodexDesktopIdentityProjectionModes(t *testing.T) {
	for _, mode := range []codexFingerprintMode{codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintFull} {
		for _, compact := range []bool{false, true} {
			t.Run(fmt.Sprintf("mode=%s/compact=%t", mode, compact), func(t *testing.T) {
				path := "/v1/responses"
				if compact {
					path += "/compact"
				}
				c := newCodexSessionHeaderTestContext(t, path)
				c.Request.Header.Set("x-codex-installation-id", "foreign-installation")
				c.Request.Header.Set("x-codex-window-id", "foreign-window")
				svc := newCodexSimulationTestService(true, codexContinuationOff)
				account := openAIFingerprintAccount(711, map[string]any{codexFingerprintModeExtraKey: string(mode)})
				body := []byte(`{"model":"gpt-5.5","input":"hello","client_metadata":{"session_id":"fixture-session","x-codex-turn-metadata":"{\"thread_source\":\"user\",\"tool_namespaces_info\":[{\"name\":\"fixture-tool\"}]}"}}`)
				prepared, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
				require.NoError(t, err)
				ids := resolveCodexFingerprintIDsFromGinContext(account, c)
				if mode != codexFingerprintOff && ids == nil {
					ids = resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
				}
				prepared, _, err = applyCodexFingerprintClientMetadataToBody(prepared, ids, c)
				require.NoError(t, err)
				req, err := svc.buildUpstreamRequestWithFingerprint(context.Background(), c, account, prepared, "fixture-token", false, "", true, ids)
				require.NoError(t, err)
				data, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				if req.Header.Get("Content-Encoding") == "zstd" {
					data = decodeZstdBody(t, data)
				}
				installation := gjson.GetBytes(data, "client_metadata.x-codex-installation-id").String()
				if ids != nil && ids.fullSimulation && !ids.directInstallationHeader {
					require.Empty(t, req.Header.Get("x-codex-installation-id"))
				} else {
					require.Equal(t, installation, req.Header.Get("x-codex-installation-id"))
				}
				if mode == codexFingerprintOff || mode == codexFingerprintDevice {
					require.Empty(t, req.Header.Get("x-codex-window-id"))
				} else {
					require.Equal(t, gjson.GetBytes(data, "client_metadata.x-codex-window-id").String(), req.Header.Get("x-codex-window-id"))
				}
				require.False(t, gjson.Get(req.Header.Get(openAIWSTurnMetadataHeader), "tool_namespaces_info").Exists())
				if ids != nil {
					require.True(t, gjson.Get(gjson.GetBytes(data, "client_metadata.x-codex-turn-metadata").String(), "tool_namespaces_info").Exists())
				}
			})
		}
	}
}

func TestCodexDesktopForwardMetadataAndEncryptedHistory(t *testing.T) {
	for _, full := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("full=%t/passthrough=%t/stream=%t", full, passthrough, stream), func(t *testing.T) {
					c := newCodexSimulationTestContext("/v1/responses")
					c.Request.Header.Set("x-codex-installation-id", "foreign-installation")
					c.Request.Header.Set("x-codex-window-id", "foreign-window")
					svc := newCodexSimulationTestService(full, codexContinuationOff)
					upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_desktop", "gpt-5.5")}
					svc.httpUpstream = upstream
					account := openAIFingerprintAccount(712, map[string]any{"openai_passthrough": passthrough})
					account.Credentials = map[string]any{"access_token": "fixture-token", "chatgpt_account_id": "fixture-principal"}
					if full {
						account.Extra[codexFingerprintModeExtraKey] = "full"
					}
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"input":[{"type":"reasoning","encrypted_content":"opaque-reasoning+/=="},{"type":"function_call","call_id":"call_fixture","name":"echo","arguments":"{\"text\":\"hello\"}","encrypted_function_args":"opaque-arguments+/=="}],"client_metadata":{"session_id":"fixture-session","custom_counter":9007199254740993}}`, stream))
					prepared, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
					require.NoError(t, err)
					_, err = svc.Forward(context.Background(), c, account, prepared)
					require.NoError(t, err)
					require.NotNil(t, upstream.lastReq)
					counter := gjson.GetBytes(upstream.lastBody, "client_metadata.custom_counter").String()
					if full {
						counter = gjson.Get(gjson.GetBytes(upstream.lastBody, "client_metadata.x-codex-turn-metadata").String(), "custom_counter").String()
					}
					t.Logf("full_forward_metadata_counter=%s encrypted_reasoning=%s encrypted_function_args=%s", counter, gjson.GetBytes(upstream.lastBody, "input.0.encrypted_content").String(), gjson.GetBytes(upstream.lastBody, "input.1.encrypted_function_args").String())
					require.Equal(t, "9007199254740993", counter)
					require.Equal(t, "opaque-reasoning+/==", gjson.GetBytes(upstream.lastBody, "input.0.encrypted_content").String())
					require.Equal(t, "opaque-arguments+/==", gjson.GetBytes(upstream.lastBody, "input.1.encrypted_function_args").String())
				})
			}
		}
	}
}
