package openai

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ValidateSIWCTokenResponse rejects partial grants that cannot make direct
// model requests or refresh later. SIWC refresh rotates both tokens.
func ValidateSIWCTokenResponse(token *TokenResponse, requireIDToken bool) error {
	if token == nil || strings.TrimSpace(token.AccessToken) == "" || strings.TrimSpace(token.RefreshToken) == "" {
		return fmt.Errorf("SIWC response requires access_token and refresh_token")
	}
	if token.ExpiresIn <= 0 {
		return fmt.Errorf("SIWC response requires a positive expires_in")
	}
	if requireIDToken && strings.TrimSpace(token.IDToken) == "" {
		return fmt.Errorf("SIWC response requires an id_token")
	}
	if !slices.Contains(strings.Fields(token.Scope), SIWCDirectTokenScope) {
		return fmt.Errorf("SIWC grant is missing %s", SIWCDirectTokenScope)
	}
	return nil
}

// UnmarshalJSON accepts either OIDC audience representation. Legacy Codex
// returns an array; dynamically registered clients may receive a string.
func (c *IDTokenClaims) UnmarshalJSON(data []byte) error {
	type claims IDTokenClaims
	var decoded struct {
		*claims
		Audience json.RawMessage `json:"aud"`
	}
	decoded.claims = (*claims)(c)
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if len(decoded.Audience) == 0 || string(decoded.Audience) == "null" {
		return nil
	}
	var audience string
	if json.Unmarshal(decoded.Audience, &audience) == nil {
		c.Aud = []string{audience}
		return nil
	}
	return json.Unmarshal(decoded.Audience, &c.Aud)
}
