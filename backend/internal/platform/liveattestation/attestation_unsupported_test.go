//go:build !darwin

package liveattestation

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedProviderReturnsExplicitPlatformError(t *testing.T) {
	provider := NewProvider()
	if err := provider.Check(context.Background()); !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Check() error = %v, want ErrUnsupportedPlatform", err)
	}
	_, err := provider.Generate(context.Background())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Generate() error = %v, want ErrUnsupportedPlatform", err)
	}
	rawProvider, ok := provider.(DeviceCheckTokenProvider)
	if !ok {
		t.Fatal("unsupported provider does not expose DeviceCheckTokenProvider")
	}
	_, err = rawProvider.GenerateDeviceCheckToken(context.Background())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("GenerateDeviceCheckToken() error = %v, want ErrUnsupportedPlatform", err)
	}
}

func TestBuildDeviceCheckHeaders(t *testing.T) {
	headers, err := BuildDeviceCheckHeaders("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", "com.openai.codex")
	if err != nil {
		t.Fatal(err)
	}
	if got := headers.Get("x-sentinel-dc"); got != `{"token":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="}` {
		t.Fatalf("x-sentinel-dc = %q", got)
	}
	if got := headers.Get("OAI-Package-Name"); got != "com.openai.codex" {
		t.Fatalf("OAI-Package-Name = %q", got)
	}
}

func TestLinuxCodexHeaderWrapsUnsupportedTokenAsOpaqueSuccess(t *testing.T) {
	want := `{"v":1,"s":0,"t":"v1.omplcnJvcl9jb2RlAWlidW5kbGVfaWRwY29tLm9wZW5haS5jb2RleA"}`
	if got := CodexHeader(context.Background(), NewProvider()); got != want {
		t.Fatalf("Linux x-oai-attestation = %s, want %s", got, want)
	}
}
