package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/Wei-Shaw/sub2api/internal/shared/servertiming"
)

// ErrOrderNotFound is returned by HandlePaymentNotification when the webhook
// references an out_trade_no that does not exist in our DB. Callers (webhook
// handlers) should treat this as a terminal, non-retryable condition and still
// respond with a 2xx success to the provider — otherwise the provider will keep
// retrying forever (e.g. when a foreign environment's webhook endpoint is
// misconfigured to point at us, or when our orders table has been wiped).
var ErrOrderNotFound = errors.New("payment order not found")

// ErrPaymentUpstreamUnconfirmed asks webhook transports to return a retryable 502.
var ErrPaymentUpstreamUnconfirmed = errors.New("payment upstream confirmation failed")

func (s *PaymentService) HandlePaymentNotification(ctx context.Context, n *payment.PaymentNotification, pk string) error {
	return s.handlePaymentNotification(ctx, n, pk, nil)
}

// HandleVerifiedPaymentNotification binds upstream confirmation to the exact
// instance that verified the callback. Reconciliation keeps its existing entry
// point because its notification already comes from an authoritative query.
func (s *PaymentService) HandleVerifiedPaymentNotification(ctx context.Context, n *payment.PaymentNotification, prov payment.Provider) error {
	if prov == nil {
		return errors.New("verified payment provider is missing")
	}
	return s.handlePaymentNotification(ctx, n, prov.ProviderKey(), prov)
}

func (s *PaymentService) handlePaymentNotification(ctx context.Context, n *payment.PaymentNotification, pk string, prov payment.Provider) error {
	if n == nil || n.Status != payment.NotificationStatusSuccess {
		return nil
	}
	// Look up order by out_trade_no (the external order ID we sent to the provider)
	order, err := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(n.OrderID)).Only(ctx)
	if err != nil {
		// Fallback only for true legacy "sub2_N" DB-ID payloads when the
		// current out_trade_no lookup genuinely did not find an order.
		if oid, ok := parseLegacyPaymentOrderID(n.OrderID, err); ok {
			legacy, lookupErr := s.entClient.PaymentOrder.Get(ctx, oid)
			if lookupErr != nil || strings.TrimSpace(legacy.OutTradeNo) != "" {
				if lookupErr != nil && !dbent.IsNotFound(lookupErr) {
					return fmt.Errorf("load legacy payment order: %w", lookupErr)
				}
				return fmt.Errorf("%w: legacy order reference does not match", ErrOrderNotFound)
			}
			if prov == nil {
				return s.confirmPayment(ctx, oid, n.TradeNo, n.Amount, pk, n.Metadata)
			}
			return s.confirmPaymentWithProvider(ctx, oid, n, pk, prov)
		}
		if dbent.IsNotFound(err) {
			return fmt.Errorf("%w: out_trade_no=%s", ErrOrderNotFound, n.OrderID)
		}
		return fmt.Errorf("lookup order failed for out_trade_no %s: %w", n.OrderID, err)
	}
	if prov == nil {
		return s.confirmPayment(ctx, order.ID, n.TradeNo, n.Amount, pk, n.Metadata)
	}
	return s.confirmPaymentWithProvider(ctx, order.ID, n, pk, prov)
}

// ConfirmNotificationUpstream asks providers that implement
// payment.NotificationConfirmer to confirm a verified success notification
// against the upstream order, using the same instance that verified it.
// Failures are recorded on the order's audit trail and returned so the webhook
// answers non-success and the gateway retries.
func (s *PaymentService) ConfirmNotificationUpstream(ctx context.Context, prov payment.Provider, n *payment.PaymentNotification) error {
	confirmer, ok := prov.(payment.NotificationConfirmer)
	if !ok || n == nil || n.Status != payment.NotificationStatusSuccess {
		return nil
	}
	notifiedTradeNo := n.TradeNo
	finishProviderCall := servertiming.ObserveDependency(ctx, "payment")
	err := confirmer.ConfirmNotification(ctx, n)
	finishProviderCall()
	if err == nil {
		return nil
	}
	if order, lookupErr := s.entClient.PaymentOrder.Query().Where(paymentorder.OutTradeNo(n.OrderID)).Only(ctx); lookupErr == nil {
		s.writeAuditLog(ctx, order.ID, "PAYMENT_UPSTREAM_UNCONFIRMED", prov.ProviderKey(), map[string]any{
			"detail":  err.Error(),
			"tradeNo": notifiedTradeNo,
			"amount":  n.Amount,
		})
	}
	return err
}

