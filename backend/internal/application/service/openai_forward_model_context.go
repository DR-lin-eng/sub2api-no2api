package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
)

type openAIForwardModelContextKey struct{}

type openAIForwardModel struct {
	model                  string
	useCompactModelMapping bool
}

// WithOpenAIForwardModel records the channel-mapped model that Forward sees.
// The scheduler uses it only for upstream channel restriction checks; normal
// account capability and model matching continue to use the requested model.
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

func shouldForwardOpenAIResponsesViaRawChatCompletions(account *Account) bool {
	return account != nil &&
		account.Type == AccountTypeAPIKey &&
		!openai_compat.ShouldUseResponsesAPI(account.Extra)
}
