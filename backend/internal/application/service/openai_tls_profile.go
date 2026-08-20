package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
)

// SetTLSFingerprintProfileService attaches the shared account Profile resolver
// after construction. Keeping this as a setter preserves the existing public
// constructor used by focused service tests while allowing Wire to provide the
// transport profile service in production.
func (s *OpenAIGatewayService) SetTLSFingerprintProfileService(profileService *TLSFingerprintProfileService) {
	if s != nil {
		s.tlsFPProfileService = profileService
	}
}

func (s *OpenAIGatewayService) resolveTLSProfile(account *Account) *tlsfingerprint.Profile {
	if s == nil || s.tlsFPProfileService == nil {
		return nil
	}
	return s.tlsFPProfileService.ResolveTLSProfile(account)
}

// doAccountHTTPUpstream sends an OpenAI gateway request with the profile
// resolved for the selected account. Keeping this at the service boundary
// covers auxiliary paths (images, embeddings, live bridge, search and probes)
// that do not use the main Responses request builder.
func (s *OpenAIGatewayService) doAccountHTTPUpstream(
	req *http.Request,
	proxyURL string,
	account *Account,
) (*http.Response, error) {
	if req != nil {
		req = req.WithContext(WithHTTPUpstreamTLSProfile(req.Context(), s.resolveTLSProfile(account)))
	}
	return doAccountHTTPUpstream(s.httpUpstream, req, proxyURL, account)
}
