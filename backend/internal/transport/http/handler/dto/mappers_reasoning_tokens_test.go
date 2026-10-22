package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestReasoningTokensAdminOnly(t *testing.T) {
	count, zero := 1234, 0
	for _, value := range []*int{&count, &zero, nil} {
		log := &service.UsageLog{ReasoningTokens: value, OutputTokens: 2000}
		userJSON, err := json.Marshal(UsageLogFromService(log))
		require.NoError(t, err)
		require.NotContains(t, string(userJSON), "reasoning_tokens")
		adminJSON, err := json.Marshal(UsageLogFromServiceAdmin(log))
		require.NoError(t, err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(adminJSON, &fields))
		require.Contains(t, fields, "reasoning_tokens")
		var got *int
		require.NoError(t, json.Unmarshal(fields["reasoning_tokens"], &got))
		require.Equal(t, value, got)
	}
}