func parseLegacyPaymentOrderID(orderID string, lookupErr error) (int64, bool) {
	if !dbent.IsNotFound(lookupErr) {
		return 0, false
	}
	orderID = strings.TrimSpace(orderID)
	if !strings.HasPrefix(orderID, orderIDPrefix) {
		return 0, false
	}
	trimmed := strings.TrimPrefix(orderID, orderIDPrefix)
	if trimmed == "" || trimmed == orderID {
		return 0, false
	}
	oid, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || oid <= 0 {
		return 0, false
	}
	return oid, true
}

func (s *PaymentService) confirmPayment(ctx context.Context, oid int64, tradeNo string, paid float64, pk string, metadata map[string]string) error {
	return s.confirmPaymentWithProvider(ctx, oid, &payment.PaymentNotification{
		TradeNo: tradeNo, Amount: paid, Metadata: metadata, Status: payment.NotificationStatusSuccess,
	}, pk, nil)
}

func (s *PaymentService) confirmPaymentWithProvider(ctx context.Context, oid int64, n *payment.PaymentNotification, pk string, prov payment.Provider) error {
	tradeNo, paid, metadata := n.TradeNo, n.Amount, n.Metadata
	o, err := s.entClient.PaymentOrder.Get(ctx, oid)
	if err != nil {
		if dbent.IsNotFound(err) {
			return fmt.Errorf("%w: id=%d", ErrOrderNotFound, oid)
		}
		return fmt.Errorf("load payment order: %w", err)
	}
	instanceProviderKey := ""
	inst, instErr := s.getOrderProviderInstance(ctx, o)
	if instErr != nil && psHasPinnedProviderInstance(o) {
		return fmt.Errorf("load pinned payment provider: %w", instErr)
	}
	if instErr == nil && inst != nil {
		instanceProviderKey = inst.ProviderKey
	}
	expectedProviderKey := expectedNotificationProviderKeyForOrder(s.registry, o, instanceProviderKey)
	if expectedProviderKey != "" && strings.TrimSpace(pk) != "" && !strings.EqualFold(expectedProviderKey, strings.TrimSpace(pk)) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_PROVIDER_MISMATCH", pk, map[string]any{
			"expectedProvider": expectedProviderKey,
			"actualProvider":   pk,
			"tradeNo":          tradeNo,
		})
		return fmt.Errorf("provider mismatch: expected %s, got %s", expectedProviderKey, pk)
	}
	if psOrderProviderSnapshot(o) != nil && len(metadata) == 0 {
		return errors.New("payment notification is missing merchant metadata")
	}
	if err := validateProviderNotificationMetadata(o, pk, metadata); err != nil {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_PROVIDER_METADATA_MISMATCH", pk, map[string]any{
			"detail":  err.Error(),
			"tradeNo": tradeNo,
		})
		return err
	}
	if strings.EqualFold(strings.TrimSpace(pk), payment.TypeEasyPay) && strings.TrimSpace(tradeNo) == "" &&
		strings.TrimSpace(metadata["notification_source"]) != "upstream_query" {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_MISSING_TRADE_NO", pk, map[string]any{
			"paidAmount": paid,
		})
		return fmt.Errorf("easypay notification missing trade number")
	}
	if !isValidProviderAmount(paid) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_INVALID_AMOUNT", pk, map[string]any{
			"expected": o.PayAmount,
			"paid":     paid,
			"tradeNo":  tradeNo,
		})
		return fmt.Errorf("invalid paid amount from provider: %v", paid)
	}
	if math.Abs(paid-o.PayAmount) > paymentAmountToleranceForCurrency(PaymentOrderCurrency(o)) {
		s.writeAuditLog(ctx, o.ID, "PAYMENT_AMOUNT_MISMATCH", pk, map[string]any{"expected": o.PayAmount, "paid": paid, "tradeNo": tradeNo})
		return fmt.Errorf("amount mismatch: expected %s, got %s", strconv.FormatFloat(o.PayAmount, 'f', -1, 64), strconv.FormatFloat(paid, 'f', -1, 64))
	}
	if paymentOrderRequiresInitialConfirmation(o) && !paymentOrderWithinRecoveryWindow(o, time.Now()) {
		return s.rejectLatePayment(ctx, o, n, pk, time.Now())
	}
	if !paymentOrderRequiresInitialConfirmation(o) {
		return s.alreadyProcessed(ctx, o)
	}
	if prov != nil {
		if err := s.ConfirmNotificationUpstream(ctx, prov, n); err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentUpstreamUnconfirmed, err)
		}
		tradeNo = n.TradeNo
	}
	return s.toPaid(ctx, o, tradeNo, paid, pk)
}

