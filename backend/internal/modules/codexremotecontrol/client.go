// Package codexremotecontrol owns desktop client enrollment and device-key
// challenge lifecycles. The separate remotecontrol server protocol is unchanged.
package codexremotecontrol

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/remotecontrol"
	"github.com/coder/websocket"
)

const clientPath = "codex/remote/control/client"
const SessionTokenHeader = "x-codex-client-session-token"

type DeviceKeys interface {
	remotecontrol.DeviceSigner
	CreateDeviceKey(context.Context, string) (remotecontrol.DeviceKeyPublic, error)
	GetDeviceKeyPublic(context.Context, string) (remotecontrol.DeviceKeyPublic, error)
	DeleteDeviceKey(context.Context, string) error
}

type Record struct {
	AccountUserID string                        `json:"accountUserId"`
	ClientID      string                        `json:"clientId"`
	Key           remotecontrol.DeviceKeyPublic `json:"deviceKey"`
}

// Stores are scoped to this account, installation, and API origin. Store only
// key references/public metadata; session and step-up tokens are never persisted.
type EnrollmentStore interface {
	Load(context.Context) (*Record, error)
	Save(context.Context, Record) error
	Clear(context.Context) error
}

type Auth struct {
	Token         string
	AccountUserID string
	AccountID     string
}

type StepUp func(context.Context, string) (string, error)

type Config struct {
	DesktopHeaders   http.Header
	BaseURL          string
	Auth             Auth
	Keys             DeviceKeys
	Store            EnrollmentStore
	HTTPClient       *http.Client
	RequestStepUp    StepUp
	HandshakeTimeout time.Duration
}

type Manager struct {
	config Config
	base   *url.URL
	client *http.Client
	mu     sync.Mutex
	now    func() time.Time
}

type Session struct {
	ClientID               string
	AccountUserID          string
	Token                  string
	ExpiresAt              int64
	Scopes                 []string
	RequiresDeviceKeyProof bool
	key                    remotecontrol.DeviceKeyPublic
}

func New(config Config) (*Manager, error) {
	base, err := url.Parse(config.BaseURL)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("invalid remote control client target")
	}
	host := strings.ToLower(base.Hostname())
	ip := net.ParseIP(host)
	local := host == "localhost" || (ip != nil && ip.IsLoopback())
	firstParty := host == "chatgpt.com" || host == "chatgpt-staging.com" || strings.HasSuffix(host, ".chatgpt.com") || strings.HasSuffix(host, ".chatgpt-staging.com")
	if (!local && (!firstParty || base.Scheme != "https" || base.Port() != "")) || (local && base.Scheme != "http" && base.Scheme != "https") {
		return nil, errors.New("remote control client target must be first-party HTTPS or loopback")
	}
	if config.Auth.Token == "" || config.Auth.AccountUserID == "" || config.Keys == nil || config.Store == nil {
		return nil, errors.New("remote control client identity is incomplete")
	}
	if strings.ContainsAny(config.Auth.Token+config.Auth.AccountID, "\r\n") {
		return nil, errors.New("invalid remote control authentication")
	}
	if err := validateAuth(config.Auth); err != nil {
		return nil, err
	}
	client := http.Client{Timeout: 20 * time.Second}
	if config.HTTPClient != nil {
		client = *config.HTTPClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	base.Path = strings.TrimSuffix(base.Path, "/") + "/"
	config.DesktopHeaders = config.DesktopHeaders.Clone()
	return &Manager{config: config, base: base, client: &client, now: time.Now}, nil
}

func (m *Manager) endpoint(path string) string {
	u := *m.base
	u.Path += clientPath + path
	return u.String()
}

func (m *Manager) authHeaders() http.Header {
	h := make(http.Header)
	for _, name := range []string{"Originator", "User-Agent", "OAI-Product-Sku", "X-Codex-Surface", "Version"} {
		if value := m.config.DesktopHeaders.Get(name); value != "" && !strings.ContainsAny(value, "\r\n") {
			h.Set(name, value)
		}
	}
	if h.Get("Originator") == "" {
		h.Set("Originator", "Codex Desktop")
	}
	if h.Get("OAI-Product-Sku") == "" {
		h.Set("OAI-Product-Sku", "CODEX")
	}
	h.Set("Authorization", "Bearer "+m.config.Auth.Token)
	if m.config.Auth.AccountID != "" {
		h.Set("ChatGPT-Account-ID", m.config.Auth.AccountID)
	}
	h.Set("Content-Type", "application/json")
	return h
}

type challengeResponse struct {
	AccountUserID string                                  `json:"account_user_id"`
	ClientID      string                                  `json:"client_id"`
	Challenge     remotecontrol.EnrollmentDeviceChallenge `json:"device_key_challenge"`
}

