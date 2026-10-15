package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/platform/liveattestation"
)

// prepareOpenAIOAuthProof runs at the transport boundary, after identity and
// failover selection. Its receipt belongs to this send, not a mutable Gin
// context shared by retries or an idle WebSocket prewarm.
func (s *OpenAIGatewayService) prepareOpenAIOAuthProof(ctx context.Context, account *Account, rawURL string, headers http.Header) (context.Context, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return ctx, nil
	}
	target, _ := url.Parse(rawURL)
	path := ""
	if target != nil {
		path = target.Path
	}
	// Live already generates its own genuine backend-owned proof. Preserve it
	// on that exact endpoint; the Responses proof has a separate send lifecycle.
	liveHeader := ""
	if strings.HasSuffix(path, "/realtime/calls") && liveattestation.ValidLiveHeader(headers.Get("x-oai-attestation")) {
		liveHeader = headers.Get("x-oai-attestation")
	}
	for _, name := range []string{"X-OAI-IS", "X-OAI-IS-Update", "x-oai-attestation", "X-OpenAI-Expected-Account-Id", "X-OpenAI-Attach-Auth", "X-OpenAI-Attach-Desktop-Surface", "X-OpenAI-Attach-DeviceCheck-Token", "X-OpenAI-Attach-Integrity-State"} {
		deleteOpenAIHeaderEqualFold(headers, name)
	}
	owner, err := resolveCredentialAccount(ctx, s.accountRepo, account)
	if err != nil {
		return ctx, err
	}
	if owner == nil || owner.IsOpenAIAgentIdentity() || owner.IsOpenAIPersonalAccessToken() {
		return s.attachIntegrityState(ctx, headers, ""), nil
	}
	if scheme, token, ok := strings.Cut(headers.Get("Authorization"), " "); ok && strings.EqualFold(scheme, "Bearer") {
		if err := validateOpenAIOAuthRequestPrincipal(owner, token); err != nil {
			return ctx, err
		}
	}
	origin := openAIDesktopProofOrigin(rawURL)
	if origin != "" {
		ctx = WithHTTPUpstreamRedirectsDisabled(ctx)
	}
	if strings.HasSuffix(path, "/devicecheck") {
		origin = ""
	}
	ctx = s.attachIntegrityState(ctx, headers, openAIIntegrityKey(account, owner, origin))
	if origin != "" && liveHeader != "" {
		headers.Set("x-oai-attestation", liveHeader)
	}
	modelProofPath := path == "/backend-api/codex/responses" || path == "/backend-api/codex/responses/compact"
	if origin != "" && modelProofPath && s.cfg != nil && s.cfg.Gateway.OpenAIOAuthAttestationEnabled {
		// The selected local callback returns an opaque token or an app-server
		// failure status. No capable provider means the header is absent.
		if header := liveattestation.CodexHeader(ctx, s.liveAttestation); header != "" {
			headers.Set("x-oai-attestation", header)
		}
	}
	return ctx, nil
}

func (s *OpenAIGatewayService) prepareOpenAIWSProof(ctx context.Context, account *Account, rawURL string, headers http.Header) (context.Context, http.Header, error) {
	refreshed, err := s.refreshOpenAIAgentIdentityHeaders(ctx, account, headers)
	if err != nil {
		return ctx, nil, fmt.Errorf("refresh WS authentication: %w", err)
	}
	ctx, err = s.prepareOpenAIOAuthProof(ctx, account, rawURL, refreshed)
	return ctx, refreshed, err
}
