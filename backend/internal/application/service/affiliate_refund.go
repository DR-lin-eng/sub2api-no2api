package service

import (
	"context"
	"errors"
)

// AffiliateRefundRepository reverses only awards durably linked to the source
// payment, inside the payment refund transaction supplied in ctx.
type AffiliateRefundRepository interface {
	ReverseOrderQuota(ctx context.Context, orderID int64, refundAmount, orderAmount float64) (float64, error)
}

func (s *AffiliateService) ReversePaymentRebate(ctx context.Context, orderID int64, refundAmount, orderAmount float64) (float64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	reverser, ok := s.repo.(AffiliateRefundRepository)
	if !ok {
		return 0, errors.New("affiliate repository does not support refund reversal")
	}
	// Revocation is independent of the current feature switch or rebate rate.
	return reverser.ReverseOrderQuota(ctx, orderID, refundAmount, orderAmount)
}