type sessionResponse struct {
	AccountUserID string   `json:"account_user_id"`
	ClientID      string   `json:"client_id"`
	Token         string   `json:"remote_control_token"`
	ExpiresAt     string   `json:"expires_at"`
	Scopes        []string `json:"scopes"`
}

func (m *Manager) ConnectSession(ctx context.Context) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, err := m.config.Store.Load(ctx)
	if err != nil {
		return nil, err
	}
	if record != nil {
		if !m.permittedAccountUserID(record.AccountUserID) {
			return nil, errors.New("stored remote control enrollment belongs to another account")
		}
		public, err := m.config.Keys.GetDeviceKeyPublic(ctx, record.Key.KeyID)
		if err != nil {
			return nil, err
		}
		if public != record.Key {
			return nil, errors.New("stored remote control enrollment does not match local device key")
		}
		session, err := m.finish(ctx, *record, true)
		var rejected *requestError
		if !errors.As(err, &rejected) || !rejected.revoked {
			return session, err
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
		_ = m.config.Keys.DeleteDeviceKey(cleanupCtx, record.Key.KeyID)
		cancel()
		if err := m.config.Store.Clear(ctx); err != nil {
			return nil, err
		}
	}
	if m.config.RequestStepUp == nil {
		return nil, errors.New("remote control enrollment requires explicit step-up authorization")
	}
	var start challengeResponse
	if err := m.doJSON(ctx, "/enroll/start", struct{}{}, &start); err != nil {
		return nil, err
	}
	if !m.permittedAccountUserID(start.AccountUserID) || start.ClientID == "" {
		return nil, errors.New("remote control enrollment start does not match current account")
	}
	key, err := m.config.Keys.CreateDeviceKey(ctx, "allow_os_protected_nonextractable")
	if err != nil {
		return nil, err
	}
	record = &Record{start.AccountUserID, start.ClientID, key}
	session, err := m.enrollFinish(ctx, *record, start.Challenge)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
		defer cancel()
		_ = m.config.Keys.DeleteDeviceKey(cleanupCtx, key.KeyID)
		return nil, err
	}
	return session, nil
}

func (m *Manager) enrollFinish(ctx context.Context, record Record, challenge remotecontrol.EnrollmentDeviceChallenge) (*Session, error) {
	stepUp, err := m.config.RequestStepUp(ctx, m.config.Auth.AccountID)
	if err != nil {
		return nil, err
	}
	if err := ValidateStepUp(stepUp, m.config.Auth.AccountUserID, m.now()); err != nil {
		return nil, err
	}
	proof, err := m.signEnrollment(ctx, record, challenge, "/enroll/finish", false)
	if err != nil {
		return nil, err
	}
	body := struct {
		ClientID    string                              `json:"client_id"`
		StepUpToken string                              `json:"step_up_token"`
		Identity    any                                 `json:"device_identity"`
		Proof       remotecontrol.EnrollmentDeviceProof `json:"device_key_proof"`
	}{record.ClientID, stepUp, struct {
		KeyID           string `json:"key_id"`
		PublicKey       string `json:"public_key_spki_der_base64"`
		Algorithm       string `json:"algorithm"`
		ProtectionClass string `json:"protection_class"`
	}{record.Key.KeyID, record.Key.PublicKeySPKIDERBase64, record.Key.Algorithm, record.Key.ProtectionClass}, proof}
	var response sessionResponse
	if err := m.doJSON(ctx, "/enroll/finish", body, &response); err != nil {
		return nil, err
	}
	session, err := m.validateSession(record, response)
	if err != nil {
		return nil, err
	}
	if err := m.config.Store.Save(ctx, record); err != nil {
		return nil, err
	}
	return session, nil
}

func (m *Manager) finish(ctx context.Context, record Record, requireHash bool) (*Session, error) {
	var start challengeResponse
	if err := m.doJSON(ctx, "/refresh/start", map[string]string{"client_id": record.ClientID}, &start); err != nil {
		return nil, err
	}
	if start.ClientID != record.ClientID || start.AccountUserID != record.AccountUserID {
		return nil, errors.New("remote control refresh start does not match local enrollment")
	}
	proof, err := m.signEnrollment(ctx, record, start.Challenge, "/refresh/finish", requireHash)
	if err != nil {
		return nil, err
	}
	var response sessionResponse
	if err := m.doJSON(ctx, "/refresh/finish", struct {
		ClientID string                              `json:"client_id"`
		Proof    remotecontrol.EnrollmentDeviceProof `json:"device_key_proof"`
	}{record.ClientID, proof}, &response); err != nil {
		return nil, err
	}
	return m.validateSession(record, response)
}

