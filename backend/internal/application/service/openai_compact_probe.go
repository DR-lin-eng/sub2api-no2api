package service

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// AccountTestModeDefault follows the account's configured text endpoint.
	AccountTestModeDefault = "default"
	// AccountTestModeResponses explicitly probes /v1/responses.
	AccountTestModeResponses = "responses"
	// AccountTestModeChatCompletions explicitly probes /v1/chat/completions.
	AccountTestModeChatCompletions = "chat_completions"
	// AccountTestModeCompact drives the native remote-compaction v2 probe
	// (streaming /responses with a compaction_trigger input item).
	AccountTestModeCompact = "compact"
)

func normalizeAccountTestMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case AccountTestModeResponses, AccountTestModeChatCompletions, AccountTestModeCompact:
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return AccountTestModeDefault
	}
}

func createOpenAICompactProbePayload(model string, isOAuth bool) map[string]any {
	payload := map[string]any{
		"model":        strings.TrimSpace(model),
		"instructions": "You are a helpful coding assistant.",
		"input": []any{
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": "Respond with OK.",
			},
			map[string]any{"type": "compaction_trigger"},
		},
		"stream": true,
	}
	if isOAuth {
		payload["store"] = false
	}
	return payload
}

// openAICompactProbeFoundCompactionItem accepts the native SSE form and the
// JSON fallbacks emitted by compatible gateways. A 2xx without an item is not
// a successful capability probe: the trigger may have been silently dropped.
func openAICompactProbeFoundCompactionItem(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	bodyText := string(body)
	if _, found := findRawCompactionItemFromSSE(bodyText); found {
		return true
	}
	if finalResponse, ok := extractCodexFinalResponse(bodyText); ok && responsesOutputHasCompactionItem(finalResponse) {
		return true
	}
	return responsesOutputHasCompactionItem(body)
}

func shouldMarkOpenAICompactUnsupported(status int, body []byte) bool {
	switch status {
	case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	case http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity:
		lower := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(body) + " " + string(body)))
		if strings.Contains(lower, "compact") {
			for _, keyword := range []string{
				"unsupported",
				"not support",
				"does not support",
				"not available",
				"disabled",
			} {
				if strings.Contains(lower, keyword) {
					return true
				}
			}
		}
	}
	return false
}

func buildOpenAICompactProbeExtraUpdates(resp *http.Response, body []byte, probeErr error, compactionFound bool, now time.Time) map[string]any {
	updates := map[string]any{
		"openai_compact_checked_at":  now.Format(time.RFC3339),
		"openai_compact_last_status": nil,
	}

	if resp != nil {
		updates["openai_compact_last_status"] = resp.StatusCode
	}

	switch {
	case probeErr != nil:
		updates["openai_compact_last_error"] = truncateString(sanitizeUpstreamErrorMessage(probeErr.Error()), 2048)
	case resp == nil:
		updates["openai_compact_last_error"] = "compact probe failed"
	default:
		errMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if errMsg == "" && len(body) > 0 {
			errMsg = strings.TrimSpace(string(body))
		}
		if errMsg == "" && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
			errMsg = "HTTP " + strconv.Itoa(resp.StatusCode)
		}
		errMsg = truncateString(sanitizeUpstreamErrorMessage(errMsg), 2048)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && compactionFound {
			updates["openai_compact_supported"] = true
			updates["openai_compact_last_error"] = ""
		} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			updates["openai_compact_supported"] = false
			updates["openai_compact_last_error"] = "upstream returned 2xx without a compaction output item (native remote compaction v2 unsupported)"
		} else {
			if shouldMarkOpenAICompactUnsupported(resp.StatusCode, body) {
				updates["openai_compact_supported"] = false
			}
			updates["openai_compact_last_error"] = errMsg
		}
	}

	return updates
}

func mergeExtraUpdates(base map[string]any, more map[string]any) map[string]any {
	if len(base) == 0 && len(more) == 0 {
		return nil
	}
	out := make(map[string]any, len(base)+len(more))
	for key, value := range base {
		out[key] = value
	}
	for key, value := range more {
		out[key] = value
	}
	return out
}

func compactProbeSessionID(accountID int64) string {
	if accountID <= 0 {
		return deriveCodexStableUUID("sub2api:codex-compact-probe:v1:anonymous")
	}
	return deriveCodexStableUUID("sub2api:codex-compact-probe:v1:" + strconv.FormatInt(accountID, 10))
}
