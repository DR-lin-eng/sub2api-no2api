package service

import (
	"context"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpstreamResponseModelObserverTerminalWinsAndRecordsConflict(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observer.ObserveOpenAI([]byte(`{"type":"response.created","response":{"model":"gpt-5.5"}}`), "response.created")
	observer.ObserveOpenAI([]byte(`{"type":"response.completed","response":{"model":"gpt-5.4"}}`), "response.completed")
	require.Equal(t, "gpt-5.4", observer.Model())
	require.True(t, observer.Conflict())
}

func TestUpstreamResponseModelObserverSupportsAnthropicAndGeminiShapes(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observer.ObserveAnthropic([]byte(`{"type":"message_start","message":{"model":"claude-sonnet-4-20250514"}}`))
	require.Equal(t, "claude-sonnet-4-20250514", observer.Model())

	observer = &upstreamResponseModelObserver{}
	observer.ObserveGemini([]byte(`{"response":{"modelVersion":"gemini-2.5-pro"}}`))
	observer.ObserveGemini([]byte(`{"modelVersion":"gemini-2.5-pro-latest"}`))
	require.Equal(t, "gemini-2.5-pro-latest", observer.Model())
	require.True(t, observer.Conflict())
}

func TestUpstreamResponseModelObservationAttemptReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	beginUpstreamResponseModelObservation(c).Observe("failed-attempt-model", false)
	beginUpstreamResponseModelObservation(c).Observe("successful-attempt-model", false)
	require.Equal(t, "successful-attempt-model", observedUpstreamResponseModel(c))
	require.False(t, observedUpstreamResponseModelConflict(c))
}

func TestUpstreamModelMismatchThreeStateAndCaseInsensitiveComparison(t *testing.T) {
	require.Nil(t, upstreamModelMismatch("gpt-5.5", ""))
	matched := upstreamModelMismatch("gpt-5.5", "GPT-5.5")
	require.NotNil(t, matched)
	require.False(t, *matched)
	mismatched := upstreamModelMismatch("gpt-5.5", "gpt-5.4")
	require.NotNil(t, mismatched)
	require.True(t, *mismatched)
}

func TestUpstreamModelMismatchTreatsGrokBuildIDsAsAliases(t *testing.T) {
	for _, tt := range []struct {
		name string
		sent string
		got  string
	}{
		{name: "grok 4.5", sent: "grok-4.5-latest", got: "GROK-4.5-BUILD"},
		{name: "grok 4.6", sent: "grok-4.6", got: "grok-4.6-build"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mismatch := upstreamModelMismatch(tt.sent, tt.got)
			require.NotNil(t, mismatch)
			require.False(t, *mismatch)
		})
	}
}

func TestUpstreamModelMismatchDoesNotCollapseUnrelatedBuildIDs(t *testing.T) {
	mismatch := upstreamModelMismatch("gpt-5.5", "gpt-5.5-build")
	require.NotNil(t, mismatch)
	require.True(t, *mismatch)
}

func TestUpstreamResponseModelObserverBoundsUntrustedModelName(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observer.Observe("  "+strings.Repeat("模", upstreamResponseModelMaxLength+1)+"  ", false)
	require.Len(t, []rune(observer.Model()), upstreamResponseModelMaxLength)
}

func TestObserveOpenAISSEBodyIgnoresMalformedPayload(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observeOpenAISSEBody(observer, "data: not-json\n\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5.4\"}}\n\n")

	require.Equal(t, "gpt-5.4", observer.Model())
	require.False(t, observer.Conflict())
}

func TestObserveAntigravityGeminiSSELineReadsWrapperModelWithoutUnwrap(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "top level", payload: `{"modelVersion":"gemini-3-pro","response":{"candidates":[]}}`, want: "gemini-3-pro"},
		{name: "single wrapper", payload: `{"response":{"modelVersion":"gemini-3-pro","candidates":[]}}`, want: "gemini-3-pro"},
		{name: "nested wrapper", payload: `{"response":{"response":{"modelVersion":"gemini-3-pro","candidates":[]}}}`, want: "gemini-3-pro"},
		{name: "outer wins", payload: `{"modelVersion":"gemini-outer","response":{"modelVersion":"gemini-inner"}}`, want: "gemini-outer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(nil)
			beginUpstreamResponseModelObservation(c)
			(&AntigravityGatewayService{}).observeAntigravityGeminiSSELine(c, "data: "+tt.payload)
			require.Equal(t, tt.want, observedUpstreamResponseModel(c))
			require.False(t, observedUpstreamResponseModelConflict(c))
		})
	}
}

func TestUpstreamResponseModelObserverRejectsMalformedJSONWithModelField(t *testing.T) {
	observer := &upstreamResponseModelObserver{}
	observer.ObserveOpenAI([]byte(`{"response":{"model":"gpt-5.4"}`), "response.completed")
	require.Empty(t, observer.Model())
}

func TestBuildRecordUsageLogPersistsUpstreamResponseModelAudit(t *testing.T) {
	result := &ForwardResult{
		Model:                 "claude-sonnet-4",
		UpstreamModel:         "claude-sonnet-4-20250514",
		UpstreamResponseModel: "claude-sonnet-4-20250513",
	}
	log := (&GatewayService{}).buildRecordUsageLog(
		context.Background(),
		&recordUsageCoreInput{},
		result,
		&APIKey{ID: 2},
		&User{ID: 1},
		&Account{ID: 3, Platform: PlatformAnthropic},
		nil,
		"claude-sonnet-4",
		1,
		1,
		1,
		BillingTypeBalance,
		false,
		nil,
		&recordUsageOpts{},
	)

	require.NotNil(t, log.UpstreamResponseModel)
	require.Equal(t, "claude-sonnet-4-20250513", *log.UpstreamResponseModel)
	require.NotNil(t, log.UpstreamModelMismatch)
	require.True(t, *log.UpstreamModelMismatch)
}
