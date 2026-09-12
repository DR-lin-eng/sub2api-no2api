package service

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strings"
	"unicode/utf8"
)

// Only final answers and provider identifiers are retained. Never copy request
// bodies, credentials, errors, or reasoning text into this public projection.
type AccountQualityStageDetail struct {
	Status          string `json:"status"`
	ConversationID  string `json:"conversation_id,omitempty"`
	ResponseID      string `json:"response_id,omitempty"`
	Answer          string `json:"answer"`
	AnswerTruncated bool   `json:"answer_truncated,omitempty"`
	ReasoningTokens *int64 `json:"reasoning_tokens,omitempty"`
}

type AccountQualityProbeDetails struct {
	Stage1 *AccountQualityStageDetail `json:"stage1,omitempty"`
	Stage2 *AccountQualityStageDetail `json:"stage2,omitempty"`
}

func qualityStageDetail(probe *ScheduledTestResult) AccountQualityStageDetail {
	answer, truncated := boundedQualityText(probe.ResponseText, 16384)
	return AccountQualityStageDetail{ConversationID: qualityProviderID(probe.ConversationID), ResponseID: qualityProviderID(probe.ResponseID), Answer: answer, AnswerTruncated: truncated, ReasoningTokens: probe.ReasoningTokens}
}

func boundedQualityText(text string, limit int) (string, bool) {
	text = strings.ToValidUTF8(strings.ReplaceAll(text, "\x00", ""), "�")
	if len(text) <= limit {
		return text, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end], true
}

func qualityProviderID(value any) string {
	s, ok := value.(string)
	if !ok || len(s) > 256 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_:.", r)) {
			return ""
		}
	}
	return s
}

func (s *AccountTestService) captureQualityResponseIdentity(c *gin.Context, data map[string]any) {
	if c.Request == nil {
		return
	}
	if collect, _ := c.Request.Context().Value(accountTestUsageContextKey{}).(bool); !collect {
		return
	}
	conversationID, responseID := qualityResponseIdentity(data)
	if conversationID != "" || responseID != "" {
		s.sendEvent(c, TestEvent{Type: "response_metadata", ConversationID: conversationID, ResponseID: responseID})
	}
}

func qualityResponseIdentity(data map[string]any) (string, string) {
	if response, ok := data["response"].(map[string]any); ok {
		data = response
	}
	conversationID := qualityProviderID(data["conversation_id"])
	if conversation, ok := data["conversation"].(map[string]any); ok {
		conversationID = qualityProviderID(conversation["id"])
	}
	responseID := qualityProviderID(data["id"])
	if id := qualityProviderID(data["responseId"]); id != "" {
		responseID = id
	}
	return conversationID, responseID
}

func parseQualityResponseIdentity(body string) (conversationID, responseID string) {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event TestEvent
		if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event) != nil || event.Type != "response_metadata" {
			continue
		}
		if event.ConversationID != "" {
			conversationID = event.ConversationID
		}
		if event.ResponseID != "" {
			responseID = event.ResponseID
		}
	}
	return
}
