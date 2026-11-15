package repository

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestAliyunCaptchaVerifierUsesACS3SignatureVector(t *testing.T) {
	const body = "CaptchaVerifyParam=captcha-param&SceneId=scene-1"
	verifier := &aliyunCaptchaVerifier{
		httpClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodPost, req.Method)
			require.Equal(t, "https://captcha.cn-shanghai.aliyuncs.com/", req.URL.String())
			require.Equal(t, aliyunCaptchaContentType, req.Header.Get("Content-Type"))
			require.Equal(t, "application/json", req.Header.Get("Accept"))
			require.Equal(t, "VerifyIntelligentCaptcha", req.Header.Get("X-Acs-Action"))
			require.Equal(t, "2023-03-05", req.Header.Get("X-Acs-Version"))
			require.Equal(t, "2026-08-05T01:02:03Z", req.Header.Get("X-Acs-Date"))
			require.Equal(t, "0123456789abcdef0123456789abcdef", req.Header.Get("X-Acs-Signature-Nonce"))
			require.Equal(t, "ACS3-HMAC-SHA256 Credential=test-key,SignedHeaders=content-type;host;x-acs-action;x-acs-content-sha256;x-acs-date;x-acs-signature-nonce;x-acs-version,Signature=9025e1e9b799282f1cd933580ac1dd291b1101ab22d972db38c13b623cca77f9", req.Header.Get("Authorization"))
			data, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, body, string(data))
			return jsonResponse(http.StatusOK, map[string]any{
				"Code":      "Success",
				"RequestId": "request-1",
				"Result": map[string]any{
					"VerifyResult": true,
					"VerifyCode":   "T001",
				},
			}), nil
		})},
		scheme: "https",
		now: func() time.Time {
			return time.Date(2026, time.August, 5, 1, 2, 3, 0, time.UTC)
		},
		nonce: func() (string, error) {
			return "0123456789abcdef0123456789abcdef", nil
		},
	}

	result, err := verifier.VerifyCaptcha(context.Background(), service.AliyunCaptchaCredentials{
		AccessKeyID:     "test-key",
		AccessKeySecret: "test-secret",
		SceneID:         "scene-1",
		Endpoint:        "captcha.cn-shanghai.aliyuncs.com",
	}, "captcha-param")

	require.NoError(t, err)
	require.Equal(t, &service.AliyunCaptchaVerifyResult{VerifyResult: true, VerifyCode: "T001", RequestID: "request-1"}, result)
}

func TestAliyunCaptchaVerifierBoundsResponseBody(t *testing.T) {
	verifier := &aliyunCaptchaVerifier{
		httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", aliyunCaptchaResponseLimit+1)))}, nil
		})},
		scheme: "https",
		now:    time.Now,
		nonce: func() (string, error) {
			return "nonce", nil
		},
	}

	result, err := verifier.VerifyCaptcha(context.Background(), service.AliyunCaptchaCredentials{
		AccessKeyID: "id", AccessKeySecret: "secret", SceneID: "scene", Endpoint: "captcha.example.com",
	}, "param")

	require.Nil(t, result)
	require.ErrorContains(t, err, "exceeds")
}

func TestAliyunCaptchaVerifierRejectsIncompleteInputBeforeNetwork(t *testing.T) {
	calls := 0
	verifier := &aliyunCaptchaVerifier{
		httpClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, nil
		})},
		scheme: "https",
		now:    time.Now,
	}

	result, err := verifier.VerifyCaptcha(context.Background(), service.AliyunCaptchaCredentials{AccessKeyID: "id"}, "")

	require.Nil(t, result)
	require.ErrorContains(t, err, "credentials are incomplete")
	require.Zero(t, calls)
}
