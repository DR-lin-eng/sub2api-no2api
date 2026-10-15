package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func desktopAlignmentToken(t *testing.T, accountID string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID}})
	require.NoError(t, err)
	return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(payload) + ".fixture"
}

func desktopAlignmentContext(path string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	c.Request.Header.Set("Accept", "application/json")
	c.Request.Header["content-type"] = []string{"text/plain"}
	c.Request.Header["Content-Type"] = []string{"application/xml"}
	return c
}

func TestOpenAIOAuthDesktopAlignmentPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 81, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "fixture-a"}}
	svc := &OpenAIGatewayService{}
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":[]}`)
	builders := map[string]func(*Account, string) error{
		"http": func(a *Account, token string) error {
			_, err := svc.buildUpstreamRequest(context.Background(), desktopAlignmentContext("/v1/responses"), a, body, token, true, "", true)
			return err
		},
		"passthrough": func(a *Account, token string) error {
			_, err := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), desktopAlignmentContext("/v1/responses"), a, body, token)
			return err
		},
		"websocket": func(a *Account, token string) error {
			_, _, err := svc.buildOpenAIWSHeaders(context.Background(), desktopAlignmentContext("/v1/responses"), a, token, OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", "", "", "", "")
			return err
		},
		"authentication": func(a *Account, token string) error {
			_, err := svc.buildOpenAIAuthenticationHeaders(context.Background(), a, token)
			return err
		},
	}
	for _, name := range []string{"http", "passthrough", "websocket", "authentication"} {
		build := builders[name]
		t.Run(name, func(t *testing.T) {
			err := build(account, desktopAlignmentToken(t, "fixture-b"))
			fmt.Printf("PRINCIPAL transport=%s mismatched_token_rejected=%t\n", name, err != nil)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "fixture-a")
			require.NotContains(t, err.Error(), "fixture-b")
			var failover *UpstreamFailoverError
			require.ErrorAs(t, err, &failover)
			require.Equal(t, GatewayFailureScopeAccount, failover.Scope)
			require.Equal(t, NextAccountRetry, failover.NextAccountAction)
			require.NoError(t, build(account, desktopAlignmentToken(t, "fixture-a")))
			require.NoError(t, build(account, "opaque-fixture"))
		})
	}
}

func TestOpenAIOAuthDesktopAlignmentShadowPrincipal(t *testing.T) {
	pid := int64(100)
	parent := &Account{ID: pid, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "fixture-parent"}}
	shadow := &Account{ID: 200, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &pid}
	svc := &OpenAIGatewayService{accountRepo: &stubChatGPTHeadersRepo{byID: map[int64]*Account{pid: parent}}}
	_, err := svc.buildOpenAIAuthenticationHeaders(context.Background(), shadow, desktopAlignmentToken(t, "fixture-other"))
	require.Error(t, err)
	_, err = svc.buildOpenAIAuthenticationHeaders(context.Background(), shadow, desktopAlignmentToken(t, "fixture-parent"))
	require.NoError(t, err)
}

