package config

func (c *Config) Validate() error {
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
