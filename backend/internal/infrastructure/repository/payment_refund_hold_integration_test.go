//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v85"
)

// Exercise the real payment service and repositories against PostgreSQL. Only
// Stripe's network boundary is replaced; no merchant credentials leave the test.
func TestPaymentRefundHoldPostgres(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	var gatewayStatus atomic.Value
	gatewayStatus.Store("failed")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		status := "pending"
		if r.Method == http.MethodGet {
			status = gatewayStatus.Load().(string)
		}
		_, _ = fmt.Fprintf(w, `{"id":"re_fixture","object":"refund","status":%q}`, status)
	}))
	t.Cleanup(server.Close)
	previous := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend,
		&stripe.BackendConfig{URL: stripe.String(server.URL), HTTPClient: server.Client()}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, previous) })

	var failAudit atomic.Bool
	client.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, mutation dbent.Mutation) (dbent.Value, error) {
			if m, ok := mutation.(*dbent.PaymentAuditLogMutation); ok {
				if action, _ := m.Action(); action == "REFUND_FAILED" && failAudit.CompareAndSwap(true, false) {
					return nil, errors.New("injected refund audit failure")
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})
	users := NewUserRepository(client, nil)
	subscriptions := NewUserSubscriptionRepository(client)
	subscriptionService := service.NewSubscriptionService(nil, subscriptions, nil, client, nil)
	encryptionKey := make([]byte, payment.AES256KeySize)
	svc := service.NewPaymentService(client, payment.NewRegistry(), payment.NewDefaultLoadBalancer(client, encryptionKey), nil, subscriptionService, nil, users, nil, nil)
	create := func(t *testing.T) *dbent.PaymentOrder {
		t.Helper()
		suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
		user, err := client.User.Create().SetEmail("refund-" + suffix + "@fixture.invalid").
			SetPasswordHash("fixture").SetUsername("refund fixture").SetBalance(100).Save(ctx)
		require.NoError(t, err)
		config, err := payment.Encrypt(`{"secretKey":"fixture-only","currency":"CNY"}`, encryptionKey)
		require.NoError(t, err)
		instance, err := client.PaymentProviderInstance.Create().SetName("refund-" + suffix).
			SetProviderKey(payment.TypeStripe).SetConfig(config).
			SetSupportedTypes("stripe").SetEnabled(true).SetRefundEnabled(true).Save(ctx)
		require.NoError(t, err)
		order, err := client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).
			SetUserName(user.Username).SetAmount(100).SetPayAmount(100).SetFeeRate(0).
			SetRechargeCode("REFUND-" + suffix).SetOutTradeNo("sub2_" + suffix).
			SetPaymentType(payment.TypeStripe).SetPaymentTradeNo("pi_fixture").
			SetProviderInstanceID(strconv.FormatInt(instance.ID, 10)).SetOrderType(payment.OrderTypeBalance).
			SetStatus(service.OrderStatusCompleted).SetPaidAt(time.Now()).SetExpiresAt(time.Now().Add(time.Hour)).
			SetClientIP("127.0.0.1").SetSrcHost("fixture.invalid").Save(ctx)
		require.NoError(t, err)
		return order
	}
	begin := func(t *testing.T, order *dbent.PaymentOrder) {
		t.Helper()
		plan, early, err := svc.PrepareRefund(ctx, order.ID, 100, "fixture", false, true)
		require.NoError(t, err)
		require.Nil(t, early)
		result, err := svc.ExecuteRefund(ctx, plan)
		require.NoError(t, err)
		require.False(t, result.Success)
		reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefundPending, reloaded.Status)
	}
	for _, status := range []string{"succeeded", "failed"} {
		t.Run(status, func(t *testing.T) {
			order := create(t)
			begin(t, order)
			user, err := users.GetByID(ctx, order.UserID)
			require.NoError(t, err)
			require.Zero(t, user.Balance, "pending funds remain deducted")
			gatewayStatus.Store(status)
			result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, status == "succeeded", result.Success)
			user, err = users.GetByID(ctx, order.UserID)
			require.NoError(t, err)
			if status == "failed" {
				require.Equal(t, 100.0, user.Balance)
			} else {
				require.Zero(t, user.Balance, "successful settlement never deducts twice")
			}
			_, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
			require.Error(t, err)
		})
	}
	t.Run("audit failure rolls back status and returned funds", func(t *testing.T) {
		order := create(t)
		begin(t, order)
		gatewayStatus.Store("failed")
		failAudit.Store(true)
		_, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
		require.ErrorContains(t, err, "injected refund audit failure")
		order, err = client.PaymentOrder.Get(ctx, order.ID)
		require.NoError(t, err)
		require.Equal(t, service.OrderStatusRefundPending, order.Status)
		user, err := users.GetByID(ctx, order.UserID)
		require.NoError(t, err)
		require.Zero(t, user.Balance)
		_, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
		require.NoError(t, err)
		user, err = users.GetByID(ctx, order.UserID)
		require.NoError(t, err)
		require.Equal(t, 100.0, user.Balance)
	})
	t.Run("concurrent finalizers return funds once", func(t *testing.T) {
		order := create(t)
		begin(t, order)
		gatewayStatus.Store("failed")
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() { _, _ = svc.QueryAndFinalizeRefund(ctx, order.ID) })
		}
		wg.Wait()
		user, err := users.GetByID(ctx, order.UserID)
		require.NoError(t, err)
		require.Equal(t, 100.0, user.Balance)
		count, err := client.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)),
			paymentauditlog.ActionEQ("REFUND_FAILED")).Count(ctx)
		require.NoError(t, err)
		require.Equal(t, 1, count)
	})
	t.Run("failed refund restores revoked subscription", func(t *testing.T) {
		order := create(t)
		group, err := client.Group.Create().SetName("refund-subscription").SetPlatform("openai").Save(ctx)
		require.NoError(t, err)
		expires := time.Now().AddDate(0, 0, 5).Truncate(time.Second)
		sub, err := client.UserSubscription.Create().SetUserID(order.UserID).SetGroupID(group.ID).
			SetStartsAt(time.Now()).SetExpiresAt(expires).SetStatus(service.SubscriptionStatusActive).Save(ctx)
		require.NoError(t, err)
		order, err = client.PaymentOrder.UpdateOneID(order.ID).SetOrderType(payment.OrderTypeSubscription).
			SetSubscriptionGroupID(group.ID).SetSubscriptionDays(30).Save(ctx)
		require.NoError(t, err)
		begin(t, order)
		_, err = subscriptions.GetByID(ctx, sub.ID)
		require.Error(t, err, "the pending refund revokes the full entitlement")
		gatewayStatus.Store("failed")
		result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
		require.NoError(t, err)
		require.NotContains(t, result.Warning, "rollback failed")
		restored, err := subscriptions.GetByID(ctx, sub.ID)
		require.NoError(t, err)
		require.True(t, restored.ExpiresAt.Equal(expires))
	})
}
