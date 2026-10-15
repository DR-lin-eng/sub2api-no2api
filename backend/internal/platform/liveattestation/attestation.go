package liveattestation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

var (
	ErrUnsupportedPlatform = errors.New("live attestation is only supported when Sub2API runs on macOS; Windows support is not implemented yet")
	ErrChatGPTAppMissing   = errors.New("live attestation requires the official ChatGPT app on the Sub2API server")
)

// Provider 在发起 Live 请求前生成 ChatGPT DeviceCheck attestation。
type Provider interface {
	Check(ctx context.Context) error
	Generate(ctx context.Context) (string, error)
}

// DeviceCheckTokenProvider exposes the raw Apple DeviceCheck token for the
// desktop messaging/challenge wire (`x-sentinel-dc`). It is optional so the
// existing Live attestation provider contract remains source-compatible.
// Callers must add the token only on the two allow-listed desktop endpoints;
// it is not a general OAuth Responses header.
type DeviceCheckTokenProvider interface {
	GenerateDeviceCheckToken(ctx context.Context) (string, error)
}

// BuildDeviceCheckHeaders builds the exact desktop wire projection. The
// caller is responsible for enforcing the endpoint allow-list and for using
// the same Electron cookie/session context as the desktop client.
func BuildDeviceCheckHeaders(token, bundleID string) (http.Header, error) {
	if token != strings.TrimSpace(token) {
		return nil, errors.New("invalid DeviceCheck token whitespace")
	}
	bundleID = strings.TrimSpace(bundleID)
	if len(token) < 20 || len(token) > 16*1024 || strings.ContainsAny(token, "\r\n \t") {
		return nil, errors.New("invalid DeviceCheck token")
	}
	if decoded, err := base64.StdEncoding.DecodeString(token); err != nil || len(decoded) < 16 {
		return nil, errors.New("invalid DeviceCheck token encoding")
	}
	if bundleID != "com.openai.codex" {
		return nil, errors.New("DeviceCheck bundle identifier is required")
	}
	payload, err := json.Marshal(map[string]string{"token": token})
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	headers.Set("x-sentinel-dc", string(payload))
	headers.Set("OAI-Package-Name", bundleID)
	return headers, nil
}
