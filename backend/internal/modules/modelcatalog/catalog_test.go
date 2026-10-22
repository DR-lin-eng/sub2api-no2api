package modelcatalog

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestProviderDescriptorsPreserveExplicitFields(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"id":"model","context_window":777777,"max_output_tokens":32768,"supports_reasoning":false,"input_modalities":[],"supported_reasoning_levels":[]}]}`,
		`{"models":[{"name":"models/model","inputTokenLimit":777777,"outputTokenLimit":32768,"supportsReasoning":false,"inputModalities":[],"supportedReasoningLevels":[]}]}`,
		`{"models":{"model":{"maxTokens":777777,"maxOutputTokens":32768,"supportsThinking":false,"inputModalities":[],"supportedReasoningLevels":[]}}}`,
	} {
		metadata, err := Parse([]byte(body), []string{"model"})
		require.NoError(t, err)
		require.JSONEq(t, `777777`, string(metadata["model"]["context_window"]))
		require.JSONEq(t, `32768`, string(metadata["model"]["max_output_tokens"]))
		require.Equal(t, "false", string(metadata["model"]["supports_reasoning"]))
		require.Equal(t, "[]", string(metadata["model"]["input_modalities"]))
	}
}
func TestCatalogBoundsAndUnknownVersion(t *testing.T) {
	for _, body := range []string{`{"data":[{"id":"model"},{"id":"model","context_window":1}]}`, `{"data":[null]}`, `{"models":{"model":null}}`} {
		_, err := Parse([]byte(body), []string{"model"})
		require.Error(t, err)
	}
	_, err := Parse([]byte(strings.Repeat("x", MaxBodyBytes+1)), nil)
	require.Error(t, err)
	c := &Catalog{Models: []string{"bad id"}}
	_, err = c.Snapshot("fixture", time.Now())
	require.Error(t, err)
	require.Nil(t, ReadSnapshot(map[string]any{ExtraKey: map[string]any{"version": 99, "models": map[string]any{"x": map[string]any{"context_window": 123}}}}))
}
func TestSnapshotRemovesMissingModelsAndDoesNotMutateInput(t *testing.T) {
	d := Descriptor{"context_window": json.RawMessage(`123`), "token": json.RawMessage(`"do-not-store"`)}
	c := &Catalog{Models: []string{"x"}, Metadata: map[string]Descriptor{"x": d, "removed": d}}
	s, err := c.Snapshot("fixture", time.Now())
	require.NoError(t, err)
	require.NotContains(t, s.Models, "removed")
	require.NotContains(t, s.Models["x"], "token")
	require.Contains(t, d, "token")
	merged := Fill(Descriptor{"supports_reasoning": json.RawMessage(`false`)}, KnownFallback("gpt-6.1-sol"))
	require.NotContains(t, merged, "supported_reasoning_levels")
}
