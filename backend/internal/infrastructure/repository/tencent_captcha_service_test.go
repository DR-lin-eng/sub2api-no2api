package repository

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

type trackingReadCloser struct {
	reader *strings.Reader
	closed bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestTencentCaptchaVerifierSignsRequestAndMapsResponse(t *testing.T) {
	fixedTime := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	verifier := &tencentCaptchaVerifier{
		httpClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, tencentCaptchaEndpoint, req.URL.String())
			require.Equal(t, tencentCaptchaContentType, req.Header.Get("Content-Type"))
			require.Equal(t, tencentCaptchaAction, req.Header.Get("X-TC-Action"))
			require.Equal(t, tencentCaptchaVersion, req.Header.Get("X-TC-Version"))
			require.Equal(t, "1704164645", req.Header.Get("X-TC-Timestamp"))
			require.Equal(t,
				"TC3-HMAC-SHA256 Credential=AKIDEXAMPLE/2024-01-02/captcha/tc3_request, SignedHeaders=content-type;host, Signature=6a8852c21b52db1c3ef2bf7fd9a7a7ea7252363152b66e1083432c21e1dada8d",
				req.Header.Get("Authorization"),
			)

			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{
				"CaptchaType": 9,
				"Ticket": "ticket",
				"UserIp": "203.0.113.10",
				"Randstr": "rand",
				"CaptchaAppId": 123456,
				"AppSecretKey": "app-secret"
			}`, string(body))

			return jsonResponse(http.StatusOK, map[string]any{
				"Response": map[string]any{
					"CaptchaCode": int64(1),
					"CaptchaMsg":  "OK",
					"RequestId":   "request-id",
				},
			}), nil
		})},
		endpoint: tencentCaptchaEndpoint,
		now:      func() time.Time { return fixedTime },
	}

	result, err := verifier.VerifyTicket(
		context.Background(),
		service.TencentCaptchaCredentials{
			AppID:          123456,
			AppSecretKey:   "app-secret",
			CloudSecretID:  "AKIDEXAMPLE",
			CloudSecretKey: "cloud-secret",
		},
		service.TencentCaptchaProof{Ticket: "ticket", Randstr: "rand"},
		"203.0.113.10",
	)

	require.NoError(t, err)
	require.Equal(t, &service.TencentCaptchaVerifyResponse{
		CaptchaCode: 1,
		CaptchaMsg:  "OK",
		RequestID:   "request-id",
	}, result)
}

func TestTencentCaptchaVerifierRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  string
	}{
		{name: "non-2xx", statusCode: http.StatusBadGateway, body: `{}`, wantError: "HTTP 502"},
		{name: "malformed json", statusCode: http.StatusOK, body: `{`, wantError: "decode tencent captcha response"},
		{name: "missing response", statusCode: http.StatusOK, body: `{}`, wantError: "missing Response"},
		{name: "api error", statusCode: http.StatusOK, body: `{"Response":{"Error":{"Code":"AuthFailure.SignatureFailure","Message":"invalid"}}}`, wantError: "AuthFailure.SignatureFailure"},
		{name: "missing code", statusCode: http.StatusOK, body: `{"Response":{"RequestId":"id"}}`, wantError: "missing CaptchaCode"},
		{name: "oversized", statusCode: http.StatusOK, body: strings.Repeat("x", tencentCaptchaResponseLimit+1), wantError: "exceeds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &tencentCaptchaVerifier{
				httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: tt.statusCode,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(tt.body)),
					}, nil
				})},
				endpoint: tencentCaptchaEndpoint,
				now:      time.Now,
			}

			result, err := verifier.VerifyTicket(
				context.Background(),
				service.TencentCaptchaCredentials{AppID: 1, CloudSecretID: "id", CloudSecretKey: "key"},
				service.TencentCaptchaProof{Ticket: "ticket", Randstr: "rand"},
				"203.0.113.10",
			)

			require.Nil(t, result)
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}

func TestTencentCaptchaVerifierUsesInternationalEndpoint(t *testing.T) {
	verifier := &tencentCaptchaVerifier{
		httpClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, tencentCaptchaIntlEndpoint, req.URL.String())
			return jsonResponse(http.StatusOK, map[string]any{
				"Response": map[string]any{"CaptchaCode": int64(1)},
			}), nil
		})},
		endpoint: tencentCaptchaEndpoint,
		now:      time.Now,
	}

	_, err := verifier.VerifyTicket(
		context.Background(),
		service.TencentCaptchaCredentials{
			AppID: 1, CloudSecretID: "id", CloudSecretKey: "key", Endpoint: tencentCaptchaIntlEndpoint,
		},
		service.TencentCaptchaProof{Ticket: "ticket", Randstr: "rand"},
		"203.0.113.10",
	)

	require.NoError(t, err)
}

func TestTencentCaptchaVerifierRejectsUnknownEndpoint(t *testing.T) {
	verifier := &tencentCaptchaVerifier{
		httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			t.Fatal("unexpected request")
			return nil, nil
		})},
		endpoint: tencentCaptchaEndpoint,
		now:      time.Now,
	}

	_, err := verifier.VerifyTicket(
		context.Background(),
		service.TencentCaptchaCredentials{
			AppID: 1, CloudSecretID: "id", CloudSecretKey: "key", Endpoint: "https://example.com",
		},
		service.TencentCaptchaProof{Ticket: "ticket", Randstr: "rand"},
		"203.0.113.10",
	)

	require.ErrorContains(t, err, "unsupported tencent captcha endpoint")
}

func TestTencentCaptchaVerifierFailsClosedWithoutRestrictedHTTPClient(t *testing.T) {
	verifier := &tencentCaptchaVerifier{initErr: context.Canceled}

	result, err := verifier.VerifyTicket(
		context.Background(),
		service.TencentCaptchaCredentials{},
		service.TencentCaptchaProof{},
		"",
	)

	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
}

func TestTencentCaptchaVerifierDrainsErrorResponseForConnectionReuse(t *testing.T) {
	body := &trackingReadCloser{reader: strings.NewReader(`{"error":"temporary"}`)}
	verifier := &tencentCaptchaVerifier{
		httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Header:     make(http.Header),
				Body:       body,
			}, nil
		})},
		endpoint: tencentCaptchaEndpoint,
		now:      time.Now,
	}

	result, err := verifier.VerifyTicket(
		context.Background(),
		service.TencentCaptchaCredentials{AppID: 1, CloudSecretID: "id", CloudSecretKey: "key"},
		service.TencentCaptchaProof{Ticket: "ticket", Randstr: "rand"},
		"203.0.113.10",
	)

	require.Nil(t, result)
	require.ErrorContains(t, err, "HTTP 502")
	require.Zero(t, body.reader.Len())
	require.True(t, body.closed)
}

func BenchmarkSignTencentCaptchaRequest(b *testing.B) {
	req, err := http.NewRequest(http.MethodPost, tencentCaptchaEndpoint, nil)
	require.NoError(b, err)
	payload := []byte(`{"CaptchaType":9,"Ticket":"ticket","UserIp":"203.0.113.10","Randstr":"rand","CaptchaAppId":123456,"AppSecretKey":"app-secret"}`)
	credentials := service.TencentCaptchaCredentials{
		CloudSecretID:  "AKIDEXAMPLE",
		CloudSecretKey: "cloud-secret",
	}
	now := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		signTencentCaptchaRequest(req, payload, credentials, now)
	}
}
