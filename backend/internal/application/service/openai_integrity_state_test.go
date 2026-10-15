package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

func TestOpenAIIntegrityStateIsAccountScopedAndCASBound(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	key := openAIIntegrityKey(account, account, "https://chatgpt.com")
	headers := make(http.Header)
	headers.Set("X-OAI-IS", "forged")
	ctxA := svc.attachIntegrityState(context.Background(), headers, key)
	ctxB := svc.attachIntegrityState(context.Background(), make(http.Header), key)
	require.Empty(t, headers.Get("X-OAI-IS"))
	update := make(http.Header)
	update.Set("X-OAI-IS-Update", "ois1.a_b.c-d.e_f")
	require.True(t, svc.captureIntegrityState(ctxA, update))
	update.Set("X-OAI-IS-Update", "ois1.late.response.state")
	require.False(t, svc.captureIntegrityState(ctxB, update))
	ctxC := svc.attachIntegrityState(context.Background(), headers, key)
	require.Equal(t, "ois1.a_b.c-d.e_f", headers.Get("X-OAI-IS"))
	require.True(t, svc.captureIntegrityState(ctxC, update))
	other := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.attachIntegrityState(context.Background(), headers, openAIIntegrityKey(other, other, "https://chatgpt.com"))
	require.Empty(t, headers.Get("X-OAI-IS"))
	for _, malformed := range []string{" ois1.a.b.c", "ois1.a.b.c ", "ois1.a.b.c\n", "bad", strings.Repeat("a", 2049)} {
		require.False(t, validOpenAIIntegrityState(malformed))
	}
}

type integrityWireUpstream struct {
	requests []*http.Request
	body     []byte
}

func (u *integrityWireUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.requests = append(u.requests, req.Clone(req.Context()))
	if req.Body != nil {
		u.body, _ = io.ReadAll(req.Body)
	}
	h := make(http.Header)
	if strings.HasSuffix(req.URL.Path, "/devicecheck") {
		h.Set("Set-Cookie", "_devicecheck=local-native-fixture; Path=/; Secure; HttpOnly; Max-Age=300")
	} else {
		h.Set("X-OAI-IS-Update", "ois1.server.issued.state")
	}
	return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
}

func (u *integrityWireUpstream) DoWithTLS(req *http.Request, proxy string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, id, concurrency)
}

func TestOpenAIIntegrityHTTPWireAndOriginIsolation(t *testing.T) {
	u := &integrityWireUpstream{}
	svc := &OpenAIGatewayService{httpUpstream: u}
	a := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, target := range []string{"https://chatgpt.com/backend-api/codex/responses", "https://chatgpt.com/backend-api/codex/responses", "https://relay.example/backend-api/codex/responses"} {
		req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(`{"input":[]}`))
		require.NoError(t, err)
		req.Header.Set("X-OAI-IS", "forged")
		resp, err := svc.doAccountHTTPUpstream(req, "", a)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	require.Empty(t, u.requests[0].Header.Get("X-OAI-IS"))
	require.Equal(t, "ois1.server.issued.state", u.requests[1].Header.Get("X-OAI-IS"))
	require.Empty(t, u.requests[2].Header.Get("X-OAI-IS"))
}

type failingIntegrityCache struct{ GatewayCache }

func (failingIntegrityCache) GetOpenAIIntegrityState(context.Context, string) (string, bool, error) {
	return "", false, errors.New("offline")
}
func (failingIntegrityCache) CompareAndSwapOpenAIIntegrityState(context.Context, string, string, string, time.Duration) (bool, error) {
	return false, errors.New("offline")
}

func TestOpenAIIntegritySharedFailureDoesNotResurrectLocalState(t *testing.T) {
	svc := &OpenAIGatewayService{cache: failingIntegrityCache{}, openAIIntegrityStates: map[string]openAIIntegrityBinding{"key": {value: "ois1.stale.local.copy", expiresAt: time.Now().Add(time.Hour)}}}
	h := make(http.Header)
	ctx := svc.attachIntegrityState(context.Background(), h, "key")
	require.Empty(t, h.Get("X-OAI-IS"))
	h.Set("X-OAI-IS-Update", "ois1.a.b.c")
	require.False(t, svc.captureIntegrityState(ctx, h))
}

type nativeProofFixture struct {
	header string
	err    error
}

func (p nativeProofFixture) Check(context.Context) error              { return p.err }
func (p nativeProofFixture) Generate(context.Context) (string, error) { return p.header, p.err }
func (p nativeProofFixture) GenerateDeviceCheckToken(context.Context) (string, error) {
	return "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=", p.err
}

func TestOpenAIProofAtSendUsesProviderAndRetainsFailureStatus(t *testing.T) {
	u := &integrityWireUpstream{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: u, liveAttestation: nativeProofFixture{header: `{"v":1,"s":0,"t":"v1.bmF0aXZl"}`}}
	svc.cfg.Gateway.OpenAIOAuthAttestationEnabled = true
	a := &Account{ID: 51, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, target := range []string{"https://chatgpt.com/backend-api/codex/responses", "https://relay.example/backend-api/codex/responses"} {
		req, err := http.NewRequest(http.MethodPost, target, nil)
		require.NoError(t, err)
		req.Header.Set("x-oai-attestation", "forged")
		resp, err := svc.doAccountHTTPUpstream(req, "", a)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	require.JSONEq(t, `{"v":1,"s":0,"t":"v1.bmF0aXZl"}`, u.requests[0].Header.Get("x-oai-attestation"))
	require.Empty(t, u.requests[1].Header.Get("x-oai-attestation"))
	svc.liveAttestation = nativeProofFixture{err: context.DeadlineExceeded}
	h := make(http.Header)
	_, err := svc.prepareOpenAIOAuthProof(context.Background(), a, "https://chatgpt.com/backend-api/codex/responses", h)
	require.NoError(t, err)
	require.JSONEq(t, `{"v":1,"s":1}`, h.Get("x-oai-attestation"))
}

func TestOpenAIDeviceCheckRegistrationAndExactPath(t *testing.T) {
	u := &integrityWireUpstream{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: u, liveAttestation: nativeProofFixture{}}
	svc.cfg.Gateway.OpenAIDesktopDeviceCheckEnabled = true
	a := &Account{ID: 51, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/messaging/rooms/room/messages", strings.NewReader(`{"message":"local fixture"}`))
		require.NoError(t, err)
		resp, err := svc.doAccountHTTPUpstream(req, "", a)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}
	require.Len(t, u.requests, 3, "one cookie registration, then two messages")
	require.Equal(t, "/backend-api/devicecheck", u.requests[0].URL.Path)
	require.Empty(t, u.requests[0].Header.Get("x-sentinel-dc"))
	require.Equal(t, "com.openai.codex", u.requests[1].Header.Get("OAI-Package-Name"))
	require.Contains(t, u.requests[1].Header.Get("Cookie"), "_devicecheck=")
	require.NotEmpty(t, u.requests[1].Header.Get("x-sentinel-dc"))
	for _, path := range []string{"/evil/messaging/rooms/x/messages", "/backend-api/messaging/rooms/x/extra/messages", "/backend-api/codex/responses"} {
		_, err := svc.buildOpenAIDeviceCheckHeaders(context.Background(), http.MethodPost, path)
		require.ErrorIs(t, err, errOpenAIDeviceCheckEndpoint)
	}
}
