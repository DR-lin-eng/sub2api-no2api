package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAICodexTurnStateTestContext(t *testing.T, apiKeyID int64, sessionID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	if sessionID != "" {
		c.Request.Header.Set("session_id", sessionID)
	}
	if apiKeyID > 0 {
		c.Set("api_key", &APIKey{ID: apiKeyID})
	}
	return c, recorder
}

func TestOpenAICodexTurnStateSeed(t *testing.T) {
	c, _ := newOpenAICodexTurnStateTestContext(t, 7, "session-1")
	require.Equal(t, "7\x00session-1", openAICodexTurnStateSeed(c))

	c.Request.Header.Set("session-id", "session-hyphen")
	require.Equal(t, "7\x00session-hyphen", openAICodexTurnStateSeed(c))

	withoutSession, _ := newOpenAICodexTurnStateTestContext(t, 7, "")
	require.Empty(t, openAICodexTurnStateSeed(withoutSession))
	require.Empty(t, openAICodexTurnStateSeed(nil))
}

func TestRelayOpenAICodexTurnStateTracksAndClearsProvenance(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newOpenAICodexTurnStateTestContext(t, 7, "session-relay")
	upstream := http.Header{"X-Codex-Turn-State": []string{"state-a"}}

	svc.relayOpenAICodexTurnState(c, &Account{ID: 42}, upstream)
	require.Equal(t, "state-a", c.Writer.Header().Get(openAICodexTurnStateHeader))
	raw, ok := svc.openaiCodexTurnStateOrigins.Load("7\x00session-relay")
	require.True(t, ok)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, int64(42), origin.accountID)
	require.True(t, origin.expiresAt.After(time.Now()))

	svc.relayOpenAICodexTurnState(c, &Account{ID: 43}, http.Header{})
	require.Empty(t, c.Writer.Header().Get(openAICodexTurnStateHeader))
}

func TestStageOpenAICodexTurnStateRecordsOnlyAfterCommit(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newOpenAICodexTurnStateTestContext(t, 9, "session-staged")
	upstream := http.Header{"X-Codex-Turn-State": []string{"state-b"}}

	var staged http.Header
	stageOpenAICodexTurnState(&staged, upstream)
	require.Equal(t, "state-b", staged.Get(openAICodexTurnStateHeader))
	_, recorded := svc.openaiCodexTurnStateOrigins.Load("9\x00session-staged")
	require.False(t, recorded)

	svc.noteStagedOpenAICodexTurnStateCommitted(c, &Account{ID: 44}, staged)
	raw, recorded := svc.openaiCodexTurnStateOrigins.Load("9\x00session-staged")
	require.True(t, recorded)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, int64(44), origin.accountID)

	stageOpenAICodexTurnState(&staged, http.Header{})
	require.Empty(t, staged.Get(openAICodexTurnStateHeader))
}

func TestGuardOpenAICodexTurnStateEcho(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, _ := newOpenAICodexTurnStateTestContext(t, 11, "session-guard")
	upstream := http.Header{"X-Codex-Turn-State": []string{"state-a"}}
	svc.relayOpenAICodexTurnState(c, &Account{ID: 52}, upstream)

	sameAccount := upstream.Clone()
	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 52}, sameAccount)
	require.Equal(t, "state-a", sameAccount.Get(openAICodexTurnStateHeader))

	otherAccount := upstream.Clone()
	svc.guardOpenAICodexTurnStateEcho(c, &Account{ID: 53}, otherAccount)
	require.Empty(t, otherAccount.Get(openAICodexTurnStateHeader))

	unknownSession, _ := newOpenAICodexTurnStateTestContext(t, 11, "session-unknown")
	unknown := upstream.Clone()
	svc.guardOpenAICodexTurnStateEcho(unknownSession, &Account{ID: 53}, unknown)
	require.Equal(t, "state-a", unknown.Get(openAICodexTurnStateHeader))

	svc.openaiCodexTurnStateOrigins.Store("11\x00session-expired", openAICodexTurnStateOrigin{
		accountID: 52,
		expiresAt: time.Now().Add(-time.Minute),
	})
	expiredSession, _ := newOpenAICodexTurnStateTestContext(t, 11, "session-expired")
	expired := upstream.Clone()
	svc.guardOpenAICodexTurnStateEcho(expiredSession, &Account{ID: 53}, expired)
	require.Equal(t, "state-a", expired.Get(openAICodexTurnStateHeader))
	_, exists := svc.openaiCodexTurnStateOrigins.Load("11\x00session-expired")
	require.False(t, exists)
}

