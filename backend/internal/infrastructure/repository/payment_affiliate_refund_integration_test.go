//go:build integration

package repository

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaymentAffiliateRefundPolicyPostgres(t *testing.T) {
	for _, state := range []struct {
		name                         string
		freeze                       int
		matured, thawed, transferred bool
	}{
		{name: "available"}, {name: "frozen", freeze: 24},
		{name: "matured before lazy thaw", freeze: 24, matured: true},
		{name: "thawed", freeze: 24, matured: true, thawed: true},
		{name: "already transferred", transferred: true},
	} {
		for _, refund := range []float64{12.5, 25} {
			t.Run(fmt.Sprintf("%s/refund=%.2f", state.name, refund), func(t *testing.T) {
				ctx := t.Context()
				f := newPaymentRecoveryFixture(t)
				settings := NewSettingRepository(f.client)
				require.NoError(t, settings.SetMultiple(ctx, map[string]string{
					service.SettingKeyAffiliateEnabled: "true", service.SettingKeyAffiliateRebateRate: "10",
					service.SettingKeyAffiliateRebateFreezeHours:   strconv.Itoa(state.freeze),
					service.SettingKeyAffiliateRebatePerInviteeCap: "0",
				}))
				affiliates := NewAffiliateRepository(f.client, integrationDB)
				affiliateService := service.NewAffiliateService(affiliates, service.NewSettingService(settings, nil), nil, nil)
				users := NewUserRepository(f.client, integrationDB)
				redeems := service.NewRedeemService(NewRedeemCodeRepository(f.client), users, nil, nil, nil, f.client, nil, nil)
				registry := payment.NewRegistry()
				f.svc = service.NewPaymentService(f.client, registry, payment.NewDefaultLoadBalancer(f.client, make([]byte, payment.AES256KeySize)), redeems, nil, nil, users, nil, affiliateService)
				router := gin.New()
				router.POST("/notify", handler.NewPaymentWebhookHandler(f.svc, registry).EasyPayNotify)
				f.router = router
				order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
				_, err := f.client.PaymentOrder.UpdateOneID(order.ID).SetBonusAmount(0).Save(ctx)
				require.NoError(t, err)
				inviter, err := f.client.User.Create().SetEmail(fmt.Sprintf("refund-inviter-%d@fixture.invalid", time.Now().UnixNano())).SetPasswordHash("fixture").Save(ctx)
				require.NoError(t, err)
				_, err = affiliates.BindInviter(ctx, order.UserID, inviter.ID)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, f.notify(order).Code)
				if state.matured {
					_, err = f.client.ExecContext(ctx, "UPDATE user_affiliate_ledger SET frozen_until=NOW()-INTERVAL '1 minute' WHERE source_order_id=$1 AND action='accrue'", order.ID)
					require.NoError(t, err)
				}
				if state.thawed {
					_, err = affiliates.ThawFrozenQuota(ctx, inviter.ID)
					require.NoError(t, err)
				}
				if state.transferred {
					amount, _, err := affiliates.TransferQuotaToBalance(ctx, inviter.ID)
					require.NoError(t, err)
					require.Equal(t, 2.5, amount)
				}
				// Reversal uses the committed historical grant, even after both
				// the campaign switch and current percentage have changed.
				require.NoError(t, settings.SetMultiple(ctx, map[string]string{service.SettingKeyAffiliateEnabled: "false", service.SettingKeyAffiliateRebateRate: "90"}))
				plan, early, err := f.svc.PrepareRefund(ctx, order.ID, refund, "policy fixture", false, true)
				require.NoError(t, err)
				require.Nil(t, early)
				result, err := f.svc.ExecuteRefund(ctx, plan)
				require.NoError(t, err)
				require.True(t, result.Success)
				reversed := 2.5 * refund / 25
				summary, err := affiliates.EnsureUserAffiliate(ctx, inviter.ID)
				require.NoError(t, err)
				if state.transferred {
					require.Zero(t, summary.AffQuota)
					require.Equal(t, reversed, summary.AffDebt)
					wallet, err := users.GetByID(ctx, inviter.ID)
					require.NoError(t, err)
					require.Equal(t, 2.5, wallet.Balance, "ordinary wallet is not silently clawed back")
					_, _, err = affiliates.TransferQuotaToBalance(ctx, inviter.ID)
					require.ErrorIs(t, err, service.ErrAffiliateQuotaEmpty)
				} else if state.freeze > 0 && !state.thawed {
					require.Zero(t, summary.AffQuota)
					require.Zero(t, summary.AffDebt)
					require.Equal(t, 2.5-reversed, summary.AffFrozenQuota)
				} else {
					require.Equal(t, 2.5-reversed, summary.AffQuota)
					require.Zero(t, summary.AffDebt)
				}
				concrete, ok := affiliates.(service.AffiliateRefundRepository)
				require.True(t, ok)
				repeated, err := concrete.ReverseOrderQuota(ctx, order.ID, refund, 25)
				require.NoError(t, err)
				require.Zero(t, repeated)
				entries, err := f.client.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ("AFFILIATE_REBATE_REVERSED")).Count(ctx)
				require.NoError(t, err)
				require.Equal(t, 1, entries)
				if state.freeze > 0 && !state.thawed {
					_, err = f.client.ExecContext(ctx, "UPDATE user_affiliate_ledger SET frozen_until=NOW()-INTERVAL '1 minute' WHERE source_order_id=$1 AND action='accrue'", order.ID)
					require.NoError(t, err)
					_, err = affiliates.ThawFrozenQuota(ctx, inviter.ID)
					require.NoError(t, err)
					summary, err = affiliates.EnsureUserAffiliate(ctx, inviter.ID)
					require.NoError(t, err)
					require.Equal(t, 2.5-reversed, summary.AffQuota)
				}
				if state.transferred {
					applied, err := affiliates.AccrueQuota(ctx, inviter.ID, order.UserID, 1, 24, nil)
					require.NoError(t, err)
					require.True(t, applied)
					summary, err = affiliates.EnsureUserAffiliate(ctx, inviter.ID)
					require.NoError(t, err)
					require.Equal(t, reversed, summary.AffDebt, "future rewards remain frozen until maturity")
					_, err = f.client.ExecContext(ctx, "UPDATE user_affiliate_ledger SET frozen_until=NOW()-INTERVAL '1 minute' WHERE user_id=$1 AND frozen_until IS NOT NULL", inviter.ID)
					require.NoError(t, err)
					_, err = affiliates.ThawFrozenQuota(ctx, inviter.ID)
					require.NoError(t, err)
					summary, err = affiliates.EnsureUserAffiliate(ctx, inviter.ID)
					require.NoError(t, err)
					require.Equal(t, reversed-1, summary.AffDebt)
					require.Zero(t, summary.AffQuota)
					_, err = affiliates.AccrueQuota(ctx, inviter.ID, order.UserID, 3, 0, nil)
					require.NoError(t, err)
					summary, err = affiliates.EnsureUserAffiliate(ctx, inviter.ID)
					require.NoError(t, err)
					require.Zero(t, summary.AffDebt)
					require.Equal(t, 4-reversed, summary.AffQuota)
				}
			})
		}
	}
}

