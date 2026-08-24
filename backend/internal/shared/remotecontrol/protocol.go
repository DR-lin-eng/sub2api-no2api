// Package remotecontrol contains the source-shaped Remote Control wire
// contracts used by Codex-compatible clients. It is deliberately transport
// neutral: enrollment and pairing callers provide their own HTTP/WS client.
package remotecontrol

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	ProtocolVersion             = "3"
	ServerIDHeader              = "x-codex-server-id"
	NameHeader                  = "x-codex-name"
	ProtocolVersionHeader       = "x-codex-protocol-version"
	InstallationIDHeader        = "x-codex-installation-id"
	HostDeviceKindHeader        = "x-codex-host-device-kind"
	SubscribeCursorHeader       = "x-codex-subscribe-cursor"
	RemoteControlPath           = "wham/remote/control/server"
	RemoteControlEnrollPath     = RemoteControlPath + "/enroll"
	RemoteControlRefreshPath    = RemoteControlPath + "/refresh"
	RemoteControlPairPath       = RemoteControlPath + "/pair"
	RemoteControlPairStatusPath = RemoteControlPath + "/pair/status"
)

type Target struct {
	WebSocketURL  string
	EnrollURL     string
	RefreshURL    string
	PairURL       string
	PairStatusURL string
}

type EnrollRequest struct {
	Name             string `json:"name"`
	OS               string `json:"os"`
	Arch             string `json:"arch"`
	AppServerVersion string `json:"app_server_version"`
	InstallationID   string `json:"installation_id"`
}

type EnrollResponse struct {
	ServerID           string `json:"server_id"`
	EnvironmentID      string `json:"environment_id"`
	RemoteControlToken string `json:"remote_control_token"`
	ExpiresAt          string `json:"expires_at"`
}

type RefreshRequest struct {
	ServerID       string `json:"server_id"`
	InstallationID string `json:"installation_id"`
}

type PairRequest struct {
	ManualCode bool `json:"manual_code"`
}

type PairResponse struct {
	PairingCode       string `json:"pairing_code"`
	ManualPairingCode string `json:"manual_pairing_code,omitempty"`
	ServerID          string `json:"server_id"`
	EnvironmentID     string `json:"environment_id"`
	ExpiresAt         string `json:"expires_at"`
}

type PairStatusRequest struct {
	PairingCode       string `json:"pairing_code,omitempty"`
	ManualPairingCode string `json:"manual_pairing_code,omitempty"`
}

type PairStatusResponse struct {
	Claimed bool `json:"claimed"`
}

type ClientEnvelope struct {
	Type               string          `json:"type"`
	ClientID           string          `json:"client_id"`
	StreamID           string          `json:"stream_id,omitempty"`
	SeqID              uint64          `json:"seq_id,omitempty"`
	Cursor             string          `json:"cursor,omitempty"`
	Message            json.RawMessage `json:"message,omitempty"`
	SegmentID          *int            `json:"segment_id,omitempty"`
	SegmentCount       int             `json:"segment_count,omitempty"`
	MessageSizeBytes   int             `json:"message_size_bytes,omitempty"`
	MessageChunkBase64 string          `json:"message_chunk_base64,omitempty"`
}

type ServerEnvelope struct {
	Type               string          `json:"type"`
	ClientID           string          `json:"client_id"`
	StreamID           string          `json:"stream_id"`
	SeqID              uint64          `json:"seq_id"`
	Message            json.RawMessage `json:"message,omitempty"`
	SegmentID          int             `json:"segment_id,omitempty"`
	SegmentCount       int             `json:"segment_count,omitempty"`
	MessageSizeBytes   int             `json:"message_size_bytes,omitempty"`
	MessageChunkBase64 string          `json:"message_chunk_base64,omitempty"`
	Status             string          `json:"status,omitempty"`
}

type Enrollment struct {
	ServerID           string
	EnvironmentID      string
	ServerName         string
	RemoteControlToken string
}

