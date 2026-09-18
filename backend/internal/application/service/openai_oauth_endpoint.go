package service

import (
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/urlvalidator"
)

// openAIOAuthCodexTargetURL resolves the model-request endpoint for an OpenAI
// OAuth account.  A configured custom URL is validated with the same outbound
// URL policy as API-key base URLs, while OAuth authorization and refresh remain
// untouched because they do not call this helper.
//
// The bool result reports whether the official ChatGPT endpoint is being used;
// callers use it to decide whether the ChatGPT-only Host override is valid.
func (s *OpenAIGatewayService) openAIOAuthCodexTargetURL(account *Account) (string, bool, error) {
	if account == nil || !account.IsOpenAIOAuth() || !account.IsCustomBaseURLEnabled() {
		return chatgptCodexURL, true, nil
	}

	customURL := strings.TrimSpace(account.GetCustomBaseURL())
	if customURL == "" {
		return "", false, fmt.Errorf("custom_base_url is enabled but not configured for account %d", account.ID)
	}
	var validatedURL string
	var err error
	if s == nil || s.cfg == nil {
		validatedURL, err = urlvalidator.ValidateURLFormat(customURL, true)
	} else {
		validatedURL, err = s.validateUpstreamBaseURL(customURL)
	}
	if err != nil {
		return "", false, err
	}
	return buildOpenAIOAuthCodexResponsesURL(validatedURL), false, nil
}

func isOpenAIOAuthCustomRelay(account *Account) bool {
	return account != nil && account.IsOpenAIOAuth() && account.IsCustomBaseURLEnabled() &&
		strings.TrimSpace(account.GetCustomBaseURL()) != ""
}

func validateOpenAIOAuthCustomRelayBaseURL(raw string, cfg *config.Config) (string, error) {
	if cfg != nil && !cfg.Security.URLAllowlist.Enabled {
		return urlvalidator.ValidateURLFormat(raw, cfg.Security.URLAllowlist.AllowInsecureHTTP)
	}
	var allowlist config.URLAllowlistConfig
	if cfg != nil {
		allowlist = cfg.Security.URLAllowlist
	}
	return urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
		AllowedHosts:     allowlist.UpstreamHosts,
		RequireAllowlist: true,
		AllowPrivate:     allowlist.AllowPrivateHosts,
	})
}
