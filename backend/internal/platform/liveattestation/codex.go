package liveattestation

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"strings"
)

// CodexTokenProvider corresponds to the desktop attestation/generate callback.
// Unsupported platforms return an error_code CBOR token, not an Apple token.
type CodexTokenProvider interface {
	GenerateCodexToken(context.Context) (string, error)
}

type deviceSignals struct {
	SchemaVersion      int      `json:"schemaVersion"`
	PreferredLanguages []string `json:"preferredLanguages"`
	Locale             string   `json:"locale"`
	Timezone           string   `json:"timezone"`
	ScreenSizeSum      int      `json:"screenSizeSum"`
	ScreenScale        float64  `json:"screenScale"`
	AppSessionID       string   `json:"appSessionId"`
}

func codexUnavailableToken(code byte) string {
	return codexUnavailableWithSignals(code, nil)
}

func codexUnavailableWithSignals(code byte, signals *deviceSignals) string {
	fields := byte(2)
	if signals != nil {
		fields++
	}
	body := []byte{0xa0 + fields}
	body = appendCBORText(body, "error_code")
	body = appendCBORHead(body, 0, uint32(code))
	body = appendCBORText(body, "bundle_id")
	body = appendCBORText(body, "com.openai.codex")
	if signals != nil {
		fingerprint := []byte{0xa7, 0x00}
		fingerprint = appendCBORHead(fingerprint, 0, uint32(max(0, signals.SchemaVersion)))
		fingerprint = append(fingerprint, 0x01)
		fingerprint = appendCBORHead(fingerprint, 0x80, uint32(len(signals.PreferredLanguages)))
		for _, language := range signals.PreferredLanguages {
			fingerprint = appendCBORText(fingerprint, language)
		}
		fingerprint = append(fingerprint, 0x02)
		fingerprint = appendCBORText(fingerprint, signals.Locale)
		fingerprint = append(fingerprint, 0x03)
		fingerprint = appendCBORText(fingerprint, signals.Timezone)
		fingerprint = append(fingerprint, 0x04)
		fingerprint = appendCBORHead(fingerprint, 0, uint32(max(0, signals.ScreenSizeSum)))
		fingerprint = append(fingerprint, 0x05)
		if signals.ScreenScale >= 0 && signals.ScreenScale <= math.MaxUint32 && signals.ScreenScale == math.Trunc(signals.ScreenScale) {
			fingerprint = appendCBORHead(fingerprint, 0, uint32(signals.ScreenScale))
		} else {
			fingerprint = append(fingerprint, 0xfb)
			fingerprint = binary.BigEndian.AppendUint64(fingerprint, math.Float64bits(signals.ScreenScale))
		}
		fingerprint = append(fingerprint, 0x06)
		fingerprint = appendCBORText(fingerprint, signals.AppSessionID)
		body = appendCBORText(body, "f")
		body = appendCBORHead(body, 0x40, uint32(len(fingerprint)))
		body = append(body, fingerprint...)
	}
	return "v1." + base64.RawURLEncoding.EncodeToString(body)
}

func appendCBORText(body []byte, value string) []byte {
	body = appendCBORHead(body, 0x60, uint32(len(value)))
	return append(body, []byte(value)...)
}

func appendCBORHead(body []byte, major byte, value uint32) []byte {
	switch {
	case value < 24:
		return append(body, major+byte(value))
	case value <= 255:
		return append(body, major+24, byte(value))
	case value <= 65535:
		body = append(body, major+25)
		return binary.BigEndian.AppendUint16(body, uint16(value))
	default:
		body = append(body, major+26)
		return binary.BigEndian.AppendUint32(body, value)
	}
}

// CodexHeader mirrors app-server/src/attestation.rs. The provider supplies the
// real opaque device envelope, never a client-supplied request header. Failure
// status 1/2/3/4 denotes timeout/failed/canceled/malformed, without a success t.
func CodexHeader(ctx context.Context, provider Provider) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if provider == nil {
		return `{"v":1,"s":2}`
	}
	header := ""
	var err error
	if callback, ok := provider.(CodexTokenProvider); ok {
		var token string
		token, err = callback.GenerateCodexToken(ctx)
		if err == nil {
			encoded, encodeErr := json.Marshal(struct {
				Version int    `json:"v"`
				Status  int    `json:"s"`
				Token   string `json:"t"`
			}{1, 0, token})
			header, err = string(encoded), encodeErr
		}
	} else {
		header, err = provider.Generate(ctx)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return `{"v":1,"s":1}`
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return `{"v":1,"s":3}`
	}
	if err != nil {
		return `{"v":1,"s":2}`
	}
	if !ValidCodexHeader(header) {
		return `{"v":1,"s":4}`
	}
	return header
}

func ValidCodexHeader(header string) bool {
	if len(header) == 0 || len(header) > 16*1024 || strings.ContainsAny(header, "\r\n") {
		return false
	}
	var envelope struct {
		Version int     `json:"v"`
		Status  *int    `json:"s"`
		Token   *string `json:"t"`
	}
	if json.Unmarshal([]byte(header), &envelope) != nil || envelope.Version != 1 || envelope.Status == nil {
		return false
	}
	if *envelope.Status > 0 && *envelope.Status <= 4 {
		return envelope.Token == nil
	}
	if *envelope.Status != 0 || envelope.Token == nil || !strings.HasPrefix(*envelope.Token, "v1.") || len(*envelope.Token) <= 3 {
		return false
	}
	for _, c := range strings.TrimPrefix(*envelope.Token, "v1.") {
		allowed := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if !allowed {
			return false
		}
	}
	return true
}
