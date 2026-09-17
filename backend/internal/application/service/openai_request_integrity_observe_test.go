package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAIRequestIntegrityTreatsEquivalentInputShapesAsEqual(t *testing.T) {
	original := []byte(`{"model":"gpt","input":"hello"}`)
	forwarded := []byte(`{"model":"mapped","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}],"instructions":"default"}`)
	differences, err := openAIRequestIntegrityDifferences(original, forwarded)
	require.NoError(t, err)
	require.Empty(t, differences)
}

func TestOpenAIRequestIntegrityReportsProtectedFieldNamesOnly(t *testing.T) {
	original := []byte(`{"input":[{"type":"message","role":"user","content":"hello"}],"tools":[{"type":"function","name":"lookup"}],"previous_response_id":"resp_1"}`)
	forwarded := []byte(`{"input":[{"type":"message","role":"user","content":"hello"}],"tools":[],"previous_response_id":"resp_2"}`)
	differences, err := openAIRequestIntegrityDifferences(original, forwarded)
	require.NoError(t, err)
	require.Equal(t, []string{"previous_response_id", "tools"}, differences)
}
