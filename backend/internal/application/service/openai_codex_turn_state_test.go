package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