func (m *Manager) signEnrollment(ctx context.Context, record Record, challenge remotecontrol.EnrollmentDeviceChallenge, path string, requireHash bool) (remotecontrol.EnrollmentDeviceProof, error) {
	u, _ := url.Parse(m.endpoint(path))
	return remotecontrol.SignEnrollmentChallenge(ctx, m.config.Keys, record.Key, challenge, record.AccountUserID, record.ClientID, u.Scheme+"://"+u.Host, u.Path, requireHash, m.now())
}

func (m *Manager) validateSession(record Record, response sessionResponse) (*Session, error) {
	expires, err := time.Parse(time.RFC3339Nano, response.ExpiresAt)
	if err != nil || !expires.After(m.now()) || response.ClientID != record.ClientID || response.AccountUserID != record.AccountUserID || response.Token == "" || strings.ContainsAny(response.Token, "\r\n") || len(response.Scopes) != 1 || response.Scopes[0] != remotecontrol.DeviceControllerScope {
		return nil, errors.New("remote control token response does not match local enrollment")
	}
	return &Session{response.ClientID, response.AccountUserID, response.Token, expires.Unix(), append([]string(nil), response.Scopes...), true, record.Key}, nil
}

func (m *Manager) Dial(ctx context.Context, session *Session, subscribeCursor string) (*websocket.Conn, *http.Response, error) {
	if session == nil || !session.RequiresDeviceKeyProof || !m.permittedAccountUserID(session.AccountUserID) || session.ExpiresAt <= m.now().Unix() {
		return nil, nil, errors.New("remote control client session unavailable")
	}
	u, _ := url.Parse(m.endpoint(""))
	origin := u.Scheme + "://" + u.Host
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	headers := m.authHeaders()
	headers.Set(SessionTokenHeader, "Bearer "+session.Token)
	headers.Set("x-codex-client-id", session.ClientID)
	headers.Set(remotecontrol.ProtocolVersionHeader, remotecontrol.ProtocolVersion)
	if subscribeCursor != "" {
		headers.Set(remotecontrol.SubscribeCursorHeader, subscribeCursor)
	}
	if strings.ContainsAny(subscribeCursor, "\r\n") {
		return nil, nil, errors.New("invalid remote control subscribe cursor")
	}
	timeout := m.config.HandshakeTimeout
	if timeout <= 0 || timeout > time.Minute {
		timeout = 10 * time.Second
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, response, err := websocket.Dial(handshakeCtx, u.String(), &websocket.DialOptions{HTTPClient: m.client, HTTPHeader: headers})
	if err != nil {
		return nil, response, errors.New("remote control client websocket connection failed")
	}
	conn.SetReadLimit(32 * 1024)
	for count := 0; count < 64; count++ {
		kind, data, readErr := conn.Read(handshakeCtx)
		if readErr != nil {
			_ = conn.CloseNow()
			return nil, response, errors.New("remote control device-key challenge unavailable")
		}
		var challenge remotecontrol.ConnectionDeviceChallenge
		if kind != websocket.MessageText || json.Unmarshal(data, &challenge) != nil || challenge.Type != "device_key_challenge" {
			continue
		}
		proof, err := remotecontrol.SignConnectionChallenge(handshakeCtx, m.config.Keys, session.key, challenge, session.AccountUserID, session.ClientID, origin, u.Path, session.Token, session.ExpiresAt, m.now())
		if err != nil {
			_ = conn.CloseNow()
			return nil, response, err
		}
		payload, err := json.Marshal(proof)
		if err == nil {
			err = conn.Write(handshakeCtx, websocket.MessageText, payload)
		}
		if err != nil {
			_ = conn.CloseNow()
			return nil, response, errors.New("remote control device-key proof send failed")
		}
		conn.SetReadLimit(8 << 20)
		return conn, response, nil
	}
	_ = conn.CloseNow()
	return nil, response, errors.New("remote control device-key challenge unavailable")
}

func (m *Manager) doJSON(ctx context.Context, path string, input, output any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint(path), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header = m.authHeaders()
	response, err := m.client.Do(request)
	if err != nil {
		return errors.New("remote control client request failed")
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, 32*1024+1))
	if err != nil || len(body) > 32*1024 {
		return errors.New("invalid remote control client response size")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var parsed struct {
			Detail string `json:"detail"`
		}
		detail := strings.TrimSpace(string(body))
		if json.Unmarshal(body, &parsed) == nil && parsed.Detail != "" {
			detail = parsed.Detail
		}
		revoked := (response.StatusCode == 404 && detail == "Remote-control client not found") ||
			(response.StatusCode == 401 && detail == "Remote-control client enrollment is incomplete") ||
			(response.StatusCode == 403 && detail == "Remote-control client key material missing") ||
			((response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 404) && detail == "Remote-control client has been revoked")
		return &requestError{revoked: revoked}
	}
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("malformed remote control client response")
	}
	return nil
}

