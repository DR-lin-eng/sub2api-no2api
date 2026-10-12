package service

import "testing"

func TestNormalizeGroupI18nRejectsDuplicateCanonicalLocales(t *testing.T) {
	_, err := normalizeGroupI18n(GroupI18n{
		"en": {Name: "one"},
		"EN": {Name: "two"},
	})
	if err == nil {
		t.Fatal("expected duplicate locale error")
	}
}

func TestNormalizeGroupI18nTrimsAndDropsEmptyEntries(t *testing.T) {
	got, err := normalizeGroupI18n(GroupI18n{
		"en-us": {Name: "  English  ", Description: "  "},
		"zh":    {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["en-US"].Name != "English" || got["en-US"].Description != "" {
		t.Fatalf("normalized i18n = %#v", got)
	}
}
