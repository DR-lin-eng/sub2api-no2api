//go:build unit

package provider

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
)

func TestWxpayNotificationCannotCrossMerchantIdentity(t *testing.T) {
	provider := &Wxpay{config: map[string]string{"mchId": "merchant-A", "appId": "native-A", "mpAppId": "public-account-A"}}
	for _, app := range []string{"native-A", "public-account-A"} {
		require.NoError(t, provider.validateNotificationIdentity(&payments.Transaction{Mchid: core.String("merchant-A"), Appid: core.String(app)}))
	}
	for _, transaction := range []*payments.Transaction{
		nil,
		{Mchid: core.String("merchant-B"), Appid: core.String("native-A")},
		{Mchid: core.String("merchant-A"), Appid: core.String("foreign-app")},
		{Mchid: core.String("merchant-A")},
	} {
		require.Error(t, provider.validateNotificationIdentity(transaction))
	}
}
