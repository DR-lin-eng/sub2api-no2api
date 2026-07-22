package service

import (
	"context"
	"testing"
)

func TestWithHTTPUpstreamProfile_DefaultKeepsContext(t *testing.T) {
	ctx := context.Background()
	got := WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileDefault)
	if got != ctx {
		t.Fatal("default profile should not wrap context")
	}
}

func TestWithHTTPUpstreamProfile_OpenAI(t *testing.T) {
	ctx := WithHTTPUpstreamProfile(context.TODO(), HTTPUpstreamProfileOpenAI)
	if profile := HTTPUpstreamProfileFromContext(ctx); profile != HTTPUpstreamProfileOpenAI {
		t.Fatalf("expected profile %q, got %q", HTTPUpstreamProfileOpenAI, profile)
	}
}

func TestWithHTTPUpstreamRedirectsDisabled(t *testing.T) {
	//nolint:staticcheck // Exercises the defensive nil-context fallback.
	ctx := WithHTTPUpstreamRedirectsDisabled(nil)
	if !HTTPUpstreamRedirectsDisabled(ctx) {
		t.Fatal("expected redirects to be disabled")
	}
	if HTTPUpstreamRedirectsDisabled(context.Background()) {
		t.Fatal("redirects should remain enabled by default")
	}
}

func TestOpenAIHTTPUpstreamProfile_StreamOnly(t *testing.T) {
	apiKey := &Account{Type: AccountTypeAPIKey}
	oauth := &Account{Type: AccountTypeOAuth}

	if got := openAIHTTPUpstreamProfile(context.Background(), apiKey, true); got != HTTPUpstreamProfileOpenAIStream {
		t.Fatalf("API-key stream profile = %q, want %q", got, HTTPUpstreamProfileOpenAIStream)
	}
	if got := openAIHTTPUpstreamProfile(context.Background(), apiKey, false); got != HTTPUpstreamProfileOpenAI {
		t.Fatalf("API-key non-stream profile = %q, want %q", got, HTTPUpstreamProfileOpenAI)
	}
	if got := openAIHTTPUpstreamProfile(context.Background(), oauth, true); got != HTTPUpstreamProfileOpenAIStream {
		t.Fatalf("OAuth stream profile = %q, want %q", got, HTTPUpstreamProfileOpenAIStream)
	}
	imageCtx := WithOpenAIImageGenerationIntent(context.Background())
	if got := openAIHTTPUpstreamProfile(imageCtx, oauth, true); got != HTTPUpstreamProfileOpenAI {
		t.Fatalf("image generation stream profile = %q, want %q", got, HTTPUpstreamProfileOpenAI)
	}
}
