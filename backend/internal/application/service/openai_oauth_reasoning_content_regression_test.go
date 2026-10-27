//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const reasoningArrayParamError = "[ArrayParam] [input[93].content] [array_above_max_length] Invalid 'input[93].content': array too long. Expected an array with maximum length 0, but got an array with length 1 instead."

func oauthReasoningContentFixture(stream bool) []byte {
	items := make([]string, 0, 96)
	for i := 0; i < 93; i++ {
		items = append(items, `{"type":"message","role":"user","content":[{"type":"input_text","text":"history"}]}`)
	}
	items = append(items,
		`{"type":"reasoning","id":"rs_history","summary":[{"type":"summary_text","text":"summary to keep"}],"content":[{"type":"reasoning_text","text":"response-only reasoning"}],"encrypted_content":"fixture-cipher"}`,
		`{"type":"reasoning","summary":[],"content":[{"type":"reasoning_text","text":"another response-only part"}],"encrypted_content":"fixture-cipher-2"}`,
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"},{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}`)
	return []byte(fmt.Sprintf(`{"model":"gpt-5.5","instructions":"fixture instructions","stream":%v,"input":[%s]}`, stream, strings.Join(items, ",")))
}

func requireOAuthReasoningContentProjection(t *testing.T, body []byte, oauth bool) {
	t.Helper()
	require.Len(t, gjson.GetBytes(body, "input").Array(), 96)
	require.Equal(t, "history", gjson.GetBytes(body, "input.0.content.0.text").String())
	require.Equal(t, "continue", gjson.GetBytes(body, "input.95.content.0.text").String())
	require.Equal(t, "data:image/png;base64,AA==", gjson.GetBytes(body, "input.95.content.1.image_url").String())
	require.Equal(t, "summary to keep", gjson.GetBytes(body, "input.93.summary.0.text").String())
	require.Equal(t, "fixture-cipher", gjson.GetBytes(body, "input.93.encrypted_content").String())
	require.Equal(t, "fixture-cipher-2", gjson.GetBytes(body, "input.94.encrypted_content").String())
	for _, index := range []int{93, 94} {
		content := gjson.GetBytes(body, fmt.Sprintf("input.%d.content", index))
		if oauth {
			require.True(t, !content.Exists() || (content.IsArray() && len(content.Array()) == 0), "OAuth must not replay response-only reasoning content")
		} else {
			require.Len(t, content.Array(), 1, "API-key request content must not be stripped")
		}
	}
}

func TestOAuthReasoningContentHTTP(t *testing.T) {
	for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
		for _, passthrough := range []bool{false, true} {
			for _, endpoint := range []string{"/v1/responses", "/v1/responses/compact"} {
				for _, stream := range []bool{false, true} {
					if strings.HasSuffix(endpoint, "/compact") && stream {
						continue
					}
					name := fmt.Sprintf("%s/passthrough=%v%s/stream=%v", kind, passthrough, endpoint, stream)
					t.Run(name, func(t *testing.T) {
						ginContext := newOpenAIRejectedFieldTestContext(nil)
						recorder := ginContext.Writer
						ginContext.Request.URL.Path = endpoint
						account := newOpenAIOAuthNamespaceTestAccount()
						if kind == AccountTypeAPIKey {
							account = newOpenAIRejectedFieldTestAccount()
						}
						account.Extra = map[string]any{"openai_passthrough": passthrough}
						upstream := &httpUpstreamRecorder{}
						upstream.onDo = func() {
							content := gjson.GetBytes(upstream.lastBody, "input.93.content")
							if kind == AccountTypeOAuth && len(content.Array()) > 0 {
								errorBody, _ := json.Marshal(map[string]any{"error": map[string]any{"type": "invalid_request_error", "code": "array_above_max_length", "param": "input[93].content", "message": reasoningArrayParamError}})
								upstream.resp = newOpenAIRejectedFieldTestResponse(http.StatusBadRequest, string(errorBody))
								return
							}
							response := `{"id":"resp_fixture","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
							if (kind == AccountTypeOAuth || stream) && !strings.HasSuffix(endpoint, "/compact") {
								upstream.resp = &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":" + response + "}\n\ndata: [DONE]\n\n"))}
							} else {
								upstream.resp = newOpenAIRejectedFieldTestResponse(http.StatusOK, response)
							}
						}
						svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
						body := oauthReasoningContentFixture(stream)
						original := bytes.Clone(body)
						result, err := svc.Forward(context.Background(), ginContext, account, body)
						fmt.Printf("OBSERVED HTTP kind=%s passthrough=%v endpoint=%s stream=%v status=%d upstream_requests=%d content_length=%d error=%v\n", kind, passthrough, endpoint, stream, recorder.Status(), len(upstream.bodies), len(gjson.GetBytes(upstream.lastBody, "input.93.content").Array()), err)
						require.NoError(t, err)
						require.NotNil(t, result)
						require.Equal(t, http.StatusOK, recorder.Status())
						require.Len(t, upstream.bodies, 1)
						requireOAuthReasoningContentProjection(t, upstream.lastBody, kind == AccountTypeOAuth)
						require.Equal(t, original, body, "caller buffer must not be mutated")
					})
				}
			}
		}
	}
}

func TestOAuthReasoningContentDecoded(t *testing.T) {
	for _, single := range []bool{false, true} {
		t.Run(fmt.Sprintf("single=%v", single), func(t *testing.T) {
			var request map[string]any
			require.NoError(t, json.Unmarshal(oauthReasoningContentFixture(false), &request))
			original := request["input"].([]any)[93].(map[string]any)
			if single {
				request["input"] = original
			}
			applyCodexOAuthTransform(request, false, false)
			index := 93
			if single {
				index = 0
			}
			item := request["input"].([]any)[index].(map[string]any)
			content, _ := item["content"].([]any)
			fmt.Printf("OBSERVED DECODED single=%v content_length=%d encrypted=%s\n", single, len(content), item["encrypted_content"])
			require.Empty(t, content)
			require.Equal(t, "fixture-cipher", item["encrypted_content"])
			require.Len(t, item["summary"], 1)
			require.Len(t, original["content"], 1, "original item must stay intact")
		})
	}
}

type reasoningContentStagedWSConn struct{ *stagedPassthroughConn }

func (c *reasoningContentStagedWSConn) WriteJSON(ctx context.Context, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, payload)
}

func TestOAuthReasoningContentWebSocketAllTurns(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				cfg := passthroughLifecycleConfig()
				cfg.Gateway.OpenAIWS.OAuthEnabled = true
				cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
				cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5
				cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
				cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
				staged := newStagedPassthroughConn()
				conn := &reasoningContentStagedWSConn{staged}
				svc := newPassthroughLifecycleService(cfg, staged)
				svc.openaiWSPassthroughDialer = &stagedPassthroughDialer{conn: conn}
				pool := newOpenAIWSConnPool(cfg)
				t.Cleanup(pool.Close)
				pool.setClientDialerForTest(&stagedPassthroughDialer{conn: conn})
				svc.openaiWSPool = pool
				account := passthroughLifecycleAccount()
				account.Type = kind
				account.Credentials = map[string]any{"access_token": "fixture", "api_key": "fixture"}
				account.Extra = map[string]any{"openai_oauth_responses_websockets_v2_mode": mode, "openai_oauth_responses_websockets_v2_enabled": true, "openai_apikey_responses_websockets_v2_mode": mode, "openai_apikey_responses_websockets_v2_enabled": true}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
				defer server.Close()
				body, err := sjson.SetBytes(oauthReasoningContentFixture(false), "type", "response.create")
				require.NoError(t, err)
				client := dialPassthroughLifecycleClientWithPayload(t, server, body)
				defer func() { _ = client.CloseNow() }()
				for turn := 1; turn <= 2; turn++ {
					if turn == 2 {
						writeCtx, stop := context.WithTimeout(ctx, 5*time.Second)
						err := client.Write(writeCtx, coderws.MessageText, body)
						stop()
						require.NoError(t, err)
					}
					forwarded := requirePassthroughUpstreamWrite(t, staged, 5*time.Second)
					fmt.Printf("OBSERVED WS mode=%s kind=%s turn=%d content_length=%d\n", mode, kind, turn, len(gjson.GetBytes(forwarded, "input.93.content").Array()))
					requireOAuthReasoningContentProjection(t, forwarded, kind == AccountTypeOAuth)
					staged.Send(`{"type":"response.completed","response":{"id":"resp_content","status":"completed","model":"gpt-5.5","usage":{"input_tokens":1,"output_tokens":1}}}`)
					_, err = readPassthroughLifecycleFrame(t, client, 5*time.Second)
					require.NoError(t, err)
				}
				require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
				select {
				case err := <-serverErr:
					require.NoError(t, err)
				case <-time.After(5 * time.Second):
					t.Fatal("WS fixture did not stop")
				}
			})
		}
	}
}
