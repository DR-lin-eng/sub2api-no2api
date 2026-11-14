package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
)

// ErrPaymentAfterRecoveryWindow is returned for a cryptographically valid
// payment that arrived after the order's recovery deadline. Transports should
// acknowledge the provider callback, while the order remains unpaid and the
// audit trail records the event for manual reconciliation.
var ErrPaymentAfterRecoveryWindow = errors.New("payment arrived after recovery window")

// checkPaidResultRecoveryWindowClosed tells expiry/cancellation sweeps that a
// provider reported a late payment, so they can still close the unpaid order.
const checkPaidResultRecoveryWindowClosed = "recovery_window_closed"

func paymentOrderRequiresInitialConfirmation(order *dbent.PaymentOrder) bool {
	if order == nil || order.PaidAt != nil {
		return false
	}
	switch order.Status {
	case OrderStatusPending, OrderStatusExpired, OrderStatusCancelled, OrderStatusFailed:
		return true
	default:
		return false
	}
}

func paymentOrderWithinRecoveryWindow(order *dbent.PaymentOrder, now time.Time) bool {
	if order == nil || order.ExpiresAt.IsZero() {
		return false
	}
	return !now.After(order.ExpiresAt.Add(paymentRecoveryWindow))
}

func paymentRecoveryDeadline(order *dbent.PaymentOrder) time.Time {
	if order == nil || order.ExpiresAt.IsZero() {
		return time.Time{}
	}
	return order.ExpiresAt.Add(paymentRecoveryWindow)
}

func paymentLateRecoveryAuditAction(status string) string {
	if status == OrderStatusCancelled {
		return "PAYMENT_AFTER_CANCEL"
	}
	return "PAYMENT_AFTER_EXPIRY"
}

func (s *PaymentService) rejectLatePayment(ctx context.Context, order *dbent.PaymentOrder, n *payment.PaymentNotification, providerKey string, now time.Time) error {
	detail := map[string]any{
		"status":      order.Status,
		"expiresAt":   order.ExpiresAt,
		"recoveryEnd": paymentRecoveryDeadline(order),
		"updatedAt":   order.UpdatedAt,
		"reason":      "payment arrived after order recovery window",
	}
	if n != nil {
		detail["tradeNo"] = n.TradeNo
		detail["paidAmount"] = n.Amount
	}
	if providerKey != "" {
		detail["provider"] = providerKey
	}
	if !now.IsZero() {
		detail["observedAt"] = now
	}
	operator := providerKey
	if operator == "" {
		operator = "system"
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal late payment audit: %w", err)
	}
	err = s.entClient.PaymentAuditLog.Create().
		SetOrderID(strconv.FormatInt(order.ID, 10)).SetAction(paymentLateRecoveryAuditAction(order.Status)).
		SetOperator(operator).SetDetail(string(raw)).OnConflict().DoNothing().Exec(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		// Ent's INSERT ... RETURNING reports no rows on DO NOTHING. Only
		// acknowledge it when this order's rejection evidence already exists.
		exists, lookupErr := s.entClient.PaymentAuditLog.Query().Where(
			paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
			paymentauditlog.ActionEQ(paymentLateRecoveryAuditAction(order.Status)),
		).Exist(ctx)
		if lookupErr != nil {
			return fmt.Errorf("load late payment audit: %w", lookupErr)
		}
		if exists {
			return ErrPaymentAfterRecoveryWindow
		}
	}
	if err != nil {
		// Do not acknowledge a rejected payment until its reconciliation
		// evidence is durable; the provider must be able to retry this event.
		return fmt.Errorf("record late payment audit: %w", err)
	}
	return ErrPaymentAfterRecoveryWindow
}
