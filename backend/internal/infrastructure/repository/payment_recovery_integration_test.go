//go:build integration

package repository

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentauditlog"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Real HTTP webhook verification, payment orchestration, balance/subscription
// repositories and migrations run against disposable PostgreSQL. Only the
// merchant gateway is a local fixture; no external payment is made.
type paymentRecoveryFixture struct {
	client          *dbent.Client
	svc             *service.PaymentService
	instanceID      int64
	router          http.Handler
	refundCalls     atomic.Int64
	createCalls     atomic.Int64
	omitTrade       atomic.Bool
	createMu        sync.RWMutex
	createHook      func(string)
	createError     atomic.Bool
	inflate         atomic.Bool
	loseRefundReply atomic.Bool
}

func newPaymentRecoveryFixture(t *testing.T) *paymentRecoveryFixture {
	t.Helper()
	ctx := context.Background()
	f := &paymentRecoveryFixture{client: testEntClient(t)}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/mapi.php" {
			f.createCalls.Add(1)
			orderID := r.FormValue("out_trade_no")
			f.createMu.RLock()
			hook := f.createHook
			f.createMu.RUnlock()
			if hook != nil {
				hook(orderID)
			}
			if f.createError.Load() {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":0,"msg":"fixture create failure"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "trade_no": orderID, "payurl": "https://fixture.invalid/pay"})
			return
		}
		if r.URL.Query().Get("act") == "refund" {
			f.refundCalls.Add(1)
			if f.loseRefundReply.Load() {
				if hijacker, ok := w.(http.Hijacker); ok {
					connection, _, err := hijacker.Hijack()
					if err == nil {
						_ = connection.Close()
					}
				}
				return
			}
			_, _ = w.Write([]byte(`{"code":1,"msg":"ok"}`))
			return
		}
		orderID := r.URL.Query().Get("out_trade_no")
		tradeNo := "trade-" + orderID
		if f.omitTrade.Load() {
			tradeNo = ""
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "status": 1, "money": "25.00", "out_trade_no": orderID, "trade_no": tradeNo,
		})
	}))
	t.Cleanup(upstream.Close)
	key := make([]byte, payment.AES256KeySize)
	config, err := json.Marshal(map[string]string{
		"pid": "fixture-merchant", "pkey": "fixture-only", "apiBase": upstream.URL,
		"notifyUrl": "https://fixture.invalid/notify", "returnUrl": "https://fixture.invalid/return",
	})
	require.NoError(t, err)
	encrypted, err := payment.Encrypt(string(config), key)
	require.NoError(t, err)
	inst, err := f.client.PaymentProviderInstance.Create().SetName("recovery fixture").
		SetProviderKey(payment.TypeEasyPay).SetConfig(encrypted).SetSupportedTypes("alipay").
		SetEnabled(true).SetRefundEnabled(true).Save(ctx)
	require.NoError(t, err)
	f.instanceID = inst.ID
	users := NewUserRepository(f.client, integrationDB)
	groups := NewGroupRepository(f.client, integrationDB)
	subscriptions := service.NewSubscriptionService(groups, NewUserSubscriptionRepository(f.client), nil, f.client, nil)
	redeems := service.NewRedeemService(NewRedeemCodeRepository(f.client), users, subscriptions, nil, nil, f.client, nil, nil)
	redeems.SetInflationResolver(paymentFixtureInflation{fixture: f})
	registry := payment.NewRegistry()
	f.svc = service.NewPaymentService(f.client, registry, payment.NewDefaultLoadBalancer(f.client, key), redeems, subscriptions, nil, users, groups, nil)
	router := gin.New()
	webhook := handler.NewPaymentWebhookHandler(f.svc, registry)
	router.POST("/notify", webhook.EasyPayNotify)
	f.router = router
	return f
}

type paymentFixtureInflation struct{ fixture *paymentRecoveryFixture }

func (r paymentFixtureInflation) ResolveInflatedBalance(_ context.Context, _ int64, amount float64) (float64, int64, string, string, float64, error) {
	if r.fixture.inflate.Load() {
		return amount + 2.5, 0, "fixture uplift", "fixed", 0, nil
	}
	return amount, 0, "", "", 0, nil
}

