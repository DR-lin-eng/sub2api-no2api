//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

type paymentSuccessfulRefundProvider struct {
	refundProviderTestDouble
	calls int
}

func (p *paymentSuccessfulRefundProvider) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	p.calls++
	return &payment.RefundResponse{RefundID: "refund-fixture", Status: payment.ProviderStatusSuccess}, nil
}

func TestPaymentRefundMissingIntentReferenceCannotReportSuccess(t *testing.T) {
	ctx := t.Context()
	client := newPaymentConfigServiceTestClient(t)
	order := createCompletedRefundableOrderForTest(t, ctx, client, "missing-intent")
	order.PaymentTradeNo = ""
	prov := &paymentSuccessfulRefundProvider{}
	restore := replacePaymentProviderFactoryForTest(t, prov)
	defer restore()
	svc := &PaymentService{entClient: client, loadBalancer: &captureLoadBalancer{}}
	resp, err := svc.gwRefund(ctx, &RefundPlan{Order: order, OrderID: order.ID, GatewayAmount: 25})
	require.Nil(t, resp)
	require.ErrorContains(t, err, "upstream trade number")
	require.Zero(t, prov.calls)
}
