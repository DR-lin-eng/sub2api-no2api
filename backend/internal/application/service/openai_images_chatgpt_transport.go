package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// One session captures the selected protocol, credential principal and route.
// Auxiliary retries may resolve a file or resume a turn, never POST a new turn.
type chatGPTImageSession struct {
	gateway            *OpenAIGatewayService
	account            *Account
	credential         *Account
	token              string
	base               *url.URL
	headers            http.Header
	integrity          http.Header
	requestID          string
	model              string
	generationAccepted bool
	outputBytes        int64
}

type chatGPTImageResponseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *chatGPTImageResponseBody) Close() error { b.cancel(); return b.ReadCloser.Close() }

func (s *OpenAIGatewayService) newChatGPTImageSession(ctx context.Context, c *gin.Context, account *Account) (*chatGPTImageSession, error) {
	credential, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, err
	}
	expected := snapshotOAuthRefreshAccount(credential)
	token, _, err := s.GetAccessToken(ctx, credential)
	if err != nil {
		return nil, err
	}
	if err = validateOpenAIOAuthRequestPrincipal(expected, token); err != nil {
		return nil, err
	}
	endpointAccount := account
	if account.IsShadow() && !account.IsCustomBaseURLEnabled() {
		endpointAccount = credential
	}
	base, official, err := resolveOpenAIOAuthCodexBaseURL(ctx, s.settingService, s.cfg, endpointAccount)
	if err != nil {
		return nil, err
	}
	parsed, err := chatGPTImageBackendBase(base)
	if err != nil {
		return nil, err
	}
	routing, err := s.resolveOpenAIWorkspaceRouting(ctx, endpointAccount, token)
	if err != nil {
		return nil, err
	}
	if official && routing != nil {
		routed, err := applyOpenAIWorkspaceRoutingURL(parsed.String(), routing, true)
		if err != nil {
			return nil, err
		}
		parsed, err = url.Parse(routed)
		if err != nil {
			return nil, err
		}
	}
	headers, err := s.buildOpenAIAuthenticationHeaders(ctx, credential, token)
	if err != nil {
		return nil, err
	}
	setOpenAIChatGPTAccountHeaders(headers, credential)
	if accountID := credential.GetChatGPTAccountID(); accountID != "" {
		headers.Set("X-OpenAI-Expected-Account-ID", accountID)
	}
	applyOpenAIWorkspaceRoutingHeader(headers, routing)
	version := "26.1103.32239"
	if runtime.GOOS == "darwin" {
		version = "26.1103.41825"
	}
	headers.Set("User-Agent", fmt.Sprintf("Codex Desktop/%s (%s; %s)", version, runtime.GOOS, runtime.GOARCH))
	headers.Set("originator", "Codex Desktop")
	headers.Set("x-openai-web-frontend", "codex_webview")
	headers.Set("x-openai-codex-window-type", "electron")
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	if language := strings.TrimSpace(c.GetHeader("Accept-Language")); language != "" {
		headers.Set("Accept-Language", language)
	}
	language := strings.TrimSpace(strings.Split(strings.Split(headers.Get("Accept-Language"), ",")[0], ";")[0])
	if language == "" || language == "*" || len(language) > 64 {
		language = "en"
	}
	headers.Set("OAI-Language", language)
	headers.Set("oai-did", resolveConvergedInstallationID(credential))
	s.applyOpenAILocationHeader(ctx, headers, account)
	return &chatGPTImageSession{gateway: s, account: account, credential: expected, token: token, base: parsed, headers: headers}, nil
}

func chatGPTImageBackendBase(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	parsed.Path = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(parsed.Path, "/"), "/responses"), "/codex")
	return parsed, nil
}