func paymentAmountToleranceForCurrency(currency string) float64 {
	minorUnit := payment.CurrencyMinorUnit(currency)
	if minorUnit <= 2 {
		return amountToleranceCNY
	}
	return math.Pow10(-minorUnit) / 2
}

func isValidProviderAmount(amount float64) bool {
	return amount > 0 && !math.IsNaN(amount) && !math.IsInf(amount, 0)
}

func validateProviderNotificationMetadata(order *dbent.PaymentOrder, providerKey string, metadata map[string]string) error {
	return validateProviderSnapshotMetadata(order, providerKey, metadata)
}

func expectedNotificationProviderKey(registry *payment.Registry, orderPaymentType string, orderProviderKey string, instanceProviderKey string) string {
	if key := strings.TrimSpace(instanceProviderKey); key != "" {
		return key
	}
	if key := strings.TrimSpace(orderProviderKey); key != "" {
		return key
	}
	if registry != nil {
		if key := strings.TrimSpace(registry.GetProviderKey(payment.PaymentType(orderPaymentType))); key != "" {
			return key
		}
	}
	return strings.TrimSpace(orderPaymentType)
}

func (s *PaymentService) toPaid(ctx context.Context, o *dbent.PaymentOrder, tradeNo string, paid float64, pk string) error {
	previousStatus := o.Status
	now := time.Now()
	if paymentOrderRequiresInitialConfirmation(o) && !paymentOrderWithinRecoveryWindow(o, now) {
		return s.rejectLatePayment(ctx, o, &payment.PaymentNotification{TradeNo: tradeNo, Amount: paid}, pk, now)
	}
	c, err := s.entClient.PaymentOrder.Update().Where(
		paymentorder.IDEQ(o.ID),
		paymentorder.PaidAtIsNil(),
		paymentorder.ExpiresAtGTE(now.Add(-paymentRecoveryWindow)),
		paymentorder.StatusIn(OrderStatusPending, OrderStatusCancelled, OrderStatusExpired, OrderStatusFailed),
	).SetStatus(OrderStatusPaid).SetPayAmount(paid).SetPaymentTradeNo(tradeNo).SetPaidAt(now).ClearFailedAt().ClearFailedReason().Save(ctx)
	if err != nil {
		return fmt.Errorf("update to PAID: %w", err)
	}
	if c == 0 {
		current, getErr := s.entClient.PaymentOrder.Get(ctx, o.ID)
		if getErr != nil {
			return fmt.Errorf("reload payment transition: %w", getErr)
		}
		if paymentOrderRequiresInitialConfirmation(current) {
			if !paymentOrderWithinRecoveryWindow(current, time.Now()) {
				return s.rejectLatePayment(ctx, current, &payment.PaymentNotification{TradeNo: tradeNo, Amount: paid}, pk, time.Now())
			}
			return nil
		}
		return s.alreadyProcessed(ctx, current)
	}
	if previousStatus == OrderStatusCancelled || previousStatus == OrderStatusExpired {
		slog.Info("order recovered from webhook payment success",
			"orderID", o.ID,
			"previousStatus", previousStatus,
			"tradeNo", tradeNo,
			"provider", pk,
		)
		s.writeAuditLog(ctx, o.ID, "ORDER_RECOVERED", pk, map[string]any{
			"previous_status": previousStatus,
			"tradeNo":         tradeNo,
			"paidAmount":      paid,
			"reason":          "webhook payment success received after order " + previousStatus,
		})
	}
	s.writeAuditLog(ctx, o.ID, "ORDER_PAID", pk, map[string]any{"tradeNo": tradeNo, "paidAmount": paid})
	return s.executeFulfillment(ctx, o.ID)
}

func (s *PaymentService) alreadyProcessed(ctx context.Context, o *dbent.PaymentOrder) error {
	cur, err := s.entClient.PaymentOrder.Get(ctx, o.ID)
	if err != nil {
		return fmt.Errorf("reload processed payment order: %w", err)
	}
	switch cur.Status {
	case OrderStatusCompleted, OrderStatusRefunded:
		return nil
	case OrderStatusFailed, OrderStatusPaid, OrderStatusRecharging:
		if cur.PaidAt == nil {
			return nil
		}
		return s.executeFulfillment(ctx, o.ID)
	default:
		return nil
	}
}
