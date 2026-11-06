package repository

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/tidwall/gjson"
)

func (s *openaiOAuthService) ExchangeSIWCCode(ctx context.Context, code, codeVerifier, redirectURI, proxyURL, clientID string) (*openai.TokenResponse, error) {
	form := url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID},
		"code": {code}, "code_verifier": {codeVerifier}, "redirect_uri": {redirectURI},
		"resource": {openai.SIWCResource},
	}
	return s.requestSIWCToken(ctx, form, proxyURL, true)
}

func (s *openaiOAuthService) RefreshSIWCToken(ctx context.Context, refreshToken, proxyURL, clientID string) (*openai.TokenResponse, error) {
	form := url.Values{
		"grant_type": {"refresh_token"}, "client_id": {clientID},
		"refresh_token": {refreshToken}, "resource": {openai.SIWCResource},
	}
	return s.requestSIWCToken(ctx, form, proxyURL, false)
}

func (s *openaiOAuthService) requestSIWCToken(ctx context.Context, form url.Values, proxyURL string, requireIDToken bool) (*openai.TokenResponse, error) {
	clientID := strings.TrimSpace(form.Get("client_id"))
	if clientID == "" || clientID == openai.SIWCDynamicClientID {
		return nil, infraerrors.BadRequest("OPENAI_SIWC_CLIENT_ID_REQUIRED", "SIWC requires the issued client_id")
	}
	client, err := createOpenAIReqClientForContext(ctx, proxyURL)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_CLIENT_INIT_FAILED", "create HTTP client: %v", err)
	}
	endpoint := s.siwcTokenURL
	if endpoint == "" {
		endpoint = openai.SIWCTokenURL
	}
	var token openai.TokenResponse
	resp, err := client.R().SetContext(ctx).
		SetHeader("User-Agent", openai.SIWCAgentNameHint).
		SetHeader("Accept", "application/json").
		SetFormDataFromValues(form).SetSuccessResult(&token).Post(endpoint)
	if err != nil {
		if shouldReturnOpenAINoProxyHint(ctx, proxyURL, err) {
			return nil, newOpenAINoProxyHintError(err)
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_OAUTH_REQUEST_FAILED", "SIWC token request failed: %v", err)
	}
	if !resp.IsSuccessState() {
		// Preserve known OAuth codes for the existing permanent-failure and
		// refresh-race recovery policies without including token-bearing bodies.
		code := gjson.Get(resp.String(), "error.code").String()
		if code == "" {
			code = gjson.Get(resp.String(), "error").String()
		}
		switch code {
		case "invalid_grant", "invalid_client", "unauthorized_client", "invalid_refresh_token", "token_expired", "refresh_token_reused", "refresh_token_invalidated", "app_session_terminated", "access_denied", "invalid_scope":
		default:
			code = "oauth_error"
		}
		return nil, infraerrors.Newf(http.StatusBadGateway, "OPENAI_SIWC_TOKEN_FAILED", "SIWC token request failed: status %d, code %s", resp.StatusCode, code)
	}
	if err := openai.ValidateSIWCTokenResponse(&token, requireIDToken); err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SIWC_GRANT_INVALID", err.Error())
	}
	return &token, nil
}