func (s *chatGPTImageSession) endpoint(path string) (string, error) {
	u, err := url.Parse(path)
	if err != nil || u.User != nil || u.Fragment != "" {
		return "", fmt.Errorf("invalid ChatGPT image endpoint")
	}
	if u.IsAbs() {
		if !strings.EqualFold(u.Scheme, s.base.Scheme) || !strings.EqualFold(u.Host, s.base.Host) {
			return "", fmt.Errorf("ChatGPT image endpoint changed origin")
		}
		if !strings.HasPrefix(u.Path, strings.TrimSuffix(s.base.Path, "/")+"/") {
			return "", fmt.Errorf("ChatGPT image endpoint changed backend")
		}
		return u.String(), nil
	}
	if !strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, "..") {
		return "", fmt.Errorf("invalid ChatGPT image path")
	}
	target := *s.base
	if strings.HasPrefix(u.Path, strings.TrimSuffix(s.base.Path, "/")+"/") {
		target.Path = u.Path
	} else {
		target.Path = strings.TrimSuffix(s.base.Path, "/") + u.Path
	}
	query := s.base.Query()
	for key, values := range u.Query() {
		query[key] = values
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func (s *chatGPTImageSession) request(ctx context.Context, method, path string, body []byte, stream bool) (*http.Response, error) {
	if err := validateOpenAIOAuthRequestPrincipal(s.credential, s.token); err != nil {
		return nil, err
	}
	target, err := s.endpoint(path)
	if err != nil {
		return nil, err
	}
	ctx = WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(ctx, HTTPUpstreamProfileOpenAI))
	ctx, cancel := context.WithCancel(ctx)
	initialOpen := time.AfterFunc(60*time.Second, cancel)
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		initialOpen.Stop()
		cancel()
		return nil, err
	}
	req.Header = s.headers.Clone()
	// Electron consumes these control headers before sending. Principal checks
	// above enforce the expected identity locally; these are not wire fields.
	for _, name := range []string{"X-OpenAI-Attach-Auth", "X-OpenAI-Attach-Desktop-Surface", "X-OpenAI-Attach-DeviceCheck-Token", "X-OpenAI-Expected-Account-ID"} {
		deleteOpenAIHeaderEqualFold(req.Header, name)
	}
	if strings.HasPrefix(path, "/f/conversation") {
		for key, values := range s.integrity {
			req.Header[key] = append([]string(nil), values...)
		}
	}
	if stream {
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("x-oai-mcp-form-version", "2")
	}
	proxy := ""
	if s.account.ProxyID != nil && s.account.Proxy != nil {
		proxy = s.account.Proxy.URL()
	}
	resp, err := s.gateway.doAccountHTTPUpstream(req, proxy, s.account)
	initialOpen.Stop()
	if err != nil {
		cancel()
		if !s.generationAccepted {
			if limited := openAIOAuthGatewayRateLimitFailover(err); limited != nil {
				return nil, limited
			}
		}
		return nil, fmt.Errorf("ChatGPT image upstream transport failed")
	}
	resp.Body = &chatGPTImageResponseBody{ReadCloser: resp.Body, cancel: cancel}
	if id := resp.Header.Get("x-request-id"); id != "" && s.requestID == "" && strings.HasPrefix(path, "/f/conversation") {
		s.requestID = id
	}
	if resp.StatusCode >= 300 {
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, openAIUpstreamErrorBodyReadLimit))
		// Preserve the existing structured account error policy. Do not create a
		// second generation after an ambiguous request or authentication failure.
		if resp.StatusCode >= 400 {
			if method == http.MethodPost && path == "/f/conversation" && (resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 429) {
				message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(data))
				if s.gateway.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, message, data) {
					disabled := s.gateway.handleFailoverSideEffects(ctx, resp, s.account, data, s.model)
					return nil, newOpenAIUpstreamFailoverError(resp.StatusCode, resp.Header, data, message, !disabled && s.account.IsPoolMode() && s.account.IsPoolModeRetryableStatus(resp.StatusCode))
				}
			}
			return nil, openAIImagesUpstreamErrorFromHTTP(resp.StatusCode, resp.Header, data)
		}
		return nil, fmt.Errorf("ChatGPT image upstream redirect rejected")
	}
	if method == http.MethodPost && path == "/f/conversation" {
		s.generationAccepted = true
	}
	return resp, nil
}

func (s *chatGPTImageSession) json(ctx context.Context, method, path string, value any) ([]byte, error) {
	var body []byte
	var err error
	if value != nil {
		body, err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	resp, err := s.request(ctx, method, path, body, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	limit := int64(4 << 20)
	if strings.HasPrefix(path, "/conversation/") {
		limit = 96 << 20
	}
	return readChatGPTImageBody(resp.Body, limit)
}

func readChatGPTImageBody(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("ChatGPT image response exceeds size limit")
	}
	return data, nil
}

func chatGPTImageWait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
