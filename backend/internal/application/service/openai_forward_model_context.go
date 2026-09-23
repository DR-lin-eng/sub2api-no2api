package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
	"github.com/gin-gonic/gin"
)

type openAIForwardModelContextKey struct{}

type openAIForwardModel struct {
	model                  string
	useCompactModelMapping bool
}

// WithOpenAIForwardModel records the channel-mapped model that Forward sees.
// The scheduler uses it for both upstream channel restrictions and account
// capability checks. The original client model remains the billing/sticky
// identity; this context only describes the model sent upstream.
func WithOpenAIForwardModel(ctx context.Context, model string, useCompactModelMapping bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIForwardModelContextKey{}, openAIForwardModel{
		model:                  model,
		useCompactModelMapping: useCompactModelMapping,
	})
}

func openAIForwardModelFromContext(ctx context.Context) (openAIForwardModel, bool) {
	if ctx == nil {
		return openAIForwardModel{}, false
	}
	model, ok := ctx.Value(openAIForwardModelContextKey{}).(openAIForwardModel)
	return model, ok
}

func openAIRequestModelForSupport(ctx context.Context, requestedModel string) string {
	if forwardModel, ok := openAIForwardModelFromContext(ctx); ok {
		if model := strings.TrimSpace(forwardModel.model); model != "" {
			return model
		}
	}
	return strings.TrimSpace(requestedModel)
}

func shouldForwardOpenAIResponsesViaRawChatCompletions(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey {
		return false
	}
	if account.IsOpenCodeGo() {
		return false
	}
	if account.IsCNProvider() {
		switch account.GetAPIProtocol() {
		case APIProtocolChatCompletions:
			return true
		case APIProtocolAdaptive:
			return !account.SupportsNativeCNResponses()
		default:
			return false
		}
	}
	return !openai_compat.ShouldUseResponsesAPI(account.Extra)
}

func stampOpenAIResponsesUpstreamEndpoint(c *gin.Context, result *OpenAIForwardResult) {
	const endpoint = "/v1/responses"
	SetActualOpenAIUpstreamEndpoint(c, endpoint)
	if result != nil && strings.TrimSpace(result.UpstreamEndpoint) == "" {
		result.UpstreamEndpoint = endpoint
	}
}
