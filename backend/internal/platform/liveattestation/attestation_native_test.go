package liveattestation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestNativeCodexCallback(t *testing.T) {
	if os.Getenv("SUB2API_TEST_NATIVE_DEVICECHECK") != "1" {
		t.Skip("explicit native probe required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	header := CodexHeader(ctx, NewProvider())
	if !ValidCodexHeader(header) {
		t.Fatal("invalid Codex callback envelope")
	}
	var parsed struct {
		Status int    `json:"s"`
		Token  string `json:"t"`
	}
	if json.Unmarshal([]byte(header), &parsed) != nil {
		t.Fatal("invalid callback response")
	}
	deviceError := false
	if parsed.Token != "" {
		payload, err := base64.RawURLEncoding.DecodeString(parsed.Token[3:])
		if err != nil {
			t.Fatal("invalid native CBOR encoding")
		}
		deviceError = bytes.Contains(payload, []byte("error_code"))
	}
	t.Logf("NATIVE_CODEX_CALLBACK_STATUS=%d DEVICE_ERROR_REPORTED=%t APPLE_PROOF_VERIFIED=%t", parsed.Status, deviceError, parsed.Status == 0 && !deviceError)
}

// Opt in explicitly. Ordinary unit/integration tests never contact Apple or
// read an installed desktop runtime. Neither token is printed or written.
func TestNativeDeviceCheckGeneration(t *testing.T) {
	if os.Getenv("SUB2API_TEST_NATIVE_DEVICECHECK") != "1" || runtime.GOOS != "darwin" {
		t.Skip("native DeviceCheck opt-in required")
	}
	provider := NewProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := provider.Check(ctx); err != nil {
		t.Fatalf("native runtime unavailable: %v", err)
	}
	header, err := provider.Generate(ctx)
	if err != nil {
		t.Fatalf("native envelope generation failed: %v", err)
	}
	if !ValidCodexHeader(header) {
		t.Fatal("native envelope has invalid format")
	}
	var parsed struct {
		Status int    `json:"s"`
		Token  string `json:"t"`
	}
	if json.Unmarshal([]byte(header), &parsed) != nil || parsed.Status != 0 || parsed.Token == "" {
		t.Fatal("native proof was not generated successfully")
	}
	rawProvider, ok := provider.(DeviceCheckTokenProvider)
	if !ok {
		t.Fatal("raw DeviceCheck provider missing")
	}
	raw, err := rawProvider.GenerateDeviceCheckToken(ctx)
	if err != nil {
		t.Fatalf("native raw token generation failed: %v", err)
	}
	if _, err := BuildDeviceCheckHeaders(raw, "com.openai.codex"); err != nil {
		t.Fatal("native raw token projection invalid")
	}
	t.Logf("NATIVE_DEVICECHECK_SUPPORTED=true ATTESTATION_STATUS=0 ENVELOPE_BYTES=%d RAW_TOKEN_BYTES=%d", len(header), len(raw))
}
