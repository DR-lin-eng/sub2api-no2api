package service

import (
	"context"
	"fmt"
	"math"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/redeemcode"
	"github.com/Wei-Shaw/sub2api/ent/redeemcodeusage"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/shopspring/decimal"
)

// Payment redemption can include an Activity Center uplift. Refunds reclaim
// the actual committed credit proportionally, while the order's price snapshot
// continues to determine how much cash the gateway returns.
func (s *PaymentService) refundBalanceCredit(ctx context.Context, order *dbent.PaymentOrder, amount float64) (float64, error) {
	if order.OrderType != payment.OrderTypeBalance || order.Amount <= 0 {
		return amount, nil
	}
	usage, err := s.entClient.RedeemCodeUsage.Query().Where(
		redeemcodeusage.UserIDEQ(order.UserID),
		redeemcodeusage.HasRedeemCodeWith(redeemcode.CodeEQ(order.RechargeCode)),
	).First(ctx)
	if dbent.IsNotFound(err) {
		return amount, nil // legacy orders and manually migrated balances
	}
	if err != nil {
		return 0, fmt.Errorf("load actual payment credit: %w", err)
	}
	if usage.Value <= 0 || math.IsNaN(usage.Value) || math.IsInf(usage.Value, 0) {
		return 0, fmt.Errorf("invalid committed payment credit")
	}
	return decimal.NewFromFloat(amount).Mul(decimal.NewFromFloat(usage.Value)).
		Div(decimal.NewFromFloat(order.Amount)).Round(2).InexactFloat64(), nil
}
