package provider

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

func newEasyPayNotificationSecurityProvider(t *testing.T) *EasyPay {
	t.Helper()

	provider, err := NewEasyPay("security-test", map[string]string{
		"pid":         "1000",
		"pkey":        "merchant-secret",
		"apiBase":     "https://pay.example.com",
		"notifyUrl":   "https://site.example/api/v1/payment/webhook/easypay",
		"returnUrl":   "https://site.example/payment/result",
		"paymentMode": "popup",
	})
	require.NoError(t, err)
	return provider
}

func signedEasyPayNotification(t *testing.T, params url.Values) string {
	t.Helper()

	signParams := make(map[string]string, len(params))
	for key, values := range params {
		require.Len(t, values, 1, "test notification parameter %s must be single-valued", key)
		signParams[key] = values[0]
	}
	params.Set("sign", easyPaySign(signParams, "merchant-secret"))
	params.Set("sign_type", signTypeMD5)
	return params.Encode()
}

func standardEasyPayNotification() url.Values {
	return url.Values{
		"pid":          {"1000"},
		"trade_no":     {"UPSTREAM-123"},
		"out_trade_no": {"ORDER-123"},
		"type":         {"alipay"},
		"name":         {"Sub2API 650.00 CNY"},
		"money":        {"650.00"},
		"trade_status": {tradeStatusSuccess},
		"param":        {""},
	}
}

func TestEasyPayVerifyNotificationAcceptsStandardCallback(t *testing.T) {
	provider := newEasyPayNotificationSecurityProvider(t)

	notification, err := provider.VerifyNotification(context.Background(), signedEasyPayNotification(t, standardEasyPayNotification()), nil)
	require.NoError(t, err)
	require.Equal(t, payment.NotificationStatusSuccess, notification.Status)
	require.Equal(t, "ORDER-123", notification.OrderID)
	require.Equal(t, "UPSTREAM-123", notification.TradeNo)
	require.Equal(t, 650.0, notification.Amount)
	require.Equal(t, "1000", notification.Metadata["pid"])
}

func TestEasyPayVerifyNotificationRejectsSignatureReusePayload(t *testing.T) {
	provider := newEasyPayNotificationSecurityProvider(t)
	checkout, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "ORDER123",
		Amount:      "650.00",
		PaymentType: payment.TypeAlipay,
		Subject:     "balance recharge",
		ReturnURL:   "https://site.example/payment/result?order_id=99&out_trade_no=ORDER123&status=success&trade_status=TRADE_SUCCESS",
	})
	require.NoError(t, err)

	signed, err := url.Parse(checkout.PayURL)
	require.NoError(t, err)
	query := signed.Query()
	callback := url.Values{}
	for _, key := range []string{"pid", "type", "out_trade_no", "notify_url", "name", "money"} {
		callback.Set(key, query.Get(key))
	}
	callback.Set("return_url", "https://site.example/payment/result?order_id=99&out_trade_no=ORDER123&status=success")
	raw := callback.Encode() + "&trade_status=TRADE_SUCCESS&sign=" + query.Get("sign") + "&sign_type=MD5"

	_, err = provider.VerifyNotification(context.Background(), raw, nil)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unexpected notify param"), "error = %v", err)
}

func TestEasyPayVerifyNotificationRejectsAmbiguousOrMismatchedFields(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) string
	}{
		{
			name: "unknown field",
			build: func(t *testing.T) string {
				raw := signedEasyPayNotification(t, standardEasyPayNotification())
				return raw + "&return_url=https%3A%2F%2Fsite.example%2Fpayment%2Fresult"
			},
		},
		{
			name: "duplicate field",
			build: func(t *testing.T) string {
				raw := signedEasyPayNotification(t, standardEasyPayNotification())
				return raw + "&pid=1000"
			},
		},
		{
			name: "wrong merchant",
			build: func(t *testing.T) string {
				params := standardEasyPayNotification()
				params.Set("pid", "2000")
				return signedEasyPayNotification(t, params)
			},
		},
		{
			name: "missing successful trade number",
			build: func(t *testing.T) string {
				params := standardEasyPayNotification()
				params.Del("trade_no")
				return signedEasyPayNotification(t, params)
			},
		},
		{
			name: "unsupported sign type",
			build: func(t *testing.T) string {
				params := standardEasyPayNotification()
				raw := signedEasyPayNotification(t, params)
				return strings.Replace(raw, "sign_type=MD5", "sign_type=SHA256", 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := newEasyPayNotificationSecurityProvider(t)
			_, err := provider.VerifyNotification(context.Background(), tt.build(t), nil)
			require.Error(t, err)
		})
	}
}
