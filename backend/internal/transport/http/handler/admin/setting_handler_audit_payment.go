package admin

import "github.com/DR-lin-eng/sub2api-no2api/internal/application/service"

func appendPaymentSettingChanges(changed []string, before, after *service.SystemSettings) []string {
	if before.PaymentVisibleMethodAlipaySource != after.PaymentVisibleMethodAlipaySource {
		changed = append(changed, "payment_visible_method_alipay_source")
	}
	if before.PaymentVisibleMethodWxpaySource != after.PaymentVisibleMethodWxpaySource {
		changed = append(changed, "payment_visible_method_wxpay_source")
	}
	return changed
}
