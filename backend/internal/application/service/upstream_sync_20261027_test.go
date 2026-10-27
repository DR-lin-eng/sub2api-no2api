//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGPT61CompatEffortAliases(t *testing.T) {
	for _, model := range []string{"gpt-6.1-sol", "gpt-6.1-terra"} {
		for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
			req := &apicompat.AnthropicRequest{Model: "openai/" + model + "-" + effort}
			applyOpenAICompatModelNormalization(req)
			require.Equal(t, model, req.Model)
			require.NotNil(t, req.OutputConfig)
			require.Equal(t, effort, req.OutputConfig.Effort)
			converted, err := apicompat.AnthropicToResponses(req)
			require.NoError(t, err)
			require.Equal(t, effort, converted.Reasoning.Effort)
			req.Thinking = &apicompat.AnthropicThinking{Type: "disabled"}
			converted, err = apicompat.AnthropicToResponses(req)
			require.NoError(t, err)
			require.Equal(t, "none", converted.Reasoning.Effort)
		}
	}
	req := &apicompat.AnthropicRequest{Model: "gpt-6.1-terra-max", OutputConfig: &apicompat.AnthropicOutputConfig{Effort: "low"}}
	applyOpenAICompatModelNormalization(req)
	require.Equal(t, "low", req.OutputConfig.Effort)
}
func TestGPT61ChatOnlyToolGateUsesMappedModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"gpt-6.1-sol", "gpt-6.1-terra"} {
		for _, entry := range []string{"chat", "responses", "messages"} {
			t.Run(model+"/"+entry, func(t *testing.T) {
				account := rawChatCompletionsTestAccount()
				account.Credentials["model_mapping"] = map[string]any{"alias": model}
				upstream := &httpUpstreamRecorder{}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				body := []byte(`{"model":"alias","messages":[{"role":"user","content":"hi"}],"input":"hi","tools":[{"type":"function","name":"lookup","function":{"name":"lookup","parameters":{}},"input_schema":{}}]}`)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/fixture", bytes.NewReader(body))
				var err error
				switch entry {
				case "chat":
					_, err = svc.forwardAsRawChatCompletionsWithStartTime(context.Background(), c, account, body, "", time.Now())
				case "responses":
					_, err = svc.forwardResponsesViaRawChatCompletions(context.Background(), c, account, body, time.Now())
				case "messages":
					_, err = svc.forwardAnthropicViaRawChatCompletions(context.Background(), c, account, body, "")
				}
				require.Error(t, err)
				require.Contains(t, err.Error(), "requires Responses")
				require.Empty(t, upstream.requests)
			})
		}
	}
}
func TestGrokCatalogMalformedResponsesNeverPersist(t *testing.T) {
	for _, raw := range []string{`{"error":"failed","data":[{"id":"grok-4.6"}]}`, `{"data":null}`, `{"data":[null]}`, `{"data":[1]}`, `{"data":[{}]}`, `{"data":["bad id"]}`, `{}`, `null`} {
		ids, err := extractGrokUpstreamModelIDs([]byte(raw))
		require.Error(t, err, raw)
		require.Empty(t, ids)
	}
	ids, err := extractGrokUpstreamModelIDs([]byte(`{"data":["grok-4.6",{"name":"Display","_meta":{"model":"grok-4.5"}}]}`))
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.5", "grok-4.6"}, ids)
	account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture", "base_url": "https://upstream.example/v1"}, Extra: map[string]any{"other": "kept"}}
	upstream := &httpUpstreamRecorder{responses: []*http.Response{catalogHTTPResponse(`{"data":[{"id":"grok-4.6"}]}`)}}
	repo := &sync20261027Repo{updates: map[int64]map[string]any{}}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.NotEmpty(t, catalog.DiscoveryScope)
	require.Equal(t, "fresh", GetGrokModelDiagnostics(account).State)
	before, _ := json.Marshal(repo.updates)
	upstream.responses = []*http.Response{catalogHTTPResponse(`{"data":[{"id":"grok-4.5"}],"has_more":true}`)}
	_, err = svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.Error(t, err)
	after, _ := json.Marshal(repo.updates)
	require.Equal(t, string(before), string(after))
	account.Credentials["base_url"] = "https://changed.example/v1"
	require.Equal(t, "source_changed", GetGrokModelDiagnostics(account).State)
}
func TestGrokResponsesPropagateAuditForJSONAndSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, stream := range []bool{false, true} {
		body := []byte(fmt.Sprintf(`{"model":"grok-4.5","input":"hello","stream":%t}`, stream))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/responses", bytes.NewReader(body))
		beginUpstreamResponseModelObservation(c)
		response := `{"id":"resp_fixture","model":"grok-4.6","status":"completed","output":[],"usage":{"input_tokens":100,"output_tokens":1}}`
		contentType := "application/json"
		if stream {
			contentType = "text/event-stream"
			response = "data: {\"type\":\"response.created\",\"response\":{\"model\":\"grok-4.5\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
		}
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response))}}
		account := &Account{ID: 1, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture", "base_url": "https://api.x.ai"}, Concurrency: 1}
		result, err := (&OpenAIGatewayService{httpUpstream: upstream}).forwardGrokResponses(context.Background(), c, account, body, "grok-4.5", stream, time.Now())
		require.NoError(t, err)
		require.Equal(t, "grok-4.6", result.UpstreamResponseModel)
		require.Equal(t, stream, result.UpstreamResponseModelConflict)
	}
}
func TestGrokBufferedAuditGatedAndBounded(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("grok_buffered_model_audit", enabled)
		o := beginUpstreamResponseModelObservation(c)
		resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.created\",\"response\":{\"model\":\"grok-4.5\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"grok-4.6\"}}"))}
		final, _, _, err := (&OpenAIGatewayService{}).readOpenAICompatBufferedTerminal(resp, "fixture", "fixture", c)
		require.NoError(t, err)
		o.Observe(final.Model, true)
		require.Equal(t, enabled, o.Conflict())
	}
	require.Equal(t, strings.Repeat("界", 200), normalizeObservedUpstreamResponseModel(strings.Repeat("界", 1<<20)))
	// JSON string-model entries still pass through the shared metadata parser.
	_, err := modelcatalog.Parse([]byte(`{"data":["grok-4.6"]}`), []string{"grok-4.6"})
	require.NoError(t, err)
}

type sync20261027Repo struct {
	AccountRepository
	updates map[int64]map[string]any
}

func (r *sync20261027Repo) UpdateUpstreamModelCatalog(_ context.Context, a *Account, s *modelcatalog.Snapshot) error {
	r.updates[a.ID] = map[string]any{modelcatalog.ExtraKey: s}
	return nil
}
