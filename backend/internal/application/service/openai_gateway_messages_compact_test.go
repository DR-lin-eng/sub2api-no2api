package service

import (
	"strings"
	"testing"
)

func TestRetryAnthropicCompactFallbackSummariesSplitsSingleOversizedSummary(t *testing.T) {
	oversized := strings.Repeat("important-state ", 1200)

	retry := retryAnthropicCompactFallbackSummaries("compact prompt", []string{oversized}, 4_000)

	if len(retry) < 2 {
		t.Fatalf("retry summaries = %d, want split oversized single summary", len(retry))
	}
	for i, summary := range retry {
		if !strings.Contains(summary, "Oversized summary split") {
			t.Fatalf("retry summary %d missing split heading: %q", i, summary[:min(len(summary), 80)])
		}
		if runeLen(summary) > 4_500 {
			t.Fatalf("retry summary %d too large: %d runes", i, runeLen(summary))
		}
	}
}

func TestBuildAnthropicCompactMergePromptIncludesQualityContract(t *testing.T) {
	prompt := buildAnthropicCompactMergePrompt("Create a compact summary", []string{"## Chunk 1/1\nstate"})

	for _, want := range []string{
		"# Compact Capsule",
		"## Current State",
		"## Active User Intent",
		"## Files Touched",
		"## Commands And Evidence",
		"## Errors And Blockers",
		"## Decisions And Config",
		"## Next Command",
		"Do not include meta-statements",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("merge prompt missing %q\n%s", want, prompt)
		}
	}
}

func TestBuildAnthropicCompactEmergencyResponseReturnsCompletedSummary(t *testing.T) {
	usage := OpenAIUsage{InputTokens: 123, OutputTokens: 45}
	resp := buildAnthropicCompactEmergencyResponse("gpt-5.3-codex-spark", "summary text", usage)

	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Status != "completed" {
		t.Fatalf("status = %q, want completed", resp.Status)
	}
	if resp.Model != "gpt-5.3-codex-spark" {
		t.Fatalf("model = %q", resp.Model)
	}
	if got := openAIResponsesOutputText(resp); got != "summary text" {
		t.Fatalf("output text = %q", got)
	}
	if resp.Usage == nil || resp.Usage.InputTokens != 123 || resp.Usage.OutputTokens != 45 || resp.Usage.TotalTokens != 168 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestBuildAnthropicCompactEmergencySummaryCapsMiddle(t *testing.T) {
	parts := []string{
		"head-" + strings.Repeat("a", 70_000),
		"tail-" + strings.Repeat("b", 70_000),
	}

	summary := buildAnthropicCompactEmergencySummary("", parts)

	if !strings.Contains(summary, "# Compact Capsule") {
		t.Fatalf("missing capsule heading")
	}
	if !strings.Contains(summary, "middle omitted by compact fallback emergency guard") {
		t.Fatalf("summary was not middle-trimmed")
	}
	if runeLen(summary) > openAIAnthropicCompactEmergencyMaxRunes+2_000 {
		t.Fatalf("summary too large: %d", runeLen(summary))
	}
}

func TestAnthropicCompactQualityContractEval(t *testing.T) {
	summaries := []string{
		"User asked to continue session 07613379. Files: backend/internal/application/service/openai_gateway_messages.go. Error: compact fallback merge exceeded context window. Next: fix recursive merge and test.",
		"Commands: go test ./internal/application/service. Config: CLAUDE_CODE_AUTO_COMPACT_WINDOW=240000, ANTHROPIC_SMALL_FAST_MODEL=gpt-5.3-codex-spark.",
	}

	prompt := buildAnthropicCompactMergePrompt("Create a compact summary", summaries)
	required := []string{
		"# Compact Capsule",
		"## Current State",
		"## Active User Intent",
		"## Files Touched",
		"## Commands And Evidence",
		"## Errors And Blockers",
		"## Decisions And Config",
		"## Next Command",
		"Do not invent completed tests",
	}
	for _, item := range required {
		if !strings.Contains(prompt, item) {
			t.Fatalf("quality contract missing %q", item)
		}
	}
	if strings.Contains(prompt, "current intent is to produce a summary") {
		t.Fatalf("quality contract still encourages meta compact intent")
	}
}

func TestSanitizeAnthropicCompactSummaryForMergeDropsMetaIntentBlock(t *testing.T) {
	raw := `# Compact Capsule

## Current State
- Task #55 still needs BDD repair.

## Active User Intent
- Current inferred active intent (from latest user-visible turn): produce a merged, detailed compact summary of prior conversation due prior chunking, preserving:
  - exact operational state,
  - pending tasks/blockers,
  - files and commands.
- Security/interaction constraints currently in force: respond text-only.
- Real task: finish Task #55 BDD breakages and reduce false-positive claim-evidence blocking behavior.

## Next Command
- Run docker compose test.`

	clean := sanitizeAnthropicCompactSummaryForMerge(raw)

	for _, banned := range []string{
		"produce a merged, detailed compact summary",
		"prior chunking",
		"exact operational state",
		"pending tasks/blockers",
	} {
		if strings.Contains(clean, banned) {
			t.Fatalf("sanitized summary still contains meta compact intent %q\n%s", banned, clean)
		}
	}
	for _, want := range []string{
		"Task #55 still needs BDD repair",
		"Security/interaction constraints",
		"Real task: finish Task #55 BDD breakages",
		"Run docker compose test",
	} {
		if !strings.Contains(clean, want) {
			t.Fatalf("sanitized summary dropped real state %q\n%s", want, clean)
		}
	}
}

func TestBuildAnthropicCompactMergePromptSanitizesMetaIntent(t *testing.T) {
	prompt := buildAnthropicCompactMergePrompt("Create a compact summary", []string{
		"## Active User Intent\n- Current inferred active intent: produce a detailed compact summary due to context compaction.\n- Real task: fix recursive merge and test.",
	})

	if strings.Contains(prompt, "produce a detailed compact summary due to context compaction") {
		t.Fatalf("merge prompt preserved meta compact intent\n%s", prompt)
	}
	if !strings.Contains(prompt, "Real task: fix recursive merge and test") {
		t.Fatalf("merge prompt dropped real task\n%s", prompt)
	}
}

func BenchmarkAnthropicCompactFallbackGrouping(b *testing.B) {
	summary := strings.Repeat("file backend/internal/application/service/openai_gateway_messages.go error compact fallback merge exceeded context window next recursive merge test ", 500)
	summaries := make([]string, 24)
	for i := range summaries {
		summaries[i] = summary
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		groups := groupAnthropicCompactSummariesForMerge("Create a compact summary", summaries, 35_000)
		if len(groups) == 0 {
			b.Fatal("no groups")
		}
		_ = buildAnthropicCompactEmergencySummary("Create a compact summary", summaries)
	}
}

func BenchmarkEstimateOpenAIAnthropicStreamInputTokens(b *testing.B) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{name: "64_KiB", size: 64 << 10},
		{name: "1_MiB", size: 1 << 20},
	} {
		payload := []byte(`{"model":"gpt-5.6","input":"` + strings.Repeat("a", tc.size) + `"}`)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(payload)))
			for range b.N {
				if estimateOpenAIAnthropicStreamInputTokens(payload, "gpt-5.6") <= 0 {
					b.Fatal("missing estimate")
				}
			}
		})
	}
}
