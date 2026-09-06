// Package cpasync owns the read-only CLIProxyAPI account import contract.
package cpasync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const MaxFiles = 10000
const MaxBatchSize = 100
const maxListBytes = 16 << 20
const maxCredentialBytes = 1 << 20

type File struct {
	Name           string     `json:"name"`
	Label          string     `json:"label"`
	Email          string     `json:"email"`
	Provider       string     `json:"provider"`
	Type           string     `json:"type"`
	Status         string     `json:"status"`
	Disabled       bool       `json:"disabled"`
	Unavailable    bool       `json:"unavailable"`
	RuntimeOnly    bool       `json:"runtime_only"`
	NextRetryAfter *time.Time `json:"next_retry_after"`
}

func (f File) Platform() string {
	provider := f.Provider
	if provider == "" {
		provider = f.Type
	}
	return Platform(provider)
}

func Platform(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return "openai"
	case "claude":
		return "anthropic"
	case "gemini", "gemini-cli":
		return "gemini"
	case "antigravity":
		return "antigravity"
	default:
		return ""
	}
}

// SkipReason is shared by preview and execution. Unknown lifecycle states are
// deliberately not treated as healthy, even when disabled is absent/false.
func (f File) SkipReason(now time.Time) string {
	if f.Disabled || strings.EqualFold(strings.TrimSpace(f.Status), "disabled") {
		return "disabled"
	}
	if !strings.EqualFold(strings.TrimSpace(f.Status), "active") || f.Unavailable || (f.NextRetryAfter != nil && f.NextRetryAfter.After(now)) {
		return "abnormal"
	}
	if f.RuntimeOnly {
		return "runtime_only"
	}
	if f.Platform() == "" {
		return "unsupported_provider"
	}
	if !ValidName(f.Name) {
		return "invalid_file_name"
	}
	return ""
}

func ValidName(name string) bool {
	return name != "" && len(name) <= 255 && name == strings.TrimSpace(name) &&
		!strings.ContainsAny(name, "/\\\x00\r\n") && strings.HasSuffix(strings.ToLower(name), ".json")
}

// Endpoint accepts the same base URL variants as the existing CPA capacity UI.
func Endpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("invalid CPA management URL")
	}
	u.Host = strings.ToLower(u.Host)
	if (u.Scheme == "https" && u.Port() == "443") || (u.Scheme == "http" && u.Port() == "80") {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	path := strings.TrimRight(u.Path, "/")
	switch {
	case strings.HasSuffix(path, "/v0/management/auth-files"):
	case strings.HasSuffix(path, "/v0/management"):
		path += "/auth-files"
	case strings.HasSuffix(path, "/v1"):
		path = strings.TrimSuffix(path, "/v1") + "/v0/management/auth-files"
	default:
		path += "/v0/management/auth-files"
	}
	u.Path, u.RawPath = path, ""
	return u.String(), nil
}

type Client struct {
	endpoint, password string
	http               *http.Client
}

func NewClient(endpoint, password string, client *http.Client) *Client {
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{endpoint: endpoint, password: password, http: &copyClient}
}

func (c *Client) get(ctx context.Context, target string, limit int64, output any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errors.New("invalid CPA request")
	}
	req.Header.Set("Authorization", "Bearer "+c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("CPA request failed or timed out")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("CPA returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return errors.New("CPA response could not be read")
	}
	if int64(len(raw)) > limit {
		return errors.New("CPA response exceeds size limit")
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return errors.New("CPA returned invalid JSON")
	}
	return nil
}

func (c *Client) List(ctx context.Context) ([]File, error) {
	var response struct {
		Files *[]File `json:"files"`
	}
	if err := c.get(ctx, c.endpoint, maxListBytes, &response); err != nil {
		return nil, err
	}
	if response.Files == nil {
		return nil, errors.New("CPA response is missing files")
	}
	if len(*response.Files) > MaxFiles {
		return nil, errors.New("CPA account count exceeds limit")
	}
	return *response.Files, nil
}

func (c *Client) Download(ctx context.Context, name string) (map[string]any, error) {
	if !ValidName(name) {
		return nil, errors.New("invalid CPA file name")
	}
	var data map[string]any
	if err := c.get(ctx, c.endpoint+"/download?"+url.Values{"name": {name}}.Encode(), maxCredentialBytes, &data); err != nil {
		return nil, err
	}
	if data == nil {
		return nil, errors.New("CPA credential must be an object")
	}
	return data, nil
}
