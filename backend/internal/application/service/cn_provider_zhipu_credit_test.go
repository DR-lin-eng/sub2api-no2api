//go:build unit

package service

import (
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"testing"
)

// TestParseZhipuTokenTiers_CreditLimitUnitClassification GLM Coding Plan 当前把 5h / weekly
// 两条窗口都标成 CREDIT_LIMIT（实测 level=lite）。CREDIT_LIMIT 同样携带 unit，
// 必须和 TOKENS_LIMIT 一样按 unit 显式分类，否则会退化成 reset 时间排序 ——
// 此处刻意让 weekly 的 nextResetTime 早于 5h（周期末尾），两档会被标反。
func TestParseZhipuTokenTiers_CreditLimitUnitClassification(t *testing.T) {
	t.Parallel()
	data := gjson.Parse(`{
		"limits": [
			{"type":"CREDIT_LIMIT","unit":3,"number":5,"percentage":14,"nextResetTime":1700000099999},
			{"type":"CREDIT_LIMIT","unit":6,"number":1,"percentage":90,"nextResetTime":1700000000000}
		]
	}`)
	tiers := parseZhipuTokenTiers(data)
	require.Len(t, tiers, 2)
	require.Equal(t, "5h", tiers[0].Window)
	require.InDelta(t, 14.0, tiers[0].UsedPercent, 1e-9)
	require.Equal(t, "weekly", tiers[1].Window)
	require.InDelta(t, 90.0, tiers[1].UsedPercent, 1e-9)
}

// TestParseZhipuTokenTiers_CreditLimitWithoutUnitFallback CREDIT_LIMIT 缺 unit 时，
// 保留原有 fallback：无 nextResetTime 的优先归 5h，其余按 reset 升序填入剩余槽位。
func TestParseZhipuTokenTiers_CreditLimitWithoutUnitFallback(t *testing.T) {
	t.Parallel()
	data := gjson.Parse(`{
		"limits": [
			{"type":"CREDIT_LIMIT","percentage":30,"nextResetTime":1700000000000},
			{"type":"CREDIT_LIMIT","percentage":80}
		]
	}`)
	tiers := parseZhipuTokenTiers(data)
	require.Len(t, tiers, 2)
	require.Equal(t, "5h", tiers[0].Window)
	require.InDelta(t, 80.0, tiers[0].UsedPercent, 1e-9)
	require.Equal(t, "weekly", tiers[1].Window)
	require.InDelta(t, 30.0, tiers[1].UsedPercent, 1e-9)
}

// TestParseZhipuTokenTiers_TokensLimitWinsOverCreditLimit 两种 type 同时返回时，
// TOKENS_LIMIT 仍然优先占槽、CREDIT_LIMIT 不参与（原有语义保持不变）。
func TestParseZhipuTokenTiers_TokensLimitWinsOverCreditLimit(t *testing.T) {
	t.Parallel()
	data := gjson.Parse(`{
		"limits": [
			{"type":"TOKENS_LIMIT","unit":3,"percentage":20,"nextResetTime":1700000000000},
			{"type":"CREDIT_LIMIT","unit":6,"percentage":99,"nextResetTime":1700000099999}
		]
	}`)
	tiers := parseZhipuTokenTiers(data)
	require.Len(t, tiers, 1)
	require.Equal(t, "5h", tiers[0].Window)
	require.InDelta(t, 20.0, tiers[0].UsedPercent, 1e-9)
}