type StoredEnrollment struct {
	Enrollment
	ExpiresAt time.Time
}

// EnrollmentStore is intentionally token-opaque. A production adapter should
// encrypt RemoteControlToken before persistence; the manager never serializes
// or logs it itself.
type EnrollmentStore interface {
	Load(ctx context.Context) (*StoredEnrollment, error)
	Save(ctx context.Context, enrollment StoredEnrollment) error
	Clear(ctx context.Context) error
}

type MemoryEnrollmentStore struct {
	mu         sync.RWMutex
	enrollment *StoredEnrollment
}

func NewMemoryEnrollmentStore() *MemoryEnrollmentStore {
	return &MemoryEnrollmentStore{}
}

func (s *MemoryEnrollmentStore) Load(context.Context) (*StoredEnrollment, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.enrollment == nil {
		return nil, nil
	}
	copy := *s.enrollment
	return &copy, nil
}

func (s *MemoryEnrollmentStore) Save(_ context.Context, enrollment StoredEnrollment) error {
	if s == nil {
		return fmt.Errorf("remote control enrollment store is unavailable")
	}
	s.mu.Lock()
	copy := enrollment
	s.enrollment = &copy
	s.mu.Unlock()
	return nil
}

func (s *MemoryEnrollmentStore) Clear(context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.enrollment = nil
	s.mu.Unlock()
	return nil
}

const enrollmentRefreshSkew = 5 * time.Minute

// LifecycleManager owns enrollment state and refresh timing while leaving
// account selection, encryption and long-running socket ownership to callers.
type LifecycleManager struct {
	client       *Client
	store        EnrollmentStore
	now          func() time.Time
	accountToken string
}

func NewLifecycleManager(client *Client, store EnrollmentStore, accountToken string) *LifecycleManager {
	if client == nil {
		client = NewClient(nil)
	}
	return &LifecycleManager{client: client, store: store, now: time.Now, accountToken: accountToken}
}

func (m *LifecycleManager) Current(ctx context.Context) (*StoredEnrollment, error) {
	if m == nil || m.store == nil {
		return nil, nil
	}
	return m.store.Load(ctx)
}

func (m *LifecycleManager) Enroll(ctx context.Context, target Target, request EnrollRequest) (*StoredEnrollment, error) {
	response, err := m.client.Enroll(ctx, target, m.accountToken, request)
	if err != nil {
		return nil, err
	}
	expiresAt, err := time.Parse(time.RFC3339, response.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("parse remote control enrollment expiry: %w", err)
	}
	record := StoredEnrollment{Enrollment: Enrollment{
		ServerID:           response.ServerID,
		EnvironmentID:      response.EnvironmentID,
		ServerName:         request.Name,
		RemoteControlToken: response.RemoteControlToken,
	}, ExpiresAt: expiresAt}
	if m.store != nil {
		if err := m.store.Save(ctx, record); err != nil {
			return nil, err
		}
	}
	return &record, nil
}

func (m *LifecycleManager) RefreshIfNeeded(ctx context.Context, target Target, installationID string) (*StoredEnrollment, bool, error) {
	if m == nil {
		return nil, false, fmt.Errorf("remote control lifecycle manager is unavailable")
	}
	record, err := m.Current(ctx)
	if err != nil || record == nil {
		return record, false, err
	}
	if m.now().Add(enrollmentRefreshSkew).Before(record.ExpiresAt) {
		return record, false, nil
	}
	response, err := m.client.Refresh(ctx, target, m.accountToken, RefreshRequest{ServerID: record.ServerID, InstallationID: installationID})
	if err != nil {
		return record, false, err
	}
	expiresAt, err := time.Parse(time.RFC3339, response.ExpiresAt)
	if err != nil {
		return record, false, fmt.Errorf("parse remote control refresh expiry: %w", err)
	}
	refreshed := StoredEnrollment{Enrollment: Enrollment{
		ServerID:           response.ServerID,
		EnvironmentID:      response.EnvironmentID,
		ServerName:         record.ServerName,
		RemoteControlToken: response.RemoteControlToken,
	}, ExpiresAt: expiresAt}
	if m.store != nil {
		if err := m.store.Save(ctx, refreshed); err != nil {
			return record, false, err
		}
	}
	return &refreshed, true, nil
}

