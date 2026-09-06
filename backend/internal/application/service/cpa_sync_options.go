package service

import (
	"fmt"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

// CPAOAuthOptions is deliberately a typed subset of local OAuth settings, not
// arbitrary extra/credentials supplied by the remote CPA server.
type CPAOAuthOptions struct {
	TLSFingerprint      *bool   `json:"tls_fingerprint,omitempty"`
	SessionIDMasking    *bool   `json:"session_id_masking,omitempty"`
	InterceptWarmup     *bool   `json:"intercept_warmup,omitempty"`
	Passthrough         *bool   `json:"passthrough,omitempty"`
	FlattenNamespaces   *bool   `json:"flatten_namespaces,omitempty"`
	PrewarmContinuation *bool   `json:"prewarm_continuation,omitempty"`
	CodexCLIOnly        *bool   `json:"codex_cli_only,omitempty"`
	AllowAppServer      *bool   `json:"allow_app_server,omitempty"`
	LongContextBilling  *bool   `json:"long_context_billing,omitempty"`
	WSMode              *string `json:"ws_mode,omitempty"`
	FingerprintMode     *string `json:"fingerprint_mode,omitempty"`
	CompactMode         *string `json:"compact_mode,omitempty"`
}

func (o CPAOAuthOptions) validate(platform string) error {
	invalid := func(field string) error {
		return infraerrors.BadRequest("INVALID_CPA_OAUTH_OPTIONS", fmt.Sprintf("invalid OAuth option: %s", field))
	}
	for _, field := range []struct {
		name    string
		value   *string
		allowed []string
	}{
		{"ws_mode", o.WSMode, []string{"off", "ctx_pool", "passthrough", "http_bridge"}},
		{"fingerprint_mode", o.FingerprintMode, []string{"off", "device", "session", "full"}},
		{"compact_mode", o.CompactMode, []string{"auto", "force_on", "force_off"}},
	} {
		if field.value == nil {
			continue
		}
		valid := false
		for _, value := range field.allowed {
			if *field.value == value {
				valid = true
				break
			}
		}
		if !valid {
			return invalid(field.name)
		}
	}
	if platform != PlatformOpenAI && (o.Passthrough != nil || o.FlattenNamespaces != nil || o.PrewarmContinuation != nil || o.CodexCLIOnly != nil || o.AllowAppServer != nil || o.LongContextBilling != nil || o.WSMode != nil || o.FingerprintMode != nil || o.CompactMode != nil) {
		return invalid("OpenAI-only settings")
	}
	if platform != PlatformAnthropic && (o.SessionIDMasking != nil || o.InterceptWarmup != nil) {
		return invalid("Anthropic-only settings")
	}
	if o.AllowAppServer != nil && *o.AllowAppServer && (o.CodexCLIOnly == nil || !*o.CodexCLIOnly) {
		return invalid("allow_app_server requires codex_cli_only")
	}
	return nil
}

func (o CPAOAuthOptions) apply(credentials, extra map[string]any) {
	for key, value := range map[string]*bool{
		TLSFingerprintEnabledExtraKey:         o.TLSFingerprint,
		"session_id_masking_enabled":          o.SessionIDMasking,
		"openai_passthrough":                  o.Passthrough,
		"openai_responses_flatten_namespaces": o.FlattenNamespaces,
		CodexPrewarmContinuationExtraKey:      o.PrewarmContinuation,
		"codex_cli_only":                      o.CodexCLIOnly,
		"codex_cli_only_allow_app_server":     o.AllowAppServer,
		"openai_long_context_billing_enabled": o.LongContextBilling,
	} {
		if value != nil {
			extra[key] = *value
		}
	}
	if o.InterceptWarmup != nil {
		credentials["intercept_warmup_requests"] = *o.InterceptWarmup
	}
	if o.TLSFingerprint != nil && !*o.TLSFingerprint {
		delete(extra, TLSFingerprintProfileIDExtraKey)
	}
	if o.Passthrough != nil {
		delete(extra, "openai_oauth_passthrough")
	}
	if o.CodexCLIOnly != nil && !*o.CodexCLIOnly {
		extra["codex_cli_only_allow_app_server"] = false
	}
	if o.WSMode != nil {
		extra["openai_oauth_responses_websockets_v2_mode"] = *o.WSMode
		extra["openai_oauth_responses_websockets_v2_enabled"] = *o.WSMode != "off"
		delete(extra, "responses_websockets_v2_enabled")
		delete(extra, "openai_ws_enabled")
	}
	if o.FingerprintMode != nil {
		extra["codex_fingerprint_mode"] = *o.FingerprintMode
	}
	if o.CompactMode != nil {
		extra["openai_compact_mode"] = *o.CompactMode
	}
}
