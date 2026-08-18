package service

import (
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/stretchr/testify/require"
)

func TestNormalizeBulkOpenAISettingsCanonicalizesCapabilitiesAndMode(t *testing.T) {
	input := &BulkUpdateAccountsInput{
		Credentials: map[string]any{
			openAIEndpointCapabilitiesCredentialKey: []any{"embeddings", "chat_completions", "embeddings"},
		},
		Extra: map[string]any{
			"openai_responses_mode": "force_chat_completions",
		},
	}

	settings, err := normalizeBulkOpenAISettings(input)
	require.NoError(t, err)
	require.True(t, settings.endpointCapabilities)
	require.True(t, settings.responsesMode)
	require.Equal(t, []string{"chat_completions", "embeddings"}, input.Credentials[openAIEndpointCapabilitiesCredentialKey])
	require.Equal(t, "force_chat_completions", input.Extra["openai_responses_mode"])
}

func TestNormalizeBulkOpenAISettingsRejectsForcedResponsesWithoutChatCapability(t *testing.T) {
	input := &BulkUpdateAccountsInput{
		Credentials: map[string]any{
			openAIEndpointCapabilitiesCredentialKey: []string{"embeddings"},
		},
		Extra: map[string]any{
			"openai_responses_mode": "force_responses",
		},
	}

	_, err := normalizeBulkOpenAISettings(input)
	require.Error(t, err)
	require.Equal(t, "OPENAI_RESPONSES_MODE_INVALID", infraerrors.Reason(err))
}

func TestValidateBulkOpenAISettingsTargetsRejectsNonAPIKeyEndpointUpdates(t *testing.T) {
	input := &BulkUpdateAccountsInput{AccountIDs: []int64{7}}
	settings := bulkOpenAISettings{endpointCapabilities: true, capabilitiesIncludeChat: true}
	_, err := validateBulkOpenAISettingsTargets(input, settings, map[int64]*Account{
		7: {ID: 7, Platform: PlatformOpenAI, Type: AccountTypeOAuth},
	})
	require.Error(t, err)
	require.Equal(t, "OPENAI_BULK_TARGET_INVALID", infraerrors.Reason(err))
}
