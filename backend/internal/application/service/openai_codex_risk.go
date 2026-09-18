package service

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	CodexVerificationRecommendationTrustedAccessForCyber = "trusted_access_for_cyber"
	CodexRiskRecommendationContextKey                    = "openai_codex_risk_recommendation"
)

// CodexRiskRecommendation is a bounded, request-local projection of the
// upstream recommendation. The raw response is deliberately not retained:
// it may contain workspace or account metadata that must not enter logs.
type CodexRiskRecommendation struct {
	Recommendation string
	UpstreamStatus int
}

func detectOpenAIVerificationRecommendation(payload []byte) (string, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return "", false
	}
	if strings.TrimSpace(gjson.GetBytes(payload, "type").String()) != "response.metadata" {
		return "", false
	}
	values := gjson.GetBytes(payload, "metadata.openai_verification_recommendation")
	if !values.Exists() || !values.IsArray() {
		return "", false
	}
	for _, item := range values.Array() {
		if strings.EqualFold(strings.TrimSpace(item.String()), CodexVerificationRecommendationTrustedAccessForCyber) {
			return CodexVerificationRecommendationTrustedAccessForCyber, true
		}
	}
	return "", false
}

func isOpenAIVerificationRecommendation(payload []byte) bool {
	_, ok := detectOpenAIVerificationRecommendation(payload)
	return ok
}

// MarkOpenAIVerificationRecommendation records a known recommendation without
// changing the wire response or retry semantics. The caller can use the
// request-local marker for observability while clients receive the original
// response.metadata event.
func MarkOpenAIVerificationRecommendation(c *gin.Context, payload []byte, upstreamStatus int) bool {
	if c == nil {
		return false
	}
	recommendation, ok := detectOpenAIVerificationRecommendation(payload)
	if !ok {
		return false
	}
	if _, exists := GetOpenAIVerificationRecommendation(c); exists {
		return true
	}
	c.Set(CodexRiskRecommendationContextKey, CodexRiskRecommendation{
		Recommendation: recommendation,
		UpstreamStatus: upstreamStatus,
	})
	return true
}

func GetOpenAIVerificationRecommendation(c *gin.Context) (CodexRiskRecommendation, bool) {
	if c == nil {
		return CodexRiskRecommendation{}, false
	}
	value, ok := c.Get(CodexRiskRecommendationContextKey)
	if !ok {
		return CodexRiskRecommendation{}, false
	}
	recommendation, ok := value.(CodexRiskRecommendation)
	return recommendation, ok && strings.TrimSpace(recommendation.Recommendation) != ""
}

func ClearOpenAIVerificationRecommendation(c *gin.Context) {
	if c != nil {
		c.Set(CodexRiskRecommendationContextKey, CodexRiskRecommendation{})
	}
}

func markOpenAIVerificationRecommendationFromBody(c *gin.Context, body []byte, status int) bool {
	if MarkOpenAIVerificationRecommendation(c, body, status) {
		return true
	}
	var marked bool
	forEachOpenAISSEDataPayload(string(body), func(payload []byte) {
		if !marked && MarkOpenAIVerificationRecommendation(c, payload, status) {
			marked = true
		}
	})
	return marked
}
