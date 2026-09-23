package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAICodexGuardianHeader = "x-codex-guardian"
	openAIMemgenRequestHeader = "x-openai-memgen-request"
)

type openAICodexRequestSemantics struct {
	subagent     string
	requestKind  string
	threadSource string
	turnTrigger  string
}

func parseOpenAICodexRequestSemantics(body []byte) openAICodexRequestSemantics {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return openAICodexRequestSemantics{}
	}
	semantics := openAICodexRequestSemantics{
		subagent: strings.TrimSpace(gjson.GetBytes(body, "client_metadata.x-openai-subagent").String()),
	}
	turnMetadata := gjson.GetBytes(body, "client_metadata.x-codex-turn-metadata")
	if turnMetadata.Type != gjson.String || !gjson.Valid(turnMetadata.String()) {
		return semantics
	}
	metadata := turnMetadata.String()
	semantics.requestKind = strings.TrimSpace(gjson.Get(metadata, "request_kind").String())
	semantics.threadSource = strings.TrimSpace(gjson.Get(metadata, "thread_source").String())
	semantics.turnTrigger = strings.TrimSpace(gjson.Get(metadata, "turn_trigger").String())
	if semantics.subagent == "" {
		semantics.subagent = strings.TrimSpace(gjson.Get(metadata, "subagent_kind").String())
	}
	return semantics
}

func applyOpenAICodexSemanticRequestHeaders(headers http.Header, c *gin.Context, account *Account, body []byte) {
	if headers == nil {
		return
	}
	deleteOpenAIHeaderEqualFold(headers, openAICodexGuardianHeader)
	deleteOpenAIHeaderEqualFold(headers, openAIMemgenRequestHeader)
	deleteOpenAIHeaderEqualFold(headers, "x-openai-subagent")
	if account == nil || !account.IsOpenAIOAuth() {
		return
	}
	normalizeOpenAICodexTurnMetadataHeader(headers, c, account, body)

	semantics := parseOpenAICodexRequestSemantics(body)
	if semantics.subagent == "" {
		semantics.subagent = codexInboundHeaderValue(c, "x-openai-subagent")
	}
	if validCodexSubagentValue(semantics.subagent) {
		headers.Set("x-openai-subagent", semantics.subagent)
	}

	switch codexInboundHeaderValue(c, openAICodexGuardianHeader) {
	case "reviewer":
		if semantics.subagent != "guardian" || semantics.threadSource != "guardian_review" {
			break
		}
		headers.Set(openAICodexGuardianHeader, "reviewer")
	case "classifier":
		if semantics.subagent != "guardian" || semantics.threadSource != "guardian_classifier" || semantics.turnTrigger != "guardian_classifier" {
			break
		}
		headers.Set(openAICodexGuardianHeader, "classifier")
	}

	if codexInboundHeaderValue(c, openAIMemgenRequestHeader) == "true" &&
		semantics.subagent == "memory_consolidation" &&
		semantics.requestKind == "memory" &&
		semantics.threadSource == "memory_consolidation" &&
		semantics.turnTrigger == "memory_consolidation" {
		headers.Set(openAIMemgenRequestHeader, "true")
	}
}

// normalizeOpenAICodexTurnMetadataHeader closes the header-only bypass around
// client_metadata.x-codex-turn-metadata. The body projection is preferred when
// present because it has already passed through the account/session rewrite;
// otherwise the raw header is parsed, bounded, and stripped of app-server-owned
// identity/permission fields before it reaches an OAuth upstream.
func normalizeOpenAICodexTurnMetadataHeader(headers http.Header, c *gin.Context, account *Account, body []byte) {
	if headers == nil || account == nil || !account.IsOpenAIOAuth() {
		return
	}
	source := strings.TrimSpace(codexBodyMetadataValue(body, openAIWSTurnMetadataHeader))
	if source == "" {
		source = strings.TrimSpace(headers.Get(openAIWSTurnMetadataHeader))
	}
	if source == "" {
		return
	}

	if fingerprintIDs := resolveCodexFingerprintIDsFromGinContext(account, c); fingerprintIDs != nil && fingerprintIDs.mode != codexFingerprintOff {
		normalized := rewriteCodexTurnMetadataValue(source, fingerprintIDs)
		if normalized == "" || !gjson.Valid(normalized) {
			deleteOpenAIHeaderEqualFold(headers, openAIWSTurnMetadataHeader)
			return
		}
		headers.Set(openAIWSTurnMetadataHeader, normalized)
		return
	}

	ids := resolveCodexOutboundSessionIDs(c, account, body, "")
	normalized, ok := normalizeUntrustedCodexTurnMetadataValue(source, ids)
	if !ok {
		deleteOpenAIHeaderEqualFold(headers, openAIWSTurnMetadataHeader)
		return
	}
	headers.Set(openAIWSTurnMetadataHeader, normalized)
}

func stagedCodexOutboundSessionBody(c *gin.Context) []byte {
	if c != nil {
		if value, exists := c.Get(codexOutboundSessionBodyContextKey); exists {
			body, _ := value.([]byte)
			return body
		}
	}
	return nil
}
