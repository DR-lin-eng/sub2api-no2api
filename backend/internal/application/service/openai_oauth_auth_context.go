package service

import (
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
)

// The 20261107 desktop report recovers workspace tags 6..18. Keep an exact
// list: an unfamiliar future plan must not acquire workspace compatibility.
// This class only guards credential continuity; it grants no entitlement.
func openAIOAuthPlanClass(plan string) string {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "":
		return ""
	case "team", "self_serve_business_prolite", "self_serve_business_usage_based",
		"business", "ent26", "enterprise_cbp_automation", "enterprise_cbp_usage_based",
		"enterprise", "edu", "edu_plus", "edu_pro", "sci", "sci_plus":
		return "workspace"
	default:
		return "non_workspace"
	}
}

func openAIOAuthAccountPlanClass(account *Account) string {
	if account == nil {
		return ""
	}
	if plan := openAIOAuthPlanClass(account.GetCredential("plan_type")); plan != "" {
		return plan
	}
	if claims, err := openai.DecodeIDToken(strings.TrimSpace(account.GetOpenAIAccessToken())); err == nil && claims.OpenAIAuth != nil {
		return openAIOAuthPlanClass(claims.OpenAIAuth.ChatGPTPlanType)
	}
	return ""
}

func openAIOAuthPlanClassesChanged(expected, current string) bool {
	return expected != "" && current != "" && expected != current
}

// JWT decoding can shorten local token use, never extend the stored expiry or
// authenticate a token. Opaque tokens retain their existing expires_at policy.
func openAIOAuthAccessTokenExpiresAt(account *Account, accessToken string) *time.Time {
	expiresAt := account.GetCredentialAsTime("expires_at")
	// A newer cached token is not governed by the selected row's old expiry.
	if strings.TrimSpace(accessToken) != strings.TrimSpace(account.GetOpenAIAccessToken()) {
		expiresAt = nil
	}
	if account.IsOpenAIPersonalAccessToken() {
		return expiresAt
	}
	if claims, err := openai.DecodeIDToken(strings.TrimSpace(accessToken)); err == nil && claims.Exp > 0 {
		tokenExpiry := time.Unix(claims.Exp, 0)
		if expiresAt == nil || tokenExpiry.Before(*expiresAt) {
			return &tokenExpiry
		}
	}
	return expiresAt
}

func validateOpenAIOAuthAccessToken(account *Account, accessToken string) error {
	if err := validateOpenAIOAuthRequestPrincipal(account, accessToken); err != nil {
		return err
	}
	if expiresAt := openAIOAuthAccessTokenExpiresAt(account, accessToken); expiresAt != nil &&
		!time.Now().Before(*expiresAt) && !account.IsOpenAIPersonalAccessToken() {
		return errors.New("openai access_token is expired after refresh attempt")
	}
	return nil
}
