package setup

import "testing"

func BenchmarkAdminEmailValidation(b *testing.B) {
	if !validateEmail("owner@example.com") {
		b.Fatal("valid login email rejected")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !validateEmail("owner@example.com") {
			b.Fatal("valid login email rejected")
		}
	}
}

func BenchmarkAdminPasswordValidation(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := validatePassword("synthetic-admin-password"); err != nil {
			b.Fatal(err)
		}
	}
}
