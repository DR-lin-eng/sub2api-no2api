package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

func (c *Config) Validate() error {
	if c.IPv6Egress.Enabled {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("ipv6_egress.enabled is only supported on Linux")
		}
		if c.Deployment.IsMultiInstance() {
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
