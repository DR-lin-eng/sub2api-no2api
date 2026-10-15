package liveattestation

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
)

func TestLinuxCodexAttestationReportsUnsupportedWithoutAppleToken(t *testing.T) {
	encoded := codexUnavailableToken(1)
	payload, err := base64.RawURLEncoding.DecodeString(encoded[3:])
	if err != nil {
		t.Fatal("invalid unsupported envelope")
	}
	if !bytes.Contains(payload, []byte("error_code")) || bytes.Contains(payload, []byte("token")) {
		t.Fatal("unsupported callback must carry only its explicit device error")
	}
	if !bytes.Contains(payload, []byte("com.openai.codex")) {
		t.Fatal("missing bundle attribution")
	}
}

type codexFixtureProvider struct {
	value string
	err   error
}

func (p codexFixtureProvider) Check(context.Context) error              { return p.err }
func (p codexFixtureProvider) Generate(context.Context) (string, error) { return p.value, p.err }

func TestCodexEnvelopeFailureSemantics(t *testing.T) {
	for _, test := range []struct {
		name, value, expected string
		err                   error
	}{
		{"native", `{"v":1,"s":0,"t":"v1.YWJj"}`, `{"v":1,"s":0,"t":"v1.YWJj"}`, nil},
		{"timeout", "", `{"v":1,"s":1}`, context.DeadlineExceeded},
		{"canceled", "", `{"v":1,"s":3}`, context.Canceled},
		{"failed", "", `{"v":1,"s":2}`, errors.New("native failed")},
		{"malformed", `{"v":1,"s":0,"t":""}`, `{"v":1,"s":4}`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual := CodexHeader(context.Background(), codexFixtureProvider{value: test.value, err: test.err})
			if actual != test.expected {
				t.Fatalf("wrong envelope status for %s", test.name)
			}
			if !ValidCodexHeader(actual) {
				t.Fatal("invalid generated envelope")
			}
		})
	}
	for _, invalid := range []string{`{"v":1,"s":0}`, `{"v":1,"s":2,"t":"v1.fake"}`, `{"v":2,"s":0,"t":"v1.YWJj"}`, "bad\r\n"} {
		if ValidCodexHeader(invalid) {
			t.Fatal("malformed envelope was accepted")
		}
	}
}
