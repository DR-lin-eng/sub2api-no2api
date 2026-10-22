package service

import (
	"context"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
	"net/http"
	"testing"
)

type accountProofBehaviorProvider struct{ scope string }

func (p accountProofBehaviorProvider) Check(context.Context) error { return nil }
func (p accountProofBehaviorProvider) Generate(context.Context) (string, error) {
	return `{"v":1,"s":0,"t":"v1.fixture"}`, nil
}
func (p accountProofBehaviorProvider) GenerateCodexToken(context.Context) (string, error) {
	return p.scope, nil
}

// Structural optional interface: identical probe compiles before and after implementation.
func (p accountProofBehaviorProvider) ForScope(scope string) liveattestation.Provider {
	return accountProofBehaviorProvider{scope: scope}
}

func TestAccountProofBindingBehaviorProbe(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIOAuthAttestationEnabled = true
	svc := &OpenAIGatewayService{cfg: cfg, liveAttestation: accountProofBehaviorProvider{scope: "global-unscoped"}}
	a := &Account{ID: 901, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "workspace-a"}, Extra: map[string]any{"openai_device_id": "installation-a"}}
	b := &Account{ID: 902, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "workspace-b"}, Extra: map[string]any{"openai_device_id": "installation-b"}}
	run := func(a *Account) string {
		h := make(http.Header)
		h.Set("thread-id", "wire-thread")
		h.Set("x-codex-installation-id", "untrusted-installation")
		_, err := svc.prepareOpenAIOAuthProof(context.Background(), a, "https://chatgpt.com/backend-api/codex/responses", h)
		if err != nil {
			t.Fatal(err)
		}
		return h.Get("x-oai-attestation")
	}
	first := run(a)
	different := first != run(b)
	stable := first == run(a)
	changed := *a
	changed.Extra = map[string]any{"openai_device_id": "installation-a-new"}
	rotated := first != run(&changed)
	unknownA := &Account{ID: 911, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_device_id": "shared-installation"}}
	unknownB := &Account{ID: 912, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_device_id": "shared-installation"}}
	isolated := openAIIntegrityKey(unknownA, unknownA, "https://chatgpt.com") != openAIIntegrityKey(unknownB, unknownB, "https://chatgpt.com")
	fmt.Printf("ACCOUNT_PROOF_BEHAVIOR account_scopes_differ=%t same_scope_stable=%t installation_change_rescopes=%t missing_principal_state_isolated=%t\n", different, stable, rotated, isolated)
	if !different || !stable || !rotated || !isolated {
		t.Fatal("account proof binding differs from required isolation")
	}
}
