package repository

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestRecaptchaVerifierSendsStandardSiteverifyForm(t *testing.T) {
	verifier := &recaptchaVerifier{
		httpClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			values, err := url.ParseQuery(string(body))
			require.NoError(t, err)
			require.Equal(t, "secret", values.Get("secret"))
			require.Equal(t, "token", values.Get("response"))
			require.Equal(t, "127.0.0.1", values.Get("remoteip"))
			return jsonResponse(http.StatusOK, service.RecaptchaVerifyResponse{Success: true}), nil
		})},
		verifyURL: "http://in-process/recaptcha",
	}

	result, err := verifier.VerifyToken(context.Background(), "secret", "token", "127.0.0.1")
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestCapVerifierPostsJSONToSiteEndpoint(t *testing.T) {
	verifier := &capVerifier{httpClient: &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "http://cap.example/site-key/siteverify", req.URL.String())
		var payload map[string]string
		require.NoError(t, json.NewDecoder(req.Body).Decode(&payload))
		require.Equal(t, "secret", payload["secret"])
		require.Equal(t, "token", payload["response"])
		return jsonResponse(http.StatusOK, service.CapVerifyResponse{Success: true}), nil
	})}}

	result, err := verifier.VerifyToken(context.Background(), "http://cap.example/site-key/", "secret", "token")
	require.NoError(t, err)
	require.True(t, result.Success)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func jsonResponse(status int, payload any) *http.Response {
	body, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}
