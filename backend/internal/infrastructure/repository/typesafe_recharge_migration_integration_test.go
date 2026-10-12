//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	dbmigrations "github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestTypeSafeRechargeMigrationsPreserveLegacyWriters(t *testing.T) {
	tx := testTx(t)
	ctx := context.Background()
	// Recreate the preceding release's constraints and column set inside this
	// rolled-back transaction; the shared fresh-install harness is unchanged.
	previous, err := dbmigrations.FS.ReadFile("244_add_opencode_go_platform.sql")
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, string(previous))
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, "ALTER TABLE payment_orders DROP COLUMN bonus_amount")
	require.NoError(t, err)
	var orderID, userID, groupID int64
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO payment_orders (user_id,amount,pay_amount,expires_at,out_trade_no) VALUES (999,100,100,NOW(),'legacy-bonus-upgrade') RETURNING id`).Scan(&orderID))
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO users (email,password_hash) VALUES ('typesafe-migrate@example.com','fixture') RETURNING id`).Scan(&userID))
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO groups (name,platform) VALUES ('typesafe-migrate','composite') RETURNING id`).Scan(&groupID))
	for _, filename := range []string{"246_add_typesafe_platform.sql", "247_add_payment_order_bonus_amount.sql"} {
		migration, readErr := dbmigrations.FS.ReadFile(filename)
		require.NoError(t, readErr)
		for range 2 {
			_, err = tx.ExecContext(ctx, string(migration))
			require.NoError(t, err)
		}
	}
	var amount, paid, bonus float64
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT amount,pay_amount,bonus_amount FROM payment_orders WHERE id=$1", orderID).Scan(&amount, &paid, &bonus))
	require.Equal(t, float64(100), amount)
	require.Equal(t, amount, paid)
	require.Zero(t, bonus)
	// An old binary's INSERT (without the new column) remains valid afterwards.
	require.NoError(t, tx.QueryRowContext(ctx, `INSERT INTO payment_orders (user_id,amount,pay_amount,expires_at,out_trade_no) VALUES (999,50,50,NOW(),'old-writer-after-upgrade') RETURNING bonus_amount`).Scan(&bonus))
	require.Zero(t, bonus)
	for _, platform := range []string{"anthropic", "openai", "gemini", "antigravity", "grok", "kimi", "zhipu", "deepseek", "minimax", "opencode_go", "typesafe"} {
		_, err = tx.ExecContext(ctx, `INSERT INTO user_platform_quotas (user_id,platform) VALUES ($1,$2)`, userID, platform)
		require.NoError(t, err, platform)
		_, err = tx.ExecContext(ctx, `INSERT INTO composite_model_routes (group_id,public_model,target_platform) VALUES ($1,$2,$2)`, groupID, platform)
		require.NoError(t, err, platform)
	}
	// Existing CN/OpenCode monitor coverage must survive the native-only addition.
	requireConstraintDefinitionContains(t, tx, "channel_monitors", "channel_monitors_provider_check", "opencode_go", "minimax")
}

func TestTypeSafeQuotaEntAndRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()
	userID := mustCreateUserForQuota(t, client)
	repo := NewUserPlatformQuotaRepository(client)
	daily := 3.0
	require.NoError(t, repo.BulkInsertInitial(txCtx, []UserPlatformQuotaRecord{{UserID: userID, Platform: "typesafe", DailyLimitUSD: &daily}}))
	items, err := repo.ListByUser(txCtx, userID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "typesafe", items[0].Platform)
	require.Equal(t, daily, *items[0].DailyLimitUSD)
}

func TestRechargeGiftPostgresFulfillmentCreditsSnapshotOnce(t *testing.T) {
	ctx := context.Background()
	client := integrationEntClient
	stamp := fmt.Sprintf("gift-%d", time.Now().UnixNano())
	user, err := client.User.Create().SetEmail(stamp + "@example.com").SetPasswordHash("fixture").SetBalance(5).Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(stamp).
		SetAmount(120).SetPayAmount(100).SetBonusAmount(20).SetRechargeCode(stamp).SetOutTradeNo(stamp).
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo(stamp).SetStatus(service.OrderStatusPaid).
		SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("fixture.test").Save(ctx)
	require.NoError(t, err)
	userRepo := NewUserRepository(client, integrationDB)
	redeemRepo := NewRedeemCodeRepository(client)
	redeem := service.NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)
	svc := service.NewPaymentService(client, payment.NewRegistry(), nil, redeem, nil, nil, userRepo, nil, nil)
	require.NoError(t, svc.ExecuteBalanceFulfillment(ctx, order.ID))
	require.NoError(t, svc.ExecuteBalanceFulfillment(ctx, order.ID))
	require.NoError(t, svc.HandlePaymentNotification(ctx, &payment.PaymentNotification{OrderID: stamp, TradeNo: stamp, Amount: 100, Status: payment.NotificationStatusSuccess}, payment.TypeAlipay))
	updated, err := userRepo.GetByID(ctx, user.ID)
	require.NoError(t, err)
	require.InDelta(t, 125, updated.Balance, 1e-8, "paid plus gift credits are settled only once")
	code, err := redeemRepo.GetByCode(ctx, stamp)
	require.NoError(t, err)
	require.Equal(t, float64(120), code.Value)
	require.True(t, code.IsUsed())
	settled, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusCompleted, settled.Status)
	require.Equal(t, float64(20), settled.BonusAmount)
}
