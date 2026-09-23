package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeCodexTurnMetadataWorkspacePrivacy(t *testing.T) {
	raw := `{"sandbox":"workspace-write","workspaces":[{"path":"/workspace/alice/private/repo","associated_remote_urls":["https://example.com/org/repo.git?tracking=fixture#fragment","git@example.com:org/repo.git"],"access_token":"drop-me","head":"abc123"}],"cwd":"/workspace/alice/private/repo"}`
	sanitized := sanitizeCodexTurnMetadataValue(raw)

	require.NotContains(t, sanitized, "/workspace/alice")
	require.NotContains(t, sanitized, "tracking=fixture")
	require.NotContains(t, sanitized, "fragment")
	require.NotContains(t, sanitized, "access_token")
	require.Equal(t, "workspace:redacted", gjson.Get(sanitized, "cwd").String())
	require.Equal(t, "workspace:redacted", gjson.Get(sanitized, "workspaces.0.path").String())
	require.Equal(t, "https://example.com/org/repo.git", gjson.Get(sanitized, "workspaces.0.associated_remote_urls.0").String())
	require.Equal(t, "example.com:org/repo.git", gjson.Get(sanitized, "workspaces.0.associated_remote_urls.1").String())
	require.Equal(t, "abc123", gjson.Get(sanitized, "workspaces.0.head").String())
	require.Equal(t, "workspace-write", gjson.Get(sanitized, "sandbox").String())
}

func TestSanitizeCodexTurnMetadataPreservesUnrelatedValues(t *testing.T) {
	raw := `{"sandbox":"workspace-write","custom":{"path":"logical/path","token_count":7}}`
	require.Equal(t, raw, sanitizeCodexTurnMetadataValue(raw))
	require.Equal(t, "not-json", sanitizeCodexTurnMetadataValue("not-json"))
}

func TestRewriteCodexOutboundSessionMetadataSanitizesEmbeddedTurnMetadata(t *testing.T) {
	turnMetadata := `{"workspaces":[{"path":"/private/repo","associated_remote_urls":["https://example.com/repo.git?tracking=fixture#fragment"]}]}`
	body, err := json.Marshal(map[string]any{
		"client_metadata": map[string]any{
			"session_id":            "raw-session",
			"thread_id":             "raw-thread",
			"x-codex-turn-metadata": turnMetadata,
		},
	})
	require.NoError(t, err)

	account := &Account{ID: 77, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	rewritten, err := rewriteCodexOutboundSessionMetadata(body, account, &codexOutboundSessionIDs{
		installationID: "installation",
		sessionID:      "isolated-session",
		threadID:       "isolated-thread",
	})
	require.NoError(t, err)
	require.Equal(t, "isolated-session", gjson.GetBytes(rewritten, "client_metadata.session_id").String())
	metadata := gjson.GetBytes(rewritten, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, "workspace:redacted", gjson.Get(metadata, "workspaces.0.path").String())
	require.Equal(t, "https://example.com/repo.git", gjson.Get(metadata, "workspaces.0.associated_remote_urls.0").String())
}

func TestCodexFullSimulationSanitizesWorkspaceBeforeProjection(t *testing.T) {
	ids := &codexFingerprintIDs{
		mode:           codexFingerprintFull,
		fullSimulation: true,
		installationID: "installation",
		sessionID:      "session",
		threadID:       "thread",
		turnID:         "turn",
		windowID:       "thread:1",
	}
	body := []byte(`{"client_metadata":{"workspaces":[{"path":"/workspace/alice/private","associated_remote_urls":["https://example.com/repo.git?tracking=fixture#fragment"]}]}}`)
	rewritten, changed, err := applyCodexFingerprintClientMetadataToBody(body, ids)
	require.NoError(t, err)
	require.True(t, changed)
	metadata := gjson.GetBytes(rewritten, "client_metadata.x-codex-turn-metadata").String()
	require.NotContains(t, metadata, "/workspace/alice")
	require.Contains(t, metadata, "workspace:redacted")
	require.Contains(t, metadata, "https://example.com/repo.git")
}

func TestSanitizeOpenAICodexClientMetadataUsesStableAccountInstallation(t *testing.T) {
	account := &Account{ID: 2048, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	ids := &codexOutboundSessionIDs{sessionID: "stable-session", threadID: "stable-thread"}
	metadata := map[string]any{
		"x-codex-installation-id":    "forged-installation",
		"session_id":                 "forged-session",
		"thread_id":                  "forged-thread",
		"turn_id":                    "forged-turn",
		"x-codex-window-id":          "forged-window",
		"guardian_credits_requested": true,
		"x-codex-turn-metadata":      `{"sandbox":"danger-full-access","workspaces":[{"path":"/private/repo"}]}`,
	}

	require.True(t, sanitizeOpenAICodexClientMetadataMap(metadata, account, ids))
	stableInstallation, ok := metadata["x-codex-installation-id"].(string)
	require.True(t, ok)
	require.NotEqual(t, "forged-installation", stableInstallation)
	require.Equal(t, stableInstallation, resolveConvergedInstallationID(account))
	require.Equal(t, "stable-session", metadata["session_id"])
	require.Equal(t, "stable-thread", metadata["thread_id"])
	require.NotContains(t, metadata, "turn_id")
	require.NotContains(t, metadata, "x-codex-window-id")
	require.NotContains(t, metadata, "guardian_credits_requested")
	turnMetadata, ok := metadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	require.Equal(t, "workspace:redacted", gjson.Get(turnMetadata, "workspaces.0.path").String())
	require.Empty(t, gjson.Get(turnMetadata, "sandbox").String())

	second := map[string]any{"x-codex-installation-id": "another-forged-value"}
	require.True(t, sanitizeOpenAICodexClientMetadataMap(second, account, ids))
	require.Equal(t, stableInstallation, second["x-codex-installation-id"])
}
