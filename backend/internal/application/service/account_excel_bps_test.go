package service

import "testing"

func TestAccountIsExcelBPSEnabledIsAccountScoped(t *testing.T) {
	base := func(extra map[string]any) *Account {
		return &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra}
	}
	if !base(map[string]any{ExcelBPSEnabledExtraKey: true}).IsExcelBPSEnabled() {
		t.Fatal("enabled OpenAI OAuth account should use Excel BPS")
	}
	if !base(map[string]any{"openai_excel_bps": true}).IsExcelBPSEnabled() {
		t.Fatal("legacy Excel BPS key should remain readable")
	}
	if !base(map[string]any{ExcelBPSEnabledExtraKey: "false", "openai_excel_bps": true}).IsExcelBPSEnabled() {
		t.Fatal("malformed new value should fall back to legacy boolean")
	}
	for name, account := range map[string]*Account{
		"disabled":          base(map[string]any{ExcelBPSEnabledExtraKey: false}),
		"legacy overridden": base(map[string]any{ExcelBPSEnabledExtraKey: false, "openai_excel_bps": true}),
		"malformed":         base(map[string]any{ExcelBPSEnabledExtraKey: "true"}),
		"apikey":            {Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{ExcelBPSEnabledExtraKey: true}},
		"shadow": func() *Account {
			id := int64(7)
			a := base(map[string]any{ExcelBPSEnabledExtraKey: true})
			a.ParentAccountID = &id
			return a
		}(),
		"pat": func() *Account {
			a := base(map[string]any{ExcelBPSEnabledExtraKey: true})
			a.Credentials = map[string]any{"auth_mode": OpenAIAuthModePersonalAccessToken}
			return a
		}(),
		"agent": func() *Account {
			a := base(map[string]any{ExcelBPSEnabledExtraKey: true})
			a.Credentials = map[string]any{"auth_mode": OpenAIAuthModeAgentIdentity}
			return a
		}(),
	} {
		if account.IsExcelBPSEnabled() {
			t.Fatalf("%s account must not use Excel BPS", name)
		}
	}
}
