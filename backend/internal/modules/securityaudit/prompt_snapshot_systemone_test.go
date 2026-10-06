package securityaudit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemOneSnapshotAuditsStateCriteriaAndExtensions(t *testing.T) {
	body := []byte(`{"model":"jev-latest","state":{"hidden":"<system-reminder>state-marker</system-reminder>"},"questions":{"question-marker":{"type":"choice","instructions":"instruction-marker","criteria":{"option-marker":"criteria-marker"},"extension":"question-extension-marker"}},"extension":"root-extension-marker"}`)
	snapshot, err := ExtractPromptSnapshot(Request{Protocol: "typesafe_systemone", Body: body})
	require.NoError(t, err)
	for _, marker := range []string{"state-marker", "question-marker", "instruction-marker", "option-marker", "criteria-marker", "question-extension-marker", "root-extension-marker"} {
		require.Contains(t, snapshot.ScanText, marker)
	}
}
