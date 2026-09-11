package service

import "testing"

func TestOpenAISessionIDRateLimitAppliesOnlyToOpenAIOAuth(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	otherPlatform := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}

	if !openAISessionIDRateLimitApplies(oauth) {
		t.Fatal("OpenAI OAuth account should be rate-limited")
	}
	if openAISessionIDRateLimitApplies(apiKey) {
		t.Fatal("OpenAI API key account must bypass Session ID rate limiting")
	}
	if openAISessionIDRateLimitApplies(otherPlatform) {
		t.Fatal("non-OpenAI account must bypass Session ID rate limiting")
	}
}
