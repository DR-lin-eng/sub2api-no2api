package payment

import (
	"encoding/json"
	"fmt"
	"github.com/shopspring/decimal"
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// RechargeBonusConfig is the immutable pricing input; Enabled gates mixed-version rollout.
type RechargeBonusConfig struct {
	Enabled                   bool
	BalanceRechargeMultiplier float64
	RechargeBonusTiers        []RechargeBonusTier
	RechargeBonusMode         string
}

func rechargeBaseCredit(amount, multiplier float64) float64 {
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		multiplier = 1
	}
	return decimal.NewFromFloat(amount).Mul(decimal.NewFromFloat(multiplier)).Round(2).InexactFloat64()
}

const (
	RechargeBonusModeBonus    = "bonus"
	RechargeBonusModeDiscount = "discount"
)

const (
	MaxRechargeBonusTiers       = 20
	maxRechargeBonusPercent     = 1000
	maxRechargeBonusNoticeRunes = 10000
	rechargeBonusAmountEpsilon  = 1e-9
)

// RechargeBonusTier 一个优惠档位：支付金额 ≥ MinAmount 时按 BonusPercent% 赠送（bonus）或打折（discount）。
type RechargeBonusTier struct {
	MinAmount    float64 `json:"min_amount"`
	BonusPercent float64 `json:"bonus_percent"`
}

// NormalizeRechargeBonusMode 归一化模式；空按 bonus。第二个返回值表示输入是否合法。
func NormalizeRechargeBonusMode(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", RechargeBonusModeBonus:
		return RechargeBonusModeBonus, true
	case RechargeBonusModeDiscount:
		return RechargeBonusModeDiscount, true
	default:
		return RechargeBonusModeBonus, false
	}
}

// ValidateRechargeBonusTiersForMode 折扣模式下百分比必须 < 100，否则实付为 0 或负数。
func ValidateRechargeBonusTiersForMode(mode string, tiers []RechargeBonusTier) error {
	if mode != RechargeBonusModeDiscount {
		return nil
	}
	for _, tier := range tiers {
		if tier.BonusPercent >= 100 {
			return fmt.Errorf("discount percent must be less than 100 (tier with min amount %s)",
				decimal.NewFromFloat(tier.MinAmount).Round(2).String())
		}
	}
	return nil
}

func rechargeBonusValueValid(v float64, max float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > max {
		return false
	}
	d := decimal.NewFromFloat(v)
	return d.Equal(d.Round(2))
}

// NormalizeRechargeBonusTiers 严格归一化（写路径）：任何非法项直接报错；
// 成功时返回按 MinAmount 升序排序的副本。
func NormalizeRechargeBonusTiers(raw []RechargeBonusTier) ([]RechargeBonusTier, error) {
	if len(raw) == 0 {
		return []RechargeBonusTier{}, nil
	}
	if len(raw) > MaxRechargeBonusTiers {
		return nil, fmt.Errorf("recharge bonus tiers exceed limit of %d", MaxRechargeBonusTiers)
	}
	out := make([]RechargeBonusTier, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, tier := range raw {
		if !rechargeBonusValueValid(tier.MinAmount, math.MaxFloat64) {
			return nil, fmt.Errorf("recharge bonus tier min amount must be a non-negative number with at most 2 decimals")
		}
		if !rechargeBonusValueValid(tier.BonusPercent, maxRechargeBonusPercent) {
			return nil, fmt.Errorf("recharge bonus tier percent must be between 0 and %d with at most 2 decimals", maxRechargeBonusPercent)
		}
		key := decimal.NewFromFloat(tier.MinAmount).Round(2).String()
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("duplicate recharge bonus tier min amount: %s", key)
		}
		seen[key] = struct{}{}
		out = append(out, RechargeBonusTier{MinAmount: tier.MinAmount, BonusPercent: tier.BonusPercent})
	}
	sortRechargeBonusTiers(out)
	return out, nil
}

func sortRechargeBonusTiers(tiers []RechargeBonusTier) {
	sort.SliceStable(tiers, func(i, j int) bool {
		return tiers[i].MinAmount < tiers[j].MinAmount
	})
}

// EncodeRechargeBonusTiers 序列化为设置值；空列表存空串，与「未配置」保持同一形态。
func EncodeRechargeBonusTiers(tiers []RechargeBonusTier) (string, error) {
	if len(tiers) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(tiers)
	if err != nil {
		return "", fmt.Errorf("marshal recharge bonus tiers: %w", err)
	}
	return string(raw), nil
}

// ParseRechargeBonusTiers 宽松解析（读路径）：非法条目丢弃而非报错，避免历史错配置阻断下单。
// 同一 MinAmount 重复时保留先出现的档位。始终返回非 nil 切片，便于 JSON 输出为 []。
func ParseRechargeBonusTiers(raw string) []RechargeBonusTier {
	out := make([]RechargeBonusTier, 0)
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 64<<10 {
		return out
	}
	var items []RechargeBonusTier
	if err := json.Unmarshal([]byte(raw), &items); err != nil {

		return out
	}
	seen := make(map[string]struct{}, len(items))
	for _, tier := range items {
		if len(out) >= MaxRechargeBonusTiers {
			break
		}
		if !rechargeBonusValueValid(tier.MinAmount, math.MaxFloat64) || !rechargeBonusValueValid(tier.BonusPercent, maxRechargeBonusPercent) {
			continue
		}
		key := decimal.NewFromFloat(tier.MinAmount).Round(2).String()
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tier)
	}
	sortRechargeBonusTiers(out)
	return out
}

