//go:build integration

package repository

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

func TestPaymentCheckoutBudgetsPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	ctx := t.Context()
	_, err := f.client.PaymentProviderInstance.Update().SetEnabled(false).Save(ctx)
	require.NoError(t, err)
	_, err = f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	settings := NewSettingRepository(f.client)
	require.NoError(t, settings.SetMultiple(ctx, map[string]string{
		service.SettingPaymentEnabled: "true", service.SettingEnabledPaymentTypes: "alipay",
		service.SettingPaymentVisibleMethodAlipayEnabled: "true", service.SettingPaymentVisibleMethodAlipaySource: service.VisibleMethodSourceEasyPayAlipay,
		service.SettingDailyRechargeLimit: "0", service.SettingMaxPendingOrders: "1",
	}))
	users := NewUserRepository(f.client, integrationDB)
	key := make([]byte, payment.AES256KeySize)
	svc := service.NewPaymentService(f.client, payment.NewRegistry(), payment.NewDefaultLoadBalancer(f.client, key), nil, nil,
		service.NewPaymentConfigService(f.client, settings, key), users, nil, nil)
	newUser := func() int64 {
		user, err := f.client.User.Create().SetEmail(fmt.Sprintf("budget-%d@fixture.invalid", time.Now().UnixNano())).SetPasswordHash("fixture").Save(ctx)
		require.NoError(t, err)
		return user.ID
	}
	create := func(user int64, amount float64) error {
		_, err := svc.CreateOrder(ctx, service.CreateOrderRequest{UserID: user, Amount: amount, PaymentType: payment.TypeAlipay,
			OrderType: payment.OrderTypeBalance, ClientIP: "127.0.0.1", SrcHost: "fixture.invalid"})
		return err
	}
	t.Run("one pending slot is reserved across concurrent requests", func(t *testing.T) {
		user := newUser()
		results := make(chan error, 8)
		var workers sync.WaitGroup
		for range 8 {
			workers.Go(func() { results <- create(user, 25) })
		}
		workers.Wait()
		accepted := 0
		for range 8 {
			if <-results == nil {
				accepted++
			}
		}
		require.Equal(t, 1, accepted)
	})
	t.Run("pending checkout reserves daily allowance", func(t *testing.T) {
		require.NoError(t, settings.SetMultiple(ctx, map[string]string{service.SettingMaxPendingOrders: "3", service.SettingDailyRechargeLimit: "100"}))
		user := newUser()
		require.NoError(t, create(user, 75))
		require.ErrorContains(t, create(user, 75), "DAILY_LIMIT_EXCEEDED")
	})
	t.Run("disabled visible method rejects direct API request", func(t *testing.T) {
		require.NoError(t, settings.Set(ctx, service.SettingPaymentVisibleMethodAlipayEnabled, "false"))
		require.ErrorContains(t, create(newUser(), 25), "PAYMENT_DISABLED")
	})
}
