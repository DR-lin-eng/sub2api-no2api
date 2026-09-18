package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/urlvalidator"
)

// DefaultOpenAIOAuthForceRelayBaseURL is prefilled in the administrator
// settings form but remains disabled until explicitly enabled.
const DefaultOpenAIOAuthForceRelayBaseURL = "https://codex-relay.oaifree.com/backend-api/codex"

// resolveOpenAIOAuthCodexBaseURL returns the base URL for OpenAI OAuth model
// traffic and whether that URL is the official ChatGPT endpoint. Global relay
// settings take precedence over the account-level relay override.
func resolveOpenAIOAuthCodexBaseURL(
	ctx context.Context,
	settingService *SettingService,
	cfg *config.Config,
	account *Account,
) (string, bool, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return "https://chatgpt.com/backend-api/codex", true, nil
	}
	if settingService != nil {
		enabled, baseURL, err := settingService.GetOpenAIOAuthForceRelaySettings(ctx)
		if err != nil {
			return "", false, fmt.Errorf("openai oauth force relay settings: %w", err)
		}
		if enabled {
			return baseURL, false, nil
		}
	}
	if !account.IsCustomBaseURLEnabled() {
		return "https://chatgpt.com/backend-api/codex", true, nil
	}
	customURL := strings.TrimSpace(account.GetCustomBaseURL())
	if customURL == "" {
		return "", false, fmt.Errorf("custom_base_url is enabled but not configured for account %d", account.ID)
	}
	validatedURL, err := validateOpenAIOAuthCustomRelayBaseURL(customURL, cfg)
	if err != nil {
		return "", false, err
	}
	return validatedURL, false, nil
}

// openAIOAuthCodexTargetURL resolves the model-request endpoint for an OpenAI
// OAuth account.  A configured custom URL is validated with the same outbound
// URL policy as API-key base URLs, while OAuth authorization and refresh remain
// untouched because they do not call this helper.
//
// The bool result reports whether the official ChatGPT endpoint is being used;
// callers use it to decide whether the ChatGPT-only Host override is valid.
func (s *OpenAIGatewayService) openAIOAuthCodexTargetURL(account *Account) (string, bool, error) {
	return s.openAIOAuthCodexTargetURLWithContext(context.Background(), account)
}

func (s *OpenAIGatewayService) openAIOAuthCodexTargetURLWithContext(ctx context.Context, account *Account) (string, bool, error) {
	var settingService *SettingService
	var cfg *config.Config
	if s != nil {
		settingService = s.settingService
		cfg = s.cfg
	}
	baseURL, official, err := resolveOpenAIOAuthCodexBaseURL(ctx, settingService, cfg, account)
	if err != nil {
		return "", false, err
	}
	return buildOpenAIOAuthCodexResponsesURL(baseURL), official, nil
}

func validateOpenAIOAuthCustomRelayBaseURL(raw string, cfg *config.Config) (string, error) {
	if cfg == nil {
		return urlvalidator.ValidateURLFormat(raw, true)
	}
	if !cfg.Security.URLAllowlist.Enabled {
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
