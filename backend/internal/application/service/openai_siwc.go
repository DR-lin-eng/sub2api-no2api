package service

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
)

const OpenAIAuthModeSIWC = "siwc"

func (a *Account) IsOpenAISIWC() bool {
	return a != nil && a.IsOpenAIOAuth() && strings.EqualFold(strings.TrimSpace(a.GetCredential("auth_mode")), OpenAIAuthModeSIWC)
}

// parseSIWCCallback accepts a full loopback callback or explicit code/state/
// client_id API fields. The callback may provide only grant values; redirect,
// device identity, PKCE, flow and token endpoint stay owned by the session.
func parseSIWCCallback(input *OpenAIExchangeCodeInput, session *openai.OAuthSession) (string, string, error) {
	code, state, clientID := strings.TrimSpace(input.Code), strings.TrimSpace(input.State), strings.TrimSpace(input.ClientID)
	if input.RedirectURI != "" && input.RedirectURI != session.RedirectURI {
		return "", "", infraerrors.BadRequest("OPENAI_SIWC_CALLBACK_INVALID", "SIWC redirect_uri does not match the authorization session")
	}
	if strings.Contains(code, "://") {
		callback, err := url.Parse(code)
		expected, expectedErr := url.Parse(session.RedirectURI)
		if err != nil || expectedErr != nil || callback.User != nil || callback.Fragment != "" || callback.Opaque != "" ||
			callback.Scheme != expected.Scheme || !strings.EqualFold(callback.Host, expected.Host) || callback.Path != expected.Path {
			return "", "", infraerrors.BadRequest("OPENAI_SIWC_CALLBACK_INVALID", "SIWC callback URL does not match the authorization session")
		}
		values, err := url.ParseQuery(callback.RawQuery)
		if err != nil {
			return "", "", infraerrors.BadRequest("OPENAI_SIWC_CALLBACK_INVALID", "invalid SIWC callback query")
		}
		for _, key := range []string{"code", "state", "client_id", "error"} {
			if len(values[key]) > 1 {
				return "", "", infraerrors.BadRequest("OPENAI_SIWC_CALLBACK_INVALID", "duplicate SIWC callback parameter")
			}
		}
		if values.Get("error") != "" {
			return "", "", infraerrors.BadRequest("OPENAI_SIWC_ACCESS_DENIED", "SIWC authorization was not completed")
		}
		callbackState, callbackClientID := strings.TrimSpace(values.Get("state")), strings.TrimSpace(values.Get("client_id"))
		if (state != "" && state != callbackState) || (clientID != "" && clientID != callbackClientID) {
			return "", "", infraerrors.BadRequest("OPENAI_SIWC_CALLBACK_INVALID", "SIWC callback fields disagree")
		}
		code, state, clientID = strings.TrimSpace(values.Get("code")), callbackState, callbackClientID
	}
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(session.State)) != 1 {
		return "", "", infraerrors.BadRequest("OPENAI_OAUTH_INVALID_STATE", "invalid oauth state")
	}
	if code == "" {
		return "", "", infraerrors.BadRequest("OPENAI_SIWC_CODE_REQUIRED", "SIWC authorization code is required")
	}
	if clientID == "" || clientID == openai.SIWCDynamicClientID {
		return "", "", infraerrors.BadRequest("OPENAI_SIWC_CLIENT_ID_REQUIRED", "paste the complete SIWC callback URL including client_id")
	}
	return code, clientID, nil
}

func (s *OpenAIOAuthService) exchangeSIWCCode(ctx context.Context, input *OpenAIExchangeCodeInput, session *openai.OAuthSession) (*OpenAITokenInfo, error) {
	code, clientID, err := parseSIWCCallback(input, session)
	if err != nil {
		return nil, err
	}
	client, ok := s.oauthClient.(OpenAISIWCOAuthClient)
	if !ok {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_SIWC_UNAVAILABLE", "SIWC OAuth is unavailable")
	}
	proxyURL := session.ProxyURL
	if input.ProxyID != nil {
		proxy, err := s.proxyRepo.GetByID(ctx, *input.ProxyID)
		if err != nil {
			return nil, infraerrors.BadRequest("OPENAI_OAUTH_PROXY_NOT_FOUND", "proxy not found")
		}
		if proxy != nil {
			proxyURL = proxy.URL()
		}
	}
	token, err := client.ExchangeSIWCCode(ctx, code, session.CodeVerifier, session.RedirectURI, proxyURL, clientID)
	if err != nil {
		return nil, err
	}
	if err := openai.ValidateSIWCTokenResponse(token, true); err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SIWC_GRANT_INVALID", err.Error())
	}
	claims, err := openai.ParseIDToken(token.IDToken)
	if err != nil || session.Nonce == "" || subtle.ConstantTimeCompare([]byte(claimsNonce(claims)), []byte(session.Nonce)) != 1 {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SIWC_NONCE_INVALID", "SIWC ID token does not match the authorization nonce")
	}
	if err := s.sessionStore.DeleteContext(ctx, input.SessionID); err != nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_OAUTH_SESSION_STORE_FAILED", "failed to consume SIWC authorization session")
	}
	return siwcTokenInfo(token, clientID, session.DeviceID), nil
}

func claimsNonce(claims *openai.IDTokenClaims) string {
	if claims == nil {
		return ""
	}
	return claims.Nonce
}

func (s *OpenAIOAuthService) refreshSIWCAccountToken(ctx context.Context, account *Account, proxyURL string) (*OpenAITokenInfo, error) {
	clientID, refreshToken := strings.TrimSpace(account.GetCredential("client_id")), strings.TrimSpace(account.GetCredential("refresh_token"))
	if clientID == "" || clientID == openai.SIWCDynamicClientID || refreshToken == "" {
		return nil, infraerrors.BadRequest("OPENAI_SIWC_CREDENTIALS_MISSING", "SIWC requires its issued client_id and refresh_token; reconnect ChatGPT")
	}
	client, ok := s.oauthClient.(OpenAISIWCOAuthClient)
	if !ok {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "OPENAI_SIWC_UNAVAILABLE", "SIWC OAuth is unavailable")
	}
	token, err := client.RefreshSIWCToken(ctx, refreshToken, proxyURL, clientID)
	if err != nil {
		return nil, err
	}
	if err := openai.ValidateSIWCTokenResponse(token, false); err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_SIWC_GRANT_INVALID", err.Error())
	}
	return siwcTokenInfo(token, clientID, account.GetCredential("device_id")), nil
}

func siwcTokenInfo(token *openai.TokenResponse, clientID, deviceID string) *OpenAITokenInfo {
	info := &OpenAITokenInfo{
		AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, IDToken: token.IDToken,
		ExpiresIn: token.ExpiresIn, ExpiresAt: time.Now().Unix() + token.ExpiresIn,
		ClientID: clientID, AuthMode: OpenAIAuthModeSIWC, Scope: token.Scope, DeviceID: deviceID,
	}
	if claims, err := openai.ParseIDToken(token.IDToken); err == nil {
		user := claims.GetUserInfo()
		info.Email, info.ChatGPTAccountID, info.ChatGPTUserID = user.Email, user.ChatGPTAccountID, user.ChatGPTUserID
		info.OrganizationID, info.PlanType = user.OrganizationID, user.PlanType
	}
	return info
}
