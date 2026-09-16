package service

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const (
	OAuth2ProviderConfigKey       = "oauth2_provider_config"
	oauth2ProviderClientKeyPrefix = "oauth2_provider_client."
	OAuth2AuthorizationCodeTTL    = 5 * time.Minute
	OAuth2DefaultAccessTokenTTL   = time.Hour
	OAuth2MinAccessTokenTTL       = 60 * time.Second
	OAuth2MaxAccessTokenTTL       = 24 * time.Hour
	OAuth2MaxClients              = 100
	OAuth2ClientTypeConfidential  = "confidential"
	OAuth2ClientTypePublic        = "public"
)

var (
	ErrOAuth2ProviderDisabled = infraerrors.Forbidden(
		"OAUTH2_PROVIDER_DISABLED",
		"OAuth2 provider is disabled",
	)
	ErrOAuth2ClientNotFound = infraerrors.NotFound(
		"OAUTH2_CLIENT_NOT_FOUND",
		"OAuth2 client not found",
	)
	ErrOAuth2ClientInvalid = infraerrors.BadRequest(
		"OAUTH2_CLIENT_INVALID",
		"OAuth2 client configuration is invalid",
	)
	ErrOAuth2IssuerInvalid = infraerrors.BadRequest(
		"OAUTH2_ISSUER_INVALID",
		"OAuth2 issuer must be an HTTPS origin (or a loopback HTTP origin)",
	)
	ErrOAuth2GrantNotFound = errors.New("oauth2 grant not found")
)

// OAuth2ProtocolError is serialized using the RFC 6749 error response shape.
// It deliberately stays separate from the site's JSON error envelope because
// OAuth clients parse these fields directly.
type OAuth2ProtocolError struct {
	HTTPStatus  int
	Code        string
	Description string
	Headers     map[string]string
}

func (e *OAuth2ProtocolError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Description == "" {
		return e.Code
	}
	return e.Code + ": " + e.Description
}

func newOAuth2ProtocolError(status int, code, description string) *OAuth2ProtocolError {
	return &OAuth2ProtocolError{HTTPStatus: status, Code: code, Description: description}
}

type OAuth2ProviderConfig struct {
	Enabled               bool   `json:"enabled"`
	Issuer                string `json:"issuer"`
	AccessTokenTTLSeconds int    `json:"access_token_ttl_seconds"`
}

