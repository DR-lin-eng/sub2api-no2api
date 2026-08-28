package service

import "context"

// OpenAIFailureCounterCache tracks consecutive OpenAI OAuth account-level
// 429/502 failures across application instances. A successful response resets it.
type OpenAIFailureCounterCache interface {
	IncrementOpenAIFailureCount(ctx context.Context, accountID int64) (int64, error)
	ResetOpenAIFailureCount(ctx context.Context, accountID int64) error
}
