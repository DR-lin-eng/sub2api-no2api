//go:build unit

package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment/provider"
	"github.com/stretchr/testify/require"
)

func TestEasyPayAsyncNotificationFulfillsOnceWithoutBrowserReturn(t *testing.T) {
	ctx := t.Context()
	client := newPaymentOrderLifecycleTestClient(t)
	user, err := client.User.Create().SetEmail("async@example.invalid").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	order, err := client.PaymentOrder.Create().
		SetUserID(user.ID).SetUserEmail(user.Email).SetUserName("async").
		SetAmount(10).SetPayAmount(10).SetFeeRate(0).
		SetRechargeCode("ASYNC-RECHARGE").SetOutTradeNo("ASYNC-ORDER").
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").SetProviderKey(payment.TypeEasyPay).
		SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusPending).
		SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("merchant.example").Save(ctx)
	require.NoError(t, err)

	var paid atomic.Bool
	var queries atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/mapi.php":
			require.Equal(t, http.MethodPost, r.Method)
			require.NoError(t, r.ParseForm())
			require.Equal(t, order.OutTradeNo, r.PostForm.Get("out_trade_no"))
			_, _ = w.Write([]byte(`{"code":1,"trade_no":"ASYNC-TRADE","qrcode":"weixin://wxpay/async"}`))
		case "/api.php":
			queries.Add(1)
			status := 0
			if paid.Load() {
				status = 1
			}
			_, _ = fmt.Fprintf(w, `{"code":1,"out_trade_no":"ASYNC-ORDER","trade_no":"ASYNC-TRADE","money":"10.00","status":%d}`, status)
		default:
			t.Errorf("unexpected browser return or submit request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	prov, err := provider.NewEasyPay("async", map[string]string{
		"pid": "1000", "pkey": "fixture-key", "apiBase": upstream.URL, "paymentMode": "async",
		"notifyUrl": "https://merchant.example/notify", "returnUrl": "https://merchant.example/return",
	})
	require.NoError(t, err)
	created, err := prov.CreatePayment(ctx, payment.CreatePaymentRequest{
		OrderID: order.OutTradeNo, Amount: "10.00", PaymentType: payment.TypeAlipay, Subject: "Recharge",
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.QRCode)
	_, err = client.PaymentOrder.UpdateOneID(order.ID).SetPaymentTradeNo(created.TradeNo).Save(ctx)
	require.NoError(t, err)

	userRepo := &mockUserRepo{getByIDUser: &User{ID: user.ID, Email: user.Email}}
	userRepo.updateBalanceFn = func(_ context.Context, id int64, amount float64) error {
		require.Equal(t, user.ID, id)
		userRepo.getByIDUser.Balance += amount
		return nil
	}
	redeemRepo := &paymentOrderLifecycleRedeemRepo{codesByCode: map[string]*RedeemCode{
		order.RechargeCode: {ID: 1, Code: order.RechargeCode, Type: RedeemTypeBalance, Value: 10, Status: StatusUnused},
	}}
	svc := &PaymentService{entClient: client, registry: payment.NewRegistry(), userRepo: userRepo,
		redeemService: NewRedeemService(redeemRepo, userRepo, nil, nil, nil, client, nil, nil)}

	notify := url.Values{"pid": {"1000"}, "trade_no": {created.TradeNo}, "out_trade_no": {order.OutTradeNo},
		"type": {"alipay"}, "money": {"10.00"}, "trade_status": {"TRADE_SUCCESS"}}
	keys := make([]string, 0, len(notify))
	for key := range notify {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+notify.Get(key))
	}
	hash := md5.Sum([]byte(strings.Join(parts, "&") + "fixture-key"))
	notify.Set("sign", hex.EncodeToString(hash[:]))
	notify.Set("sign_type", "MD5")
	notification, err := prov.VerifyNotification(ctx, notify.Encode(), nil)
	require.NoError(t, err)

	require.ErrorIs(t, svc.HandleVerifiedPaymentNotification(ctx, notification, prov), ErrPaymentUpstreamUnconfirmed)
	require.Zero(t, userRepo.getByIDUser.Balance)
	pending, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusPending, pending.Status)

	paid.Store(true)
	for range 2 {
		require.NoError(t, svc.HandleVerifiedPaymentNotification(ctx, notification, prov))
	}
	completed, err := client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, completed.Status)
	require.Equal(t, 10.0, userRepo.getByIDUser.Balance)
	require.Len(t, redeemRepo.useCalls, 1)
	require.Equal(t, int32(2), queries.Load(), "a completed duplicate must not query or credit again")
}
