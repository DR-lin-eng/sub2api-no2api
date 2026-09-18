package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	openaiutil "github.com/Wei-Shaw/sub2api/internal/shared/openai"
)

const (
	openAIAccountRoutingOverrideHeader = "x-openai-account-routing-override"
	openAIWorkspaceRoutingDiscoveryURL = "https://chatgpt.com/backend-api/wham/accounts/check"
	openAIWorkspaceRoutingTimeout      = 10 * time.Second
	openAIWorkspaceRoutingBodyLimit    = 1 << 20
)

var errOpenAIWorkspaceRoutingUnavailable = errors.New("openai workspace routing unavailable")

type openAIWorkspaceRouting struct {
	chatGPTAccountID       string
	backendOrigin          string
	accountRoutingOverride string
}

type cachedOpenAIWorkspaceRouting struct {
	tokenDigest      [sha256.Size]byte
	chatGPTAccountID string
	routing          openAIWorkspaceRouting
}

type openAIWorkspaceRoutingWireEntry struct {
	ID                     string                             `json:"id"`
	AccountID              string                             `json:"account_id"`
	WorkspaceBackendOrigin *string                            `json:"workspace_backend_origin"`
	AccountRoutingOverride *string                            `json:"account_routing_override"`
	Account                *openAIWorkspaceRoutingWireAccount `json:"account"`
}

type openAIWorkspaceRoutingWireAccount struct {
	AccountID              string  `json:"account_id"`
	WorkspaceBackendOrigin *string `json:"workspace_backend_origin"`
	AccountRoutingOverride *string `json:"account_routing_override"`
}

func (s *OpenAIGatewayService) enableOpenAIWorkspaceRouting() {
	if s != nil {
		s.openAIWorkspaceRoutingEnabled = true
	}
}

func (s *OpenAIGatewayService) resolveOpenAIWorkspaceRouting(
	ctx context.Context,
	account *Account,
	accessToken string,
) (*openAIWorkspaceRouting, error) {
	if s == nil || !s.openAIWorkspaceRoutingEnabled || account == nil || !account.IsOpenAIOAuth() || account.IsOpenAIAgentIdentity() || account.IsOpenAIPersonalAccessToken() {
		return nil, nil
	}
	// OAuth custom/global relays do not expose the first-party WHAM discovery
	// contract. Never send the account bearer to the discovery endpoint for a
	// relay; the relay's configured route remains authoritative.
	if _, official, err := s.openAIOAuthCodexTargetURLWithContext(ctx, account); err != nil {
		return nil, fmt.Errorf("%w: resolve OAuth target", errOpenAIWorkspaceRoutingUnavailable)
	} else if !official {
		return nil, nil
	}
	// Workspace discovery is an account-authenticated request. Keep the bearer
	// token on the verified first-party ChatGPT origin; custom/global relays are
	// deliberately left on their existing route until an explicit trusted
	// discovery contract exists for that relay.
	// This checkout uses the official ChatGPT Codex target directly. Custom
	// relay support lives on a newer branch; keeping discovery tied to the
	// fixed first-party origin preserves the bearer-token boundary here.
	if s.httpUpstream == nil {
		return nil, fmt.Errorf("%w: HTTP upstream is not configured", errOpenAIWorkspaceRoutingUnavailable)
	}

	credentialAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve credential account", errOpenAIWorkspaceRoutingUnavailable)
	}
	if credentialAccount == nil {
		return nil, fmt.Errorf("%w: credential account is missing", errOpenAIWorkspaceRoutingUnavailable)
	}
	chatGPTAccountID, accountChanged := openAIWorkspaceRoutingAccountID(credentialAccount, accessToken)
	if accountChanged {
		return nil, fmt.Errorf("%w: token account changed", errOpenAIWorkspaceRoutingUnavailable)
	}
	if chatGPTAccountID == "" {
		return nil, nil
	}
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("%w: access token is missing", errOpenAIWorkspaceRoutingUnavailable)
	}

	tokenDigest := sha256.Sum256([]byte(accessToken))
	if cached, ok := s.loadOpenAIWorkspaceRouting(credentialAccount.ID, tokenDigest, chatGPTAccountID); ok {
		return &cached, nil
	}

	flightKey := fmt.Sprintf("%d:%x:%s", credentialAccount.ID, tokenDigest, chatGPTAccountID)
	value, err, _ := s.openAIWorkspaceRoutingSF.Do(flightKey, func() (any, error) {
		if cached, ok := s.loadOpenAIWorkspaceRouting(credentialAccount.ID, tokenDigest, chatGPTAccountID); ok {
			return cached, nil
		}
		routing, discoverErr := s.discoverOpenAIWorkspaceRouting(
			ctx,
			account,
			credentialAccount,
			accessToken,
			chatGPTAccountID,
		)
		if discoverErr != nil {
			return nil, discoverErr
		}
		s.openAIWorkspaceRoutingCache.Store(credentialAccount.ID, cachedOpenAIWorkspaceRouting{
			tokenDigest:      tokenDigest,
			chatGPTAccountID: chatGPTAccountID,
			routing:          routing,
		})
		return routing, nil
	})
	if err != nil {
		slog.Warn("openai workspace routing discovery failed",
			"account_id", account.ID,
			"credential_account_id", credentialAccount.ID,
			"error", err,
		)
		return nil, err
	}
	routing, ok := value.(openAIWorkspaceRouting)
	if !ok {
		return nil, fmt.Errorf("%w: invalid discovery result", errOpenAIWorkspaceRoutingUnavailable)
	}
	return &routing, nil
}

