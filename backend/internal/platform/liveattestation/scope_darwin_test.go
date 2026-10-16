//go:build darwin

package liveattestation

import "testing"

func TestNativeAccountSessionsAreIsolatedWithoutMutatingRuntime(t *testing.T) {
	base := &darwinProvider{appSessionID: "process-session", appPaths: []string{"/real/runtime"}}
	aProvider := base.ForScope("account-a")
	bProvider := base.ForScope("account-b")
	a, ok := aProvider.(*darwinProvider)
	if !ok {
		t.Fatal("account-a provider lost its native type")
	}
	b, ok := bProvider.(*darwinProvider)
	if !ok {
		t.Fatal("account-b provider lost its native type")
	}
	if a.appSessionID == b.appSessionID || a.appSessionID == base.appSessionID {
		t.Fatal("account application sessions were merged")
	}
	aAgainProvider := base.ForScope("account-a")
	aFromBProvider := b.ForScope("account-a")
	aAgain, ok := aAgainProvider.(*darwinProvider)
	if !ok {
		t.Fatal("reselected account-a provider lost its native type")
	}
	aFromB, ok := aFromBProvider.(*darwinProvider)
	if !ok {
		t.Fatal("derived account-a provider lost its native type")
	}
	if a.appSessionID != aAgain.appSessionID || a.appSessionID != aFromB.appSessionID {
		t.Fatal("scoped application session is not stable within the runtime")
	}
	if base.appSessionID != "process-session" || base.scopeKey != "" || a.appPaths[0] != base.appPaths[0] || b.appPaths[0] != base.appPaths[0] {
		t.Fatal("selection mutated the original runtime or changed the hardware source")
	}
}
