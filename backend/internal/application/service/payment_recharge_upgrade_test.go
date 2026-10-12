//go:build unit

package service

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

func TestRechargeOffersDefaultOffPreservesLegacyQuote(t *testing.T) {
	cfg := &PaymentConfig{BalanceRechargeMultiplier: 2, RechargeBonusTiers: []RechargeBonusTier{{MinAmount: 10, BonusPercent: 20}}, RechargeBonusMode: RechargeBonusModeDiscount}
	quote := quoteRechargeBonus(cfg, 100, "USD")
	require.Equal(t, float64(100), quote.PayBase)
	require.Equal(t, float64(200), quote.Credited)
	require.Zero(t, quote.Bonus)
	cfg.RechargeBonusEnabled = true
	quote = quoteRechargeBonus(cfg, 100, "USD")
	require.Equal(t, float64(80), quote.PayBase)
	require.Equal(t, float64(200), quote.Credited)
	require.Equal(t, float64(40), quote.Bonus)
}

func TestRechargeOfferSnapshotSurvivesConfigChangeAndRefunds(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	user, err := client.User.Create().SetEmail("bonus-snapshot@example.com").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	cfg := &PaymentConfig{RechargeBonusEnabled: true, BalanceRechargeMultiplier: 1, RechargeBonusTiers: []RechargeBonusTier{{MinAmount: 100, BonusPercent: 20}}, OrderTimeoutMin: 30, MaxPendingOrders: 3}
	quote := quoteRechargeBonus(cfg, 100, "USD")
	svc := &PaymentService{entClient: client}
	order, err := svc.createOrderInTx(ctx, CreateOrderRequest{UserID: user.ID, PaymentType: payment.TypeAlipay, OrderType: payment.OrderTypeBalance}, &User{ID: user.ID, Email: user.Email}, nil, cfg, quote.Credited, quote.PayBase, 0, quote.PayBase, quote.Bonus, nil)
	require.NoError(t, err)
	cfg.RechargeBonusTiers[0].BonusPercent = 90
	reloaded, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, float64(120), reloaded.Amount)
	require.Equal(t, float64(100), reloaded.PayAmount)
	require.Equal(t, float64(20), reloaded.BonusAmount)
	require.Equal(t, float64(100), affiliateRebateBaseAmount(reloaded))
	require.Equal(t, float64(100), calculateGatewayRefundAmount(reloaded.Amount, reloaded.PayAmount, 120, "USD"))
	require.Equal(t, float64(50), calculateGatewayRefundAmount(reloaded.Amount, reloaded.PayAmount, 60, "USD"))
	reloaded.RefundAmount = 60
	plan := svc.refundFinalizePlan(reloaded)
	require.Equal(t, float64(50), plan.GatewayAmount)
	require.Equal(t, float64(60), plan.BalanceToDeduct, "refunding half the paid amount revokes half the paid+gift credits")
	legacy := &dbent.PaymentOrder{OrderType: payment.OrderTypeBalance, Amount: 100, PayAmount: 100}
	require.Equal(t, float64(100), affiliateRebateBaseAmount(legacy))
	require.Equal(t, float64(50), calculateGatewayRefundAmount(legacy.Amount, legacy.PayAmount, 50, "USD"))
}
