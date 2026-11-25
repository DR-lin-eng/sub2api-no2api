package antigravity

import "testing"

func TestNeutralizeSystemIdentityVersionlessCodexOpener(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"leading opener", "You are Codex, a coding agent. Keep the task.", "You are an AI agent. Keep the task."},
		{"leading whitespace", " \t\r\nYou are Codex, a coding agent.\r\nKeep indentation.", "You are an AI agent.\r\nKeep indentation."},
		{"case insensitive", "you are codex, a coding agent. Continue.", "You are an AI agent. Continue."},
		{"later mention", "Example:\nYou are Codex, a coding agent.", "Example:\nYou are Codex, a coding agent."},
		{"longer opener", "You are Codex, a coding agent helping with tests.", "You are Codex, a coding agent helping with tests."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeutralizeSystemIdentity(tt.input); got != tt.want {
				t.Fatalf("NeutralizeSystemIdentity() = %q, want %q", got, tt.want)
			}
		})
	}
}
