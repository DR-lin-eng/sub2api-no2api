package service

import (
	"context"
	"time"
)

// VideoBalanceHoldRepository owns each hold's funds, terminal state and billing
// effects in a single transaction. It is optional on the ordinary billing port;
// unrelated gateways and test doubles do not need video persistence methods.
type VideoBalanceHoldRepository interface {
	ReserveVideoBalance(context.Context, *BalanceHoldRecord) (*BalanceHoldRecord, error)
	CaptureVideoBalance(context.Context, *VideoBalanceCapture) (*VideoBalanceCaptureResult, error)
	ReleaseVideoBalance(context.Context, *BalanceHoldRecord) (*BalanceHoldRecord, error)
	BindBalanceHoldUpstreamRequest(context.Context, string, string, int64) error
	GetBalanceHoldByUpstreamRequestID(context.Context, string) (*BalanceHoldRecord, error)
	ListExpiredBalanceHolds(context.Context, time.Time, int) ([]*BalanceHoldRecord, error)
}

type VideoBalanceCapture struct {
	HoldKey  string
	Billing  *UsageBillingCommand
	UsageLog *UsageLog
}

type VideoBalanceCaptureResult struct {
	Record  *BalanceHoldRecord
	Billing *UsageBillingApplyResult
}

func (s *OpenAIGatewayService) videoBalanceRepository() VideoBalanceHoldRepository {
	if s == nil {
		return nil
	}
	repo, _ := s.usageBillingRepo.(VideoBalanceHoldRepository)
	return repo
}
