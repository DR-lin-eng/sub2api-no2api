package payment

import (
	"context"
	"fmt"
	"math"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/shopspring/decimal"
)

const OrderRecoveryWindow = 30 * time.Minute

// Provider budgets measure checkout quotations (including fees). Callback
// amount tolerance cannot change a reservation after it has been admitted.
const QuotedPayAmountSnapshotKey = "quoted_pay_amount"

func ProviderBudgetOrders(now time.Time) predicate.PaymentOrder {
	now = now.UTC()
	return paymentorder.Or(
		paymentorder.PaidAtGTE(startOfDay(now)),
		paymentorder.And(paymentorder.PaidAtIsNil(),
			paymentorder.StatusIn(OrderStatusPending, OrderStatusCancelled, OrderStatusExpired, OrderStatusFailed),
			paymentorder.ExpiresAtGTE(now.Add(-OrderRecoveryWindow))),
		paymentorder.And(paymentorder.PaidAtIsNil(), paymentorder.CreatedAtGTE(startOfDay(now)),
			paymentorder.StatusIn(OrderStatusPaid, OrderStatusRecharging, OrderStatusCompleted,
				OrderStatusRefundRequested, OrderStatusRefunding, OrderStatusRefundPending,
				OrderStatusPartiallyRefunded, OrderStatusRefunded, OrderStatusRefundFailed)),
	)
}

func ProviderDailyUsage(ctx context.Context, client *dbent.Client, instanceIDs []string, now time.Time) (map[string]float64, error) {
	usage := make(map[string]float64, len(instanceIDs))
	if len(instanceIDs) == 0 {
		return usage, nil
	}
	var rows []struct {
		InstanceID string  `json:"provider_instance_id"`
		Sum        float64 `json:"sum"`
	}
	err := client.PaymentOrder.Query().Where(paymentorder.ProviderInstanceIDIn(instanceIDs...), ProviderBudgetOrders(now)).
		GroupBy(paymentorder.FieldProviderInstanceID).Aggregate(func(selector *sql.Selector) string {
		amount := selector.C(paymentorder.FieldPayAmount)
		snapshot := selector.C(paymentorder.FieldProviderSnapshot)
		quote := fmt.Sprintf("COALESCE((%s->>'%s')::numeric,%s)", snapshot, QuotedPayAmountSnapshotKey, amount)
		if selector.Dialect() == dialect.SQLite {
			quote = fmt.Sprintf("COALESCE(json_extract(%s,'$.%s'),%s)", snapshot, QuotedPayAmountSnapshotKey, amount)
		}
		return sql.As("SUM("+quote+")", "sum")
	}).Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("read provider budget usage: %w", err)
	}
	for _, row := range rows {
		usage[row.InstanceID] = row.Sum
	}
	return usage, nil
}

func CheckChannelBudget(limits ChannelLimits, used, amount float64) error {
	for _, value := range []float64{limits.SingleMin, limits.SingleMax, limits.DailyLimit, used, amount} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("invalid provider budget amount")
		}
	}
	if limits.SingleMin > 0 && limits.SingleMax > 0 && limits.SingleMin > limits.SingleMax {
		return fmt.Errorf("invalid provider budget range")
	}
	quote := decimal.NewFromFloat(amount)
	if limits.SingleMin > 0 && quote.LessThan(decimal.NewFromFloat(limits.SingleMin)) {
		return fmt.Errorf("order is below provider minimum")
	}
	if limits.SingleMax > 0 && quote.GreaterThan(decimal.NewFromFloat(limits.SingleMax)) {
		return fmt.Errorf("order exceeds provider maximum")
	}
	if limits.DailyLimit > 0 && decimal.NewFromFloat(used).Add(quote).GreaterThan(decimal.NewFromFloat(limits.DailyLimit)) {
		return fmt.Errorf("provider daily allowance is exhausted")
	}
	return nil
}