func TestPaymentAffiliateReversalThawTransferConcurrentPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	f := newPaymentRecoveryFixture(t)
	order := f.order(t, service.OrderStatusCompleted, payment.OrderTypeBalance, time.Now().Add(time.Hour))
	inviter, err := f.client.User.Create().SetEmail(fmt.Sprintf("concurrent-rebate-%d@fixture.invalid", time.Now().UnixNano())).SetPasswordHash("fixture").Save(ctx)
	require.NoError(t, err)
	affiliates := NewAffiliateRepository(f.client, integrationDB)
	_, err = affiliates.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	_, err = affiliates.AccrueQuota(ctx, inviter.ID, order.UserID, 5, 24, &order.ID)
	require.NoError(t, err)
	_, err = f.client.ExecContext(ctx, "UPDATE user_affiliate_ledger SET frozen_until=NOW()-INTERVAL '1 minute' WHERE source_order_id=$1", order.ID)
	require.NoError(t, err)
	reverser, ok := affiliates.(service.AffiliateRefundRepository)
	require.True(t, ok)
	type operationResult struct {
		kind   int
		amount float64
		err    error
	}
	start := make(chan struct{})
	results := make(chan operationResult, 12)
	for i := range 12 {
		go func(kind int) {
			<-start
			result := operationResult{kind: kind}
			switch kind {
			case 0:
				result.amount, result.err = reverser.ReverseOrderQuota(ctx, order.ID, 25, 25)
			case 1:
				result.amount, result.err = affiliates.ThawFrozenQuota(ctx, inviter.ID)
			case 2:
				result.amount, _, result.err = affiliates.TransferQuotaToBalance(ctx, inviter.ID)
			}
			results <- result
		}(i % 3)
	}
	close(start)
	var reversed float64
	for range 12 {
		result := <-results
		if result.kind == 2 && errors.Is(result.err, service.ErrAffiliateQuotaEmpty) {
			continue
		}
		require.NoError(t, result.err, "concurrent operation %d", result.kind)
		if result.kind == 0 {
			reversed += result.amount
		}
	}
	require.Equal(t, 5.0, reversed, "repeated concurrent reversals claim the award once")
	summary, err := affiliates.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	wallet, err := f.client.User.Get(ctx, inviter.ID)
	require.NoError(t, err)
	require.Zero(t, summary.AffFrozenQuota)
	require.Equal(t, 5.0, summary.AffHistoryQuota)
	require.Zero(t, wallet.Balance+summary.AffQuota-summary.AffDebt, "a transfer that wins the race must be fully offset by debt")
	require.Equal(t, 1, querySingleInt(t, ctx, f.client, "SELECT COUNT(*) FROM user_affiliate_ledger WHERE source_order_id=$1 AND action='refund'", order.ID))
}
func TestPaymentAffiliateReversalAuditFailureRollsBackPostgres(t *testing.T) {
	ctx := t.Context()
	f := newPaymentRecoveryFixture(t)
	order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
	require.Equal(t, http.StatusOK, f.notify(order).Code)
	inviter, err := f.client.User.Create().SetEmail(fmt.Sprintf("rollback-rebate-%d@fixture.invalid", time.Now().UnixNano())).SetPasswordHash("fixture").Save(ctx)
	require.NoError(t, err)
	affiliates := NewAffiliateRepository(f.client, integrationDB)
	_, err = affiliates.BindInviter(ctx, order.UserID, inviter.ID)
	require.NoError(t, err)
	_, err = affiliates.AccrueQuota(ctx, inviter.ID, order.UserID, 0.12345678, 0, &order.ID)
	require.NoError(t, err)
	users := NewUserRepository(f.client, integrationDB)
	svc := service.NewPaymentService(f.client, payment.NewRegistry(), payment.NewDefaultLoadBalancer(f.client, make([]byte, payment.AES256KeySize)), nil, nil, nil, users, nil, service.NewAffiliateService(affiliates, nil, nil, nil))
	var fail atomic.Bool
	fail.Store(true)
	f.client.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
			if audit, ok := m.(*dbent.PaymentAuditLogMutation); ok {
				action, _ := audit.Action()
				if action == "AFFILIATE_REBATE_REVERSED" && fail.CompareAndSwap(true, false) {
					return nil, errors.New("injected reversal audit failure")
				}
			}
			return next.Mutate(ctx, m)
		})
	})
	plan, early, err := svc.PrepareRefund(ctx, order.ID, 5, "reversal rollback", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	_, err = svc.ExecuteRefund(ctx, plan)
	require.ErrorContains(t, err, "injected reversal audit failure")
	summary, err := affiliates.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	require.Equal(t, 0.12345678, summary.AffQuota)
	stored, err := f.client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusRefundPending, stored.Status, "a failed local commit retains a recoverable success receipt")
	result, err := svc.QueryAndFinalizeRefund(ctx, order.ID)
	require.NoError(t, err)
	require.True(t, result.Success)
	summary, err = affiliates.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	require.Equal(t, 0.09876542, summary.AffQuota, "retry settles exactly one eight-decimal reversal")
	stored, err = f.client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusPartiallyRefunded, stored.Status)
	require.EqualValues(t, 1, f.refundCalls.Load(), "local settlement retry cannot repeat a gateway refund")
	concrete, ok := affiliates.(service.AffiliateRefundRepository)
	require.True(t, ok)
	reversed, err := concrete.ReverseOrderQuota(ctx, order.ID, 5, 25)
	require.NoError(t, err)
	require.Zero(t, reversed, "the retried receipt already settled the historical award")
}
