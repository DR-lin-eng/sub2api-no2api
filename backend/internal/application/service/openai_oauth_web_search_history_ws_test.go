//go:build unit

package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type webSearchStagedWSConn struct{ *stagedPassthroughConn }

func (c *webSearchStagedWSConn) WriteJSON(ctx context.Context, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.WriteFrame(ctx, coderws.MessageText, data)
}

func TestOAuthWebSearchHistoryWebSocketAllTurns20261009(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		for _, lite := range []bool{false, true} {
			for _, kind := range []string{AccountTypeOAuth, AccountTypeAPIKey} {
				name := mode + "/" + map[bool]string{true: "lite", false: "standard"}[lite] + "/" + kind
				t.Run(name, func(t *testing.T) {
					cfg := passthroughLifecycleConfig()
					cfg.Gateway.OpenAIWS.OAuthEnabled = true
					cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
					cfg.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds = 5
					cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
					cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
					staged := newStagedPassthroughConn()
					conn := &webSearchStagedWSConn{staged}
					svc := newPassthroughLifecycleService(cfg, staged)
					svc.openaiWSPassthroughDialer = &stagedPassthroughDialer{conn: conn}
					pool := newOpenAIWSConnPool(cfg)
					t.Cleanup(pool.Close)
					pool.setClientDialerForTest(&stagedPassthroughDialer{conn: conn})
					svc.openaiWSPool = pool
					account := passthroughLifecycleAccount()
					account.Type = kind
					account.Credentials = map[string]any{"access_token": "fixture", "api_key": "fixture"}
					account.Extra = map[string]any{
						"openai_oauth_responses_websockets_v2_mode":     mode,
						"openai_oauth_responses_websockets_v2_enabled":  true,
						"openai_apikey_responses_websockets_v2_mode":    mode,
						"openai_apikey_responses_websockets_v2_enabled": true,
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					server, serverErr := startPassthroughLifecycleServer(t, ctx, svc, account)
					defer server.Close()
					body := []byte(`{"type":"response.create","model":"gpt-6-astra","stream":false,"input":[{"type":"web_search_call","status":"completed"},{"type":"message","role":"user","content":"continue"},{"type":"compaction_trigger"}],"tools":[],"tool_choice":"auto"}`)
					if lite {
						var err error
						body, err = sjson.SetBytes(body, "client_metadata."+responsesLiteWSMetadataKey, "true")
						require.NoError(t, err)
					}
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
						path := "tools.0.type"
						if lite {
							path = `input.#(type=="additional_tools").tools.0.type`
						}
						if kind == AccountTypeOAuth {
							require.Equal(t, "web_search", gjson.GetBytes(forwarded, path).String(), "mode=%s turn=%d", mode, turn)
							require.Equal(t, "none", gjson.GetBytes(forwarded, "tool_choice").String())
						} else {
							require.False(t, gjson.GetBytes(forwarded, path).Exists())
							require.Equal(t, "auto", gjson.GetBytes(forwarded, "tool_choice").String())
						}
						items := gjson.GetBytes(forwarded, "input").Array()
						require.Equal(t, "compaction_trigger", items[len(items)-1].Get("type").String())
						if lite {
							require.Empty(t, gjson.GetBytes(forwarded, "tools").Array())
						}
						staged.Send(`{"type":"response.completed","response":{"id":"resp_search","status":"completed","model":"gpt-6-astra","usage":{"input_tokens":1,"output_tokens":1}}}`)
						_, err := readPassthroughLifecycleFrame(t, client, 5*time.Second)
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
}
