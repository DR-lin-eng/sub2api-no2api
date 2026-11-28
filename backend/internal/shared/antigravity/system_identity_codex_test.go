package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNeutralizeSystemIdentity_VersionlessCodexOpener(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "versionless coding agent with period",
			in:   "You are Codex, a coding agent. Follow user instructions.",
			want: "You are an AI agent. Follow user instructions.",
		},
		{
			name: "versionless an agent without comma",
			in:   "You are Codex an agent\nStay concise.",
			want: "You are an AI agent.\nStay concise.",
		},
		{
			name: "versionless coding agent followed by comma clause",
			in:   "You are Codex, a coding agent, running in terminal.",
			want: "You are an AI agent, running in terminal.",
		},
		{
			name: "word boundary preserved",
			in:   "You are Codex, a coding agentic helper.",
			want: "You are Codex, a coding agentic helper.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, NeutralizeSystemIdentity(tc.in))
		})
	}
}
