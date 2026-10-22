package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type alignmentBehaviorProof struct{}

func (alignmentBehaviorProof) Check(context.Context) error { return nil }
func (alignmentBehaviorProof) Generate(context.Context) (string, error) {
	return `{"v":1,"s":0,"t":"v1.Zml4dHVyZQ"}`, nil
}
func (alignmentBehaviorProof) GenerateDeviceCheckToken(context.Context) (string, error) {
	return "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", nil
}

type alignmentBehaviorTransport struct {
	headers []http.Header
	paths   []string
}

func (u *alignmentBehaviorTransport) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.headers = append(u.headers, req.Header.Clone())
	u.paths = append(u.paths, req.URL.Path)
	h := make(http.Header)
	h.Set("X-OAI-IS-Update", "ois1.real.fixture.update")
	if strings.HasSuffix(req.URL.Path, "/devicecheck") {
		h.Set("Set-Cookie", "_devicecheck=fixture; Path=/; Secure; Max-Age=300")
	}
	return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
}
func (u *alignmentBehaviorTransport) DoWithTLS(req *http.Request, p string, id int64, n int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, p, id, n)
}

// This probe intentionally uses only pre-existing gateway entry points. The
// same source compiles against pristine, modified and restored checkouts.
func TestOpenAIOAuthAlignmentBehaviorProbe(t *testing.T) {
	u := &alignmentBehaviorTransport{}
	cfg := &config.Config{}
	for _, name := range []string{"OpenAIOAuthAttestationEnabled", "OpenAIDesktopDeviceCheckEnabled"} {
		f := reflect.ValueOf(&cfg.Gateway).Elem().FieldByName(name)
		if f.IsValid() {
			f.SetBool(true)
		}
	}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: u, liveAttestation: alignmentBehaviorProof{}}
	a := &Account{ID: 101, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, target := range []string{"https://chatgpt.com/backend-api/codex/responses", "https://chatgpt.com/backend-api/codex/responses", "https://relay.example/backend-api/codex/responses", "https://chatgpt.com/backend-api/messaging/rooms/fixture/messages"} {
		req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(`{}`))
		require.NoError(t, err)
		req.Header.Set("X-OAI-IS", "untrusted-client-value")
		req.Header.Set("x-oai-attestation", "untrusted-client-value")
		resp, err := svc.doAccountHTTPUpstream(req, "", a)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	firstRemoved := u.headers[0].Get("X-OAI-IS") == ""
	echo := u.headers[1].Get("X-OAI-IS") == "ois1.real.fixture.update"
	trusted := u.headers[0].Get("x-oai-attestation") == `{"v":1,"s":0,"t":"v1.Zml4dHVyZQ"}`
	relayClean := u.headers[2].Get("X-OAI-IS") == "" && u.headers[2].Get("x-oai-attestation") == ""
	registered := false
	for _, p := range u.paths {
		if p == "/backend-api/devicecheck" {
			registered = true
		}
	}
	last := u.headers[len(u.headers)-1]
	cookie := strings.Contains(last.Get("Cookie"), "_devicecheck=")
	fmt.Printf("ALIGNMENT_BEHAVIOR untrusted_integrity_removed=%t server_update_echoed=%t generated_attestation_used=%t relay_proof_removed=%t device_cookie_registered=%t device_cookie_sent=%t\n", firstRemoved, echo, trusted, relayClean, registered, cookie)
	require.True(t, firstRemoved && echo && trusted && relayClean && registered && cookie)
}
