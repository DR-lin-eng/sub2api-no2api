package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDetectOpenAIVerificationRecommendation(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
		ok      bool
	}{
		{name: "response metadata array", payload: `{"type":"response.metadata","metadata":{"openai_verification_recommendation":["trusted_access_for_cyber"]}}`, want: CodexVerificationRecommendationTrustedAccessForCyber, ok: true},
		{name: "duplicate and unknown values", payload: `{"type":"response.metadata","metadata":{"openai_verification_recommendation":["unknown","TRUSTED_ACCESS_FOR_CYBER","trusted_access_for_cyber"]}}`, want: CodexVerificationRecommendationTrustedAccessForCyber, ok: true},
		{name: "wrong event shape", payload: `{"type":"response.failed","response":{"metadata":{"openai_verification_recommendation":["trusted_access_for_cyber"]}}}`, ok: false},
		{name: "scalar ignored", payload: `{"type":"response.metadata","metadata":{"openai_verification_recommendation":"trusted_access_for_cyber"}}`, ok: false},
		{name: "unknown ignored", payload: `{"type":"response.metadata","metadata":{"openai_verification_recommendation":["custom_review"]}}`, ok: false},
		{name: "missing", payload: `{"response":{"metadata":{}}}`, ok: false},
		{name: "malformed", payload: `{`, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := detectOpenAIVerificationRecommendation([]byte(tt.payload))
			require.Equal(t, tt.ok, ok)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestVerificationRecommendationRecordsWithoutChangingRetry(t *testing.T) {
	payload := []byte(`{"type":"response.metadata","metadata":{"openai_verification_recommendation":["trusted_access_for_cyber"]}}`)
	require.True(t, isOpenAIVerificationRecommendation(payload))
	require.True(t, (&OpenAIGatewayService{}).shouldFailoverOpenAIUpstreamResponse(http.StatusInternalServerError, "retry me", []byte(`{"error":{"code":"server_error"}}`)))

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, MarkOpenAIVerificationRecommendation(c, payload, http.StatusOK))
	mark, ok := GetOpenAIVerificationRecommendation(c)
	require.True(t, ok)
	require.Equal(t, CodexVerificationRecommendationTrustedAccessForCyber, mark.Recommendation)
	require.False(t, HasOpsClientBusinessLimited(c))
}

func TestVerificationRecommendationObservesSuccessfulMetadataWithoutErrorMark(t *testing.T) {
	payload := []byte(`{"type":"response.metadata","metadata":{"openai_verification_recommendation":["trusted_access_for_cyber"]}}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, MarkOpenAIVerificationRecommendation(c, payload, http.StatusOK))
	mark, ok := GetOpenAIVerificationRecommendation(c)
	require.True(t, ok)
	require.Equal(t, http.StatusOK, mark.UpstreamStatus)
	require.False(t, HasOpsClientBusinessLimited(c))

	ClearOpenAIVerificationRecommendation(c)
	_, ok = GetOpenAIVerificationRecommendation(c)
	require.False(t, ok)
	require.True(t, MarkOpenAIVerificationRecommendation(c, payload, http.StatusOK))
	_, ok = GetOpenAIVerificationRecommendation(c)
	require.True(t, ok)
}

func TestVerificationRecommendationDetectedInsideSSEBody(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte("data: {\"type\":\"response.metadata\",\"metadata\":{\"openai_verification_recommendation\":[\"trusted_access_for_cyber\"]}}\n\n")
	require.True(t, markOpenAIVerificationRecommendationFromBody(c, body, http.StatusOK))
	mark, ok := GetOpenAIVerificationRecommendation(c)
	require.True(t, ok)
	require.Equal(t, CodexVerificationRecommendationTrustedAccessForCyber, mark.Recommendation)
}
