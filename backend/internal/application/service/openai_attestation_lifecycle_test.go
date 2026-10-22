package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
	"github.com/stretchr/testify/require"
)

type lifecycleProofFixture struct {
	nativeProofFixture
	generate func(context.Context) (string, error)
}

func (p lifecycleProofFixture) GenerateCodexToken(ctx context.Context) (string, error) {
	return p.generate(ctx)
}

func TestOpenAIAttestationLifecycleAtHTTPAndWSDial(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider liveattestation.Provider
		want     string
	}{
		{name: "no capable provider"},
		{name: "opaque token", provider: lifecycleProofFixture{generate: func(context.Context) (string, error) {
			return "opaque+/=", nil
		}}, want: `{"v":1,"s":0,"t":"opaque+/="}`},
		{name: "callback timeout", provider: lifecycleProofFixture{generate: func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}}, want: `{"v":1,"s":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIOAuthAttestationEnabled = true
			u := &integrityWireUpstream{}
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: u, liveAttestation: tc.provider}
			account := &Account{ID: 97, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
			for _, path := range []string{"/backend-api/codex/responses", "/backend-api/codex/responses/compact"} {
				req, err := http.NewRequest(http.MethodPost, "https://chatgpt.com"+path, nil)
				require.NoError(t, err)
				req.Header.Set("x-oai-attestation", "untrusted")
				resp, err := svc.doAccountHTTPUpstream(req, "", account)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				observed := u.requests[len(u.requests)-1].Header
				require.Equal(t, tc.want, observed.Get("x-oai-attestation"))
				if tc.want == "" {
					require.NotContains(t, observed, http.CanonicalHeaderKey("x-oai-attestation"), "omit instead of sending an empty header")
				}
			}
			dialer := &fingerprintHeaderCaptureDialer{}
			pool := &openAIWSConnPool{clientDialer: dialer}
			wsURL := "wss://chatgpt.com/backend-api/codex/responses"
			_, err := pool.dialConn(context.Background(), openAIWSAcquireRequest{
				Account: account, WSURL: wsURL,
				Headers: http.Header{"X-Oai-Attestation": {"untrusted"}},
				ProofFactory: func(ctx context.Context, headers http.Header) (context.Context, http.Header, error) {
					return svc.prepareOpenAIWSProof(ctx, account, wsURL, headers)
				},
			})
			require.Error(t, err, "capture dialer deliberately stops before making a network connection")
			require.Equal(t, tc.want, dialer.headers.Get("x-oai-attestation"))
			if tc.want == "" {
				require.NotContains(t, dialer.headers, http.CanonicalHeaderKey("x-oai-attestation"))
			}
		})
	}
}

func TestOpenAILiveDoesNotPreserveOpaqueOrFailureCallbackHeader(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 98, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for _, header := range []string{`{"v":1,"s":0,"t":"opaque+/="}`, `{"v":1,"s":1}`, `{"v":1,"s":0,"t":"v1.YWJj"}`} {
		headers := make(http.Header)
		headers.Set("x-oai-attestation", header)
		_, err := svc.prepareOpenAIOAuthProof(context.Background(), account, "https://chatgpt.com/backend-api/realtime/calls", headers)
		require.NoError(t, err)
		if header == `{"v":1,"s":0,"t":"v1.YWJj"}` {
			require.Equal(t, header, headers.Get("x-oai-attestation"))
		} else {
			require.NotContains(t, headers, http.CanonicalHeaderKey("x-oai-attestation"))
		}
	}
}
