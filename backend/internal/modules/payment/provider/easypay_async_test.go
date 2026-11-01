package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/stretchr/testify/require"
)

func TestEasyPayCreatesSignedAPIPaymentForSelectedModes(t *testing.T) {
	for _, mode := range []string{"", "qrcode", "api", "redirect", paymentModeAsync} {
		for _, mobile := range []bool{false, true} {
			name := mode + "/desktop"
			if mobile {
				name = mode + "/mobile"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				prov, form := newCreateTestEasyPay(t, `{"code":1,"trade_no":"T1","payurl":"https://pay.example.com/checkout/T1","payurl2":"https://pay.example.com/mobile/T1","qrcode":"weixin://wxpay/test"}`)
				prov.config["paymentMode"] = mode
				result, err := prov.CreatePayment(context.Background(), payment.CreatePaymentRequest{
					OrderID: "ORDER1", Amount: "10.00", Subject: "Recharge",
					PaymentType: payment.TypeWxpay, ClientIP: "192.0.2.1", IsMobile: mobile,
					NotifyURL: "https://merchant.example/notify", ReturnURL: "https://merchant.example/return",
				})
				require.NoError(t, err)
				require.Equal(t, "T1", result.TradeNo)
				require.Equal(t, "weixin://wxpay/test", result.QRCode)
				wantURL := "https://pay.example.com/checkout/T1"
				if mobile {
					wantURL = "https://pay.example.com/mobile/T1"
					require.Equal(t, "mobile", form.Get("device"))
				}
				require.Equal(t, wantURL, result.PayURL)
				require.Equal(t, "ORDER1", form.Get("out_trade_no"))
				require.Equal(t, "10.00", form.Get("money"))
				require.Equal(t, "wxpay", form.Get("type"))
				require.Equal(t, "192.0.2.1", form.Get("clientip"))
				require.Equal(t, "https://merchant.example/notify", form.Get("notify_url"))
				require.Equal(t, "https://merchant.example/return", form.Get("return_url"))
				params := make(map[string]string, len(*form))
				for key := range *form {
					params[key] = form.Get(key)
				}
				require.Equal(t, "MD5", form.Get("sign_type"))
				require.True(t, easyPayVerifySign(params, "pkey-1", form.Get("sign")))
			})
		}
	}
}

func TestEasyPayLegacyPopupKeepsSynchronousCheckout(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		t.Run(map[bool]string{false: "desktop", true: "mobile"}[mobile], func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "synchronous checkout must not call the API", http.StatusInternalServerError)
			}))
			t.Cleanup(upstream.Close)
			prov := newTestEasyPay(t, upstream.URL)
			prov.config["paymentMode"] = paymentModePopup
			result, err := prov.CreatePayment(t.Context(), payment.CreatePaymentRequest{
				OrderID: "SYNC1", Amount: "10.00", PaymentType: payment.TypeAlipay, Subject: "Recharge", IsMobile: mobile,
			})
			require.NoError(t, err)
			require.Zero(t, calls.Load())
			require.Empty(t, result.TradeNo)
			checkout, err := url.Parse(result.PayURL)
			require.NoError(t, err)
			require.Equal(t, "/submit.php", checkout.Path)
			params := make(map[string]string)
			for key, values := range checkout.Query() {
				require.Len(t, values, 1)
				params[key] = values[0]
			}
			require.Equal(t, "SYNC1", params["out_trade_no"])
			require.Equal(t, "10.00", params["money"])
			require.True(t, easyPayVerifySign(params, "pkey-1", params["sign"]))
			if mobile {
				require.Equal(t, "mobile", params["device"])
			}
		})
	}
}

func TestEasyPayAsyncFailureDoesNotSwitchToSynchronousCheckout(t *testing.T) {
	prov, _ := newCreateTestEasyPay(t, `{"code":0,"msg":"async temporarily unavailable"}`)
	prov.config["paymentMode"] = paymentModeAsync
	result, err := prov.CreatePayment(t.Context(), payment.CreatePaymentRequest{OrderID: "FAIL1", Amount: "10.00", PaymentType: payment.TypeAlipay})
	require.ErrorContains(t, err, "async temporarily unavailable")
	require.Nil(t, result)
}

func newCreateTestEasyPay(t *testing.T, response string) (*EasyPay, *url.Values) {
	t.Helper()
	var gotForm url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/mapi.php", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, r.ParseForm())
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	return newTestEasyPay(t, server.URL), &gotForm
}