func (f *paymentRecoveryFixture) order(t *testing.T, status, orderType string, expires time.Time) *dbent.PaymentOrder {
	t.Helper()
	ctx := context.Background()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	user, err := f.client.User.Create().SetEmail("recovery-" + suffix + "@fixture.invalid").
		SetPasswordHash("fixture").SetUsername("recovery fixture").Save(ctx)
	require.NoError(t, err)
	b := f.client.PaymentOrder.Create().SetUserID(user.ID).SetUserEmail(user.Email).SetUserName(user.Username).
		SetAmount(25).SetPayAmount(25).SetFeeRate(0).SetBonusAmount(5).
		SetRechargeCode("RECOVERY-" + suffix).SetOutTradeNo("sub2_recovery_" + suffix).
		SetPaymentType(payment.TypeAlipay).SetPaymentTradeNo("").
		SetProviderInstanceID(strconv.FormatInt(f.instanceID, 10)).SetProviderKey(payment.TypeEasyPay).
		SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_key": payment.TypeEasyPay,
			"provider_instance_id": strconv.FormatInt(f.instanceID, 10), "merchant_id": "fixture-merchant"}).
		SetOrderType(orderType).SetStatus(status).SetExpiresAt(expires).SetUpdatedAt(time.Now().Add(-24 * time.Hour)).
		SetClientIP("127.0.0.1").SetSrcHost("fixture.invalid")
	if orderType == payment.OrderTypeSubscription {
		group, err := f.client.Group.Create().SetName("recovery-" + suffix).SetPlatform("openai").
			SetSubscriptionType(service.SubscriptionTypeSubscription).Save(ctx)
		require.NoError(t, err)
		b.SetSubscriptionGroupID(group.ID).SetSubscriptionDays(30)
	}
	order, err := b.Save(ctx)
	require.NoError(t, err)
	return order
}

func (f *paymentRecoveryFixture) notify(order *dbent.PaymentOrder) *httptest.ResponseRecorder {
	params := url.Values{"pid": {"fixture-merchant"}, "out_trade_no": {order.OutTradeNo},
		"trade_no": {"trade-" + order.OutTradeNo}, "type": {"alipay"},
		"money": {"25.00"}, "trade_status": {"TRADE_SUCCESS"}}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params.Get(key))
	}
	digest := md5.Sum([]byte(strings.Join(parts, "&") + "fixture-only"))
	params.Set("sign", hex.EncodeToString(digest[:]))
	params.Set("sign_type", "MD5")
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, req)
	return recorder
}

func (f *paymentRecoveryFixture) assertResult(t *testing.T, order *dbent.PaymentOrder, wantStatus string, credited bool) {
	t.Helper()
	ctx := context.Background()
	stored, err := f.client.PaymentOrder.Get(ctx, order.ID)
	require.NoError(t, err)
	require.Equal(t, wantStatus, stored.Status)
	user, err := f.client.User.Get(ctx, order.UserID)
	require.NoError(t, err)
	if order.OrderType == payment.OrderTypeBalance && credited {
		require.Equal(t, 25.0, user.Balance, "credit uses the order snapshot once")
	} else {
		require.Zero(t, user.Balance)
	}
	if order.SubscriptionGroupID != nil {
		sub, err := NewUserSubscriptionRepository(f.client).GetByUserIDAndGroupID(ctx, order.UserID, *order.SubscriptionGroupID)
		if credited {
			require.NoError(t, err)
			require.WithinDuration(t, time.Now().AddDate(0, 0, 30), sub.ExpiresAt, time.Minute)
		} else {
			require.Error(t, err)
		}
	}
	if credited {
		require.NotNil(t, stored.PaidAt)
	} else {
		require.Nil(t, stored.PaidAt)
		action := "PAYMENT_AFTER_EXPIRY"
		if order.Status == service.OrderStatusCancelled {
			action = "PAYMENT_AFTER_CANCEL"
		}
		entries, err := f.client.PaymentAuditLog.Query().Where(paymentauditlog.OrderIDEQ(strconv.FormatInt(order.ID, 10)), paymentauditlog.ActionEQ(action)).All(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, entries)
		require.Contains(t, entries[0].Detail, "recoveryEnd")
	}
}

func TestPaymentRecoveryPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	for _, kind := range []string{payment.OrderTypeBalance, payment.OrderTypeSubscription} {
		for _, status := range []string{service.OrderStatusPending, service.OrderStatusExpired, service.OrderStatusCancelled, service.OrderStatusFailed} {
			for _, late := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/late=%t", kind, status, late), func(t *testing.T) {
					expires := time.Now().Add(-time.Minute)
					if late {
						expires = time.Now().Add(-3 * time.Hour)
					}
					order := f.order(t, status, kind, expires)
					response := f.notify(order)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					require.Equal(t, "success", response.Body.String())
					wantStatus := service.OrderStatusCompleted
					if late {
						wantStatus = status
					}
					f.assertResult(t, order, wantStatus, !late)
					if late {
						require.Equal(t, http.StatusOK, f.notify(order).Code, "duplicate late callbacks must acknowledge the existing audit")
					}
				})
			}
		}
	}
	t.Run("verify expired order with new trade number", func(t *testing.T) {
		order := f.order(t, service.OrderStatusExpired, payment.OrderTypeBalance, time.Now().Add(-3*time.Hour))
		_, err := f.svc.VerifyOrderByOutTradeNo(t.Context(), order.OutTradeNo, order.UserID)
		require.NoError(t, err)
		f.assertResult(t, order, service.OrderStatusExpired, false)
	})
	t.Run("recent metadata cannot reopen cancelled order", func(t *testing.T) {
		order := f.order(t, service.OrderStatusCancelled, payment.OrderTypeBalance, time.Now().Add(-3*time.Hour))
		_, err := f.client.PaymentOrder.UpdateOneID(order.ID).SetUpdatedAt(time.Now()).Save(t.Context())
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, f.notify(order).Code)
		f.assertResult(t, order, service.OrderStatusCancelled, false)
	})
	t.Run("duplicate callbacks credit once", func(t *testing.T) {
		order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
		var group sync.WaitGroup
		for range 4 {
			group.Go(func() { _ = f.notify(order) })
		}
		group.Wait()
		require.Equal(t, http.StatusOK, f.notify(order).Code)
		f.assertResult(t, order, service.OrderStatusCompleted, true)
	})
}

func TestPaymentRefundMissingTradePostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	f.omitTrade.Store(true)
	order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
	paid, err := f.svc.VerifyOrderByOutTradeNo(t.Context(), order.OutTradeNo, order.UserID)
	require.NoError(t, err)
	require.Equal(t, service.OrderStatusCompleted, paid.Status)
	require.Empty(t, paid.PaymentTradeNo, "legacy EasyPay lookup can omit the trade number")
	plan, early, err := f.svc.PrepareRefund(t.Context(), order.ID, 25, "fixture refund", false, true)
	require.NoError(t, err)
	require.Nil(t, early)
	result, err := f.svc.ExecuteRefund(t.Context(), plan)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.EqualValues(t, 1, f.refundCalls.Load(), "a successful refund must actually contact the gateway")
}

func TestPaymentRefundConcurrentSpendPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
			require.Equal(t, http.StatusOK, f.notify(order).Code)
			plan, early, err := f.svc.PrepareRefund(t.Context(), order.ID, 25, "concurrent spend", force, true)
			require.NoError(t, err)
			require.Nil(t, early)
			_, err = f.client.User.UpdateOneID(order.UserID).SetBalance(5).Save(t.Context())
			require.NoError(t, err)
			before := f.refundCalls.Load()
			result, err := f.svc.ExecuteRefund(t.Context(), plan)
			user, userErr := f.client.User.Get(t.Context(), order.UserID)
			require.NoError(t, userErr)
			if force {
				require.NoError(t, err)
				require.True(t, result.Success)
				require.Zero(t, user.Balance)
				require.Equal(t, before+1, f.refundCalls.Load())
			} else {
				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, 5.0, user.Balance, "aborted refund rolls back its partial deduction")
				require.Equal(t, before, f.refundCalls.Load())
			}
		})
	}
}

func TestPaymentCreationCallbackRacePostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	ctx := t.Context()
	// This test creates orders through the public service with one configured
	// checkout instance. The database is disposable and the suite is sequential.
	_, err := f.client.PaymentProviderInstance.Update().SetEnabled(false).Save(ctx)
	require.NoError(t, err)
	_, err = f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	settings := NewSettingRepository(f.client)
	require.NoError(t, settings.SetMultiple(ctx, map[string]string{
		service.SettingPaymentEnabled: "true", service.SettingEnabledPaymentTypes: "alipay",
		service.SettingPaymentVisibleMethodAlipayEnabled: "true",
		service.SettingPaymentVisibleMethodAlipaySource:  service.VisibleMethodSourceEasyPayAlipay,
	}))
	users := NewUserRepository(f.client, integrationDB)
	key := make([]byte, payment.AES256KeySize)
	cfg := service.NewPaymentConfigService(f.client, settings, key)
	redeems := service.NewRedeemService(NewRedeemCodeRepository(f.client), users, nil, nil, nil, f.client, nil, nil)
	svc := service.NewPaymentService(f.client, payment.NewRegistry(), payment.NewDefaultLoadBalancer(f.client, key), redeems, nil, cfg, users, nil, nil)
	for _, createError := range []bool{false, true} {
		t.Run(fmt.Sprintf("provider_error=%t", createError), func(t *testing.T) {
			f.createError.Store(createError)
			user, err := f.client.User.Create().SetEmail(fmt.Sprintf("create-race-%d@fixture.invalid", time.Now().UnixNano())).SetPasswordHash("fixture").Save(ctx)
			require.NoError(t, err)
			acceptedOrders := make(chan *dbent.PaymentOrder, 1)
			f.createMu.Lock()
			f.createHook = func(orderID string) {
				order, lookupErr := f.client.PaymentOrder.Query().Where(paymentorder.OutTradeNoEQ(orderID)).Only(ctx)
				if lookupErr != nil {
					return
				}
				if f.notify(order).Code == http.StatusOK {
					accepted, _ := f.client.PaymentOrder.Get(ctx, order.ID)
					acceptedOrders <- accepted
				}
			}
			f.createMu.Unlock()
			_, createErr := svc.CreateOrder(ctx, service.CreateOrderRequest{UserID: user.ID, Amount: 25, PaymentType: payment.TypeAlipay,
				OrderType: payment.OrderTypeBalance, SrcHost: "fixture.invalid", ClientIP: "127.0.0.1"})
			if createError {
				require.Error(t, createErr)
			} else {
				require.NoError(t, createErr)
			}
			var accepted *dbent.PaymentOrder
			select {
			case accepted = <-acceptedOrders:
			default:
				t.Fatal("the callback must finish before CreatePayment returns")
			}
			require.NotNil(t, accepted)
			stored, err := f.client.PaymentOrder.Get(ctx, accepted.ID)
			require.NoError(t, err)
			require.Equal(t, service.OrderStatusCompleted, stored.Status)
			require.Equal(t, accepted.PaymentTradeNo, stored.PaymentTradeNo)
			require.True(t, accepted.UpdatedAt.Equal(stored.UpdatedAt))
			wallet, err := f.client.User.Get(ctx, user.ID)
			require.NoError(t, err)
			require.Equal(t, 25.0, wallet.Balance)
		})
	}
}

func TestPaymentWebhookRetriesTransientStorageFailuresPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	order := f.order(t, service.OrderStatusExpired, payment.OrderTypeBalance, time.Now().Add(-3*time.Hour))
	var failLookup atomic.Bool
	f.client.PaymentOrder.Intercept(dbent.InterceptFunc(func(next dbent.Querier) dbent.Querier {
		return dbent.QuerierFunc(func(ctx context.Context, query dbent.Query) (dbent.Value, error) {
			if failLookup.Load() {
				return nil, errors.New("injected payment order lookup failure")
			}
			return next.Query(ctx, query)
		})
	}))
	failLookup.Store(true)
	require.Equal(t, http.StatusServiceUnavailable, f.notify(order).Code)
	failLookup.Store(false)

	var failAudit atomic.Bool
	f.client.PaymentAuditLog.Use(func(next dbent.Mutator) dbent.Mutator {
		return dbent.MutateFunc(func(ctx context.Context, mutation dbent.Mutation) (dbent.Value, error) {
			if failAudit.Load() {
				return nil, errors.New("injected late payment audit failure")
			}
			return next.Mutate(ctx, mutation)
		})
	})
	failAudit.Store(true)
	require.Equal(t, http.StatusInternalServerError, f.notify(order).Code)
	failAudit.Store(false)
	require.Equal(t, http.StatusOK, f.notify(order).Code)
	f.assertResult(t, order, service.OrderStatusExpired, false)
}
