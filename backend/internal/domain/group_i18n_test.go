package domain

import "testing"

func TestGroupI18nLocalizeUsesPreferredLanguageAndFallback(t *testing.T) {
	m := GroupI18n{
		"zh":    {Name: "中文名称", Description: "中文描述"},
		"en-US": {Name: "English name"},
	}
	name, description := m.Localize("fr;q=0.9,en-US;q=0.8,zh;q=0.7", "默认名称", "默认描述")
	if name != "默认名称" || description != "默认描述" {
		t.Fatalf("unsupported highest language must preserve defaults: %q %q", name, description)
	}
	name, description = m.Localize("en-GB,en;q=0.8", "默认名称", "默认描述")
	if name != "默认名称" || description != "默认描述" {
		t.Fatalf("regional language must not silently choose a lower-ranked primary language: %q %q", name, description)
	}
	name, description = m.Localize("zh-TW, en;q=0.1", "默认名称", "默认描述")
	if name != "中文名称" || description != "中文描述" {
		t.Fatalf("primary-language fallback failed: %q %q", name, description)
	}
}

func TestCanonicalLocale(t *testing.T) {
	if got := CanonicalLocale("zh-hant-tw"); got != "zh-Hant-TW" {
		t.Fatalf("CanonicalLocale() = %q", got)
	}
}
