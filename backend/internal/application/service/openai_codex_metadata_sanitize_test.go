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

	rewritten, err := rewriteCodexOutboundSessionMetadata(body, &codexOutboundSessionIDs{
		sessionID: "isolated-session",
		threadID:  "isolated-thread",
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
