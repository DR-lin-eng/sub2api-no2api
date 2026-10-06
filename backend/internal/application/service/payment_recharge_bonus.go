package service

import (
	"context"
	"fmt"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/modules/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/shopspring/decimal"
)

const SettingRechargeBonusEnabled = "RECHARGE_BONUS_ENABLED"
const RechargeBonusModeBonus = payment.RechargeBonusModeBonus
const RechargeBonusModeDiscount = payment.RechargeBonusModeDiscount

type RechargeBonusTier = payment.RechargeBonusTier
type rechargeBonusQuote = payment.RechargeBonusQuote

func NormalizeRechargeBonusMode(s string) (string, bool) {
	return payment.NormalizeRechargeBonusMode(s)
}
func ValidateRechargeBonusTiersForMode(s string, t []RechargeBonusTier) error {
	return payment.ValidateRechargeBonusTiersForMode(s, t)
}
func NormalizeRechargeBonusTiers(t []RechargeBonusTier) ([]RechargeBonusTier, error) {
	return payment.NormalizeRechargeBonusTiers(t)
}
func encodeRechargeBonusTiers(t []RechargeBonusTier) (string, error) {
	return payment.EncodeRechargeBonusTiers(t)
}
func parseRechargeBonusTiers(s string) []RechargeBonusTier { return payment.ParseRechargeBonusTiers(s) }
func validateRechargeBonusNotice(s string) error           { return payment.ValidateRechargeBonusNotice(s) }
func quoteRechargeBonus(cfg *PaymentConfig, a float64, c string) rechargeBonusQuote {
	var domain *payment.RechargeBonusConfig
	if cfg != nil {
		domain = &payment.RechargeBonusConfig{Enabled: cfg.RechargeBonusEnabled, BalanceRechargeMultiplier: cfg.BalanceRechargeMultiplier, RechargeBonusTiers: cfg.RechargeBonusTiers, RechargeBonusMode: cfg.RechargeBonusMode}
	}
	return payment.QuoteRechargeBonus(domain, a, c)
}

const (
	// SettingRechargeBonusTiers 存 JSON 数组（RechargeBonusTier 列表），空/缺失表示未启用优惠。
	SettingRechargeBonusTiers = "RECHARGE_BONUS_TIERS"
	// SettingRechargeBonusNotice 充值页金额卡顶部展示的 Markdown 活动文案，空表示不展示。
	SettingRechargeBonusNotice = "RECHARGE_BONUS_NOTICE"
	// SettingRechargeBonusMode 阶梯模式：bonus / discount；空/非法按 bonus 解析（兼容早期配置）。
	SettingRechargeBonusMode = "RECHARGE_BONUS_MODE"
)

// resolveRechargeBonusUpdate 校验并归一化阶梯/模式更新。任一字段缺省时读取现值做交叉校验
// （折扣模式下所有档位百分比必须 < 100）。返回值仅在对应请求字段非 nil 时有意义。
func (s *PaymentConfigService) resolveRechargeBonusUpdate(ctx context.Context, req UpdatePaymentConfigRequest) (tiersValue string, modeValue string, err error) {
	if req.RechargeBonusTiers == nil && req.RechargeBonusMode == nil {
		return "", "", nil
	}
	stored := map[string]string{}
	if (req.RechargeBonusTiers == nil || req.RechargeBonusMode == nil) && s != nil && s.settingRepo != nil {
		stored, err = s.settingRepo.GetMultiple(ctx, []string{SettingRechargeBonusTiers, SettingRechargeBonusMode})
		if err != nil {
			return "", "", fmt.Errorf("get recharge bonus settings: %w", err)
		}
	}

	var tiers []RechargeBonusTier
	if req.RechargeBonusTiers != nil {
		tiers, err = NormalizeRechargeBonusTiers(*req.RechargeBonusTiers)
		if err != nil {
			return "", "", infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", err.Error())
		}
	} else {
		tiers = parseRechargeBonusTiers(stored[SettingRechargeBonusTiers])
	}

	var mode string
	if req.RechargeBonusMode != nil {
		normalized, ok := NormalizeRechargeBonusMode(*req.RechargeBonusMode)
		if !ok {
			return "", "", infraerrors.BadRequest("INVALID_RECHARGE_BONUS_MODE", "recharge bonus mode must be bonus or discount")
		}
		mode = normalized
	} else {
		mode, _ = NormalizeRechargeBonusMode(stored[SettingRechargeBonusMode])
	}

	if err := ValidateRechargeBonusTiersForMode(mode, tiers); err != nil {
		return "", "", infraerrors.BadRequest("INVALID_RECHARGE_BONUS_TIERS", err.Error())
	}
	tiersValue, err = encodeRechargeBonusTiers(tiers)
	if err != nil {
		return "", "", err
	}
	return tiersValue, mode, nil
}

// paymentOrderAmountWithoutBonus 订单到账金额剔除免费额度后的实付部分（USD），用于推广返利基数。
func paymentOrderAmountWithoutBonus(o *dbent.PaymentOrder) float64 {
	if o == nil {
		return 0
	}
	if o.OrderType != payment.OrderTypeBalance || o.BonusAmount <= 0 {
		return o.Amount
	}
	base := decimal.NewFromFloat(o.Amount).
		Sub(decimal.NewFromFloat(o.BonusAmount)).
		Round(2).
		InexactFloat64()
	if base < 0 {
		return 0
	}
	return base
}
