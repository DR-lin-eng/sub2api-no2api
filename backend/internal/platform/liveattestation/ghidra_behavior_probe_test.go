//go:build darwin

package liveattestation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// This probe uses only the provider API/fields already present in main. The
// same source compiles before the change, after it, and after rollback.
func TestGhidraAttestationMissingNativeModuleBehavior(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("Apple Silicon callback behavior")
	}
	node := os.Getenv("SUB2API_TEST_NODE_PATH")
	if node == "" {
		t.Skip("explicit Node runtime required")
	}
	app := filepath.Join(t.TempDir(), "Fixture.app")
	bin := filepath.Join(app, "Contents", "Resources", "cua_node", "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(node, filepath.Join(bin, "node")); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>CFBundleIdentifier</key><string>com.openai.codex</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := &darwinProvider{appSessionID: "fixture-app-session", appPaths: []string{app}}
	header := CodexHeader(context.Background(), provider)
	var response struct {
		Status int    `json:"s"`
		Token  string `json:"t"`
	}
	if err := json.Unmarshal([]byte(header), &response); err != nil {
		t.Fatal(err)
	}
	var body []byte
	if response.Token != "" {
		var err error
		body, err = base64.RawURLEncoding.DecodeString(response.Token[3:])
		if err != nil {
			t.Fatal(err)
		}
	}
	reportedFailure := bytes.Contains(body, append([]byte{0x6a}, append([]byte("error_code"), byte(4))...))
	signalsPresent := bytes.Contains(body, []byte("fixture-app-session"))
	_, legacyErr := provider.Generate(context.Background())
	legacyRejects := legacyErr != nil
	fmt.Printf("GHIDRA_BEHAVIOR callback_status=%d native_load_error_4=%t real_signals_present=%t legacy_live_rejects=%t\n", response.Status, reportedFailure, signalsPresent, legacyRejects)
	if response.Status != 0 || !reportedFailure || !signalsPresent || !legacyRejects {
		t.Fatal("native-load failure must be a truthful error_code=4 callback; Live must still reject it")
	}
}