func (m *LifecycleManager) Clear(ctx context.Context) error {
	if m == nil || m.store == nil {
		return nil
	}
	return m.store.Clear(ctx)
}

func (m *LifecycleManager) StartPairing(ctx context.Context, target Target, manualCode bool) (*PairResponse, error) {
	if m == nil || m.client == nil {
		return nil, fmt.Errorf("remote control lifecycle manager is unavailable")
	}
	return m.client.StartPairing(ctx, target, m.accountToken, PairRequest{ManualCode: manualCode})
}

func (m *LifecycleManager) PairingStatus(ctx context.Context, target Target, code PairStatusRequest) (*PairStatusResponse, error) {
	if m == nil || m.client == nil {
		return nil, fmt.Errorf("remote control lifecycle manager is unavailable")
	}
	return m.client.PairingStatus(ctx, target, m.accountToken, code)
}

func (m *LifecycleManager) Dial(ctx context.Context, target Target, installationID, hostDeviceKind, subscribeCursor string, httpClient *http.Client) (*websocket.Conn, *http.Response, error) {
	if m == nil {
		return nil, nil, fmt.Errorf("remote control lifecycle manager is unavailable")
	}
	record, err := m.Current(ctx)
	if err != nil {
		return nil, nil, err
	}
	if record == nil {
		return nil, nil, fmt.Errorf("remote control enrollment is not available")
	}
	return DialWebSocket(ctx, target, record.Enrollment, installationID, hostDeviceKind, subscribeCursor, httpClient)
}

// Client provides bounded JSON operations for enrollment and pairing. It does
// not retain tokens or own a background WebSocket, so callers can bind it to
// their account-scoped HTTP/TLS transport and lifecycle manager.
type Client struct {
	HTTPClient *http.Client
	BodyLimit  int64
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{HTTPClient: httpClient, BodyLimit: 4096}
}

