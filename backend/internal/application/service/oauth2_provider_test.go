//go:build unit

package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type oauth2ProviderSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func newOAuth2ProviderSettingRepo() *oauth2ProviderSettingRepo {
	return &oauth2ProviderSettingRepo{values: make(map[string]string)}
}

func (r *oauth2ProviderSettingRepo) Get(ctx context.Context, key string) (*Setting, error) {
	value, err := r.GetValue(ctx, key)
	if err != nil {
		return nil, err
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *oauth2ProviderSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *oauth2ProviderSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *oauth2ProviderSettingRepo) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, err := r.GetValue(ctx, key); err == nil {
			result[key] = value
		}
	}
	return result, nil
}

func (r *oauth2ProviderSettingRepo) SetMultiple(ctx context.Context, values map[string]string) error {
	for key, value := range values {
		if err := r.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *oauth2ProviderSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *oauth2ProviderSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

type oauth2ProviderMemoryStore struct {
	mu     sync.Mutex
	codes  map[string]*OAuth2AuthorizationCodeData
	tokens map[string]*OAuth2AccessTokenData
}

func newOAuth2ProviderMemoryStore() *oauth2ProviderMemoryStore {
	return &oauth2ProviderMemoryStore{
		codes:  make(map[string]*OAuth2AuthorizationCodeData),
		tokens: make(map[string]*OAuth2AccessTokenData),
	}
}

func (s *oauth2ProviderMemoryStore) StoreAuthorizationCode(_ context.Context, hash string, data *OAuth2AuthorizationCodeData, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *data
	s.codes[hash] = &copy
	return nil
}

func (s *oauth2ProviderMemoryStore) ConsumeAuthorizationCode(_ context.Context, hash string) (*OAuth2AuthorizationCodeData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.codes[hash]
	if !ok {
		return nil, ErrOAuth2GrantNotFound
	}
	delete(s.codes, hash)
	copy := *data
	return &copy, nil
}

func (s *oauth2ProviderMemoryStore) StoreAccessToken(_ context.Context, hash string, data *OAuth2AccessTokenData, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *data
	s.tokens[hash] = &copy
	return nil
}

func (s *oauth2ProviderMemoryStore) GetAccessToken(_ context.Context, hash string) (*OAuth2AccessTokenData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.tokens[hash]
	if !ok {
		return nil, ErrOAuth2GrantNotFound
	}
	copy := *data
	return &copy, nil
}

func (s *oauth2ProviderMemoryStore) DeleteAccessToken(_ context.Context, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, hash)
	return nil
}

func newOAuth2ProviderForTest(t *testing.T) (*OAuth2ProviderService, *oauth2ProviderSettingRepo) {
	t.Helper()
	settings := newOAuth2ProviderSettingRepo()
	user := &User{
		ID:           42,
		Email:        "owner@example.test",
		Username:     "owner",
		PasswordHash: "password-hash",
		Role:         RoleUser,
		Status:       StatusActive,
		TokenVersion: 3,
	}
	provider := NewOAuth2ProviderService(
		settings,
		&userRepoStub{user: user},
		newOAuth2ProviderMemoryStore(),
		&config.Config{JWT: config.JWTConfig{Secret: "oauth2-subject-test-secret"}},
	)
	_, err := provider.UpdateAdminConfig(context.Background(), OAuth2ProviderConfigUpdate{
		Enabled:               true,
		Issuer:                "https://issuer.example.test/",
		AccessTokenTTLSeconds: 600,
	})
	require.NoError(t, err)
	return provider, settings
}

func createOAuth2ClientForTest(t *testing.T, provider *OAuth2ProviderService, clientType string) *OAuth2ClientSecretResult {
	t.Helper()
	result, err := provider.CreateClient(context.Background(), OAuth2ClientCreateInput{
		Name:          "Test integration",
		ClientType:    clientType,
		RedirectURIs:  []string{"https://client.example.test/callback?fixed=1"},
		AllowedScopes: []string{"profile", "email"},
	})
	require.NoError(t, err)
	return result
}

func oauth2PKCE(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func authorizationRequest(clientID, verifier string) OAuth2AuthorizationRequest {
	return OAuth2AuthorizationRequest{
		ClientID:            clientID,
		RedirectURI:         "https://client.example.test/callback?fixed=1",
		ResponseType:        "code",
		Scope:               "profile email",
		State:               "state-123",
		CodeChallenge:       oauth2PKCE(verifier),
		CodeChallengeMethod: "S256",
	}
}

func authorizationCodeFromRedirect(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := urlParse(raw)
	require.NoError(t, err)
	require.Equal(t, "1", parsed.Query().Get("fixed"))
	require.Equal(t, "state-123", parsed.Query().Get("state"))
	return parsed.Query().Get("code")
}

var urlParse = func(raw string) (*url.URL, error) { return url.Parse(raw) }

func TestOAuth2ProviderAuthorizationCodeFlowAndImmediateRevocation(t *testing.T) {
	provider, _ := newOAuth2ProviderForTest(t)
	client := createOAuth2ClientForTest(t, provider, OAuth2ClientTypeConfidential)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	request := authorizationRequest(client.Client.ID, verifier)

	preview, err := provider.ValidateAuthorizationRequest(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, client.Client.ID, preview.ClientID)
	require.Equal(t, []OAuth2Scope{
		{Name: "profile", Description: "Read the user's stable identifier and public profile"},
		{Name: "email", Description: "Read the user's email address"},
	}, preview.Scopes)

	authorized, err := provider.Authorize(context.Background(), 42, request, true)
	require.NoError(t, err)
	code := authorizationCodeFromRedirect(t, authorized.RedirectURL)
	require.NotEmpty(t, code)

	token, err := provider.ExchangeToken(context.Background(), OAuth2TokenRequest{
		GrantType:    "authorization_code",
		Code:         code,
		RedirectURI:  request.RedirectURI,
		ClientID:     client.Client.ID,
		ClientSecret: client.ClientSecret,
		CodeVerifier: verifier,
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer", token.TokenType)
	require.Equal(t, 600, token.ExpiresIn)
	require.Equal(t, "profile email", token.Scope)
	require.NotEmpty(t, token.AccessToken)

	_, err = provider.ExchangeToken(context.Background(), OAuth2TokenRequest{
		GrantType:    "authorization_code",
		Code:         code,
		RedirectURI:  request.RedirectURI,
		ClientID:     client.Client.ID,
		ClientSecret: client.ClientSecret,
		CodeVerifier: verifier,
	})
	requireOAuth2Error(t, err, "invalid_grant")

	userinfo, err := provider.UserInfo(context.Background(), token.AccessToken)
	require.NoError(t, err)
	require.NotEmpty(t, userinfo.Subject)
	require.Equal(t, "owner", userinfo.PreferredUsername)
	require.Equal(t, "owner@example.test", userinfo.Email)

	_, err = provider.UpdateClient(context.Background(), client.Client.ID, OAuth2ClientUpdateInput{Enabled: oauth2BoolPointer(false)})
	require.NoError(t, err)
	_, err = provider.UserInfo(context.Background(), token.AccessToken)
	requireOAuth2Error(t, err, "invalid_token")
}

func TestOAuth2ProviderRejectsRedirectScopeAndPKCEMismatch(t *testing.T) {
	provider, _ := newOAuth2ProviderForTest(t)
	client := createOAuth2ClientForTest(t, provider, OAuth2ClientTypePublic)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"

	wrongRedirect := authorizationRequest(client.Client.ID, verifier)
	wrongRedirect.RedirectURI = "https://client.example.test/callback?fixed=2"
	_, err := provider.ValidateAuthorizationRequest(context.Background(), wrongRedirect)
	requireOAuth2Error(t, err, "invalid_request")

	wrongScope := authorizationRequest(client.Client.ID, verifier)
	wrongScope.Scope = "profile admin"
	_, err = provider.ValidateAuthorizationRequest(context.Background(), wrongScope)
	requireOAuth2Error(t, err, "invalid_scope")

	authorized, err := provider.Authorize(context.Background(), 42, authorizationRequest(client.Client.ID, verifier), true)
	require.NoError(t, err)
	code := authorizationCodeFromRedirect(t, authorized.RedirectURL)
	_, err = provider.ExchangeToken(context.Background(), OAuth2TokenRequest{
		GrantType:    "authorization_code",
		Code:         code,
		RedirectURI:  "https://client.example.test/callback?fixed=1",
		ClientID:     client.Client.ID,
		CodeVerifier: "zyxwvutsrqponmlkjihgfedcbaABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~",
	})
	requireOAuth2Error(t, err, "invalid_grant")
}

func TestOAuth2ProviderSecretRotationAndDisableControls(t *testing.T) {
	provider, settings := newOAuth2ProviderForTest(t)
	client := createOAuth2ClientForTest(t, provider, OAuth2ClientTypeConfidential)
	rotated, err := provider.RotateClientSecret(context.Background(), client.Client.ID)
	require.NoError(t, err)
	require.NotEqual(t, client.ClientSecret, rotated.ClientSecret)

	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	issue := func() string {
		authorized, err := provider.Authorize(context.Background(), 42, authorizationRequest(client.Client.ID, verifier), true)
		require.NoError(t, err)
		return authorizationCodeFromRedirect(t, authorized.RedirectURL)
	}
	_, err = provider.ExchangeToken(context.Background(), OAuth2TokenRequest{
		GrantType:    "authorization_code",
		Code:         issue(),
		RedirectURI:  "https://client.example.test/callback?fixed=1",
		ClientID:     client.Client.ID,
		ClientSecret: client.ClientSecret,
		CodeVerifier: verifier,
	})
	requireOAuth2Error(t, err, "invalid_client")

	code := issue()
	token, err := provider.ExchangeToken(context.Background(), OAuth2TokenRequest{
		GrantType:    "authorization_code",
		Code:         code,
		RedirectURI:  "https://client.example.test/callback?fixed=1",
		ClientID:     client.Client.ID,
		ClientSecret: rotated.ClientSecret,
		CodeVerifier: verifier,
	})
	require.NoError(t, err)

	_, err = provider.UpdateAdminConfig(context.Background(), OAuth2ProviderConfigUpdate{
		Enabled:               false,
		Issuer:                "https://issuer.example.test",
		AccessTokenTTLSeconds: 600,
	})
	require.NoError(t, err)
	_, err = provider.UserInfo(context.Background(), token.AccessToken)
	requireOAuth2Error(t, err, "invalid_token")

	values, err := settings.GetAll(context.Background())
	require.NoError(t, err)
	var persisted OAuth2ProviderClient
	require.NoError(t, json.Unmarshal([]byte(values[oauth2ClientSettingKey(client.Client.ID)]), &persisted))
	require.NotEmpty(t, persisted.SecretHash)
	require.NotContains(t, values[oauth2ClientSettingKey(client.Client.ID)], rotated.ClientSecret)
}

func TestOAuth2ProviderDeniedAuthorizationKeepsState(t *testing.T) {
	provider, _ := newOAuth2ProviderForTest(t)
	client := createOAuth2ClientForTest(t, provider, OAuth2ClientTypePublic)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~"
	result, err := provider.Authorize(context.Background(), 42, authorizationRequest(client.Client.ID, verifier), false)
	require.NoError(t, err)
	parsed, err := urlParse(result.RedirectURL)
	require.NoError(t, err)
	require.Equal(t, "access_denied", parsed.Query().Get("error"))
	require.Equal(t, "state-123", parsed.Query().Get("state"))
	require.Empty(t, parsed.Query().Get("code"))
}

func TestOAuth2ProviderNormalizesLoopbackIPv6Issuer(t *testing.T) {
	issuer, err := normalizeOAuth2Issuer("http://[::1]:8080/")
	require.NoError(t, err)
	require.Equal(t, "http://[::1]:8080", issuer)

	issuer, err = normalizeOAuth2Issuer("http://[::1]/")
	require.NoError(t, err)
	require.Equal(t, "http://[::1]", issuer)
}

func requireOAuth2Error(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var protocol *OAuth2ProtocolError
	require.True(t, errors.As(err, &protocol))
	require.Equal(t, code, protocol.Code)
}

func oauth2BoolPointer(value bool) *bool { return &value }
