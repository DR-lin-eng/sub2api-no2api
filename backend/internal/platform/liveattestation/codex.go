package liveattestation

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
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

const codexGenerateTimeout = 100 * time.Millisecond

// Keep uncooperative providers from accumulating unbounded goroutines after
// their callers time out. Waiting for a slot consumes the same request budget.
var codexGenerationSlots = make(chan struct{}, 64)

// CodexHeader bounds the selected local callback like app-server/attestation.rs.
// A nil provider means no capable host and omits the header. Status 1/2/3/4
// denotes timeout/failed/canceled/malformed, without a success t. Tokens remain
// opaque; callback success does not establish successful hardware attestation.
func CodexHeader(ctx context.Context, provider Provider) string {
	return codexHeader(ctx, provider, codexGenerationSlots)
}

func codexHeader(ctx context.Context, provider Provider, slots chan struct{}) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if provider == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, codexGenerateTimeout)
	defer cancel()
	if ctx.Err() != nil {
		return codexContextStatus(ctx.Err())
	}
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return codexContextStatus(ctx.Err())
	}
	if ctx.Err() != nil {
		<-slots
		return codexContextStatus(ctx.Err())
	}
	type result struct {
		header string
		err    error
	}
	response := make(chan result, 1)
	go func() {
		defer func() { <-slots }()
		header, err := generateCodexHeader(ctx, provider)
		response <- result{header, err}
	}()
	select {
	case <-ctx.Done():
		return codexContextStatus(ctx.Err())
	case generated := <-response:
		if ctx.Err() != nil {
			return codexContextStatus(ctx.Err())
		}
		if errors.Is(generated.err, context.DeadlineExceeded) || errors.Is(generated.err, context.Canceled) {
			return codexContextStatus(generated.err)
		}
		if generated.err != nil {
			return `{"v":1,"s":2}`
		}
		if !ValidCodexHeader(generated.header) {
			return `{"v":1,"s":4}`
		}
		return generated.header
	}
}

func codexContextStatus(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return `{"v":1,"s":1}`
	}
	return `{"v":1,"s":3}`
}

func generateCodexHeader(ctx context.Context, provider Provider) (string, error) {
	if scope, ok := RequestScopeFromContext(ctx); ok {
		provider = ProviderForScope(provider, scope.Key)
		if provider == nil {
			return "", ErrScopeMismatch
		}
	}
	if callback, ok := provider.(CodexTokenProvider); ok {
		token, err := callback.GenerateCodexToken(ctx)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(struct {
			Version int    `json:"v"`
			Status  int    `json:"s"`
			Token   string `json:"t"`
		}{1, 0, token})
		return string(encoded), err
	}
	return provider.Generate(ctx)
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
	return *envelope.Status == 0 && envelope.Token != nil
}

// ValidLiveHeader retains Live's native v1 token contract independently of the
// app-server callback, which accepts an opaque string (including an empty one).
func ValidLiveHeader(header string) bool {
	if !ValidCodexHeader(header) {
		return false
	}
	var envelope struct {
		Status int    `json:"s"`
		Token  string `json:"t"`
	}
	if json.Unmarshal([]byte(header), &envelope) != nil || envelope.Status != 0 || !strings.HasPrefix(envelope.Token, "v1.") || len(envelope.Token) <= 3 {
		return false
	}
	for _, c := range strings.TrimPrefix(envelope.Token, "v1.") {
		allowed := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if !allowed {
			return false
		}
	}
	return true
}
