//go:build unit

package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

type paymentReentrantQueryProvider struct {
	paymentOrderLifecycleQueryProvider
	query func() *payment.QueryOrderResponse
}

func (p *paymentReentrantQueryProvider) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return p.query(), nil
}

func TestPaymentQueryDoesNotInvalidateConcurrentFulfillmentLease(t *testing.T) {
	ctx := t.Context()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
	leaseTime := time.Now().UTC().Truncate(time.Microsecond)
	prov := &paymentReentrantQueryProvider{query: func() *payment.QueryOrderResponse {
		// A callback accepts payment and acquires the lease while this query
		// still holds its earlier PENDING snapshot.
		_, err := client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRecharging).
			SetPaidAt(leaseTime).SetUpdatedAt(leaseTime).SetPaymentTradeNo("accepted-trade").Save(ctx)
		require.NoError(t, err)
		return &payment.QueryOrderResponse{Status: payment.ProviderStatusPaid, TradeNo: "accepted-trade", Amount: order.PayAmount}
	}}
	registry := payment.NewRegistry()
	registry.Register(prov)
	svc := &PaymentService{entClient: client, registry: registry, providersLoaded: true}
	_, err := svc.VerifyOrderByOutTradeNo(ctx, order.OutTradeNo, order.UserID)
	require.NoError(t, err)
	stored, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusRecharging, stored.Status)
	require.Equal(t, "accepted-trade", stored.PaymentTradeNo)
	require.True(t, stored.UpdatedAt.Equal(leaseTime), "a stale query must leave the active fulfillment lease untouched")
}

func TestPaymentCreationWritesCannotOverwriteAcceptedPayment(t *testing.T) {
	for _, status := range []string{OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted} {
		t.Run(status, func(t *testing.T) {
			ctx := t.Context()
			client := newPaymentConfigServiceTestClient(t)
			order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, status, time.Now().UTC().Truncate(time.Microsecond))
			svc := &PaymentService{entClient: client}
			require.NoError(t, svc.savePaymentCreationDetails(ctx, order.ID, &payment.CreatePaymentResponse{TradeNo: order.OutTradeNo, PayURL: "https://fixture.invalid/pay"}))
			svc.markPaymentCreationFailed(ctx, order.ID)
			stored, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, status, stored.Status)
			require.Equal(t, order.PaymentTradeNo, stored.PaymentTradeNo)
			require.True(t, stored.UpdatedAt.Equal(order.UpdatedAt))
		})
	}
}

func TestPaymentQueryEmptyResponseKeepsOrderPending(t *testing.T) {
	ctx := t.Context()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
	prov := &paymentReentrantQueryProvider{query: func() *payment.QueryOrderResponse { return nil }}
	registry := payment.NewRegistry()
	registry.Register(prov)
	svc := &PaymentService{entClient: client, registry: registry, providersLoaded: true}
	stored, err := svc.VerifyOrderByOutTradeNo(ctx, order.OutTradeNo, order.UserID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, stored.Status)
	require.Nil(t, stored.PaidAt)
}

func TestPaymentFulfillmentRequiresAcceptedPayment(t *testing.T) {
	for _, status := range []string{OrderStatusPaid, OrderStatusFailed, OrderStatusRecharging} {
		t.Run(status, func(t *testing.T) {
			ctx := t.Context()
			client := newPaymentConfigServiceTestClient(t)
			order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, status, time.Now())
			_, err := client.PaymentOrder.UpdateOneID(order.ID).ClearPaidAt().Save(ctx)
			require.NoError(t, err)
			svc := &PaymentService{entClient: client}
			require.ErrorContains(t, svc.ExecuteSubscriptionFulfillment(ctx, order.ID), "order is not paid")
			_, err = client.PaymentOrder.UpdateOneID(order.ID).SetOrderType(payment.OrderTypeBalance).Save(ctx)
			require.NoError(t, err)
			require.ErrorContains(t, svc.ExecuteBalanceFulfillment(ctx, order.ID), "order is not paid")
		})
	}
}

func TestLegacyPaymentIDCannotSettleModernOrder(t *testing.T) {
	ctx := t.Context()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
	svc := &PaymentService{entClient: client}
	err := svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID: "sub2_" + strconv.FormatInt(order.ID, 10), TradeNo: "foreign-trade", Amount: order.PayAmount, Status: payment.NotificationStatusSuccess,
	}, payment.TypeAlipay)
	require.ErrorIs(t, err, ErrOrderNotFound)
	stored, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, stored.Status)
	require.Nil(t, stored.PaidAt)
}
