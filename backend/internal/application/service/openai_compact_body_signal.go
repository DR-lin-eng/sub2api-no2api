package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAINativeCompactionV2Key     = "openai_native_compaction_v2"
	openAIRemoteCompactionV2Feature = "remote_compaction_v2"
)

// MarkOpenAINativeCompactionV2 marks the native /responses v2 request detected
// by the HTTP handler. The marker is request-scoped and is consumed by HTTP and
// WS builders when negotiating outbound Codex features.
func MarkOpenAINativeCompactionV2(c *gin.Context) {
	if c != nil {
		c.Set(openAINativeCompactionV2Key, true)
	}
}

func isOpenAINativeCompactionV2(c *gin.Context) bool {
	return c != nil && c.GetBool(openAINativeCompactionV2Key)
}

func ensureOpenAIRemoteCompactionV2BetaFeature(headers http.Header) {
	if headers == nil {
		return
	}
	features := make([]string, 0, 4)
	for _, value := range headers.Values("x-codex-beta-features") {
		for _, token := range strings.Split(value, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			if token == openAIRemoteCompactionV2Feature {
				return
			}
			features = append(features, token)
		}
	}
	features = append(features, openAIRemoteCompactionV2Feature)
	headers.Set("x-codex-beta-features", strings.Join(features, ","))
}

func hasOpenAICodexBetaFeaturesHeader(headers http.Header) bool {
	if headers == nil {
		return false
	}
	for _, value := range headers.Values("x-codex-beta-features") {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// applyOpenAICodexBetaFeatures mirrors the session-level Codex negotiation:
// native compaction is always explicit, while ordinary OAuth requests receive
// the default feature only when the client did not declare a non-empty set.
// API-key/third-party upstreams remain untouched except for native v2 turns.
func applyOpenAICodexBetaFeatures(c *gin.Context, account *Account, headers http.Header) {
	if headers == nil {
		return
	}
	if isOpenAINativeCompactionV2(c) {
		ensureOpenAIRemoteCompactionV2BetaFeature(headers)
		return
	}
	if account == nil || !account.IsOpenAIOAuth() || hasOpenAICodexBetaFeaturesHeader(headers) {
		return
	}
	headers.Set("x-codex-beta-features", openAIRemoteCompactionV2Feature)
}

// HasCompactionTriggerInInput detects an input item with
// type="compaction_trigger". The handler combines this body signal with the
// request path and stream flag to distinguish the native remote compaction v2
// wire from the legacy /responses/compact bridge.
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			found = true
			return false
		}
		return true
	})
	return found
}
