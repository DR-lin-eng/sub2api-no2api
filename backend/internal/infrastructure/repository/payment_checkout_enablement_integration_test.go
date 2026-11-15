//go:build integration

package repository

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/modules/payment"
	"github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/handler"
	"github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaymentCheckoutEnablementPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	ctx := t.Context()
	_, err := f.client.PaymentProviderInstance.Update().SetEnabled(false).Save(ctx)
	require.NoError(t, err)
	_, err = f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).
		SetEnabled(true).SetSupportedTypes("alipay,wxpay").Save(ctx)
	require.NoError(t, err)
	settings := NewSettingRepository(f.client)
	require.NoError(t, settings.SetMultiple(ctx, map[string]string{
		service.SettingPaymentEnabled: "true", service.SettingEnabledPaymentTypes: "easypay",
		// Generic settings saves persist these old defaults, while the current
		// payment UI and checkout-info use enabled provider instances instead.
		service.SettingPaymentVisibleMethodAlipayEnabled: "false",
		service.SettingPaymentVisibleMethodWxpayEnabled:  "false",
		service.SettingPaymentVisibleMethodAlipaySource:  "",
		service.SettingPaymentVisibleMethodWxpaySource:   "",
		service.SettingDailyRechargeLimit:                "0",
		service.SettingMaxPendingOrders:                  "3",
		service.SettingBalancePayDisabled:                "false",
	}))
	key := make([]byte, payment.AES256KeySize)
	config := service.NewPaymentConfigService(f.client, settings, key)
	svc := service.NewPaymentService(f.client, payment.NewRegistry(), payment.NewDefaultLoadBalancer(f.client, key),
		nil, nil, config, NewUserRepository(f.client, integrationDB), nil, nil)
	h := handler.NewPaymentHandler(svc, config)

	checkout := func(t *testing.T) map[string]json.RawMessage {
		t.Helper()
		router := gin.New()
		router.GET("/api/v1/payment/checkout-info", h.GetCheckoutInfo)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/payment/checkout-info", nil))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		var result struct {
			Data struct {
				Methods map[string]json.RawMessage `json:"methods"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		return result.Data.Methods
	}
	create := func(t *testing.T, method string) *httptest.ResponseRecorder {
		t.Helper()
		user, err := f.client.User.Create().SetEmail(fmt.Sprintf("enablement-%d@fixture.invalid", time.Now().UnixNano())).
			SetPasswordHash("fixture").Save(ctx)
		require.NoError(t, err)
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: user.ID})
		})
		router.POST("/api/v1/payment/orders", h.CreateOrder)
		body := fmt.Sprintf(`{"amount":25,"payment_type":%q,"order_type":"balance"}`, method)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/payment/orders", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}

	t.Run("advertised methods accept orders despite legacy false defaults", func(t *testing.T) {
		methods := checkout(t)
		require.Contains(t, methods, payment.TypeAlipay)
		require.Contains(t, methods, payment.TypeWxpay)
		for _, method := range []string{payment.TypeAlipay, payment.TypeWxpay, payment.TypeAlipayDirect, payment.TypeWxpayDirect} {
			t.Run(method, func(t *testing.T) {
				before := f.createCalls.Load()
				response := create(t, method)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var result struct {
					Data service.CreateOrderResponse `json:"data"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
				require.Positive(t, result.Data.OrderID)
				require.Equal(t, payment.OrderStatusPending, result.Data.Status)
				require.NotEmpty(t, result.Data.PayURL)
				require.Equal(t, before+1, f.createCalls.Load())
				order, err := f.client.PaymentOrder.Get(ctx, result.Data.OrderID)
				require.NoError(t, err)
				require.Equal(t, service.NormalizeVisibleMethod(method), order.PaymentType)
				require.Equal(t, 25.0, order.PayAmount)
				require.Nil(t, order.PaidAt)
			})
		}
	})
	t.Run("global payment shutdown still rejects orders", func(t *testing.T) {
		require.NoError(t, settings.Set(ctx, service.SettingPaymentEnabled, "false"))
		t.Cleanup(func() { require.NoError(t, settings.Set(ctx, service.SettingPaymentEnabled, "true")) })
		before := f.createCalls.Load()
		response := create(t, payment.TypeAlipay)
		require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"reason":"PAYMENT_DISABLED"`)
		require.Equal(t, before, f.createCalls.Load())
	})
	t.Run("balance recharge shutdown still rejects orders", func(t *testing.T) {
		require.NoError(t, settings.Set(ctx, service.SettingBalancePayDisabled, "true"))
		t.Cleanup(func() { require.NoError(t, settings.Set(ctx, service.SettingBalancePayDisabled, "false")) })
		before := f.createCalls.Load()
		response := create(t, payment.TypeAlipay)
		require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), `"reason":"BALANCE_PAYMENT_DISABLED"`)
		require.Equal(t, before, f.createCalls.Load())
	})
	t.Run("provider method buttons control checkout and direct orders", func(t *testing.T) {
		_, err := f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetSupportedTypes("alipay").Save(ctx)
		require.NoError(t, err)
		methods := checkout(t)
		require.Contains(t, methods, payment.TypeAlipay)
		require.NotContains(t, methods, payment.TypeWxpay)
		before := f.createCalls.Load()
		response := create(t, payment.TypeWxpay)
		require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
		require.Equal(t, before, f.createCalls.Load())
		response = create(t, payment.TypeAlipay)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Equal(t, before+1, f.createCalls.Load())
		_, err = f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetSupportedTypes("alipay,wxpay").Save(ctx)
		require.NoError(t, err)
	})
	t.Run("disabled provider disappears and rejects direct orders", func(t *testing.T) {
		_, err := f.client.PaymentProviderInstance.UpdateOneID(f.instanceID).SetEnabled(false).Save(ctx)
		require.NoError(t, err)
		require.Empty(t, checkout(t))
		before := f.createCalls.Load()
		for _, method := range []string{payment.TypeAlipay, payment.TypeWxpay} {
			response := create(t, method)
			require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"reason":"PAYMENT_GATEWAY_ERROR"`)
		}
		require.Equal(t, before, f.createCalls.Load())
	})
}
