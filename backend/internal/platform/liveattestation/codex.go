package liveattestation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// CodexTokenProvider corresponds to the desktop attestation/generate callback.
// Unsupported platforms return an error_code CBOR token, not an Apple token.
type CodexTokenProvider interface {
	GenerateCodexToken(context.Context) (string, error)
}

func codexUnavailableToken(code byte) string {
	// Same CBOR field order/types as the Linux desktop's ui/li helpers.
	body := []byte{0xa2, 0x6a}
	body = append(body, []byte("error_code")...)
	body = append(body, code, 0x69)
	body = append(body, []byte("bundle_id")...)
	body = append(body, 0x70)
	body = append(body, []byte("com.openai.codex")...)
	return "v1." + base64.RawURLEncoding.EncodeToString(body)
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
