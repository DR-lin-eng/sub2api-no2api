package antigravity

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransformClaudeToGeminiStripsLeadingAttributionOnly(t *testing.T) {
	const attribution = "x-anthropic-billing-header: cc_version=2.1.271.4bf; cc_entrypoint=claude-desktop-3p;"
	for _, tt := range []struct {
		name string
		text string
		want string
	}{
		{"metadata only", attribution, ""},
		{"instructions follow", attribution + "\nKeep these instructions.", "Keep these instructions."},
		{"crlf", " \t" + attribution + "\r\n  Keep indentation.", "  Keep indentation."},
		{"ordinary", "Explain this metadata: " + attribution, "Explain this metadata: " + attribution},
		{"later line", "Example:\n" + attribution, "Example:\n" + attribution},
	} {
		t.Run(tt.name, func(t *testing.T) {
			system, err := json.Marshal(tt.text)
			require.NoError(t, err)
			got := buildSystemInstruction(system, "gemini-3.8-flash-high", TransformOptions{}, nil)
			if tt.want == "" {
				for _, part := range got.Parts {
					require.NotContains(t, part.Text, attribution)
				}
				return
			}
			require.NotEmpty(t, got.Parts)
			require.Equal(t, tt.want, got.Parts[0].Text)
		})
	}
}

func TestTransformClaudeToGeminiStripsAttributionInSystemBlockArray(t *testing.T) {
	const attribution = "x-anthropic-billing-header: cc_version=example;"
	system, err := json.Marshal([]SystemBlock{{Type: "text", Text: attribution + "\nKeep policy."}, {Type: "text", Text: "Second block."}})
	require.NoError(t, err)
	got := buildSystemInstruction(system, "gemini-3.8-flash-high", TransformOptions{}, nil)
	var texts []string
	for _, part := range got.Parts {
		if part.Text != "\n--- [SYSTEM_PROMPT_END] ---" {
			texts = append(texts, part.Text)
		}
	}
	require.Equal(t, []string{"Keep policy.", "Second block."}, texts)
}
