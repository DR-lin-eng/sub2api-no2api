package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const multimodalVideoOwnerLookupTimeout = 2 * time.Second

// ResolveMultimodalVideoTaskOwner resolves only a task already owned by this
// authenticated key and group. Composite lookups have no model to route by.
// A persisted hold is the authority in balance mode; subscription/simple modes
// retain the owner-scoped sticky binding used when the task was submitted.
func (s *OpenAIGatewayService) ResolveMultimodalVideoTaskOwner(ctx context.Context, apiKey *APIKey, requestID string) (int64, error) {
	requestID = strings.TrimSpace(requestID)
	if s == nil || apiKey == nil || apiKey.UserID <= 0 || apiKey.ID <= 0 ||
		apiKey.GroupID == nil || apiKey.Group == nil || apiKey.Group.ID != *apiKey.GroupID ||
		requestID == "" || len(requestID) > 255 {
		return 0, nil
	}
	if apiKey.Group.Platform != PlatformComposite && apiKey.Group.Platform != PlatformMultimodal {
		return 0, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, multimodalVideoOwnerLookupTimeout)
	defer cancel()
	if s.MultimodalVideoBalanceHoldEnabled(apiKey) {
		record, err := s.lookupMultimodalVideoBalance(lookupCtx, requestID)
		if err != nil {
			return 0, err
		}
		if record == nil || record.UserID != apiKey.UserID || record.APIKeyID != apiKey.ID ||
			record.GroupID != *apiKey.GroupID || record.AccountID <= 0 ||
			(record.Status != BalanceHoldStatusPending && record.Status != BalanceHoldStatusSettled) {
			return 0, nil
		}
		return record.AccountID, nil
	}
	accountID, err := s.ResolveMultimodalMediaVideoRequestAccount(lookupCtx, apiKey.GroupID, requestID, apiKey.UserID, apiKey.ID)
	if errors.Is(err, ErrStickySessionNotFound) {
		return 0, nil
	}
	return accountID, err
}

// Composite subscription/simple requests have no PostgreSQL hold. Their
// owner-scoped cache binding must succeed before returning an accepted task.
func (s *OpenAIGatewayService) bindCompositeMultimodalVideoOwner(ctx context.Context, c *gin.Context, account *Account, responseBody []byte) error {
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil || apiKey.Group == nil || apiKey.Group.Platform != PlatformComposite {
		return nil
	}
	requestID := extractGrokMediaVideoRequestID(responseBody)
	if apiKey.GroupID == nil || account == nil || requestID == "" {
		return ErrBalanceHoldFailed.WithCause(fmt.Errorf("composite multimodal video response has no recoverable owner"))
	}
	if err := s.BindMultimodalMediaVideoRequestAccount(ctx, apiKey.GroupID, requestID, apiKey.UserID, apiKey.ID, account.ID); err != nil {
		return ErrBalanceHoldFailed.WithCause(err)
	}
	return nil
}
