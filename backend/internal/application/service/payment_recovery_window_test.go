//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

func newPaymentRecoveryWindowOrder(t *testing.T, ctx context.Context, client *dbent.Client, status string, expiresAt, updatedAt time.Time, paidAt *time.Time) *dbent.PaymentOrder {
	t.Helper()
	user, err := client.User.Create().
		SetEmail("payment-recovery-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.com").
		SetPasswordHash("hash").
		SetUsername("payment-recovery-user").
		Save(ctx)
	require.NoError(t, err)

	b := client.PaymentOrder.Create().
		SetUserID(user.ID).
		SetUserEmail(user.Email).
		SetUserName(user.Username).
		SetAmount(25).
		SetPayAmount(25).
		SetFeeRate(0).
		SetRechargeCode("PAY-RECOVERY-" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetOutTradeNo("sub2_recovery_" + strconv.FormatInt(time.Now().UnixNano(), 10)).
		SetPaymentType(payment.TypeAlipay).
		SetPaymentTradeNo("").
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(status).
		SetExpiresAt(expiresAt).
		SetClientIP("127.0.0.1").
		SetSrcHost("example.com").
		SetUpdatedAt(updatedAt)
	if paidAt != nil {
		b.SetPaidAt(*paidAt)
	}
	order, err := b.Save(ctx)
	require.NoError(t, err)
	return order
}

func TestLatePaymentUsesExpiresAtAfterTradeNoRefresh(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := newPaymentRecoveryWindowOrder(t, ctx, client, OrderStatusExpired, time.Now().Add(-3*time.Hour), time.Now(), nil)

	// Reconciliation writes the upstream trade number and refreshes updated_at.
	_, err := client.PaymentOrder.UpdateOneID(order.ID).
		SetPaymentTradeNo("upstream-trade").
		SetUpdatedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client}
	err = svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID: order.OutTradeNo,
		TradeNo: "upstream-trade",
		Amount:  order.PayAmount,
		Status:  payment.NotificationStatusSuccess,
	}, payment.TypeAlipay)
	require.ErrorIs(t, err, ErrPaymentAfterRecoveryWindow)

	stored, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusExpired, stored.Status)
	require.Nil(t, stored.PaidAt)
	count, err := client.PaymentAuditLog.Query().
		Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("PAYMENT_AFTER_EXPIRY")).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestUnpaidFailedOrderCannotBeRevivedAfterRecoveryWindow(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := newPaymentRecoveryWindowOrder(t, ctx, client, OrderStatusFailed, time.Now().Add(-2*time.Hour), time.Now(), nil)

	svc := &PaymentService{entClient: client}
	err := svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{
		OrderID: order.OutTradeNo, TradeNo: "late-failed-trade", Amount: order.PayAmount,
		Status: payment.NotificationStatusSuccess,
	}, payment.TypeAlipay)
	require.ErrorIs(t, err, ErrPaymentAfterRecoveryWindow)

	stored, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, stored.Status)
	require.Nil(t, stored.PaidAt)
}

func TestAlreadyProcessedDoesNotFulfillUnpaidFailedOrder(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := newPaymentRecoveryWindowOrder(t, ctx, client, OrderStatusFailed, time.Now().Add(-2*time.Hour), time.Now(), nil)
	svc := &PaymentService{entClient: client}
	require.NoError(t, svc.alreadyProcessed(ctx, order))
	stored, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusFailed, stored.Status)
	require.Nil(t, stored.PaidAt)
}

func TestUnpaidFailedOrderRequiresUpstreamConfirmation(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := newPaymentRecoveryWindowOrder(t, ctx, client, OrderStatusFailed, time.Now().Add(-time.Minute), time.Now(), nil)
	calls := 0
	prov := paymentConfirmTestProvider{
		paymentFulfillmentTestProvider: paymentFulfillmentTestProvider{key: payment.TypeAlipay},
		err:                            errors.New("not paid upstream"), calls: &calls,
	}
	svc := &PaymentService{entClient: client}
	err := svc.HandleVerifiedPaymentNotification(ctx, &payment.PaymentNotification{
		OrderID: order.OutTradeNo, TradeNo: "in-window-failed-trade", Amount: order.PayAmount,
		Status: payment.NotificationStatusSuccess,
	}, prov)
	require.ErrorIs(t, err, ErrPaymentUpstreamUnconfirmed)
	require.Equal(t, 1, calls)

	stored, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Nil(t, stored.PaidAt)
	require.Equal(t, OrderStatusFailed, stored.Status)
}

func TestExpirySweepClosesLatePaidPendingOrderWithoutFulfillment(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := newPaymentRecoveryWindowOrder(t, ctx, client, OrderStatusPending, time.Now().Add(-2*time.Hour), time.Now(), nil)
	provider := &paymentOrderLifecycleQueryProvider{
		key: payment.TypeAlipay,
		resp: &payment.QueryOrderResponse{
			TradeNo: order.OutTradeNo,
			Status:  payment.ProviderStatusPaid,
			Amount:  order.PayAmount,
		},
	}
	registry := payment.NewRegistry()
	registry.Register(provider)
	svc := &PaymentService{entClient: client, registry: registry, providersLoaded: true}

	expired, err := svc.ExpireTimedOutOrders(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, expired)
	stored, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusExpired, stored.Status)
	require.Nil(t, stored.PaidAt)
}
