//go:build unit

package provider

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/smartwalle/alipay/v3"
	"github.com/stretchr/testify/require"
)

func TestAlipayRefundTimeoutKeepsStableQueryableRequestID(t *testing.T) {
	previous := alipayTradeRefund
	t.Cleanup(func() { alipayTradeRefund = previous })
	var requestIDs []string
	alipayTradeRefund = func(_ context.Context, _ *alipay.Client, req alipay.TradeRefund) (*alipay.TradeRefundRsp, error) {
		requestIDs = append(requestIDs, req.OutRequestNo)
		return nil, context.DeadlineExceeded
	}
	provider := &Alipay{client: &alipay.Client{}}
	var first string
	for _, amount := range []string{"12.50", "12.5"} {
		resp, err := provider.Refund(t.Context(), payment.RefundRequest{OrderID: "sub2_lost_reply", Amount: amount})
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NotNil(t, resp)
		require.Equal(t, payment.ProviderStatusPending, resp.Status)
		if first == "" {
			first = resp.RefundID
		}
		require.Equal(t, first, resp.RefundID)
	}
	require.Len(t, requestIDs, 2)
	require.Equal(t, "sub2_lost_reply-refund-1250", first)
	require.Equal(t, requestIDs[0], requestIDs[1])
}

func TestAlipayRefundRetryAddressesSameGatewayOperation(t *testing.T) {
	previous := alipayTradeRefund
	t.Cleanup(func() { alipayTradeRefund = previous })
	var requestIDs []string
	alipayTradeRefund = func(_ context.Context, _ *alipay.Client, req alipay.TradeRefund) (*alipay.TradeRefundRsp, error) {
		requestIDs = append(requestIDs, req.OutRequestNo)
		return &alipay.TradeRefundRsp{FundChange: alipayFundChangeYes}, nil
	}
	provider := &Alipay{client: &alipay.Client{}}
	for _, amount := range []string{"12.50", "12.5", "12.51"} {
		resp, err := provider.Refund(t.Context(), payment.RefundRequest{OrderID: "sub2_retry", Amount: amount})
		require.NoError(t, err)
		require.Equal(t, payment.ProviderStatusSuccess, resp.Status)
	}
	require.Equal(t, requestIDs[0], requestIDs[1], "a retry cannot create a second refund operation")
	require.NotEqual(t, requestIDs[1], requestIDs[2])
}
