package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
)

func TestEasyPayCreatePaymentResolvesRelativeReturnedRefs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"trade_no":"TRADE_NO","payurl":"/pc/pay/ORDER_ID","payurl2":"/h5/pay/ORDER_ID","qrcode":"/api/pay/toapp/ORDER_ID"}`))
	}))
	t.Cleanup(server.Close)

	provider, err := NewEasyPay("test-instance", map[string]string{
		"pid":       "pid-1",
		"pkey":      "pkey-1",
		"apiBase":   server.URL + "/xpay/epay",
		"notifyUrl": "https://example.com/notify",
		"returnUrl": "https://example.com/return",
	})
	if err != nil {
		t.Fatalf("NewEasyPay: %v", err)
	}

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:     "sub2-relative",
		Amount:      "1.00",
		PaymentType: payment.TypeWxpay,
		Subject:     "Relative refs",
		IsMobile:    true,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if want := server.URL + "/h5/pay/ORDER_ID"; resp.PayURL != want {
		t.Fatalf("PayURL = %q, want %q", resp.PayURL, want)
	}
	if want := server.URL + "/api/pay/toapp/ORDER_ID"; resp.QRCode != want {
		t.Fatalf("QRCode = %q, want %q", resp.QRCode, want)
	}
}

func TestEasyPayMerchantSecretTransportRequiresTLS(t *testing.T) {
	for _, raw := range []string{"http://pay.example.com", "http://10.0.0.2", "ftp://pay.example.com", "https://user:secret@pay.example.com"} {
		if err := validateEasyPayAPIBase(raw); err == nil {
			t.Fatalf("unsafe payment endpoint accepted: %s", raw)
		}
	}
	for _, raw := range []string{"https://pay.example.com/epay", "http://127.0.0.1:8080/test"} {
		if err := validateEasyPayAPIBase(raw); err != nil {
			t.Fatal(err)
		}
	}
}
