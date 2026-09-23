package service

import (
	"encoding/json"
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

func stagedCodexOutboundSessionBody(c *gin.Context) []byte {
	if c != nil {
		if value, exists := c.Get(codexOutboundSessionBodyContextKey); exists {
			body, _ := value.([]byte)
			return body
		}
	}
	return nil
}

func rewriteCodexTurnMetadataStringField(raw, key, value string) string {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return raw
	}
	metadata := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return raw
	}
	metadata[key] = value
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return raw
	}
	return string(rebuilt)
}
