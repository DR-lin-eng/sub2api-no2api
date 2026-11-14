package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
)

// Creation can finish after a callback has accepted payment. Only the unpaid
// pending order may be changed; provider identity was pinned before the call.
func (s *PaymentService) savePaymentCreationDetails(ctx context.Context, orderID int64, response *payment.CreatePaymentResponse) error {
	_, err := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(orderID), paymentorder.StatusEQ(OrderStatusPending), paymentorder.PaidAtIsNil()).
		SetNillablePaymentTradeNo(psNilIfEmpty(response.TradeNo)).
		SetNillablePayURL(psNilIfEmpty(response.PayURL)).
		SetNillableQrCode(psNilIfEmpty(response.QRCode)).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update order with payment details: %w", err)
	}
	return nil
}

func (s *PaymentService) markPaymentCreationFailed(ctx context.Context, orderID int64) {
	_, err := s.entClient.PaymentOrder.Update().
		Where(paymentorder.IDEQ(orderID), paymentorder.StatusEQ(OrderStatusPending), paymentorder.PaidAtIsNil()).
		SetStatus(OrderStatusFailed).
		Save(ctx)
	if err != nil {
		slog.Error("mark payment creation failed", "orderID", orderID, "error", err)
	}
}
