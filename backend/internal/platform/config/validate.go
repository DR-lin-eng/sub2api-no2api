package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

const maxProxyProbeTargets = 8

func normalizeProxyProbeURLs(targets []ProbeURLConfig) ([]ProbeURLConfig, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	if len(targets) > maxProxyProbeTargets {
		return nil, fmt.Errorf("at most %d probe targets are allowed", maxProxyProbeTargets)
	}
	normalized := make([]ProbeURLConfig, 0, len(targets))
	for i, target := range targets {
		rawURL := strings.TrimSpace(target.URL)
		parser := strings.ToLower(strings.TrimSpace(target.Parser))
		if rawURL == "" {
			return nil, fmt.Errorf("entry %d: url is required", i)
		}
		if parser == "" {
			return nil, fmt.Errorf("entry %d: parser is required", i)
		}
		switch parser {
		case "ip-api", "ipify", "chatgpt-trace":
		default:
			return nil, fmt.Errorf("entry %d: unsupported parser %q", i, target.Parser)
		}
		parsed, err := url.Parse(rawURL)
		if err != nil || strings.TrimSpace(parsed.Hostname()) == "" {
			return nil, fmt.Errorf("entry %d: invalid url %q", i, target.URL)
		}
		if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
			return nil, fmt.Errorf("entry %d: url scheme must be http or https", i)
		}
		normalized = append(normalized, ProbeURLConfig{URL: rawURL, Parser: parser})
	}
	return normalized, nil
}

func (c *Config) Validate() error {
	ipv6Configured := c.IPv6Egress.Enabled || c.IPv6Egress.ControlEnabled || strings.TrimSpace(c.IPv6Egress.AllocationSecret) != ""
	if ipv6Configured {
		if runtime.GOOS != "linux" {
			if c.IPv6Egress.Enabled {
				return fmt.Errorf("ipv6_egress.enabled is only supported on Linux")
			}
		}
		if c.IPv6Egress.Enabled && len(strings.TrimSpace(c.IPv6Egress.AllocationSecret)) < 32 {
			return fmt.Errorf("ipv6_egress.allocation_secret must contain at least 32 characters when enabled")
		}
		if c.IPv6Egress.Enabled && c.Deployment.IsMultiInstance() {
			return fmt.Errorf("ipv6_egress.enabled currently requires deployment.mode=standalone")
		}
		if c.IPv6Egress.ReconcileIntervalSeconds < 5 || c.IPv6Egress.ReconcileIntervalSeconds > 3600 {
			return fmt.Errorf("ipv6_egress.reconcile_interval_seconds must be between 5 and 3600")
		}
		probeURL, err := url.Parse(strings.TrimSpace(c.IPv6Egress.ProbeURL))
		if err != nil || !strings.EqualFold(probeURL.Scheme, "https") || strings.TrimSpace(probeURL.Hostname()) == "" {
			return fmt.Errorf("ipv6_egress.probe_url must be an absolute HTTPS URL")
		}
		if c.IPv6Egress.ProbeTimeoutSeconds < 1 || c.IPv6Egress.ProbeTimeoutSeconds > 30 {
			return fmt.Errorf("ipv6_egress.probe_timeout_seconds must be between 1 and 30")
		}
		if c.IPv6Egress.ControlEnabled {
			if !filepath.IsAbs(strings.TrimSpace(c.IPv6Egress.ControlDir)) {
				return fmt.Errorf("ipv6_egress.control_dir must be an absolute path when control is enabled")
			}
			if c.IPv6Egress.ControlAgentStaleSeconds < 5 || c.IPv6Egress.ControlAgentStaleSeconds > 300 {
				return fmt.Errorf("ipv6_egress.control_agent_stale_seconds must be between 5 and 300")
			}
		}
	}
	if err := validateDeployment(c); err != nil {
		return err
	}
	if err := validateServerRuntime(c); err != nil {
		return err
	}
	if err := validateAPIKeyAuth(c); err != nil {
		return err
	}
	if err := validateJWTSecret(c); err != nil {
		return err
	}
	if err := validateLogging(c); err != nil {
		return err
	}
	if err := validateSubscriptionMaintenance(c); err != nil {
		return err
	}
	if err := validateGeminiOAuth(c); err != nil {
		return err
	}
	if err := validateServerFrontendURL(c); err != nil {
		return err
	}
	if err := validateJWTLifetimes(c); err != nil {
		return err
	}
	if err := validateSecurity(c); err != nil {
		return err
	}
	proxyProbeURLs, err := normalizeProxyProbeURLs(c.Security.ProxyProbe.URLs)
	if err != nil {
		return fmt.Errorf("security.proxy_probe.urls: %w", err)
	}
	c.Security.ProxyProbe.URLs = proxyProbeURLs
	if err := validateWebAuthn(c); err != nil {
		return err
	}
	if err := validateLinuxDoConnect(c); err != nil {
		return err
	}
	if err := validateWeChatConnect(c); err != nil {
		return err
	}
	if err := validateOIDCConnect(c); err != nil {
		return err
	}
	if err := validateBilling(c); err != nil {
		return err
	}
	if err := validateDataStores(c); err != nil {
		return err
	}
	if err := validateBatchImage(c); err != nil {
		return err
	}
	if err := validateDashboard(c); err != nil {
		return err
	}
	if err := validateUsageCleanup(c); err != nil {
		return err
	}
	if err := validateIdempotency(c); err != nil {
		return err
	}
	if err := validateGatewayTransport(c); err != nil {
		return err
	}
	if err := validateGatewayCodexSimulation(c); err != nil {
		return err
	}
	if err := validateGatewayOpenAIWebSocket(c); err != nil {
		return err
	}
	if err := validateGatewayReliability(c); err != nil {
		return err
	}
	if err := validateGatewayRouting(c); err != nil {
		return err
	}
	if err := validateGatewayUsageRecord(c); err != nil {
		return err
	}
	if err := validateGatewayCaches(c); err != nil {
		return err
	}
	if err := validateGatewayScheduling(c); err != nil {
		return err
	}
	if err := validateOperations(c); err != nil {
		return err
	}
	if err := validateConcurrency(c); err != nil {
		return err
	}
	if err := validateDingTalkConnect(c); err != nil {
		return err
	}
	return nil
}
