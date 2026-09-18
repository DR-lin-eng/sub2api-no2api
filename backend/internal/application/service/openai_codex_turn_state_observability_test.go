package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseOpenAICodexTurnStateTimestampAndExpiry(t *testing.T) {
	issued := time.Date(2026, 9, 18, 1, 0, 0, 0, time.UTC)
	raw := make([]byte, 9)
	raw[0] = 0x80
	binary.BigEndian.PutUint64(raw[1:], uint64(issued.Unix()))
	token := base64.RawURLEncoding.EncodeToString(raw)

	metadata := parseOpenAICodexTurnState(token, issued.Add(30*time.Minute))
	require.True(t, metadata.Valid)
	require.False(t, metadata.Expired)
	require.Equal(t, 0x80, metadata.Version)
	require.Equal(t, issued, *metadata.IssuedAt)
	require.Equal(t, issued.Add(time.Hour), *metadata.EstimatedExpiresAt)

	expired := parseOpenAICodexTurnState(token, issued.Add(61*time.Minute))
	require.True(t, expired.Expired)
}

func TestParseOpenAICodexTurnStateRejectsMalformedTokens(t *testing.T) {
	for _, token := range []string{"", "gAAA", strings.Repeat("x", openAICodexTurnStateMaxBytes+1)} {
		metadata := parseOpenAICodexTurnState(token, time.Now())
		require.False(t, metadata.Valid)
		require.NotEmpty(t, metadata.ParseError)
	}
}

func TestObserveCodexEncryptedContentClassifiesSixteenByteDeltaAndRotation(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(12)
	base := `{"type":"response.completed","response":{"output":[{"type":"reasoning","encrypted_content":"` + base64.RawStdEncoding.EncodeToString(make([]byte, 20)) + `"}]}}`
	degraded := `{"type":"response.completed","response":{"output":[{"type":"reasoning","encrypted_content":"` + base64.RawStdEncoding.EncodeToString(make([]byte, 36)) + `"}]}}`
	svc.observeCodexEncryptedContentPayload(nil, nil, account, "gpt-5.6-codex", []byte(base), "test")
	svc.observeCodexEncryptedContentPayload(nil, nil, account, "gpt-5.6-codex", []byte(degraded), "test")
	svc.observeCodexEncryptedContentPayload(nil, nil, account, "gpt-5.6-codex", []byte(`{"error":{"code":"invalid_encrypted_content","message":"Encrypted content could not be verified"}}`), "test")
	snapshot := svc.CodexTurnStateObservability(nil)
	require.Len(t, snapshot.Items, 1)
	require.Equal(t, "plus_16_hint", snapshot.Items[0].EncryptedContent.Classification)
	require.Equal(t, 16, snapshot.Items[0].EncryptedContent.DeltaBytes)
	require.Equal(t, uint64(1), snapshot.Items[0].Rotation.Count)
}

func TestObserveCodexTurnStateUsesDecodedBytesAndKeepsLastUsableState(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(14)
	issued := time.Now().Add(-10 * time.Minute).UTC().Truncate(time.Second)
	raw := make([]byte, 219)
	raw[0] = 0x80
	binary.BigEndian.PutUint64(raw[1:], uint64(issued.Unix()))
	state := base64.RawURLEncoding.EncodeToString(raw)
	require.Len(t, state, openAICodexDefaultTurnStateCharacters)
	svc.observeCodexTurnStateMetadata(context.Background(), nil, account, "gpt-5.6-codex", state, "response")
	svc.observeCodexTurnStateMetadata(context.Background(), nil, account, "gpt-5.6-codex", "", "response")
	snapshot := svc.CodexTurnStateObservability(context.Background())
	require.Len(t, snapshot.Items, 1)
	require.False(t, snapshot.Items[0].LastResponseHadState)
	require.Equal(t, len(raw), snapshot.Items[0].State.TokenBytes)
	require.Equal(t, openAICodexDefaultTurnStateCharacters, snapshot.Items[0].State.TokenCharacters)
	require.NotNil(t, snapshot.Items[0].State.EstimatedExpiresAt)
}

func TestInspectCodexEncryptedContentLeavesInvalidEncodingUnknown(t *testing.T) {
	lengths, rotation := inspectCodexEncryptedContentJSON(map[string]any{"encrypted_content": "not base64?"})
	require.Empty(t, lengths)
	require.False(t, rotation)
}

func TestAutomaticCodexTurnStateReplayRejectsTokenExpiredByEmbeddedTimestamp(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(15)
	raw := make([]byte, 219)
	raw[0] = 0x80
	binary.BigEndian.PutUint64(raw[1:], uint64(time.Now().Add(-2*time.Hour).Unix()))
	state := base64.RawURLEncoding.EncodeToString(raw)
	require.Len(t, state, openAICodexDefaultTurnStateCharacters)
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, "gpt-5.6-codex", state)
	require.Empty(t, svc.loadOpenAICodexAutoTurnState(context.Background(), account, "gpt-5.6-codex"))
}

func TestCodexTurnStateObservabilityRedactsOpaqueValues(t *testing.T) {
	settings := automaticTurnStateTestSettings(t, "gpt-5.6-codex")
	svc := &OpenAIGatewayService{settingService: settings}
	account := automaticTurnStateTestAccount(13)
	state := strings.Repeat("s", openAICodexDefaultTurnStateCharacters)
	svc.observeOpenAICodexTurnState(context.Background(), nil, account, "gpt-5.6-codex", state)
	svc.observeCodexEncryptedContentPayload(context.Background(), nil, account, "gpt-5.6-codex", []byte(`{"type":"response.completed","encrypted_content":"secret-cipher"}`), "test")
	raw, err := json.Marshal(svc.CodexTurnStateObservability(context.Background()))
	require.NoError(t, err)
	require.NotContains(t, string(raw), state)
	require.NotContains(t, string(raw), "secret-cipher")
	require.Contains(t, string(raw), "state_digest")
	require.False(t, svc.CodexTurnStateObservability(context.Background()).Items[0].EncryptedContent.LastBytesKnown)
}
