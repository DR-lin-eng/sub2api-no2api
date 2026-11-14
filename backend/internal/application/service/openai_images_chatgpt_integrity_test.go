//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatGPTAppImagesProtocolVerificationStreamAndJSON(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			encoded := encodeOpenAIImageTestPNG(t, 3, 3)
			var keys []string
			prepares, generations := 0, 0
			svc, upstream := newChatGPTAppImageTestServiceWithPrepare(t, func(w http.ResponseWriter, r *http.Request) {
				prepares++
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				key := gjson.GetBytes(body, "p").String()
				require.True(t, strings.HasPrefix(key, "gAAAAAC"))
				data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(key, "gAAAAAC"))
				require.NoError(t, err)
				var env []any
				require.NoError(t, json.Unmarshal(data, &env))
				require.Len(t, env, 25)
				require.Equal(t, r.Header.Get("User-Agent"), env[4])
				require.Equal(t, "zh-CN", env[7])
				require.NotEqual(t, "persisted-account-device", env[14])
				keys = append(keys, key)
				program := []byte(fmt.Sprintf(`[[3,"fixture-vm-%d"]]`, prepares))
				for n := range program {
					program[n] ^= key[n%len(key)]
				}
				w.Header().Set("Content-Type", "application/json")
				err = json.NewEncoder(w).Encode(map[string]any{"token": fmt.Sprintf("requirements-%d", prepares), "prepare_token": "must-not-win", "proofofwork": map[string]any{"required": true, "seed": "fixture-seed", "difficulty": "00"}, "turnstile": map[string]any{"required": true, "dx": base64.StdEncoding.EncodeToString(program)}})
				require.NoError(t, err)
			}, func(w http.ResponseWriter, r *http.Request) {
				generations++
				require.Equal(t, "/backend-api/f/conversation", r.URL.Path)
				require.Equal(t, fmt.Sprintf("requirements-%d", generations), r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Token"))
				require.Empty(t, r.Header.Get("OpenAI-Sentinel-Chat-Requirements-Prepare-Token"))
				require.Equal(t, base64.StdEncoding.EncodeToString(fmt.Appendf(nil, "fixture-vm-%d", generations)), r.Header.Get("OpenAI-Sentinel-Turnstile-Token"))
				require.True(t, strings.HasPrefix(r.Header.Get("OpenAI-Sentinel-Proof-Token"), "gAAAAAB"))
				require.True(t, strings.HasSuffix(r.Header.Get("OpenAI-Sentinel-Proof-Token"), "~S"))
				chatGPTAppImageStream(w, encoded)
			})
			request := fmt.Appendf(nil, `{"model":"gpt-image-2","prompt":"draw","n":2,"stream":%t}`, stream)
			c := newOpenAIImagesContext(t, request)
			c.Request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
			account := chatGPTAppImageTestAccount()
			account.Extra = map[string]any{"openai_device_id": "persisted-account-device"}
			parsed, err := svc.ParseOpenAIImagesRequest(c, request)
			require.NoError(t, err)
			result, err := svc.ForwardImages(context.Background(), c, account, request, parsed, "")
			require.NoError(t, err)
			require.Equal(t, 2, result.ImageCount)
			require.Equal(t, 2, prepares)
			require.Equal(t, 2, generations)
			require.NotEqual(t, keys[0], keys[1])
			for _, call := range upstream.calls {
				require.Equal(t, "persisted-account-device", call.headers.Get("oai-did"))
				require.Equal(t, "zh-CN", call.headers.Get("OAI-Language"))
				require.Empty(t, call.headers.Get("X-OpenAI-Expected-Account-ID"))
				require.Empty(t, call.headers.Get("X-OpenAI-Attach-Auth"))
				if strings.Contains(call.path, "/sentinel/") {
					require.Empty(t, call.headers.Get("OpenAI-Sentinel-Proof-Token"))
				}
			}
		})
	}
}

func TestChatGPTAppImagesIntegrityForceLoginAndNoReplay(t *testing.T) {
	svc, upstream := newChatGPTAppImageTestServiceWithPrepare(t, func(w http.ResponseWriter, r *http.Request) {
		_, err := io.WriteString(w, `{"force_login":true,"proofofwork":{"required":true}}`)
		require.NoError(t, err)
	}, func(http.ResponseWriter, *http.Request) { t.Error("must not generate or finalize after force_login") })
	body := []byte(`{"model":"gpt-image-2","prompt":"draw"}`)
	c := newOpenAIImagesContext(t, body)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	result, err := svc.ForwardImages(context.Background(), c, chatGPTAppImageTestAccount(), body, parsed, "")
	require.Nil(t, result)
	require.ErrorContains(t, err, "force_login")
	require.Len(t, upstream.calls, 1)
}

func TestChatGPTAppImagesIntegrityUsesPersistedCredentialOwnerDevice(t *testing.T) {
	prepares := 0
	svc, upstream := newChatGPTAppImageTestServiceWithPrepare(t, func(w http.ResponseWriter, r *http.Request) {
		prepares++
		require.Equal(t, "persisted-parent-device", r.Header.Get("oai-did"))
		body := `{"token":"first-round-token"}`
		if prepares == 2 {
			body = `{"turnstile":{"required":true}}`
		}
		_, err := io.WriteString(w, body)
		require.NoError(t, err)
	}, func(http.ResponseWriter, *http.Request) { t.Error("unexpected conversation request") })
	parent := chatGPTAppImageTestAccount()
	parent.Extra = map[string]any{"openai_device_id": "persisted-parent-device"}
	svc.accountRepo = &stubOpenAIAccountRepo{accounts: []Account{*parent}}
	shadow := &Account{ID: 812, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parent.ID, Extra: map[string]any{"openai_device_id": "must-not-use-shadow-device"}}
	session, err := svc.newChatGPTImageSession(t.Context(), newOpenAIImagesContext(t, []byte(`{}`)), shadow)
	require.NoError(t, err)
	require.NoError(t, session.prepareIntegrity(t.Context()))
	require.Equal(t, "first-round-token", session.integrity.Get("OpenAI-Sentinel-Chat-Requirements-Token"))
	require.Error(t, session.prepareIntegrity(t.Context()))
	require.Empty(t, session.integrity)
	require.Len(t, upstream.calls, 2)
	require.Equal(t, "persisted-parent-device", parent.GetOpenAIDeviceID())
}
