package service

import "testing"

func TestOpenAISubscriptionLongContextExemptUsesSentModelAndCredentialType(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account *Account
		result  *OpenAIForwardResult
		want    bool
	}{
		{"oauth target", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, &OpenAIForwardResult{Model: "custom", UpstreamModel: "gpt-6.1-sol-low"}, true},
		{"setup token target", &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, &OpenAIForwardResult{Model: "gpt-6.1-sol"}, true},
		{"api key stays priced", &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, &OpenAIForwardResult{Model: "gpt-6.1-sol"}, false},
		{"billing label does not decide", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, &OpenAIForwardResult{Model: "gpt-6-sol", BillingModel: "gpt-6.1-sol"}, false},
		{"invalid suffix", &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, &OpenAIForwardResult{Model: "gpt-6.1-sol-preview"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := openAISubscriptionLongContextExempt(tc.account, tc.result); got != tc.want {
				t.Fatalf("exempt = %t, want %t", got, tc.want)
			}
		})
	}
}

func BenchmarkSubscriptionLongContextExemption(b *testing.B) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	result := &OpenAIForwardResult{UpstreamModel: "gpt-6.1-sol"}
	b.ReportAllocs()
	for b.Loop() {
		if !openAISubscriptionLongContextExempt(account, result) {
			b.Fatal("missing exemption")
		}
	}
}
