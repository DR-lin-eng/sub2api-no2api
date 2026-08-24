package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexFullSimulationGoldenWireProjection(t *testing.T) {
	ids := &codexFingerprintIDs{
		mode:            codexFingerprintFull,
		fullSimulation:  true,
		installationID:  "install",
		sessionID:       "session",
		threadID:        "session",
		turnID:          "turn",
		windowID:        "session:0",
		promptCacheKey:  "session",
		requestKind:     codexRequestKindTurn,
		turnStartedAtMS: 1700000000000,
	}

	headers := http.Header{
		"X-Codex-Installation-Id":           {"forged"},
		"X-Codex-Turn-Metadata":             {`{"sandbox":"workspace-write","custom":"value"}`},
		"X-Client-Request-Id":               {"forged"},
		"X-Oai-Attestation":                 {"forged-attestation"},
		"X-Openai-Internal-Codex-Residency": {"forged-residency"},
	}
	applyCodexFingerprintHeaders(headers, ids)
	require.Empty(t, headers.Get("x-codex-installation-id"))
	require.Empty(t, headers.Get("x-oai-attestation"))
	require.Empty(t, headers.Get("x-openai-internal-codex-residency"))
	require.Equal(t, "session", headers.Get("x-client-request-id"))
	require.Equal(t, `{"custom":"value","installation_id":"install","request_kind":"turn","sandbox":"workspace-write","session_id":"session","thread_id":"session","turn_id":"turn","turn_started_at_unix_ms":1700000000000,"window_id":"session:0"}`, headers.Get("x-codex-turn-metadata"))

	rewritten, changed, err := applyCodexFingerprintClientMetadataToBody(
		[]byte(`{"model":"gpt-5.5","client_metadata":{"custom":"value"}}`),
		ids,
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, `{"model":"gpt-5.5","client_metadata":{"session_id":"session","thread_id":"session","turn_id":"turn","x-codex-installation-id":"install","x-codex-turn-metadata":"{\"custom\":\"value\",\"installation_id\":\"install\",\"request_kind\":\"turn\",\"session_id\":\"session\",\"thread_id\":\"session\",\"turn_id\":\"turn\",\"turn_started_at_unix_ms\":1700000000000,\"window_id\":\"session:0\"}","x-codex-window-id":"session:0"},"prompt_cache_key":"session"}`, string(rewritten))
}