type requestError struct{ revoked bool }

func (*requestError) Error() string { return "remote control client request was rejected" }

// The desktop accepts a legacy auth-user enrollment only when the selected
// ChatGPT account header matches the account claim in the same OAuth token.
func (m *Manager) permittedAccountUserID(id string) bool {
	if id == m.config.Auth.AccountUserID {
		return true
	}
	claims, err := decodeToken(m.config.Auth.Token)
	if err != nil {
		return false
	}
	var value struct {
		Auth struct {
			UserID          string `json:"user_id"`
			AccountID       string `json:"chatgpt_account_id"`
			LegacyAccountID string `json:"account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(claims, &value) != nil {
		return false
	}
	account := value.Auth.AccountID
	if account == "" {
		account = value.Auth.LegacyAccountID
	}
	return id != "" && id == value.Auth.UserID && account != "" && m.config.Auth.AccountID == account
}

func AuthFromToken(token, accountID string) (Auth, error) {
	claims, err := decodeToken(token)
	if err != nil {
		return Auth{}, errors.New("remote control client requires OAuth identity claims")
	}
	var value struct {
		Auth struct {
			AccountUserID string `json:"chatgpt_account_user_id"`
			LegacyUserID  string `json:"account_user_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(claims, &value) != nil {
		return Auth{}, errors.New("invalid remote control OAuth identity")
	}
	user := value.Auth.AccountUserID
	if user == "" {
		user = value.Auth.LegacyUserID
	}
	auth := Auth{Token: token, AccountUserID: user, AccountID: accountID}
	if err := validateAuth(auth); err != nil {
		return Auth{}, err
	}
	return auth, nil
}

func ValidateStepUp(token, accountUserID string, now time.Time) error {
	invalid := errors.New("remote control enrollment step-up token is invalid for current account")
	claims, err := decodeToken(token)
	if err != nil {
		return invalid
	}
	var value struct {
		IssuedAt         int64    `json:"iat"`
		PasswordAuthTime int64    `json:"pwd_auth_time"`
		Scope            string   `json:"scope"`
		Scopes           []string `json:"scp"`
		Auth             struct {
			AccountUserID string `json:"chatgpt_account_user_id"`
			LegacyUserID  string `json:"account_user_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(claims, &value) != nil {
		return invalid
	}
	user := value.Auth.AccountUserID
	if user == "" {
		user = value.Auth.LegacyUserID
	}
	if user == "" || user != accountUserID || value.IssuedAt <= 0 || now.Unix()-value.IssuedAt > 300 || value.IssuedAt > now.Unix()+30 || value.PasswordAuthTime <= 0 || now.UnixMilli()-value.PasswordAuthTime > 300000 || value.PasswordAuthTime > now.UnixMilli()+30000 {
		return invalid
	}
	scopes := map[string]bool{}
	for _, scope := range append(strings.Fields(value.Scope), value.Scopes...) {
		if scope != "" {
			scopes[scope] = true
		}
	}
	if len(scopes) != 1 || !scopes["codex.remote_control.enroll"] {
		return invalid
	}
	// This is the desktop's claim consistency/freshness check. The upstream
	// enrollment endpoint remains responsible for cryptographic authorization.
	return nil
}

func validateAuth(auth Auth) error {
	claims, err := decodeToken(auth.Token)
	if err != nil {
		return errors.New("remote control client requires OAuth identity claims")
	}
	var value struct {
		Auth struct {
			AccountUserID   string `json:"chatgpt_account_user_id"`
			LegacyUserID    string `json:"account_user_id"`
			AccountID       string `json:"chatgpt_account_id"`
			LegacyAccountID string `json:"account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if json.Unmarshal(claims, &value) != nil {
		return errors.New("invalid remote control OAuth identity")
	}
	user := value.Auth.AccountUserID
	if user == "" {
		user = value.Auth.LegacyUserID
	}
	account := value.Auth.AccountID
	if account == "" {
		account = value.Auth.LegacyAccountID
	}
	if user == "" || user != auth.AccountUserID || (auth.AccountID != "" && account != "" && account != auth.AccountID) {
		return errors.New("remote control OAuth principal mismatch")
	}
	return nil
}

func decodeToken(token string) ([]byte, error) {
	if len(token) > 32*1024 {
		return nil, errors.New("invalid token encoding")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token encoding")
	}
	return base64.RawURLEncoding.DecodeString(parts[1])
}
