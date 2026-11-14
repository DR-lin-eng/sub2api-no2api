//go:build integration

package repository

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	stripe "github.com/stripe/stripe-go/v85"
)

func TestPaymentStripeWebhookPinnedMerchantPostgres(t *testing.T) {
	f := newPaymentRecoveryFixture(t)
	ctx := t.Context()
	var chosenID int64
	for _, secret := range []string{"wrong-merchant-secret", "correct-merchant-secret"} {
		config, err := json.Marshal(map[string]string{"secretKey": "fixture-only", "webhookSecret": secret, "currency": "CNY"})
		require.NoError(t, err)
		encrypted, err := payment.Encrypt(string(config), make([]byte, payment.AES256KeySize))
		require.NoError(t, err)
		inst, err := f.client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeStripe).
			SetName("stripe fixture").SetConfig(encrypted).SetSupportedTypes(payment.TypeStripe).SetEnabled(true).Save(ctx)
		require.NoError(t, err)
		chosenID = inst.ID
	}
	order := f.order(t, service.OrderStatusPending, payment.OrderTypeBalance, time.Now().Add(time.Hour))
	order, err := f.client.PaymentOrder.UpdateOneID(order.ID).SetPaymentType(payment.TypeStripe).
		SetProviderKey(payment.TypeStripe).SetProviderInstanceID(strconv.FormatInt(chosenID, 10)).
		SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_key": payment.TypeStripe,
			"provider_instance_id": strconv.FormatInt(chosenID, 10), "currency": "CNY"}).Save(ctx)
	require.NoError(t, err)
	router := gin.New()
	router.POST("/stripe", handler.NewPaymentWebhookHandler(f.svc, payment.NewRegistry()).StripeWebhook)
	payload, err := json.Marshal(map[string]any{
		"id": "evt_fixture", "object": "event", "type": "payment_intent.succeeded", "api_version": stripe.APIVersion,
		"data": map[string]any{"object": map[string]any{
			"id": "pi_fixture", "object": "payment_intent", "amount": 2500, "currency": "cny", "status": "succeeded",
			"metadata": map[string]string{"orderId": order.OutTradeNo},
		}},
	})
	require.NoError(t, err)
	for _, secret := range []string{"wrong-merchant-secret", "correct-merchant-secret"} {
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(secret))
		_, _ = mac.Write(append([]byte(timestamp+"."), payload...))
		req := httptest.NewRequest(http.MethodPost, "/stripe", strings.NewReader(string(payload)))
		req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%s,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil))))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if secret == "wrong-merchant-secret" {
			require.Equal(t, http.StatusBadRequest, response.Code, "the other merchant cannot credit this order")
		} else {
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		}
	}
	f.assertResult(t, order, service.OrderStatusCompleted, true)
}
