package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

// Claim and balance deduction commit together. A concurrent spend after refund
// preparation must abort a normal refund without leaving a partial deduction.
// Explicit force refunds retain the existing available-balance behavior.
func (s *PaymentService) claimRefund(ctx context.Context, plan *RefundPlan) error {
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin refund claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	txCtx := dbent.NewTxContext(ctx, tx)
	claimed, err := tx.PaymentOrder.Update().
		Where(paymentorder.IDEQ(plan.OrderID), paymentorder.StatusIn(OrderStatusCompleted, OrderStatusRefundRequested, OrderStatusRefundFailed)).
		SetStatus(OrderStatusRefunding).
		Save(txCtx)
	if err != nil {
		return fmt.Errorf("claim refund: %w", err)
	}
	if claimed == 0 {
		return infraerrors.Conflict("CONFLICT", "order status changed")
	}
	if plan.DeductionType == payment.DeductionTypeBalance && plan.BalanceToDeduct > 0 {
		held, err := tx.PaymentAuditLog.Query().Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(plan.OrderID, 10)),
			paymentauditlog.ActionEQ("REFUND_ROLLBACK_FAILED"),
		).Exist(txCtx)
		if err != nil {
			return fmt.Errorf("load refund deduction recovery: %w", err)
		}
		if !held {
			deducted, err := s.deductAvailableBalance(txCtx, plan.Order.UserID, plan.BalanceToDeduct)
			if err != nil {
				return fmt.Errorf("deduction: %w", err)
			}
			if !plan.Force && deducted < plan.BalanceToDeduct {
				return infraerrors.BadRequest("BALANCE_NOT_ENOUGH", "refund amount exceeds available balance")
			}
			plan.BalanceToDeduct = deducted
		} else {
			// Preserve the existing recovery of a deduction that could not be
			// returned by an earlier failed refund attempt.
			plan.BalanceToDeduct = 0
		}
	}
	if plan.DeductionType == payment.DeductionTypeSubscription && plan.SubDaysToDeduct > 0 && plan.SubscriptionID > 0 {
		if !s.hasAuditLog(txCtx, plan.OrderID, "REFUND_ROLLBACK_FAILED") {
			_, err := s.subscriptionSvc.ExtendSubscription(txCtx, plan.SubscriptionID, -plan.SubDaysToDeduct)
			if err != nil {
				if errors.Is(err, ErrAdjustWouldExpire) {
					// Deduction would expire the subscription — revoke it entirely
					slog.Info("subscription deduction would expire, revoking", "orderID", plan.OrderID, "subID", plan.SubscriptionID, "days", plan.SubDaysToDeduct)
					if revokeErr := s.subscriptionSvc.RevokeSubscription(txCtx, plan.SubscriptionID); revokeErr != nil {
						return fmt.Errorf("revoke subscription: %w", revokeErr)
					}
					plan.subscriptionRevoked = true
				} else {
					// Other errors (DB failure, not found) — abort refund
					return fmt.Errorf("deduct subscription days: %w", err)
				}
			}
		} else {
			slog.Warn("skipping subscription deduction on retry (previous rollback failed)", "orderID", plan.OrderID)
			plan.SubDaysToDeduct = 0
		}
	}
	if err := createPaymentAuditLogTx(txCtx, tx.Client(), plan.OrderID, "REFUND_PREPARED", map[string]any{
		"refundAmount": plan.RefundAmount, "gatewayAmount": plan.GatewayAmount,
		"deductionType": plan.DeductionType, "balanceDeducted": plan.BalanceToDeduct,
		"subDaysDeducted": plan.SubDaysToDeduct, "subscriptionID": plan.SubscriptionID,
		"subscriptionRevoked": plan.subscriptionRevoked,
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit refund claim: %w", err)
	}
	if plan.DeductionType == payment.DeductionTypeBalance && plan.BalanceToDeduct > 0 {
		s.invalidateRefundBalanceCache(ctx, plan.Order.UserID)
	}
	if plan.DeductionType == payment.DeductionTypeSubscription && plan.Order.SubscriptionGroupID != nil {
		// Repeat invalidation after commit: another node may have repopulated
		// its cache from the pre-commit subscription snapshot.
		if err := s.subscriptionSvc.invalidateSubscriptionCaches(plan.Order.UserID, *plan.Order.SubscriptionGroupID); err != nil {
			slog.Warn("invalidate committed refund subscription", "orderID", plan.OrderID, "error", err)
		}
	}
	return nil
}
