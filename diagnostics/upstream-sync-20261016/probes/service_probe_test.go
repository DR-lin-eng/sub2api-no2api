//go:build unit

package service

import (
    "context"
    "fmt"
    "testing"

    "github.com/Wei-Shaw/sub2api/internal/modules/payment"
    infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
    "github.com/stretchr/testify/require"
)

func TestUpstreamSyncTransactionProbe(t *testing.T) {
    ctx := context.Background()
    svc := &PaymentService{}
    _, err := svc.validateOrderInput(ctx, CreateOrderRequest{OrderType: "unsupported", Amount: 10}, &PaymentConfig{})
    fmt.Printf("OBSERVED unknown_order_type=%q\n", infraerrors.Reason(err))

    client := newPaymentConfigServiceTestClient(t)
    order := createPendingRefundOrderForTest(t, ctx, client, "transaction-probe")
    order, err = client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusRefunding).Save(ctx)
    require.NoError(t, err)
    returned := 0.0
    svc = &PaymentService{entClient: client, userRepo: &mockUserRepo{
        updateBalanceFn: func(_ context.Context, _ int64, amount float64) error {
            returned += amount
            return nil
        },
    }}
    plan := &RefundPlan{OrderID: order.ID, Order: order, RefundAmount: 10,
        DeductionType: payment.DeductionTypeBalance, BalanceToDeduct: 10}
    _, err = svc.finishRefund(ctx, plan, &payment.RefundResponse{RefundID: "probe-refund", Status: payment.ProviderStatusPending})
    require.NoError(t, err)
    order, err = client.PaymentOrder.Get(ctx, order.ID)
    require.NoError(t, err)
    fmt.Printf("OBSERVED pending_status=%s returned_balance=%.0f held_balance=%.0f\n", order.Status, returned, plan.BalanceToDeduct)
}

func TestUpstreamSyncHeldRefundCompatibilityProbe(t *testing.T) {
    for _, status := range []string{payment.ProviderStatusSuccess, payment.ProviderStatusFailed} {
      t.Run(status, func(t *testing.T) {
        ctx := context.Background()
        client := newPaymentConfigServiceTestClient(t)
        order := createPendingRefundOrderForTest(t, ctx, client, "compat-"+status)
        entry, err := client.PaymentAuditLog.Query().Only(ctx)
        require.NoError(t, err)
        _, err = client.PaymentAuditLog.UpdateOneID(entry.ID).SetDetail(
            `{"refundID":"held","deductionHeld":true,"deductionRollbackOK":false,"deductionType":"balance","balanceDeducted":60}`).Save(ctx)
        require.NoError(t, err)
        balance, returned, deducted := 40.0, 0.0, 0
        repo := &mockUserRepo{
            updateBalanceFn: func(_ context.Context, _ int64, amount float64) error {
                balance += amount
                returned += amount
                return nil
            },
            deductAvailableBalanceFn: func(_ context.Context, _ int64, amount float64) (float64, error) {
                deducted++
                balance -= amount
                return amount, nil
            },
        }
        restore := replacePaymentProviderFactoryForTest(t, &refundQueryProviderTestDouble{
            refundResponse: &payment.RefundResponse{RefundID: "held", Status: status},
        })
        svc := &PaymentService{entClient: client, userRepo: repo, loadBalancer: &captureLoadBalancer{}}
        _, err = svc.QueryAndFinalizeRefund(ctx, order.ID)
        restore()
        require.NoError(t, err)
        order, err = client.PaymentOrder.Get(ctx, order.ID)
        require.NoError(t, err)
        fmt.Printf("OBSERVED held_refund_query=%s state=%s balance=%.0f returned=%.0f extra_deductions=%d\n", status, order.Status, balance, returned, deducted)
      })
    }
}
