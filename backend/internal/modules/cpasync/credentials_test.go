package cpasync

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConvertCPAProvidersAndWhitelists(t *testing.T) {
	for _, provider := range []string{"codex", "claude", "gemini", "gemini-cli", "antigravity"} {
		t.Run(provider, func(t *testing.T) {
			raw := map[string]any{"type": provider, "access_token": "access", "refresh_token": "refresh", "expired": "2030-01-01T00:00:00Z", "email": "account@example.test", "account_id": "team", "organization_uuid": "org", "account_uuid": "user", "project_id": "project", "base_url": "https://untrusted.invalid", "cpa_management_key": "secret", "extra": map[string]any{"codex_cli_only": true}}
			if provider == "gemini" {
				raw["token"] = map[string]any{"access_token": "access", "refresh_token": "refresh", "expiry": "2030-01-01T00:00:00Z"}
				delete(raw, "access_token")
			}
			data, err := Convert(File{Name: "account.json", Provider: provider}, raw)
			require.NoError(t, err)
			require.Equal(t, "access", data.Credentials["access_token"])
			require.Equal(t, "1893456000", data.Credentials["expires_at"])
			require.Equal(t, "account@example.test", data.Name)
			require.NotContains(t, data.Credentials, "base_url")
			require.NotContains(t, data.Credentials, "cpa_management_key")
			require.NotContains(t, data.Extra, "codex_cli_only")
			if provider == "codex" {
				require.Equal(t, "team", data.Credentials["chatgpt_account_id"])
			}
			if provider == "gemini" || provider == "gemini-cli" {
				require.Equal(t, "code_assist", data.Credentials["oauth_type"])
			}
		})
	}
}

func TestConvertCPACodexIdentity(t *testing.T) {
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"member@example.test","exp":1893456000,"https://api.openai.com/auth":{"chatgpt_account_id":"team","chatgpt_user_id":"member","chatgpt_plan_type":"team"}}`)) + ".signature"
	data, err := Convert(File{Name: "account.json", Type: "codex"}, map[string]any{"type": "codex", "access_token": token})
	require.NoError(t, err)
	require.Equal(t, "team", data.Credentials["chatgpt_account_id"])
	require.Equal(t, "member", data.Credentials["chatgpt_user_id"])
	require.Equal(t, "team", data.Credentials["plan_type"])
	require.Equal(t, "1893456000", data.Credentials["expires_at"])
}

func TestConvertCPARejectsChangedOrInvalidCredentials(t *testing.T) {
	for _, raw := range []map[string]any{
		{"type": "codex", "access_token": "access", "disabled": true},
		{"type": "codex", "access_token": "access", "disabled": "false"},
		{"type": "codex", "access_token": "access", "unavailable": true},
		{"type": "codex", "access_token": "access", "status": "error"},
		{"type": "claude", "access_token": "access"},
		{"type": "codex", "api_key": "key"},
	} {
		_, err := Convert(File{Name: "one.json", Provider: "codex"}, raw)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "\"access\"")
	}
}
