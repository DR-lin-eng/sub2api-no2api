package liveattestation

import (
	"context"
	"errors"
	"testing"
)

func TestRequestScopeIsImmutableAndCanBeCleared(t *testing.T) {
	a := RequestScope{Key: "account-a", InstallationID: "installation-a", ThreadID: "thread-a"}
	b := RequestScope{Key: "account-b", InstallationID: "installation-b", ThreadID: "thread-b"}
	parent := WithRequestScope(context.Background(), a)
	child := WithRequestScope(parent, b)
	if got, ok := RequestScopeFromContext(parent); !ok || got != a {
		t.Fatal("rebinding changed another request's receipt")
	}
	if got, ok := RequestScopeFromContext(child); !ok || got != b {
		t.Fatal("new attempt did not replace the previous binding")
	}
	if _, ok := RequestScopeFromContext(WithRequestScope(child, RequestScope{})); ok {
		t.Fatal("empty binding must clear an inherited receipt")
	}
}

func TestNativeScopedProviderRejectsAnotherIdentityBeforeNativeIO(t *testing.T) {
	provider := ProviderForScope(NewProvider(), "account-a")
	wrong := WithRequestScope(context.Background(), RequestScope{Key: "account-b", InstallationID: "installation-b"})
	if err := provider.Check(wrong); !errors.Is(err, ErrScopeMismatch) {
		t.Fatal("Check accepted another identity")
	}
	if _, err := provider.Generate(wrong); !errors.Is(err, ErrScopeMismatch) {
		t.Fatal("Live generator accepted another identity")
	}
	codexProvider, ok := provider.(CodexTokenProvider)
	if !ok {
		t.Fatal("scoped provider does not implement CodexTokenProvider")
	}
	token, err := codexProvider.GenerateCodexToken(wrong)
	_ = token
	if !errors.Is(err, ErrScopeMismatch) {
		t.Fatal("Codex generator accepted another identity")
	}
	rawProvider, ok := provider.(DeviceCheckTokenProvider)
	if !ok {
		t.Fatal("scoped provider does not implement DeviceCheckTokenProvider")
	}
	rawToken, err := rawProvider.GenerateDeviceCheckToken(wrong)
	_ = rawToken
	if !errors.Is(err, ErrScopeMismatch) {
		t.Fatal("raw generator accepted another identity")
	}
	lateToken, err := codexProvider.GenerateCodexToken(context.Background())
	_ = lateToken
	if !errors.Is(err, ErrScopeMismatch) {
		t.Fatal("scoped provider accepted a missing identity")
	}
}
