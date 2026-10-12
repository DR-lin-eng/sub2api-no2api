package setup

import (
	"strings"
	"testing"
)

func TestAdminCredentialLoginCompatibility(t *testing.T) {
	t.Parallel()
	for _, email := range []string{"a@b", "Owner <owner@example.com>", "<owner@example.com>", strings.Repeat("a", 250) + "@example.com"} {
		if validateEmail(email) {
			t.Errorf("setup accepted an email rejected by login: %q", email)
		}
	}
	if !validateEmail("owner@example.com") {
		t.Error("valid login email rejected")
	}
	for _, tc := range []struct {
		name     string
		password string
		valid    bool
	}{
		{"short", "1234567", false},
		{"minimum", "12345678", true},
		{"bcrypt maximum", strings.Repeat("a", 72), true},
		{"over maximum", strings.Repeat("a", 73), false},
		{"UTF-8 at maximum", strings.Repeat("密", 24), true},
		{"UTF-8 over maximum", strings.Repeat("密", 25), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if valid := validatePassword(tc.password) == nil; valid != tc.valid {
				t.Fatalf("valid=%v, want %v for %d UTF-8 bytes", valid, tc.valid, len(tc.password))
			}
		})
	}
}