func ValidateRechargeBonusNotice(notice string) error {
	if utf8.RuneCountInString(notice) > maxRechargeBonusNoticeRunes {
		return fmt.Errorf("recharge bonus notice exceeds %d characters", maxRechargeBonusNoticeRunes)
	}
	return nil
}

// MatchRechargeBonusTier 返回不超过 paymentAmount 的最大档位；tiers 需已按 MinAmount 升序。
func MatchRechargeBonusTier(tiers []RechargeBonusTier, paymentAmount float64) (RechargeBonusTier, bool) {
	if math.IsNaN(paymentAmount) || math.IsInf(paymentAmount, 0) || paymentAmount <= 0 {
		return RechargeBonusTier{}, false
	}
	var matched RechargeBonusTier
	found := false
	for _, tier := range tiers {
		if paymentAmount+rechargeBonusAmountEpsilon < tier.MinAmount {
			break
		}
		matched = tier
		found = true
	}
	return matched, found
}

// CalculateRechargeBonus 赠送金额 = 到账基数 × 百分比，保留两位小数（四舍五入）。
func CalculateRechargeBonus(baseCredited, bonusPercent float64) float64 {
	if baseCredited <= 0 || bonusPercent <= 0 || math.IsNaN(baseCredited) || math.IsNaN(bonusPercent) {
		return 0
	}
	return decimal.NewFromFloat(baseCredited).
		Mul(decimal.NewFromFloat(bonusPercent)).
		Div(decimal.NewFromInt(100)).
		Round(2).
		InexactFloat64()
}

// AddRechargeBonus 到账总额 = 基数 + 赠送，两位小数。
func AddRechargeBonus(baseCredited, bonus float64) float64 {
	return decimal.NewFromFloat(baseCredited).
		Add(decimal.NewFromFloat(bonus)).
		Round(2).
		InexactFloat64()
}

// CalculateDiscountedPayBase 折扣模式实付基数 = 支付金额 × (1 − 百分比)，按币种精度四舍五入。
func CalculateDiscountedPayBase(paymentAmount, discountPercent float64, currency string) float64 {
	digits := int32(CurrencyMaxFractionDigits(currency))
	return decimal.NewFromFloat(paymentAmount).
		Mul(decimal.NewFromInt(100).Sub(decimal.NewFromFloat(discountPercent))).
		Div(decimal.NewFromInt(100)).
		Round(digits).
		InexactFloat64()
}

// rechargeBonusQuote 一笔余额充值的报价结果。
type RechargeBonusQuote struct {
	// PayBase 网关收款基数（支付币种，不含手续费）；赠金模式等于支付金额，折扣模式为折后金额。
	PayBase float64
	// Credited 到账总额（USD），含 Bonus。
	Credited float64
	// Bonus 免费额度（USD）：赠金模式为额外赠送，折扣模式为未付费却到账的部分。
	Bonus float64
	// Percent 命中档位的百分比；未命中或未产生优惠时为 0。
	Percent float64
}

// quoteRechargeBonus 按配置模式报价。currency 用于折扣模式实付基数的精度。
// 未配置阶梯、未命中、或折扣百分比 ≥ 100（非法历史数据，fail-safe）时按无优惠处理。
func QuoteRechargeBonus(cfg *RechargeBonusConfig, paymentAmount float64, currency string) RechargeBonusQuote {
	multiplier := 1.0
	var tiers []RechargeBonusTier
	mode := RechargeBonusModeBonus
	if cfg != nil {
		multiplier = cfg.BalanceRechargeMultiplier
		if cfg.Enabled {
			tiers = cfg.RechargeBonusTiers
		}
		mode, _ = NormalizeRechargeBonusMode(cfg.RechargeBonusMode)
	}
	base := rechargeBaseCredit(paymentAmount, multiplier)
	quote := RechargeBonusQuote{PayBase: paymentAmount, Credited: base}

	tier, ok := MatchRechargeBonusTier(tiers, paymentAmount)
	if !ok || tier.BonusPercent <= 0 {
		return quote
	}
	switch mode {
	case RechargeBonusModeDiscount:
		if tier.BonusPercent >= 100 {
			return quote
		}
		payBase := CalculateDiscountedPayBase(paymentAmount, tier.BonusPercent, currency)
		if payBase <= 0 || payBase >= paymentAmount {
			return quote
		}
		paidCredit := rechargeBaseCredit(payBase, multiplier)
		bonus := decimal.NewFromFloat(base).Sub(decimal.NewFromFloat(paidCredit)).Round(2).InexactFloat64()
		if bonus < 0 {
			bonus = 0
		}
		quote.PayBase = payBase
		quote.Bonus = bonus
		quote.Percent = tier.BonusPercent
	default:
		bonus := CalculateRechargeBonus(base, tier.BonusPercent)
		if bonus <= 0 {
			return quote
		}
		quote.Bonus = bonus
		quote.Credited = AddRechargeBonus(base, bonus)
		quote.Percent = tier.BonusPercent
	}
	return quote
}