func TestConfiguredCodexTurnStateReplayUsesRandomEligiblePool(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	settingService := NewSettingService(repo, nil)
	_, err := settingService.SetCodexSimulationSettings(context.Background(), &CodexSimulationSettings{
		TurnStateReplayEnabled: true,
		TurnStates:             []string{"global-state", "account-state", "other-state"},
		TurnStateAccountIDs: map[string][]int64{
			"account-state": {42},
			"other-state":   {99},
		},
		ContinuationMode: "off",
		StateTTLSeconds:  60,
	})
	require.NoError(t, err)
	svc := &OpenAIGatewayService{settingService: settingService}
	c, _ := newOpenAICodexTurnStateTestContext(t, 7, "random-replay")
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	seen := map[string]bool{}
	for range 200 {
		headers := http.Header{}
		svc.applyConfiguredCodexTurnStateReplay(c, account, headers)
		state := headers.Get(openAICodexTurnStateHeader)
		require.Contains(t, []string{"global-state", "account-state"}, state)
		require.NotEqual(t, "other-state", state)
		seen[state] = true
	}
	require.True(t, seen["global-state"])
	require.True(t, seen["account-state"])
}

func TestConfiguredCodexTurnStateReplayOverridesInboundState(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	settingService := NewSettingService(repo, nil)
	_, err := settingService.SetCodexSimulationSettings(context.Background(), &CodexSimulationSettings{
		TurnStateReplayEnabled: true,
		TurnStates:             []string{"admin-state"},
		ContinuationMode:       "off",
		StateTTLSeconds:        60,
	})
	require.NoError(t, err)
	svc := &OpenAIGatewayService{settingService: settingService}
	c, _ := newOpenAICodexTurnStateTestContext(t, 7, "override-replay")
	headers := http.Header{openAICodexTurnStateHeader: []string{"client-state"}}

	svc.applyConfiguredCodexTurnStateReplay(c, &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, headers)

	require.Equal(t, "admin-state", headers.Get(openAICodexTurnStateHeader))
}

func TestBuildOpenAIResponsesRequestReplaysConfiguredTurnState(t *testing.T) {
	repo := newCodexSimulationSettingRepo()
	settings := NewSettingService(repo, &config.Config{})
	_, err := settings.SetCodexSimulationSettings(context.Background(), &CodexSimulationSettings{
		TurnStateReplayEnabled: true,
		TurnStates:             []string{"configured-state", "other-account-state"},
		TurnStateAccountIDs:    map[string][]int64{"other-account-state": {99}},
		ContinuationMode:       "off",
		StateTTLSeconds:        60,
	})
	require.NoError(t, err)
	svc := &OpenAIGatewayService{cfg: &config.Config{}, settingService: settings}
	c, _ := newOpenAICodexTurnStateTestContext(t, 7, "request-replay")
	c.Request.Header.Set(openAICodexTurnStateHeader, "client-state")
	account := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	request, err := svc.buildUpstreamRequest(context.Background(), c, account,
		[]byte(`{"model":"gpt-5.5","input":"hi"}`), "token", false, "", true)

	require.NoError(t, err)
	require.Equal(t, "configured-state", request.Header.Get(openAICodexTurnStateHeader))
}

func TestWriteOpenAIPassthroughResponseHeadersRelaysAndClearsTurnState(t *testing.T) {
	destination := http.Header{}
	writeOpenAIPassthroughResponseHeaders(destination, http.Header{
		"X-Codex-Turn-State": []string{"state-p"},
	}, nil)
	require.Equal(t, "state-p", destination.Get(openAICodexTurnStateHeader))

	writeOpenAIPassthroughResponseHeaders(destination, http.Header{
		"Content-Type": []string{"application/json"},
	}, nil)
	require.Empty(t, destination.Get(openAICodexTurnStateHeader))
}

func TestOpenAIPassthroughStreamingRecordsTurnStateOnFirstOutput(t *testing.T) {
	svc := &OpenAIGatewayService{}
	c, recorder := newOpenAICodexTurnStateTestContext(t, 13, "session-stream")
	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		"",
		`data: {"type":"response.completed","response":{"id":"resp_1","usage":{"input_tokens":1,"output_tokens":1}}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":       []string{"text/event-stream"},
			"X-Codex-Turn-State": []string{"state-stream"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}

	_, err := svc.handleStreamingResponsePassthrough(
		context.Background(), resp, c, &Account{ID: 61, Platform: PlatformOpenAI},
		time.Now(), "gpt-5.6-codex", "gpt-5.6-codex",
	)
	require.NoError(t, err)
	require.Equal(t, "state-stream", recorder.Header().Get(openAICodexTurnStateHeader))
	raw, ok := svc.openaiCodexTurnStateOrigins.Load("13\x00session-stream")
	require.True(t, ok)
	origin, ok := raw.(openAICodexTurnStateOrigin)
	require.True(t, ok)
	require.Equal(t, int64(61), origin.accountID)
}
