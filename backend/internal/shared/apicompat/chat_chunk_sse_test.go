package apicompat

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func referenceChatChunkToSSE(t *testing.T, chunk ChatCompletionsChunk) string {
	t.Helper()
	data, err := json.Marshal(chunk)
	require.NoError(t, err)
	return "data: " + string(data) + "\n\n"
}

func TestChatChunkToSSE_ContentDeltaMatchesEncodingJSON(t *testing.T) {
	contents := []string{
		"", "hello", "你好，世界 😀", `quote " backslash \ slash /`, "line\nbreak\ttab\r", "<script>&amp;</script>",
		"sep\u2028arator\u2029", "bad\xffutf8", "\xed\xa0\x80", "\x00\x1f\x7f", "é\u00a0\ufeff",
	}
	rng := rand.New(rand.NewSource(1))
	alphabet := []string{"a", " ", `"`, `\`, "<", ">", "&", "\n", "\x01", "\x7f", "中", "😀", "\u2028", "\u2029", "\xff", "\xc3", "\xa9"}
	for i := 0; i < 2000; i++ {
		content := ""
		for n := rng.Intn(12); n > 0; n-- {
			content += alphabet[rng.Intn(len(alphabet))]
		}
		contents = append(contents, content)
	}
	for _, content := range contents {
		content := content
		chunk := ChatCompletionsChunk{
			ID: "chatcmpl-" + content, Object: "chat.completion.chunk", Created: 1700000000, Model: "gpt-5.4<" + content,
			Choices: []ChatChunkChoice{{Index: 0, Delta: ChatDelta{Content: &content}}},
		}
		got, err := ChatChunkToSSE(chunk)
		require.NoError(t, err)
		require.Equal(t, referenceChatChunkToSSE(t, chunk), got, "content=%q", content)
		_, fast := chatContentDeltaChunkToSSE(&chunk)
		require.True(t, fast)
	}
}

func TestChatChunkToSSE_OtherShapesUseEncodingJSON(t *testing.T) {
	text, stop := "x", "stop"
	base := func() ChatCompletionsChunk {
		content := text
		return ChatCompletionsChunk{
			ID: "id", Object: "chat.completion.chunk", Created: -3, Model: "m",
			Choices: []ChatChunkChoice{{Index: 0, Delta: ChatDelta{Content: &content}}},
		}
	}
	mutations := map[string]func(*ChatCompletionsChunk){
		"usage":              func(c *ChatCompletionsChunk) { c.Usage = &ChatUsage{} },
		"system fingerprint": func(c *ChatCompletionsChunk) { c.SystemFingerprint = "fp" },
		"service tier":       func(c *ChatCompletionsChunk) { c.ServiceTier = "priority" },
		"no choices":         func(c *ChatCompletionsChunk) { c.Choices = nil },
		"two choices":        func(c *ChatCompletionsChunk) { c.Choices = append(c.Choices, c.Choices[0]) },
		"finish reason":      func(c *ChatCompletionsChunk) { c.Choices[0].FinishReason = &stop },
		"role":               func(c *ChatCompletionsChunk) { c.Choices[0].Delta.Role = "assistant" },
		"no content":         func(c *ChatCompletionsChunk) { c.Choices[0].Delta.Content = nil },
		"refusal":            func(c *ChatCompletionsChunk) { c.Choices[0].Delta.Refusal = &text },
		"reasoning content":  func(c *ChatCompletionsChunk) { c.Choices[0].Delta.ReasoningContent = &text },
		"reasoning":          func(c *ChatCompletionsChunk) { c.Choices[0].Delta.Reasoning = &text },
		"tool calls":         func(c *ChatCompletionsChunk) { c.Choices[0].Delta.ToolCalls = []ChatToolCall{{}} },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			chunk := base()
			mutate(&chunk)
			_, fast := chatContentDeltaChunkToSSE(&chunk)
			require.False(t, fast)
			got, err := ChatChunkToSSE(chunk)
			require.NoError(t, err)
			require.Equal(t, referenceChatChunkToSSE(t, chunk), got)
		})
	}
	chunk := base()
	chunk.Choices[0].Index = 2
	got, err := ChatChunkToSSE(chunk)
	require.NoError(t, err)
	require.Equal(t, referenceChatChunkToSSE(t, chunk), got)
}
