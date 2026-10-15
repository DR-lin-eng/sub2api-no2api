package devicekey

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/remotecontrol"
)

type fixtureReader struct{ io.Reader }

func TestNativeOutputLimitAlsoAppliesToIOCopy(t *testing.T) {
	var output boundedOutput
	_, err := io.Copy(&output, fixtureReader{strings.NewReader(strings.Repeat("x", 40*1024))})
	if err == nil || !output.exceeded || output.buffer.Len() > 32*1024 {
		t.Fatal("native output exceeded the bounded writer")
	}
}

func TestNativeDeviceKeyRoundTrip(t *testing.T) {
	resources := os.Getenv("SUB2API_TEST_DEVICEKEY_RESOURCES")
	if resources == "" {
		t.Skip("explicit native key creation opt-in required")
	}
	provider := NewProvider(resources)
	if err := provider.Check(); err != nil {
		t.Fatalf("native runtime unavailable: %v", err)
	}
	policy := os.Getenv("SUB2API_TEST_DEVICEKEY_POLICY")
	if policy == "" {
		policy = HardwareOnly
	}
	key, err := provider.CreateDeviceKey(context.Background(), policy)
	if err != nil {
		t.Logf("NATIVE_DEVICE_KEY_AVAILABLE=false POLICY=%s KEY_CREATED=false PRIVATE_KEY_EXPORTED=false", policy)
		t.Skip("native device key creation unavailable")
	}
	defer func() {
		if err := provider.DeleteDeviceKey(context.Background(), key.KeyID); err != nil {
			t.Error("native test key cleanup failed")
		}
	}()
	public, err := provider.GetDeviceKeyPublic(context.Background(), key.KeyID)
	if err != nil || public != key {
		t.Fatal("native public key does not match created reference")
	}
	payload := []byte(`{"domain":"codex-device-key-sign-payload/v1","payload":{"type":"native-fixture"}}`)
	signature, err := provider.SignDeviceKey(context.Background(), key.KeyID, payload)
	if err != nil {
		t.Fatal("native signing failed")
	}
	if err := remotecontrol.VerifyDeviceSignature(public, payload, signature.SignatureDERBase64); err != nil {
		t.Fatal("native signature verification failed")
	}
	t.Logf("NATIVE_DEVICE_KEY_AVAILABLE=true PROTECTION_CLASS=%s SIGNATURE_VERIFIED=true PRIVATE_KEY_EXPORTED=false", key.ProtectionClass)
}

func TestUnavailableProviderDoesNotGenerateSoftwareKey(t *testing.T) {
	p := NewProvider(t.TempDir())
	if _, err := p.CreateDeviceKey(context.Background(), HardwareOnly); err == nil {
		t.Fatal("missing native provider unexpectedly created a key")
	}
	if _, err := p.CreateDeviceKey(context.Background(), "software"); err == nil {
		t.Fatal("unsupported key policy was accepted")
	}
}
