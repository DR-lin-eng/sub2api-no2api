package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIWSNextAttemptMessageUsesCurrentTurnPayload(t *testing.T) {
	first := []byte(`{"type":"response.create","input":"first"}`)
	current := []byte(`{"type":"response.create","input":"turn"}`)
	next, ok := openAIWSNextAttemptMessage(first, current, true)
	require.True(t, ok)
	require.Equal(t, current, next)
	next[0] = 'x'
	require.Equal(t, byte('{'), current[0])
}