func (s *OpenAIGatewayService) loadOpenAIWorkspaceRouting(
	accountID int64,
	tokenDigest [sha256.Size]byte,
	chatGPTAccountID string,
) (openAIWorkspaceRouting, bool) {
	if s == nil {
		return openAIWorkspaceRouting{}, false
	}
	value, ok := s.openAIWorkspaceRoutingCache.Load(accountID)
	if !ok {
		return openAIWorkspaceRouting{}, false
	}
	cached, ok := value.(cachedOpenAIWorkspaceRouting)
	if !ok || cached.tokenDigest != tokenDigest || cached.chatGPTAccountID != chatGPTAccountID {
		return openAIWorkspaceRouting{}, false
	}
	return cached.routing, true
}

func (s *OpenAIGatewayService) discoverOpenAIWorkspaceRouting(
	ctx context.Context,
	routeAccount *Account,
	credentialAccount *Account,
	accessToken string,
	chatGPTAccountID string,
) (openAIWorkspaceRouting, error) {
	discoveryCtx, cancel := context.WithTimeout(ctx, openAIWorkspaceRoutingTimeout)
	defer cancel()
	discoveryCtx = WithHTTPUpstreamRedirectsDisabled(
		WithHTTPUpstreamProfile(discoveryCtx, HTTPUpstreamProfileOpenAI),
	)
	req, err := http.NewRequestWithContext(discoveryCtx, http.MethodGet, openAIWorkspaceRoutingDiscoveryURL, nil)
	if err != nil {
		return openAIWorkspaceRouting{}, fmt.Errorf("%w: build discovery request", errOpenAIWorkspaceRoutingUnavailable)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	setOpenAIChatGPTAccountHeaders(req.Header, credentialAccount)
	identity := resolveCodexOutboundIdentity(s.codexIdentityOverrideUA(credentialAccount))
	req.Header.Set("User-Agent", identity.userAgent)
	req.Header.Set("originator", identity.originator)
	req.Header.Set("version", identity.version)

	proxyURL := ""
	if routeAccount.ProxyID != nil && routeAccount.Proxy != nil {
		proxyURL = routeAccount.Proxy.URL()
	}
	resp, err := s.doAccountHTTPUpstream(req, proxyURL, routeAccount)
	if err != nil {
		return openAIWorkspaceRouting{}, fmt.Errorf("%w: discovery request failed", errOpenAIWorkspaceRoutingUnavailable)
	}
	if resp == nil {
		return openAIWorkspaceRouting{}, fmt.Errorf("%w: empty discovery response", errOpenAIWorkspaceRoutingUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, openAIWorkspaceRoutingBodyLimit+1))
	if err != nil {
		return openAIWorkspaceRouting{}, fmt.Errorf("%w: read discovery response", errOpenAIWorkspaceRoutingUnavailable)
	}
	if len(body) > openAIWorkspaceRoutingBodyLimit {
		return openAIWorkspaceRouting{}, fmt.Errorf("%w: discovery response is too large", errOpenAIWorkspaceRoutingUnavailable)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return openAIWorkspaceRouting{}, fmt.Errorf("%w: discovery returned HTTP %d", errOpenAIWorkspaceRoutingUnavailable, resp.StatusCode)
	}
	routing, err := parseOpenAIWorkspaceRoutingResponse(body, chatGPTAccountID)
	if err != nil {
		return openAIWorkspaceRouting{}, fmt.Errorf("%w: %v", errOpenAIWorkspaceRoutingUnavailable, err)
	}
	routing.chatGPTAccountID = chatGPTAccountID
	return routing, nil
}

func openAIWorkspaceRoutingAccountID(account *Account, accessToken string) (string, bool) {
	stored := ""
	if account != nil {
		stored = strings.TrimSpace(account.GetChatGPTAccountID())
	}
	if claims, err := openaiutil.DecodeIDToken(strings.TrimSpace(accessToken)); err == nil && claims.OpenAIAuth != nil {
		if accountID := strings.TrimSpace(claims.OpenAIAuth.ChatGPTAccountID); accountID != "" {
			return accountID, stored != "" && !strings.EqualFold(stored, accountID)
		}
	}
	return stored, false
}

func parseOpenAIWorkspaceRoutingResponse(body []byte, chatGPTAccountID string) (openAIWorkspaceRouting, error) {
	var envelope struct {
		Accounts json.RawMessage `json:"accounts"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return openAIWorkspaceRouting{}, errors.New("invalid accounts response")
	}
	if len(envelope.Accounts) == 0 || string(envelope.Accounts) == "null" {
		return openAIWorkspaceRouting{}, errors.New("accounts response is missing accounts")
	}

	entries := make([]openAIWorkspaceRoutingWireEntry, 0)
	if err := json.Unmarshal(envelope.Accounts, &entries); err != nil {
		var mapped map[string]json.RawMessage
		if mapErr := json.Unmarshal(envelope.Accounts, &mapped); mapErr != nil {
			return openAIWorkspaceRouting{}, errors.New("accounts response has invalid accounts")
		}
		for key, raw := range mapped {
			var entry openAIWorkspaceRoutingWireEntry
			if entryErr := json.Unmarshal(raw, &entry); entryErr != nil {
				continue
			}
			if strings.TrimSpace(entry.ID) == "" && strings.TrimSpace(entry.AccountID) == "" {
				entry.ID = key
			}
			entries = append(entries, entry)
		}
	}

	var selected *openAIWorkspaceRoutingWireEntry
	for index := range entries {
		entryID := openAIWorkspaceRoutingEntryID(entries[index])
		if entryID != chatGPTAccountID {
			continue
		}
		if selected != nil {
			return openAIWorkspaceRouting{}, errors.New("selected workspace is duplicated")
		}
		selected = &entries[index]
	}
	if selected == nil {
		return openAIWorkspaceRouting{}, errors.New("selected workspace is missing")
	}

	backendOrigin, override := openAIWorkspaceRoutingEntryValues(*selected)
	return validateOpenAIWorkspaceRouting(backendOrigin, override)
}

func openAIWorkspaceRoutingEntryID(entry openAIWorkspaceRoutingWireEntry) string {
	for _, candidate := range []string{entry.ID, entry.AccountID} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
	}
	if entry.Account != nil {
		return strings.TrimSpace(entry.Account.AccountID)
	}
	return ""
}

func openAIWorkspaceRoutingEntryValues(entry openAIWorkspaceRoutingWireEntry) (string, string) {
	backendOrigin := entry.WorkspaceBackendOrigin
	override := entry.AccountRoutingOverride
	if entry.Account != nil {
		if backendOrigin == nil {
			backendOrigin = entry.Account.WorkspaceBackendOrigin
		}
		if override == nil {
			override = entry.Account.AccountRoutingOverride
		}
	}
	if backendOrigin == nil || override == nil {
		return "", ""
	}
	return *backendOrigin, *override
}

func validateOpenAIWorkspaceRouting(backendOrigin, override string) (openAIWorkspaceRouting, error) {
	if backendOrigin != strings.TrimSpace(backendOrigin) || override != strings.TrimSpace(override) {
		return openAIWorkspaceRouting{}, errors.New("workspace routing contains surrounding whitespace")
	}
	switch override {
	case "NO_CONSTRAINT", "us", "us_cr":
	default:
		return openAIWorkspaceRouting{}, errors.New("workspace routing override is invalid")
	}
	if backendOrigin == "NO_CONSTRAINT" {
		return openAIWorkspaceRouting{backendOrigin: backendOrigin, accountRoutingOverride: override}, nil
	}
	parsed, err := url.Parse(backendOrigin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return openAIWorkspaceRouting{}, errors.New("workspace backend must be an HTTPS origin")
	}
	return openAIWorkspaceRouting{
		backendOrigin:          (&url.URL{Scheme: parsed.Scheme, Host: parsed.Host}).String(),
		accountRoutingOverride: override,
	}, nil
}

func applyOpenAIWorkspaceRoutingURL(rawURL string, routing *openAIWorkspaceRouting, replaceOrigin bool) (string, error) {
	if routing == nil || !replaceOrigin || routing.backendOrigin == "NO_CONSTRAINT" {
		return rawURL, nil
	}
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid OpenAI target URL: %w", err)
	}
	backend, err := url.Parse(routing.backendOrigin)
	if err != nil {
		return "", fmt.Errorf("invalid workspace backend origin: %w", err)
	}
	target.Scheme = backend.Scheme
	target.Host = backend.Host
	target.User = nil
	return target.String(), nil
}

func isOfficialChatGPTCodexURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	return err == nil && parsed != nil && strings.EqualFold(parsed.Hostname(), "chatgpt.com")
}

func applyOpenAIWorkspaceRoutingHeader(headers http.Header, routing *openAIWorkspaceRouting) {
	if headers == nil {
		return
	}
	deleteOpenAIHeaderEqualFold(headers, openAIAccountRoutingOverrideHeader)
	if routing == nil {
		return
	}
	if routing.chatGPTAccountID != "" {
		headers.Set("chatgpt-account-id", routing.chatGPTAccountID)
	}
	switch routing.accountRoutingOverride {
	case "us", "us_cr":
		headers.Set(openAIAccountRoutingOverrideHeader, routing.accountRoutingOverride)
	}
}

func withOpenAIWorkspaceRoutingRedirectPolicy(ctx context.Context, routing *openAIWorkspaceRouting) context.Context {
	if routing == nil {
		return ctx
	}
	return WithHTTPUpstreamRedirectsDisabled(ctx)
}

func newOpenAIWorkspaceRoutingFailoverError(err error) *UpstreamFailoverError {
	_ = err
	return &UpstreamFailoverError{
		StatusCode:        http.StatusBadGateway,
		Scope:             GatewayFailureScopeAccount,
		Reason:            GatewayFailureReason("openai_workspace_routing_unavailable"),
		NextAccountAction: NextAccountRetry,
		ClientStatusCode:  http.StatusBadGateway,
		ClientMessage:     "OpenAI workspace routing is temporarily unavailable",
	}
}
