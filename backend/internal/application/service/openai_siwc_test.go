package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/stretchr/testify/require"
)

type siwcOAuthClientStub struct {
	openaiOAuthClientStateStub
	exchangeCalls, refreshCalls                      int
	code, verifier, redirect, clientID, refreshToken string
	token                                            *openai.TokenResponse
}

func (s *siwcOAuthClientStub) ExchangeSIWCCode(_ context.Context, code, verifier, redirect, _ string, clientID string) (*openai.TokenResponse, error) {
	s.exchangeCalls++
	s.code, s.verifier, s.redirect, s.clientID = code, verifier, redirect, clientID
	return s.token, nil
}

func (s *siwcOAuthClientStub) RefreshSIWCToken(_ context.Context, refreshToken, _ string, clientID string) (*openai.TokenResponse, error) {
	s.refreshCalls++
	s.refreshToken, s.clientID = refreshToken, clientID
	return s.token, nil
}

func siwcTestIDToken(t *testing.T, nonce string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"aud": "oaiapp_issued", "nonce": nonce, "email": "siwc@example.com", "exp": time.Now().Add(time.Hour).Unix(),
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acct-siwc", "chatgpt_user_id": "user-siwc", "chatgpt_plan_type": "plus"},
	})
	require.NoError(t, err)
	return "e30." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func TestSIWCAuthorizationExchangeRefreshLifecycle(t *testing.T) {
	ctx := context.Background()
	client := &siwcOAuthClientStub{}
	svc := NewOpenAIOAuthService(nil, client)
	defer svc.Stop()
	result, err := svc.GenerateAuthURLWithFlow(ctx, nil, "", PlatformOpenAI, OpenAIAuthFlowSIWC)
	require.NoError(t, err)
	authURL, err := url.Parse(result.AuthURL)
	require.NoError(t, err)
	require.Equal(t, openai.SIWCAuthorizeURL, authURL.Scheme+"://"+authURL.Host+authURL.Path)
	query := authURL.Query()
	require.Equal(t, openai.SIWCDynamicClientID, query.Get("client_id"))
	require.Equal(t, openai.SIWCResource, query.Get("resource"))
	require.Equal(t, openai.SIWCScope, query.Get("scope"))
	require.Equal(t, openai.SIWCAgentNameHint, query.Get("agent_name_hint"))
	require.Empty(t, query.Get("codex_cli_simplified_flow"))
	session, ok := svc.sessionStore.Get(result.SessionID)
	require.True(t, ok)
	require.Equal(t, session.Nonce, query.Get("nonce"))
	require.NotEmpty(t, session.Nonce)
	require.Equal(t, "urn:uuid:"+session.DeviceID, query.Get("ext_agent_host_id"))
	require.Equal(t, openai.GenerateCodeChallenge(session.CodeVerifier), query.Get("code_challenge"))
	client.token = &openai.TokenResponse{AccessToken: "siwc-at", RefreshToken: "siwc-rt", IDToken: siwcTestIDToken(t, session.Nonce), ExpiresIn: 3600, Scope: openai.SIWCScope}
	callback := openai.SIWCRedirectURI + "?" + url.Values{"code": {"issued-code"}, "state": {session.State}, "client_id": {"oaiapp_issued"}}.Encode()
	info, err := svc.ExchangeCode(ctx, &OpenAIExchangeCodeInput{SessionID: result.SessionID, Code: callback, State: session.State})
	require.NoError(t, err)
	require.Equal(t, "issued-code", client.code)
	require.Equal(t, session.CodeVerifier, client.verifier)
	require.Equal(t, openai.SIWCRedirectURI, client.redirect)
	require.Equal(t, "oaiapp_issued", info.ClientID)
	require.Equal(t, "siwc@example.com", info.Email)
	require.Equal(t, "acct-siwc", info.ChatGPTAccountID)
	require.Equal(t, 0, int(client.exchangeCalled), "legacy token endpoint must not be used")
	credentials := svc.BuildAccountCredentials(info)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: credentials}
	require.True(t, account.IsOpenAISIWC())
	require.Equal(t, session.DeviceID, credentials["device_id"])
	_, err = svc.ExchangeCode(ctx, &OpenAIExchangeCodeInput{SessionID: result.SessionID, Code: callback, State: session.State})
	require.Error(t, err, "completed authorization sessions cannot be replayed")
	client.token = &openai.TokenResponse{AccessToken: "rotated-at", RefreshToken: "rotated-rt", ExpiresIn: 3600, Scope: openai.SIWCScope}
	refreshed, err := svc.RefreshAccountToken(ctx, account)
	require.NoError(t, err)
	require.Equal(t, 1, client.refreshCalls)
	require.Equal(t, "siwc-rt", client.refreshToken)
	require.Equal(t, "oaiapp_issued", client.clientID)
	require.Equal(t, OpenAIAuthModeSIWC, refreshed.AuthMode)
	require.Equal(t, "rotated-rt", refreshed.RefreshToken)
	require.Equal(t, session.DeviceID, refreshed.DeviceID)
}

func TestSIWCCallbackRejectsInvalidGrantsBeforeUpstream(t *testing.T) {
	session := &openai.OAuthSession{State: "expected", RedirectURI: openai.SIWCRedirectURI}
	valid := openai.SIWCRedirectURI + "?code=code&state=expected&client_id=oaiapp_issued"
	for _, callback := range []string{
		strings.Replace(valid, "127.0.0.1", "evil.example", 1),
		strings.Replace(valid, "state=expected", "state=other", 1),
		valid + "&client_id=other",
		valid + "&code=other",
		valid + "#fragment",
		strings.Replace(valid, "client_id=oaiapp_issued", "client_id=dynamic_agent_client", 1),
		strings.Replace(valid, "&client_id=oaiapp_issued", "", 1),
	} {
		t.Run(callback, func(t *testing.T) {
			_, _, err := parseSIWCCallback(&OpenAIExchangeCodeInput{Code: callback, State: session.State}, session)
			require.Error(t, err)
		})
	}
	_, _, err := parseSIWCCallback(&OpenAIExchangeCodeInput{Code: "code", State: "expected"}, session)
	require.Error(t, err)
	_, _, err = parseSIWCCallback(&OpenAIExchangeCodeInput{Code: valid, State: "expected", ClientID: "different"}, session)
	require.Error(t, err)
}

func TestSIWCRejectsNonceMismatchAndMissingScope(t *testing.T) {
	for _, scenario := range []string{"nonce", "scope", "refresh_token", "expiry"} {
		t.Run(scenario, func(t *testing.T) {
			client := &siwcOAuthClientStub{}
			svc := NewOpenAIOAuthService(nil, client)
			defer svc.Stop()
			result, err := svc.GenerateAuthURLWithFlow(context.Background(), nil, "", PlatformOpenAI, "siwc")
			require.NoError(t, err)
			session, _ := svc.sessionStore.Get(result.SessionID)
			client.token = &openai.TokenResponse{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600, Scope: openai.SIWCScope, IDToken: siwcTestIDToken(t, session.Nonce)}
			switch scenario {
			case "nonce":
				client.token.IDToken = siwcTestIDToken(t, "wrong")
			case "scope":
				client.token.Scope = "openid profile"
			case "refresh_token":
				client.token.RefreshToken = ""
			case "expiry":
				client.token.ExpiresIn = 0
			}
			_, err = svc.ExchangeCode(context.Background(), &OpenAIExchangeCodeInput{SessionID: result.SessionID, Code: "code", State: session.State, ClientID: "oaiapp_issued"})
			require.Error(t, err)
		})
	}
}
