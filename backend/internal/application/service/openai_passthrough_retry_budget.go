package service

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIPassthroughRetryBudgetContextKey                      = "openai_passthrough_retry_budget"
	openAIPassthroughRetryBudgetReason     GatewayFailureReason = "openai_passthrough_attempt_budget_exhausted"
	openAIPassthroughNonReplayableReason   GatewayFailureReason = "openai_passthrough_non_replayable_transport"
)

// openAIPassthroughRetryBudget is shared by every account attempt in one HTTP
// request. The handler reuses the Gin context while switching accounts, so a
// per-Forward counter would reset and multiply the upstream request count.
type openAIPassthroughRetryBudget struct {
	mu          sync.Mutex
	maxAttempts int
	used        int
}

func defaultOpenAIPassthroughAttemptBudget() int {
	return len(openAIPassthroughTransportRetryBackoffs) + 1
}

// ConfigureOpenAIPassthroughAttemptBudget establishes the request-wide ceiling
// before the handler starts account selection. It preserves the configured
// account-switch budget and adds one bounded same-account recovery window.
func ConfigureOpenAIPassthroughAttemptBudget(c *gin.Context, maxAccountSwitches int) {
	if c == nil {
		return
	}
	if maxAccountSwitches < 0 {
		maxAccountSwitches = 0
	}
	maxAttempts := maxAccountSwitches + 1 + len(openAIPassthroughTransportRetryBackoffs)
	if maxAttempts < defaultOpenAIPassthroughAttemptBudget() {
		maxAttempts = defaultOpenAIPassthroughAttemptBudget()
	}
	if existing, ok := c.Get(openAIPassthroughRetryBudgetContextKey); ok {
		if budget, ok := existing.(*openAIPassthroughRetryBudget); ok && budget != nil {
			budget.mu.Lock()
			if maxAttempts > budget.maxAttempts {
				budget.maxAttempts = maxAttempts
			}
			budget.mu.Unlock()
			return
		}
	}
	c.Set(openAIPassthroughRetryBudgetContextKey, &openAIPassthroughRetryBudget{maxAttempts: maxAttempts})
}

func openAIPassthroughBudgetForContext(c *gin.Context) *openAIPassthroughRetryBudget {
	if c != nil {
		if existing, ok := c.Get(openAIPassthroughRetryBudgetContextKey); ok {
			if budget, ok := existing.(*openAIPassthroughRetryBudget); ok && budget != nil {
				return budget
			}
		}
		budget := &openAIPassthroughRetryBudget{maxAttempts: defaultOpenAIPassthroughAttemptBudget()}
		c.Set(openAIPassthroughRetryBudgetContextKey, budget)
		return budget
	}
	return &openAIPassthroughRetryBudget{maxAttempts: defaultOpenAIPassthroughAttemptBudget()}
}

func (b *openAIPassthroughRetryBudget) reserve() (attempt, maxAttempts int, ok bool) {
	if b == nil {
		return 0, 0, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maxAttempts <= 0 {
		b.maxAttempts = defaultOpenAIPassthroughAttemptBudget()
	}
	if b.used >= b.maxAttempts {
		return b.used, b.maxAttempts, false
	}
	b.used++
	return b.used, b.maxAttempts, true
}

func (b *openAIPassthroughRetryBudget) snapshot() (used, maxAttempts int) {
	if b == nil {
		return 0, 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used, b.maxAttempts
}

func newOpenAIPassthroughAttemptBudgetError() *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:        http.StatusBadGateway,
		ResponseBody:      append([]byte(nil), openAITransportFailoverBody...),
		Scope:             GatewayFailureScopeAccount,
		Reason:            openAIPassthroughRetryBudgetReason,
		NextAccountAction: NextAccountStop,
		ClientMessage:     "Upstream request retry budget exhausted",
		// A budget exhaustion is a request boundary, never another account or
		// same-account replay.
		RetryableOnSameAccount: false,
	}
}

func markOpenAIPassthroughNonReplayable(err *UpstreamFailoverError) {
	if err == nil {
		return
	}
	err.RetryableOnSameAccount = false
	err.NextAccountAction = NextAccountStop
	err.Scope = GatewayFailureScopeAccount
	err.Reason = openAIPassthroughNonReplayableReason
}

func normalizeOpenAIPassthroughRetrySafety(err error, replaySafe bool) error {
	if replaySafe || err == nil {
		return err
	}
	var failoverErr *UpstreamFailoverError
	if !errors.As(err, &failoverErr) || failoverErr == nil ||
		failoverErr.Reason != openAIPassthroughTransportRetryReason {
		return err
	}
	markOpenAIPassthroughNonReplayable(failoverErr)
	return failoverErr
}

func openAIPassthroughRequestReplaySafe(c *gin.Context, reqModel string, body, canonicalBody []byte, attemptImageIntentInvalidated bool) bool {
	if gjson.GetBytes(body, "store").Bool() || gjson.GetBytes(canonicalBody, "store").Bool() {
		return false
	}
	// Do not use resolveOpenAIPassthroughImageIntent here: that helper writes
	// the request's canonical image-intent hint, and this retry-safety probe must
	// never alter the routing decision made by the handler.
	if IsExplicitImageGenerationIntent(openAIResponsesEndpoint, reqModel, canonicalBody) ||
		(!attemptImageIntentInvalidated && IsExplicitImageGenerationIntent(openAIResponsesEndpoint, reqModel, body)) {
		return false
	}
	for _, candidate := range [][]byte{canonicalBody, body} {
		if openAIInputContainsNativeImageGeneration(candidate) {
			return false
		}
		if openAIRequestContainsNonReplayableHostedTool(candidate) {
			return false
		}
	}
	if strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) != "" ||
		strings.TrimSpace(gjson.GetBytes(canonicalBody, "previous_response_id").String()) != "" {
		return false
	}
	for _, input := range []gjson.Result{gjson.GetBytes(body, "input"), gjson.GetBytes(canonicalBody, "input")} {
		if !input.Exists() || !input.IsArray() {
			continue
		}
		for _, item := range input.Array() {
			typeName := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
			if strings.HasSuffix(typeName, "_output") || typeName == "function_call_output" {
				return false
			}
		}
	}
	return true
}

func openAIRequestContainsNonReplayableHostedTool(body []byte) bool {
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	for _, tool := range tools.Array() {
		typeName := strings.ToLower(strings.TrimSpace(tool.Get("type").String()))
		switch typeName {
		case "code_interpreter", "computer_use_preview", "file_search", "web_search_preview", "web_search", "mcp":
			return true
		}
	}
	return false
}

func openAIInputContainsNativeImageGeneration(body []byte) bool {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	for _, item := range input.Array() {
		if strings.TrimSpace(item.Get("type").String()) != "additional_tools" {
			continue
		}
		if openAIJSONToolsContainNativeImageGeneration(item.Get("tools")) {
			return true
		}
	}
	return false
}
