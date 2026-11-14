//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

func TestPaymentHardProviderBudgetsPostgres(t *testing.T) {
	ctx := t.Context()
	f := newPaymentRecoveryFixture(t)
	_, err := f.client.PaymentProviderInstance.Update().SetEnabled(false).Save(ctx)
	require.NoError(t, err)
	_, err = f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	settings := NewSettingRepository(f.client)
	require.NoError(t, settings.SetMultiple(ctx, map[string]string{
		service.SettingPaymentEnabled: "true", service.SettingEnabledPaymentTypes: "alipay", service.SettingPaymentVisibleMethodAlipayEnabled: "true",
		service.SettingPaymentVisibleMethodAlipaySource: service.VisibleMethodSourceEasyPayAlipay,
		service.SettingDailyRechargeLimit:               "0", service.SettingMaxPendingOrders: "20",
	}))
	users := NewUserRepository(f.client, integrationDB)
	key := make([]byte, payment.AES256KeySize)
	svc := service.NewPaymentService(f.client, payment.NewRegistry(), payment.NewDefaultLoadBalancer(f.client, key), nil, nil, service.NewPaymentConfigService(f.client, settings, key), users, nil, nil)
	newUser := func() int64 {
		user, err := f.client.User.Create().SetEmail(fmt.Sprintf("merchant-budget-%d@fixture.invalid", time.Now().UnixNano())).SetPasswordHash("fixture").Save(ctx)
		require.NoError(t, err)
		return user.ID
	}
	request := func(user int64, amount float64) error {
		_, err := svc.CreateOrder(ctx, service.CreateOrderRequest{UserID: user, Amount: amount, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance, ClientIP: "127.0.0.1", SrcHost: "fixture.invalid"})
		return err
	}
	t.Run("limits are hard and exact upper bound is valid", func(t *testing.T) {
		_, err := f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetLimits(`{"alipay":{"singleMin":20,"singleMax":25}}`).Save(ctx)
		require.NoError(t, err)
		before := f.createCalls.Load()
		require.Error(t, request(newUser(), 19))
		require.Error(t, request(newUser(), 26))
		require.Equal(t, before, f.createCalls.Load(), "rejected quotations never reach the merchant")
		require.NoError(t, request(newUser(), 25))
	})
	t.Run("malformed limits and budget read errors fail closed", func(t *testing.T) {
		_, err := f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetLimits("not-json").Save(ctx)
		require.NoError(t, err)
		before := f.createCalls.Load()
		require.Error(t, request(newUser(), 25))
		require.Equal(t, before, f.createCalls.Load())
		_, err = f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetLimits(`{"alipay":{"dailyLimit":1000}}`).Save(ctx)
		require.NoError(t, err)
		var fail atomic.Bool
		f.client.PaymentOrder.Intercept(dbent.InterceptFunc(func(next dbent.Querier) dbent.Querier {
			return dbent.QuerierFunc(func(ctx context.Context, q dbent.Query) (dbent.Value, error) {
				if fail.Load() {
					return nil, errors.New("injected merchant budget read failure")
				}
				return next.Query(ctx, q)
			})
		}))
		fail.Store(true)
		require.Error(t, request(newUser(), 25))
		fail.Store(false)
		require.Equal(t, before, f.createCalls.Load())
	})
	t.Run("different users cannot overreserve one merchant", func(t *testing.T) {
		// The prior quotation reserved25; total75 leaves exactly two more25.
		_, err := f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetLimits(`{"alipay":{"dailyLimit":75}}`).Save(ctx)
		require.NoError(t, err)
		ids := make([]int64, 8)
		for i := range ids {
			ids[i] = newUser()
		}
		before := f.createCalls.Load()
		results := make(chan error, 8)
		var workers sync.WaitGroup
		for _, id := range ids {
			workers.Go(func() { results <- request(id, 25) })
		}
		workers.Wait()
		accepted := 0
		for range ids {
			if <-results == nil {
				accepted++
			}
		}
		require.Equal(t, 2, accepted)
		require.Equal(t, before+2, f.createCalls.Load())
		usage, err := payment.ProviderDailyUsage(ctx, f.client, []string{strconv.FormatInt(f.instanceID, 10)}, time.Now())
		require.NoError(t, err)
		require.Equal(t, 75.0, usage[strconv.FormatInt(f.instanceID, 10)])
	})
}

func TestPaymentProviderBudgetLifecyclePostgres(t *testing.T) {
	ctx := t.Context()
	f := newPaymentRecoveryFixture(t)
	id := strconv.FormatInt(f.instanceID, 10)
	for _, status := range []string{service.OrderStatusPending, service.OrderStatusExpired, service.OrderStatusCancelled, service.OrderStatusFailed} {
		order := f.order(t, status, payment.OrderTypeBalance, time.Now().Add(-time.Minute))
		// Model a checkout crossing the UTC day boundary. The accepted
		// quotation remains25 even when actual callback rounding differs.
		_, err := f.client.ExecContext(ctx, "UPDATE payment_orders SET created_at=NOW()-INTERVAL '1 day',provider_snapshot=jsonb_build_object('quoted_pay_amount',25),pay_amount=25.01 WHERE id=$1", order.ID)
		require.NoError(t, err)
	}
	for _, status := range []string{service.OrderStatusRefundRequested, service.OrderStatusRefunding, service.OrderStatusRefundPending, service.OrderStatusRefundFailed, service.OrderStatusRefunded, service.OrderStatusPartiallyRefunded} {
		order := f.order(t, status, payment.OrderTypeBalance, time.Now().Add(-2*time.Hour))
		_, err := f.client.PaymentOrder.UpdateOneID(order.ID).SetPaidAt(time.Now()).Save(ctx)
		require.NoError(t, err)
	}
	_ = f.order(t, service.OrderStatusExpired, payment.OrderTypeBalance, time.Now().Add(-2*time.Hour))
	usage, err := payment.ProviderDailyUsage(ctx, f.client, []string{id}, time.Now())
	require.NoError(t, err)
	require.Equal(t, 250.0, usage[id], "recoverable reservations and paid/refunded captures count using their admitted quotation")
}
