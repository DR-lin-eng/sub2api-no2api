package service

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/modules/qualityrender"
)

// Only final answers and provider identifiers are retained. Never copy request
// bodies, credentials, errors, or reasoning text into this public projection.
type AccountQualityStageDetail struct {
	CodeMatch            *AccountQualityCodeMatch `json:"code_match,omitempty"`
	PreviewStatus        string                   `json:"preview_status,omitempty"`
	PreviewHTML          string                   `json:"preview_html,omitempty"`
	PreviewHTMLTruncated bool                     `json:"preview_html_truncated,omitempty"`
	Status               string                   `json:"status"`
	ConversationID       string                   `json:"conversation_id,omitempty"`
	ResponseID           string                   `json:"response_id,omitempty"`
	Answer               string                   `json:"answer"`
	AnswerTruncated      bool                     `json:"answer_truncated,omitempty"`
	ReasoningTokens      *int64                   `json:"reasoning_tokens,omitempty"`
}

type AccountQualityProbeDetails struct {
	Stage1 *AccountQualityStageDetail `json:"stage1,omitempty"`
	Stage2 *AccountQualityStageDetail `json:"stage2,omitempty"`
}

type qualityProbeOutputSignal struct {
	once sync.Once
	done chan struct{}
}

type qualityProbeOutputSignalContextKey struct{}

func withQualityProbeOutputSignal(ctx context.Context) (context.Context, *qualityProbeOutputSignal) {
	signal := &qualityProbeOutputSignal{done: make(chan struct{})}
	return context.WithValue(ctx, qualityProbeOutputSignalContextKey{}, signal), signal
}

func markQualityProbeOutput(ctx context.Context) {
	if signal, ok := ctx.Value(qualityProbeOutputSignalContextKey{}).(*qualityProbeOutputSignal); ok && signal != nil {
		signal.once.Do(func() { close(signal.done) })
	}
}

func markQualityProbeOutputForContext(c *gin.Context) {
	if c != nil && c.Request != nil {
		markQualityProbeOutput(c.Request.Context())
	}
}

func qualityStageDetail(probe *ScheduledTestResult) AccountQualityStageDetail {
	answer, truncated := boundedQualityText(probe.ResponseText, 16384)
	preview, previewTruncated := qualityPreviewHTML(probe.ResponseText)
	status := "unavailable"
	if preview != "" {
		status = "ready"
	}
	return AccountQualityStageDetail{ConversationID: qualityProviderID(probe.ConversationID), ResponseID: qualityProviderID(probe.ResponseID), Answer: answer, AnswerTruncated: truncated, ReasoningTokens: probe.ReasoningTokens, PreviewHTML: preview, PreviewHTMLTruncated: previewTruncated, PreviewStatus: status}
}

// qualityPreviewHTML returns a bounded, self-contained document for the
// browser sandbox. Scripts remain available for animation, while navigation,
// external resources and event-handler attributes are removed. The source used
// for grading is never truncated; only the public preview is bounded.
func qualityPreviewHTML(source string) (string, bool) {
	const maxPreviewBytes = 256 << 10
	if strings.TrimSpace(source) == "" || len(source) > MaxQualityHTMLBytes {
		return "", false
	}
	if _, err := qualityrender.MatchHTML(source, 0); err != nil {
		return "", false
	}
	value := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(source, "```html"), "```"))
	value = stripQualityPreviewTags(value)
	if len([]byte(value)) > maxPreviewBytes {
		return "", true
	}
	return `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; font-src data:; media-src data:; connect-src 'none'; frame-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'>` + value, false
}

const MaxQualityHTMLBytes = 1 << 20

var (
	qualityPreviewTagPattern   = regexp.MustCompile(`(?is)<(?:iframe|object|embed|base|link|form)\b[^>]*>.*?</(?:iframe|object|embed|base|link|form)\s*>|<(?:iframe|object|embed|base|link|form)\b[^>]*/?>`)
	qualityPreviewEventPattern = regexp.MustCompile(`(?i)\s+on[a-z0-9_-]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)`)
	qualityPreviewURLPattern   = regexp.MustCompile(`(?i)\s+(src|href|xlink:href)\s*=\s*("([^"]*)"|'([^']*)')`)
	qualityPreviewMetaPattern  = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
)

func stripQualityPreviewTags(value string) string {
	value = qualityPreviewTagPattern.ReplaceAllString(value, "")
	value = qualityPreviewEventPattern.ReplaceAllString(value, "")
	value = qualityPreviewMetaPattern.ReplaceAllString(value, "")
	return qualityPreviewURLPattern.ReplaceAllStringFunc(value, func(attribute string) string {
		match := qualityPreviewURLPattern.FindStringSubmatch(attribute)
		url := match[3]
		if url == "" {
			url = match[4]
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(url)), "#") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(url)), "data:") {
			return attribute
		}
		return ""
	})
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
		if !isQualityIDChar(r) {
			return ""
		}
	}
	return s
}

func isQualityIDChar(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_:.", r)
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
