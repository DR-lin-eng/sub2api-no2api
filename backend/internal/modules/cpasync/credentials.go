package cpasync

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type AccountData struct {
	Name        string
	Credentials map[string]any
	Extra       map[string]any
}

// Convert reads only known OAuth credential fields, never arbitrary remote
// routing settings, local account status, or management secrets.
func Convert(file File, raw map[string]any) (*AccountData, error) {
	if disabled, ok := raw["disabled"].(bool); ok && disabled {
		return nil, errors.New("CPA credential is disabled")
	}
	if raw["disabled"] != nil && raw["disabled"] != false {
		return nil, errors.New("CPA credential has an invalid disabled flag")
	}
	if unavailable, _ := raw["unavailable"].(bool); unavailable {
		return nil, errors.New("CPA credential is unavailable")
	}
	if status := text(raw, "status"); status != "" && !strings.EqualFold(status, "active") {
		return nil, errors.New("CPA credential is not active")
	}
	if Platform(text(raw, "type")) != file.Platform() || file.Platform() == "" {
		return nil, errors.New("CPA credential provider does not match listing")
	}
	tokens := raw
	if nested, ok := raw["token"].(map[string]any); ok {
		tokens = nested
	}
	credentials := map[string]any{}
	for _, key := range []string{"access_token", "refresh_token", "id_token", "token_type", "scope"} {
		if value := text(tokens, key); value != "" {
			credentials[key] = value
		}
	}
	if text(credentials, "access_token") == "" {
		return nil, errors.New("CPA credential is missing access_token")
	}
	if text(credentials, "token_type") == "" {
		credentials["token_type"] = "Bearer"
	}
	for _, value := range []any{tokens["expiry"], tokens["expires_at"], raw["expired"], raw["expires_at"]} {
		if expiry, ok := timestamp(value); ok {
			credentials["expires_at"] = strconv.FormatInt(expiry.Unix(), 10)
			break
		}
	}
	for _, key := range []string{"email", "project_id", "client_id", "client_secret", "plan_type"} {
		if value := text(raw, key); value != "" {
			credentials[key] = value
		}
	}
	if text(credentials, "email") == "" && file.Email != "" {
		credentials["email"] = file.Email
	}
	if file.Platform() == "openai" {
		if id := text(raw, "account_id"); id != "" {
			credentials["chatgpt_account_id"] = id
		}
		for _, key := range []string{"chatgpt_account_id", "chatgpt_user_id", "organization_id"} {
			if v := text(raw, key); v != "" {
				credentials[key] = v
			}
		}
		for _, token := range []string{text(credentials, "id_token"), text(credentials, "access_token")} {
			addClaims(credentials, token)
		}
	}
	if file.Platform() == "anthropic" {
		for _, key := range []string{"account_uuid", "organization_uuid"} {
			if v := text(raw, key); v != "" {
				credentials[key] = v
			}
		}
	}
	if file.Platform() == "gemini" {
		credentials["oauth_type"] = "code_assist"
	}
	extra := map[string]any{}
	if email := text(credentials, "email"); email != "" {
		extra["email"] = email
	}
	name := strings.TrimSpace(file.Label)
	if name == "" {
		name = text(credentials, "email")
	}
	if name == "" {
		name = strings.TrimSuffix(file.Name, ".json")
	}
	if runes := []rune(name); len(runes) > 100 {
		name = string(runes[:100])
	}
	return &AccountData{Name: name, Credentials: credentials, Extra: extra}, nil
}

func text(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}
func timestamp(value any) (time.Time, bool) {
	switch v := value.(type) {
	case string:
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t, true
		}
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return time.Unix(n, 0), true
		}
	case float64:
		if v > 0 && v < 1e12 {
			return time.Unix(int64(v), 0), true
		}
	}
	return time.Time{}, false
}
func addClaims(credentials map[string]any, token string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return
	}
	var claims map[string]any
	if json.Unmarshal(raw, &claims) != nil {
		return
	}
	if text(credentials, "email") == "" {
		if email := text(claims, "email"); email != "" {
			credentials["email"] = email
		}
	}
	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	for target, source := range map[string]string{"chatgpt_account_id": "chatgpt_account_id", "chatgpt_user_id": "chatgpt_user_id", "plan_type": "chatgpt_plan_type"} {
		if text(credentials, target) == "" {
			if v := text(auth, source); v != "" {
				credentials[target] = v
			}
		}
	}
	if credentials["expires_at"] == nil {
		if t, ok := timestamp(claims["exp"]); ok {
			credentials["expires_at"] = strconv.FormatInt(t.Unix(), 10)
		}
	}
}