func TestOpenAIOAuthDesktopAlignmentHTTPMedia(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 81, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, compact := range []bool{false, true} {
				name := fmt.Sprintf("passthrough=%t/stream=%t/compact=%t", passthrough, stream, compact)
				t.Run(name, func(t *testing.T) {
					path := "/v1/responses"
					wantAccept := "text/event-stream"
					if compact {
						path += "/compact"
						wantAccept = "application/json"
					}
					c := desktopAlignmentContext(path)
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%t,"input":[]}`, stream))
					var req *http.Request
					var err error
					if passthrough {
						req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "opaque-fixture")
					} else {
						req, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "opaque-fixture", stream, "", true)
					}
					require.NoError(t, err)
					contentTypes := req.Header.Values("Content-Type")
					sort.Strings(contentTypes)
					fmt.Printf("MEDIA %s content_type=%s accept=%s\n", name, strings.Join(contentTypes, ","), req.Header.Get("Accept"))
					require.Equal(t, []string{"application/json"}, contentTypes)
					require.Equal(t, wantAccept, req.Header.Get("Accept"))
					for key := range req.Header {
						if strings.EqualFold(key, "Content-Type") {
							require.Equal(t, "Content-Type", key)
						}
					}
				})
			}
		}
	}
}

func TestOpenAIOAuthDesktopAlignmentProofMetadata(t *testing.T) {
	proofKeys := []string{"x-oai-attestation", "X-OAI-IS", "x_oai_is_update", "OAI-Device-Id", "oai-did", "x-codex-client-id", "x-codex-client-session-token", "X-OpenAI-Expected-Account-ID", "X-OpenAI-Account-Routing-Override", "Authorization", "Cookie"}
	for _, mode := range []string{"ordinary", "full"} {
		t.Run(mode, func(t *testing.T) {
			turn := map[string]any{"thread_source": "cli", "safe_extra": "keep-me"}
			outer := map[string]any{"safe_extra": "keep-me"}
			for _, key := range proofKeys {
				turn[key] = "fixture-proof"
				outer[key] = "fixture-proof"
			}
			encodedTurn, err := json.Marshal(turn)
			require.NoError(t, err)
			outer["x-codex-turn-metadata"] = string(encodedTurn)
			payload := map[string]any{"model": "gpt-5.6-sol", "client_metadata": outer, "input": []any{map[string]any{"role": "user", "content": "x-oai-attestation is user content", "Cookie": "keep-user-data"}}}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			account := &Account{ID: 81, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			req, err := (&OpenAIGatewayService{}).buildUpstreamRequest(context.Background(), desktopAlignmentContext("/v1/responses"), account, body, "opaque-fixture", true, "", true)
			require.NoError(t, err)
			body, err = io.ReadAll(req.Body)
			require.NoError(t, err)
			if req.Header.Get("Content-Encoding") == "zstd" {
				decoder, decodeErr := zstd.NewReader(nil)
				require.NoError(t, decodeErr)
				defer decoder.Close()
				body, err = decoder.DecodeAll(body, nil)
				require.NoError(t, err)
			}
			var projected map[string]any
			require.NoError(t, json.Unmarshal(body, &projected))
			var ok bool
			outer, ok = projected["client_metadata"].(map[string]any)
			require.True(t, ok)
			if mode == "full" {
				projectCodexFullSimulationMetadata(outer)
			}
			turn = map[string]any{}
			turnMetadata, ok := outer["x-codex-turn-metadata"].(string)
			require.True(t, ok)
			require.NoError(t, json.Unmarshal([]byte(turnMetadata), &turn))
			retained := 0
			for _, key := range proofKeys {
				if _, ok := outer[key]; ok {
					retained++
				}
				if _, ok := turn[key]; ok {
					retained++
				}
			}
			fmt.Printf("METADATA mode=%s retained_nontransferable_fields=%d\n", mode, retained)
			require.Zero(t, retained)
			require.Equal(t, "keep-me", turn["safe_extra"])
			require.Equal(t, "keep-user-data", gjson.GetBytes(body, "input.0.Cookie").String())
		})
	}
}

func TestOpenAIOAuthDesktopAlignmentEngineIdentity(t *testing.T) {
	for _, client := range []string{"codex_rmcp_cli", "codex_http_cli", "codex_app_server_cli"} {
		t.Run(client, func(t *testing.T) {
			ua := client + "/0.161.0 (Linux 6.8; x86_64) unknown (" + client + "; 26.1007.71326)"
			identity := resolveCodexOutboundIdentity(ua)
			fmt.Printf("IDENTITY configured=%s retained=%t\n", client, identity.originator == client && identity.userAgent == ua)
			require.Equal(t, client, identity.originator)
			require.Equal(t, ua, identity.userAgent)
			require.Equal(t, "0.161.0", identity.version)
		})
	}
}

func TestOpenAIOAuthDesktopAlignmentForwardWire(t *testing.T) {
	for _, mode := range []string{"off", "device", "session", "full"} {
		for _, passthrough := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				name := fmt.Sprintf("mode=%s/passthrough=%t/stream=%t", mode, passthrough, stream)
				t.Run(name, func(t *testing.T) {
					c := newCodexSimulationTestContext("/v1/responses")
					c.Request.Header.Set("Accept", "application/json")
					c.Request.Header.Set("Content-Type", "text/plain")
					svc := newCodexSimulationTestService(mode == "full", codexContinuationOff)
					upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_desktop_alignment", "gpt-5.5")}
					svc.httpUpstream = upstream
					account := openAIFingerprintAccount(403, map[string]any{"openai_passthrough": passthrough})
					ua := "codex_app_server_cli/0.161.0 (Linux 6.8; x86_64) unknown"
					account.Credentials = map[string]any{"access_token": "opaque-fixture", "chatgpt_account_id": "fixture-principal", "user_agent": ua}
					account.Extra[codexFingerprintModeExtraKey] = mode
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"input":"x-oai-attestation remains user text","client_metadata":{"x-oai-attestation":"fixture-proof","x-codex-client-session-token":"fixture-proof","safe_extra":"keep-me","x-codex-turn-metadata":"{\"x_oai_is\":\"fixture-proof\",\"OAI-Device-Id\":\"fixture-proof\",\"safe_extra\":\"keep-me\"}"}}`, stream))
					prepared, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
					require.NoError(t, err)
					result, err := svc.Forward(context.Background(), c, account, prepared)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.NotNil(t, upstream.lastReq)
					var outer map[string]any
					require.NoError(t, json.Unmarshal([]byte(gjson.GetBytes(upstream.lastBody, "client_metadata").Raw), &outer))
					turn := map[string]any{}
					require.NoError(t, json.Unmarshal([]byte(gjson.GetBytes(upstream.lastBody, "client_metadata.x-codex-turn-metadata").String()), &turn))
					retained := 0
					for _, envelope := range []map[string]any{outer, turn} {
						for _, key := range []string{"x-oai-attestation", "x-codex-client-session-token", "x_oai_is", "OAI-Device-Id"} {
							if _, ok := envelope[key]; ok {
								retained++
							}
						}
					}
					fmt.Printf("FORWARD %s status=%d content_type=%s accept=%s retained_nontransferable_fields=%d identity_retained=%t\n", name, c.Writer.Status(), upstream.lastReq.Header.Get("Content-Type"), upstream.lastReq.Header.Get("Accept"), retained, upstream.lastReq.Header.Get("User-Agent") == ua)
					require.Equal(t, http.StatusOK, c.Writer.Status())
					require.Equal(t, "application/json", upstream.lastReq.Header.Get("Content-Type"))
					require.Equal(t, "text/event-stream", upstream.lastReq.Header.Get("Accept"))
					require.Zero(t, retained)
					require.Equal(t, ua, upstream.lastReq.Header.Get("User-Agent"))
					require.Contains(t, string(upstream.lastBody), "x-oai-attestation remains user text")
					require.Equal(t, "keep-me", turn["safe_extra"])
				})
			}
		}
	}
}