type OAuth2ProviderClient struct {
	ID            string    `json:"client_id"`
	Name          string    `json:"name"`
	ClientType    string    `json:"client_type"`
	SecretHash    string    `json:"secret_hash"`
	RedirectURIs  []string  `json:"redirect_uris"`
	AllowedScopes []string  `json:"allowed_scopes"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type OAuth2ClientView struct {
	ID               string    `json:"client_id"`
	Name             string    `json:"name"`
	ClientType       string    `json:"client_type"`
	RedirectURIs     []string  `json:"redirect_uris"`
	AllowedScopes    []string  `json:"allowed_scopes"`
	Enabled          bool      `json:"enabled"`
	SecretConfigured bool      `json:"secret_configured"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type OAuth2Scope struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func OAuth2ProviderScopes() []OAuth2Scope {
	return []OAuth2Scope{
		{Name: "profile", Description: "Read the user's stable identifier and public profile"},
		{Name: "email", Description: "Read the user's email address"},
	}
}

type OAuth2ProviderConfigView struct {
	Enabled               bool               `json:"enabled"`
	Issuer                string             `json:"issuer"`
	AccessTokenTTLSeconds int                `json:"access_token_ttl_seconds"`
	Clients               []OAuth2ClientView `json:"clients"`
	Scopes                []OAuth2Scope      `json:"scopes"`
}

type OAuth2ProviderConfigUpdate struct {
	Enabled               bool
	Issuer                string
	AccessTokenTTLSeconds int
}

type OAuth2ClientCreateInput struct {
	Name          string
	ClientType    string
	RedirectURIs  []string
	AllowedScopes []string
	Enabled       *bool
}

type OAuth2ClientUpdateInput struct {
	Name          *string
	RedirectURIs  *[]string
	AllowedScopes *[]string
	Enabled       *bool
}

type OAuth2ClientSecretResult struct {
	Client       OAuth2ClientView `json:"client"`
	ClientSecret string           `json:"client_secret,omitempty"`
}

type OAuth2AuthorizationRequest struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

type OAuth2AuthorizationPreview struct {
	ClientID    string        `json:"client_id"`
	ClientName  string        `json:"client_name"`
	RedirectURI string        `json:"redirect_uri"`
	Scopes      []OAuth2Scope `json:"scopes"`
}

type OAuth2AuthorizationResult struct {
	RedirectURL string `json:"redirect_url"`
}

type OAuth2AuthorizationCodeData struct {
	ClientID            string    `json:"client_id"`
	UserID              int64     `json:"user_id"`
	RedirectURI         string    `json:"redirect_uri"`
	Scopes              []string  `json:"scopes"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	TokenVersion        int64     `json:"token_version"`
	IssuedAt            time.Time `json:"issued_at"`
}

type OAuth2AccessTokenData struct {
	ClientID     string    `json:"client_id"`
	UserID       int64     `json:"user_id"`
	Scopes       []string  `json:"scopes"`
	Issuer       string    `json:"issuer"`
	TokenVersion int64     `json:"token_version"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type OAuth2TokenRequest struct {
	GrantType    string
	Code         string
	RedirectURI  string
	ClientID     string
	ClientSecret string
	CodeVerifier string
}

type OAuth2TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

type OAuth2UserInfo struct {
	Subject           string `json:"sub"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Name              string `json:"name,omitempty"`
	Email             string `json:"email,omitempty"`
}

// OAuth2ProviderStore is the distributed, one-time grant store. Implementations
// must make ConsumeAuthorizationCode atomic across application instances.
type OAuth2ProviderStore interface {
	StoreAuthorizationCode(ctx context.Context, hash string, data *OAuth2AuthorizationCodeData, ttl time.Duration) error
	ConsumeAuthorizationCode(ctx context.Context, hash string) (*OAuth2AuthorizationCodeData, error)
	StoreAccessToken(ctx context.Context, hash string, data *OAuth2AccessTokenData, ttl time.Duration) error
	GetAccessToken(ctx context.Context, hash string) (*OAuth2AccessTokenData, error)
	DeleteAccessToken(ctx context.Context, hash string) error
}

type OAuth2ProviderService struct {
	settingRepo SettingRepository
	userRepo    UserRepository
	store       OAuth2ProviderStore
	cfg         *config.Config
}

func NewOAuth2ProviderService(
	settingRepo SettingRepository,
	userRepo UserRepository,
	store OAuth2ProviderStore,
	cfg *config.Config,
) *OAuth2ProviderService {
	return &OAuth2ProviderService{
		settingRepo: settingRepo,
		userRepo:    userRepo,
		store:       store,
		cfg:         cfg,
	}
}

func defaultOAuth2ProviderConfig() OAuth2ProviderConfig {
	return OAuth2ProviderConfig{
		AccessTokenTTLSeconds: int(OAuth2DefaultAccessTokenTTL / time.Second),
	}
}

func (s *OAuth2ProviderService) loadConfig(ctx context.Context) (OAuth2ProviderConfig, error) {
	if s == nil || s.settingRepo == nil {
		return OAuth2ProviderConfig{}, fmt.Errorf("oauth2 provider setting repository is unavailable")
	}
	cfg := defaultOAuth2ProviderConfig()
	raw, err := s.settingRepo.GetValue(ctx, OAuth2ProviderConfigKey)
	if errors.Is(err, ErrSettingNotFound) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, fmt.Errorf("decode oauth2 provider config: %w", err)
	}
	if cfg.AccessTokenTTLSeconds == 0 {
		cfg.AccessTokenTTLSeconds = int(OAuth2DefaultAccessTokenTTL / time.Second)
	}
	if _, err := normalizeOAuth2Issuer(cfg.Issuer); err != nil && strings.TrimSpace(cfg.Issuer) != "" {
		return cfg, fmt.Errorf("invalid persisted oauth2 issuer: %w", err)
	}
	if cfg.AccessTokenTTLSeconds < int(OAuth2MinAccessTokenTTL/time.Second) || cfg.AccessTokenTTLSeconds > int(OAuth2MaxAccessTokenTTL/time.Second) {
		return cfg, fmt.Errorf("persisted oauth2 access token ttl is out of range")
	}
	return cfg, nil
}

func (s *OAuth2ProviderService) saveConfig(ctx context.Context, cfg OAuth2ProviderConfig) error {
	payload, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode oauth2 provider config: %w", err)
	}
	return s.settingRepo.Set(ctx, OAuth2ProviderConfigKey, string(payload))
}

func oauth2ClientSettingKey(clientID string) string {
	return oauth2ProviderClientKeyPrefix + clientID
}

func (s *OAuth2ProviderService) loadClient(ctx context.Context, clientID string) (*OAuth2ProviderClient, error) {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" || len(clientID) > 128 {
		return nil, ErrOAuth2ClientNotFound
	}
	raw, err := s.settingRepo.GetValue(ctx, oauth2ClientSettingKey(clientID))
	if errors.Is(err, ErrSettingNotFound) {
		return nil, ErrOAuth2ClientNotFound
	}
	if err != nil {
		return nil, err
	}
	var client OAuth2ProviderClient
	if err := json.Unmarshal([]byte(raw), &client); err != nil {
		return nil, fmt.Errorf("decode oauth2 client: %w", err)
	}
	if client.ID != clientID {
		return nil, fmt.Errorf("oauth2 client id mismatch")
	}
	return &client, nil
}

func (s *OAuth2ProviderService) saveClient(ctx context.Context, client *OAuth2ProviderClient) error {
	payload, err := json.Marshal(client)
	if err != nil {
		return fmt.Errorf("encode oauth2 client: %w", err)
	}
	return s.settingRepo.Set(ctx, oauth2ClientSettingKey(client.ID), string(payload))
}

func clientView(client *OAuth2ProviderClient) OAuth2ClientView {
	if client == nil {
		return OAuth2ClientView{}
	}
	return OAuth2ClientView{
		ID:               client.ID,
		Name:             client.Name,
		ClientType:       client.ClientType,
		RedirectURIs:     append([]string(nil), client.RedirectURIs...),
		AllowedScopes:    append([]string(nil), client.AllowedScopes...),
		Enabled:          client.Enabled,
		SecretConfigured: client.SecretHash != "",
		CreatedAt:        client.CreatedAt,
		UpdatedAt:        client.UpdatedAt,
	}
}

func (s *OAuth2ProviderService) listClients(ctx context.Context) ([]OAuth2ProviderClient, error) {
	values, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	clients := make([]OAuth2ProviderClient, 0)
	for key, raw := range values {
		if !strings.HasPrefix(key, oauth2ProviderClientKeyPrefix) {
			continue
		}
		var client OAuth2ProviderClient
		if err := json.Unmarshal([]byte(raw), &client); err != nil {
			return nil, fmt.Errorf("decode oauth2 client %q: %w", key, err)
		}
		if client.ID == "" || client.ID != strings.TrimPrefix(key, oauth2ProviderClientKeyPrefix) {
			return nil, fmt.Errorf("oauth2 client key mismatch")
		}
		clients = append(clients, client)
	}
	sort.Slice(clients, func(i, j int) bool {
		if clients[i].CreatedAt.Equal(clients[j].CreatedAt) {
			return clients[i].ID < clients[j].ID
		}
		return clients[i].CreatedAt.Before(clients[j].CreatedAt)
	})
	return clients, nil
}

func (s *OAuth2ProviderService) GetAdminConfig(ctx context.Context) (*OAuth2ProviderConfigView, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	clients, err := s.listClients(ctx)
	if err != nil {
		return nil, err
	}
	view := &OAuth2ProviderConfigView{
		Enabled:               cfg.Enabled,
		Issuer:                cfg.Issuer,
		AccessTokenTTLSeconds: cfg.AccessTokenTTLSeconds,
		Scopes:                OAuth2ProviderScopes(),
		Clients:               make([]OAuth2ClientView, 0, len(clients)),
	}
	for i := range clients {
		view.Clients = append(view.Clients, clientView(&clients[i]))
	}
	return view, nil
}

func (s *OAuth2ProviderService) UpdateAdminConfig(ctx context.Context, input OAuth2ProviderConfigUpdate) (*OAuth2ProviderConfigView, error) {
	issuer, err := normalizeOAuth2Issuer(input.Issuer)
	if err != nil && strings.TrimSpace(input.Issuer) != "" {
		return nil, ErrOAuth2IssuerInvalid
	}
	ttl := input.AccessTokenTTLSeconds
	if ttl == 0 {
		ttl = int(OAuth2DefaultAccessTokenTTL / time.Second)
	}
	if ttl < int(OAuth2MinAccessTokenTTL/time.Second) || ttl > int(OAuth2MaxAccessTokenTTL/time.Second) {
		return nil, infraerrors.BadRequest("OAUTH2_TTL_INVALID", "access token lifetime must be between 60 and 86400 seconds")
	}
	if input.Enabled && issuer == "" {
		return nil, ErrOAuth2IssuerInvalid
	}
	cfg := OAuth2ProviderConfig{Enabled: input.Enabled, Issuer: issuer, AccessTokenTTLSeconds: ttl}
	if err := s.saveConfig(ctx, cfg); err != nil {
		return nil, err
	}
	return s.GetAdminConfig(ctx)
}

func normalizeOAuth2Issuer(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", ErrOAuth2IssuerInvalid
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if scheme != "https" && (scheme != "http" || !isOAuth2Loopback(host)) {
		return "", ErrOAuth2IssuerInvalid
	}
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if port == "" {
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		return scheme + "://" + host, nil
	}
	return scheme + "://" + net.JoinHostPort(host, port), nil
}

func isOAuth2Loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func normalizeOAuth2RedirectURI(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 {
		return "", ErrOAuth2ClientInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", ErrOAuth2ClientInvalid
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && (scheme != "http" || !isOAuth2Loopback(strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")))) {
		return "", ErrOAuth2ClientInvalid
	}
	return raw, nil
}

func normalizeOAuth2Scopes(scopes []string) ([]string, error) {
	known := make(map[string]struct{}, len(OAuth2ProviderScopes()))
	for _, scope := range OAuth2ProviderScopes() {
		known[scope.Name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(scopes))
	result := make([]string, 0, len(scopes))
	for _, raw := range scopes {
		for _, scope := range strings.Fields(raw) {
			if _, ok := known[scope]; !ok {
				return nil, ErrOAuth2ClientInvalid
			}
			if _, ok := seen[scope]; ok {
				continue
			}
			seen[scope] = struct{}{}
			result = append(result, scope)
		}
	}
	if len(result) == 0 {
		return nil, ErrOAuth2ClientInvalid
	}
	sort.Strings(result)
	return result, nil
}

func validateOAuth2ClientInput(name, clientType string, redirects, scopes []string) (string, []string, []string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 {
		return "", nil, nil, ErrOAuth2ClientInvalid
	}
	clientType = strings.ToLower(strings.TrimSpace(clientType))
	if clientType == "" {
		clientType = OAuth2ClientTypeConfidential
	}
	if clientType != OAuth2ClientTypeConfidential && clientType != OAuth2ClientTypePublic {
		return "", nil, nil, ErrOAuth2ClientInvalid
	}
	if len(redirects) == 0 || len(redirects) > 20 {
		return "", nil, nil, ErrOAuth2ClientInvalid
	}
	normalizedRedirects := make([]string, 0, len(redirects))
	seenRedirects := make(map[string]struct{}, len(redirects))
	for _, redirect := range redirects {
		normalized, err := normalizeOAuth2RedirectURI(redirect)
		if err != nil {
			return "", nil, nil, ErrOAuth2ClientInvalid
		}
		if _, ok := seenRedirects[normalized]; ok {
			return "", nil, nil, ErrOAuth2ClientInvalid
		}
		seenRedirects[normalized] = struct{}{}
		normalizedRedirects = append(normalizedRedirects, normalized)
	}
	normalizedScopes, err := normalizeOAuth2Scopes(scopes)
	if err != nil {
		return "", nil, nil, err
	}
	return name, normalizedRedirects, normalizedScopes, nil
}

func randomOAuth2String(prefix string, byteLength int) (string, error) {
	buf := make([]byte, byteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashOAuth2Secret(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *OAuth2ProviderService) CreateClient(ctx context.Context, input OAuth2ClientCreateInput) (*OAuth2ClientSecretResult, error) {
	clients, err := s.listClients(ctx)
	if err != nil {
		return nil, err
	}
	if len(clients) >= OAuth2MaxClients {
		return nil, infraerrors.BadRequest("OAUTH2_CLIENT_LIMIT_REACHED", "OAuth2 client limit reached")
	}
	name, redirects, scopes, err := validateOAuth2ClientInput(input.Name, input.ClientType, input.RedirectURIs, input.AllowedScopes)
	if err != nil {
		return nil, err
	}
	clientID, err := randomOAuth2String("s2c_", 18)
	if err != nil {
		return nil, fmt.Errorf("generate oauth2 client id: %w", err)
	}
	secret := ""
	secretHash := ""
	if strings.ToLower(strings.TrimSpace(input.ClientType)) != OAuth2ClientTypePublic {
		secret, err = randomOAuth2String("s2s_", 32)
		if err != nil {
			return nil, fmt.Errorf("generate oauth2 client secret: %w", err)
		}
		secretHash = hashOAuth2Secret(secret)
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	now := time.Now().UTC()
	client := &OAuth2ProviderClient{
		ID:            clientID,
		Name:          name,
		ClientType:    strings.ToLower(strings.TrimSpace(input.ClientType)),
		SecretHash:    secretHash,
		RedirectURIs:  redirects,
		AllowedScopes: scopes,
		Enabled:       enabled,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if client.ClientType == "" {
		client.ClientType = OAuth2ClientTypeConfidential
	}
	if err := s.saveClient(ctx, client); err != nil {
		return nil, err
	}
	return &OAuth2ClientSecretResult{Client: clientView(client), ClientSecret: secret}, nil
}

func (s *OAuth2ProviderService) UpdateClient(ctx context.Context, clientID string, input OAuth2ClientUpdateInput) (*OAuth2ClientView, error) {
	client, err := s.loadClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" || len([]rune(name)) > 100 {
			return nil, ErrOAuth2ClientInvalid
		}
		client.Name = name
	}
	if input.RedirectURIs != nil {
		_, redirects, _, err := validateOAuth2ClientInput(client.Name, client.ClientType, *input.RedirectURIs, client.AllowedScopes)
		if err != nil {
			return nil, err
		}
		client.RedirectURIs = redirects
	}
	if input.AllowedScopes != nil {
		_, _, scopes, err := validateOAuth2ClientInput(client.Name, client.ClientType, client.RedirectURIs, *input.AllowedScopes)
		if err != nil {
			return nil, err
		}
		client.AllowedScopes = scopes
	}
	if input.Enabled != nil {
		client.Enabled = *input.Enabled
	}
	client.UpdatedAt = time.Now().UTC()
	if err := s.saveClient(ctx, client); err != nil {
		return nil, err
	}
	view := clientView(client)
	return &view, nil
}

func (s *OAuth2ProviderService) DeleteClient(ctx context.Context, clientID string) error {
	if _, err := s.loadClient(ctx, clientID); err != nil {
		return err
	}
	return s.settingRepo.Delete(ctx, oauth2ClientSettingKey(strings.TrimSpace(clientID)))
}

func (s *OAuth2ProviderService) RotateClientSecret(ctx context.Context, clientID string) (*OAuth2ClientSecretResult, error) {
	client, err := s.loadClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if client.ClientType != OAuth2ClientTypeConfidential {
		return nil, ErrOAuth2ClientInvalid
	}
	secret, err := randomOAuth2String("s2s_", 32)
	if err != nil {
		return nil, fmt.Errorf("generate oauth2 client secret: %w", err)
	}
	client.SecretHash = hashOAuth2Secret(secret)
	client.UpdatedAt = time.Now().UTC()
	if err := s.saveClient(ctx, client); err != nil {
		return nil, err
	}
	return &OAuth2ClientSecretResult{Client: clientView(client), ClientSecret: secret}, nil
}

func (s *OAuth2ProviderService) requireEnabledClient(ctx context.Context, clientID string) (OAuth2ProviderConfig, *OAuth2ProviderClient, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return cfg, nil, err
	}
	if !cfg.Enabled {
		return cfg, nil, ErrOAuth2ProviderDisabled
	}
	client, err := s.loadClient(ctx, clientID)
	if err != nil {
		return cfg, nil, err
	}
	if !client.Enabled {
		return cfg, nil, ErrOAuth2ClientNotFound
	}
	return cfg, client, nil
}

func (s *OAuth2ProviderService) ValidateAuthorizationRequest(ctx context.Context, input OAuth2AuthorizationRequest) (*OAuth2AuthorizationPreview, error) {
	_, client, err := s.requireEnabledClient(ctx, input.ClientID)
	if err != nil {
		return nil, err
	}
	if input.ResponseType != "code" {
		return nil, newOAuth2ProtocolError(400, "unsupported_response_type", "response_type must be code")
	}
	if input.State == "" || len(input.State) > 512 {
		return nil, newOAuth2ProtocolError(400, "invalid_request", "state is required")
	}
	if input.RedirectURI == "" || !containsExact(client.RedirectURIs, input.RedirectURI) {
		return nil, newOAuth2ProtocolError(400, "invalid_request", "redirect_uri does not match the registered client")
	}
	if input.CodeChallengeMethod != "S256" || !isOAuth2CodeChallenge(input.CodeChallenge) {
		return nil, newOAuth2ProtocolError(400, "invalid_request", "PKCE S256 is required")
	}
	scopes, err := requestedOAuth2Scopes(input.Scope, client.AllowedScopes)
	if err != nil {
		return nil, err
	}
	return &OAuth2AuthorizationPreview{
		ClientID:    client.ID,
		ClientName:  client.Name,
		RedirectURI: input.RedirectURI,
		Scopes:      scopeViews(scopes),
	}, nil
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func isOAuth2CodeChallenge(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && !strings.ContainsRune("-._~", r) {
			return false
		}
	}
	return true
}

func requestedOAuth2Scopes(raw string, allowed []string) ([]string, error) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, scope := range allowed {
		allowedSet[scope] = struct{}{}
	}
	requested := strings.Fields(raw)
	if len(requested) == 0 {
		if _, ok := allowedSet["profile"]; ok {
			requested = []string{"profile"}
		} else if len(allowed) > 0 {
			requested = []string{allowed[0]}
		}
	}
	seen := make(map[string]struct{}, len(requested))
	result := make([]string, 0, len(requested))
	for _, scope := range requested {
		if _, ok := allowedSet[scope]; !ok {
			return nil, newOAuth2ProtocolError(400, "invalid_scope", "requested scope is not allowed for this client")
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	if len(result) == 0 {
		return nil, newOAuth2ProtocolError(400, "invalid_scope", "at least one scope is required")
	}
	return result, nil
}

func scopeViews(names []string) []OAuth2Scope {
	known := make(map[string]OAuth2Scope, len(OAuth2ProviderScopes()))
	for _, scope := range OAuth2ProviderScopes() {
		known[scope.Name] = scope
	}
	result := make([]OAuth2Scope, 0, len(names))
	for _, name := range names {
		if scope, ok := known[name]; ok {
			result = append(result, scope)
		}
	}
	return result
}

func (s *OAuth2ProviderService) Authorize(ctx context.Context, userID int64, input OAuth2AuthorizationRequest, approved bool) (*OAuth2AuthorizationResult, error) {
	preview, err := s.ValidateAuthorizationRequest(ctx, input)
	if err != nil {
		return nil, err
	}
	if !approved {
		return &OAuth2AuthorizationResult{RedirectURL: oauth2RedirectWithParams(input.RedirectURI, map[string]string{
			"error":             "access_denied",
			"error_description": "The resource owner denied the request",
			"state":             input.State,
		})}, nil
	}
	if s.userRepo == nil {
		return nil, fmt.Errorf("oauth2 user repository is unavailable")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive() {
		return nil, infraerrors.Unauthorized("OAUTH2_USER_INACTIVE", "user is not active")
	}
	normalizeLoadedUserTokenVersion(user)
	code, err := randomOAuth2String("", 32)
	if err != nil {
		return nil, fmt.Errorf("generate oauth2 authorization code: %w", err)
	}
	data := &OAuth2AuthorizationCodeData{
		ClientID:            input.ClientID,
		UserID:              userID,
		RedirectURI:         input.RedirectURI,
		Scopes:              scopesFromViews(preview.Scopes),
		CodeChallenge:       input.CodeChallenge,
		CodeChallengeMethod: input.CodeChallengeMethod,
		TokenVersion:        resolvedTokenVersion(user),
		IssuedAt:            time.Now().UTC(),
	}
	if err := s.store.StoreAuthorizationCode(ctx, hashOAuth2Secret(code), data, OAuth2AuthorizationCodeTTL); err != nil {
		return nil, err
	}
	return &OAuth2AuthorizationResult{RedirectURL: oauth2RedirectWithParams(input.RedirectURI, map[string]string{
		"code":  code,
		"state": input.State,
	})}, nil
}

func scopesFromViews(scopes []OAuth2Scope) []string {
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		result = append(result, scope.Name)
	}
	return result
}

func oauth2RedirectWithParams(raw string, params map[string]string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	query := u.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func (s *OAuth2ProviderService) authenticateClient(ctx context.Context, clientID, clientSecret string) (*OAuth2ProviderClient, error) {
	if strings.TrimSpace(clientID) == "" {
		return nil, newOAuth2ProtocolError(401, "invalid_client", "client authentication is required")
	}
	_, client, err := s.requireEnabledClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, ErrOAuth2ProviderDisabled) || errors.Is(err, ErrOAuth2ClientNotFound) {
			return nil, newOAuth2ProtocolError(401, "invalid_client", "client authentication failed")
		}
		return nil, err
	}
	if client.ClientType == OAuth2ClientTypePublic {
		if strings.TrimSpace(clientSecret) != "" {
			return nil, newOAuth2ProtocolError(401, "invalid_client", "public clients must not send a client secret")
		}
		return client, nil
	}
	if client.SecretHash == "" || !hmac.Equal([]byte(client.SecretHash), []byte(hashOAuth2Secret(clientSecret))) {
		return nil, newOAuth2ProtocolError(401, "invalid_client", "client authentication failed")
	}
	return client, nil
}

func (s *OAuth2ProviderService) ExchangeToken(ctx context.Context, input OAuth2TokenRequest) (*OAuth2TokenResponse, error) {
	if input.GrantType != "authorization_code" {
		return nil, newOAuth2ProtocolError(400, "unsupported_grant_type", "only authorization_code is supported")
	}
	client, err := s.authenticateClient(ctx, input.ClientID, input.ClientSecret)
	if err != nil {
		return nil, err
	}
	if input.Code == "" || len(input.Code) > 256 || input.CodeVerifier == "" {
		return nil, newOAuth2ProtocolError(400, "invalid_grant", "authorization code and code_verifier are required")
	}
	data, err := s.store.ConsumeAuthorizationCode(ctx, hashOAuth2Secret(input.Code))
	if errors.Is(err, ErrOAuth2GrantNotFound) {
		return nil, newOAuth2ProtocolError(400, "invalid_grant", "authorization code is invalid or expired")
	}
	if err != nil {
		return nil, err
	}
	if data.ClientID != client.ID || data.RedirectURI != input.RedirectURI || !verifyOAuth2CodeVerifier(input.CodeVerifier, data.CodeChallenge) {
		return nil, newOAuth2ProtocolError(400, "invalid_grant", "authorization code validation failed")
	}
	if !containsExact(client.RedirectURIs, data.RedirectURI) {
		return nil, newOAuth2ProtocolError(400, "invalid_grant", "registered redirect URI changed after authorization")
	}
	for _, scope := range data.Scopes {
		if !containsExact(client.AllowedScopes, scope) {
			return nil, newOAuth2ProtocolError(400, "invalid_grant", "client scope changed after authorization")
		}
	}
	if s.userRepo == nil {
		return nil, fmt.Errorf("oauth2 user repository is unavailable")
	}
	user, err := s.userRepo.GetByID(ctx, data.UserID)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsActive() || resolvedTokenVersion(user) != data.TokenVersion {
		return nil, newOAuth2ProtocolError(400, "invalid_grant", "authorization is no longer valid")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || cfg.Issuer == "" {
		return nil, newOAuth2ProtocolError(400, "invalid_grant", "OAuth2 provider was disabled after authorization")
	}
	ttl := time.Duration(cfg.AccessTokenTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = OAuth2DefaultAccessTokenTTL
	}
	accessToken, err := randomOAuth2String("s2a_", 32)
	if err != nil {
		return nil, fmt.Errorf("generate oauth2 access token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(ttl)
	accessData := &OAuth2AccessTokenData{
		ClientID:     client.ID,
		UserID:       data.UserID,
		Scopes:       append([]string(nil), data.Scopes...),
		Issuer:       cfg.Issuer,
		TokenVersion: data.TokenVersion,
		ExpiresAt:    expiresAt,
	}
	if err := s.store.StoreAccessToken(ctx, hashOAuth2Secret(accessToken), accessData, ttl); err != nil {
		return nil, err
	}
	return &OAuth2TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(ttl / time.Second),
		Scope:       strings.Join(data.Scopes, " "),
	}, nil
}

func verifyOAuth2CodeVerifier(verifier, challenge string) bool {
	if !isOAuth2CodeChallenge(verifier) {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return hmac.Equal([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge))
}

func (s *OAuth2ProviderService) validateAccessToken(ctx context.Context, raw string) (*OAuth2AccessTokenData, *OAuth2ProviderClient, *User, error) {
	if strings.TrimSpace(raw) == "" || len(raw) > 256 {
		return nil, nil, nil, newOAuth2ProtocolError(401, "invalid_token", "access token is invalid")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	if !cfg.Enabled {
		return nil, nil, nil, newOAuth2ProtocolError(401, "invalid_token", "OAuth2 provider is disabled")
	}
	data, err := s.store.GetAccessToken(ctx, hashOAuth2Secret(raw))
	if errors.Is(err, ErrOAuth2GrantNotFound) || data == nil {
		return nil, nil, nil, newOAuth2ProtocolError(401, "invalid_token", "access token is invalid or expired")
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if !data.ExpiresAt.IsZero() && time.Now().UTC().After(data.ExpiresAt) {
		return nil, nil, nil, newOAuth2ProtocolError(401, "invalid_token", "access token is expired")
	}
	client, err := s.loadClient(ctx, data.ClientID)
	if err != nil {
		if errors.Is(err, ErrOAuth2ClientNotFound) {
			return nil, nil, nil, newOAuth2ProtocolError(401, "invalid_token", "access token is no longer authorized")
		}
		return nil, nil, nil, err
	}
	if client == nil || !client.Enabled || data.Issuer != cfg.Issuer {
		return nil, nil, nil, newOAuth2ProtocolError(401, "invalid_token", "access token is no longer authorized")
	}
	for _, scope := range data.Scopes {
		if !containsExact(client.AllowedScopes, scope) {
			return nil, nil, nil, newOAuth2ProtocolError(401, "invalid_token", "access token scope is no longer allowed")
		}
	}
	if s.userRepo == nil {
		return nil, nil, nil, fmt.Errorf("oauth2 user repository is unavailable")
	}
	user, err := s.userRepo.GetByID(ctx, data.UserID)
	if err != nil {
		return nil, nil, nil, err
	}
	if user == nil || !user.IsActive() || resolvedTokenVersion(user) != data.TokenVersion {
		return nil, nil, nil, newOAuth2ProtocolError(401, "invalid_token", "user authorization is no longer valid")
	}
	return data, client, user, nil
}

func (s *OAuth2ProviderService) UserInfo(ctx context.Context, rawToken string) (*OAuth2UserInfo, error) {
	data, client, user, err := s.validateAccessToken(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	result := &OAuth2UserInfo{Subject: s.pairwiseSubject(client.ID, user.ID)}
	if containsExact(data.Scopes, "profile") {
		result.PreferredUsername = user.Username
		result.Name = user.Username
	}
	if containsExact(data.Scopes, "email") {
		result.Email = user.Email
	}
	return result, nil
}

func (s *OAuth2ProviderService) pairwiseSubject(clientID string, userID int64) string {
	secret := "sub2api-oauth2-subject"
	if s != nil && s.cfg != nil && strings.TrimSpace(s.cfg.JWT.Secret) != "" {
		secret = s.cfg.JWT.Secret
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("oauth2-subject-v1\x00" + clientID + "\x00" + strconv.FormatInt(userID, 10)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *OAuth2ProviderService) RevokeToken(ctx context.Context, rawToken, clientID, clientSecret string) error {
	client, err := s.authenticateClient(ctx, clientID, clientSecret)
	if err != nil {
		return err
	}
	if strings.TrimSpace(rawToken) == "" {
		return newOAuth2ProtocolError(400, "invalid_request", "token is required")
	}
	data, err := s.store.GetAccessToken(ctx, hashOAuth2Secret(rawToken))
	if errors.Is(err, ErrOAuth2GrantNotFound) || data == nil {
		return nil
	}
	if err != nil {
		return err
	}
	if data.ClientID != client.ID {
		return nil
	}
	return s.store.DeleteAccessToken(ctx, hashOAuth2Secret(rawToken))
}

type OAuth2DiscoveryDocument struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	RevocationEndpoint                string   `json:"revocation_endpoint"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
}

func (s *OAuth2ProviderService) Discovery(ctx context.Context) (*OAuth2DiscoveryDocument, error) {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || cfg.Issuer == "" {
		return nil, ErrOAuth2ProviderDisabled
	}
	scopes := OAuth2ProviderScopes()
	scopeNames := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scopeNames = append(scopeNames, scope.Name)
	}
	base := strings.TrimRight(cfg.Issuer, "/")
	return &OAuth2DiscoveryDocument{
		Issuer:                            cfg.Issuer,
		AuthorizationEndpoint:             base + "/oauth/authorize",
		TokenEndpoint:                     base + "/oauth/token",
		RevocationEndpoint:                base + "/oauth/revoke",
		UserinfoEndpoint:                  base + "/oauth/userinfo",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic", "client_secret_post", "none"},
		ScopesSupported:                   scopeNames,
	}, nil
}
