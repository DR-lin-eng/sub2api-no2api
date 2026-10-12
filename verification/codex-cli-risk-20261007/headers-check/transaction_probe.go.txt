package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexLineageForwardWire(t *testing.T) {
	for _, full := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("full=%t/passthrough=%t/stream=%t", full, passthrough, stream), func(t *testing.T) {
					c := newCodexSimulationTestContext("/v1/responses")
					c.Request.Header.Set("session-id", "client-session")
					svc := newCodexSimulationTestService(full, codexContinuationOff)
					upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_lineage", "gpt-5.5")}
					svc.httpUpstream = upstream
					account := openAIFingerprintAccount(403, map[string]any{"openai_passthrough": passthrough})
					account.Credentials = map[string]any{"access_token": "test-token", "chatgpt_account_id": "principal"}
					if full {
						account.Extra[codexFingerprintModeExtraKey] = "full"
					}
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"input":"hello","client_metadata":{"parent_turn_id":"parent","root_turn_id":"root","x-codex-turn-metadata":"{\"parent_turn_id\":\"parent\",\"root_turn_id\":\"root\"}"}}`, stream))
					prepared, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
					require.NoError(t, err)
					_, err = svc.Forward(context.Background(), c, account, prepared)
					require.NoError(t, err)
					require.NotNil(t, upstream.lastReq)
					turn := gjson.GetBytes(upstream.lastBody, "client_metadata.x-codex-turn-metadata").String()
					for _, key := range []string{"parent_turn_id", "root_turn_id"} {
						want := gjson.Get(turn, key).String()
						require.NotEmpty(t, want, key)
						require.NotEqual(t, "parent", want)
						require.NotEqual(t, "root", want)
						require.Equal(t, want, gjson.GetBytes(upstream.lastBody, "client_metadata."+key).String(), key+" body")
						require.Equal(t, want, gjson.Get(upstream.lastReq.Header.Get(openAIWSTurnMetadataHeader), key).String(), key+" header")
					}
				})
			}
		}
	}
}