func (c *Client) Enroll(ctx context.Context, target Target, bearerToken string, request EnrollRequest) (*EnrollResponse, error) {
	var response EnrollResponse
	if err := c.doJSON(ctx, http.MethodPost, target.EnrollURL, bearerToken, request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) Refresh(ctx context.Context, target Target, bearerToken string, request RefreshRequest) (*EnrollResponse, error) {
	var response EnrollResponse
	if err := c.doJSON(ctx, http.MethodPost, target.RefreshURL, bearerToken, request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) StartPairing(ctx context.Context, target Target, bearerToken string, request PairRequest) (*PairResponse, error) {
	var response PairResponse
	if err := c.doJSON(ctx, http.MethodPost, target.PairURL, bearerToken, request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) PairingStatus(ctx context.Context, target Target, bearerToken string, request PairStatusRequest) (*PairStatusResponse, error) {
	var response PairStatusResponse
	if err := c.doJSON(ctx, http.MethodPost, target.PairStatusURL, bearerToken, request, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint, bearerToken string, input, output any) error {
	if c == nil || c.HTTPClient == nil {
		return fmt.Errorf("remote control HTTP client is unavailable")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(bearerToken) != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	limit := c.BodyLimit
	if limit <= 0 {
		limit = 4096
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("remote control response exceeds %d bytes", limit)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("remote control request returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if output == nil || len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	return json.Unmarshal(body, output)
}

// DialWebSocket opens the protocol-v3 Remote Control socket with the exact
// handshake projection. The caller owns the connection and should keep a read
// loop active so the websocket library can answer backend Ping frames.
func DialWebSocket(ctx context.Context, target Target, enrollment Enrollment, installationID, hostDeviceKind, subscribeCursor string, httpClient *http.Client) (*websocket.Conn, *http.Response, error) {
	headers, err := BuildWebSocketHeaders(target.WebSocketURL, enrollment, installationID, hostDeviceKind, subscribeCursor)
	if err != nil {
		return nil, nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	conn, response, err := websocket.Dial(ctx, target.WebSocketURL, &websocket.DialOptions{
		HTTPClient: httpClient,
		HTTPHeader: headers,
	})
	if err != nil {
		return nil, response, err
	}
	conn.SetReadLimit(8 << 20)
	return conn, response, nil
}

// NormalizeTarget validates and expands a Remote Control base URL. HTTPS is
// required for ChatGPT hosts; HTTP is accepted only for localhost test setups.
func NormalizeTarget(raw string) (Target, error) {
	base, err := normalizeBaseURL(raw)
	if err != nil {
		return Target{}, err
	}
	join := func(path string) string {
		resolved := *base
		resolved.Path = strings.TrimSuffix(base.Path, "/") + "/" + path
		return resolved.String()
	}
	ws := *base
	ws.Path = strings.TrimSuffix(base.Path, "/") + "/" + RemoteControlPath
	if base.Scheme == "https" {
		ws.Scheme = "wss"
	} else {
		ws.Scheme = "ws"
	}
	return Target{
		WebSocketURL:  ws.String(),
		EnrollURL:     join(RemoteControlEnrollPath),
		RefreshURL:    join(RemoteControlRefreshPath),
		PairURL:       join(RemoteControlPairPath),
		PairStatusURL: join(RemoteControlPairStatusPath),
	}, nil
}

// BuildWebSocketHeaders creates the protocol-v3 handshake projection.
func BuildWebSocketHeaders(rawWebSocketURL string, enrollment Enrollment, installationID, hostDeviceKind, subscribeCursor string) (http.Header, error) {
	u, err := url.Parse(rawWebSocketURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "ws" && u.Scheme != "wss") || !allowedHostOrLocalhost(u.Hostname()) {
		return nil, fmt.Errorf("invalid Remote Control WebSocket URL %q", rawWebSocketURL)
	}
	if strings.TrimSpace(enrollment.ServerID) == "" || strings.TrimSpace(enrollment.RemoteControlToken) == "" || strings.TrimSpace(installationID) == "" {
		return nil, fmt.Errorf("remote control enrollment is incomplete")
	}
	if strings.TrimSpace(enrollment.ServerName) == "" {
		return nil, fmt.Errorf("remote control server name is required")
	}
	headers := make(http.Header)
	headers.Set(ServerIDHeader, enrollment.ServerID)
	headers.Set(NameHeader, base64.StdEncoding.EncodeToString([]byte(enrollment.ServerName)))
	headers.Set(ProtocolVersionHeader, ProtocolVersion)
	headers.Set(InstallationIDHeader, installationID)
	headers.Set("Authorization", "Bearer "+enrollment.RemoteControlToken)
	if strings.TrimSpace(hostDeviceKind) != "" {
		headers.Set(HostDeviceKindHeader, hostDeviceKind)
	}
	if strings.TrimSpace(subscribeCursor) != "" {
		headers.Set(SubscribeCursorHeader, subscribeCursor)
	}
	return headers, nil
}

func normalizeBaseURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("invalid Remote Control URL %q", raw)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	localhost := host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1"
	chatgpt := allowedHost(host)
	validScheme := false
	switch u.Scheme {
	case "https":
		validScheme = chatgpt || localhost
	case "http":
		validScheme = localhost
	}
	if !validScheme {
		return nil, fmt.Errorf("remote control URL must be HTTPS ChatGPT or localhost HTTP/HTTPS")
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/"
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

func allowedHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "chatgpt.com" || host == "chatgpt-staging.com" ||
		strings.HasSuffix(host, ".chatgpt.com") || strings.HasSuffix(host, ".chatgpt-staging.com")
}

func allowedHostOrLocalhost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return allowedHost(host) || host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1"
}
