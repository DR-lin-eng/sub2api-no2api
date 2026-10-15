package apicompat

import "strings"

// AnthropicSignedThinkingCollector preserves fragmented signatures in bridges
// that buffer an upstream SSE response for a non-streaming client. Builders
// avoid quadratic string concatenation on large thinking histories.
type AnthropicSignedThinkingCollector struct {
	blocks map[int]*anthropicThinkingFragments
	failed bool
}
type anthropicThinkingFragments struct {
	text      strings.Builder
	signature strings.Builder
	dataBytes int
}

func (c *AnthropicSignedThinkingCollector) Start(index int, block AnthropicContentBlock) {
	if block.Type != "thinking" && block.Type != "redacted_thinking" {
		return
	}
	if c.blocks == nil {
		c.blocks = map[int]*anthropicThinkingFragments{}
	}
	if len(c.blocks) >= 128 || len(block.Thinking)+len(block.Signature)+len(block.Data) > maxAnthropicThinkingEnvelopeBytes {
		c.failed = true
		return
	}
	f := &anthropicThinkingFragments{dataBytes: len(block.Data)}
	_, _ = f.text.WriteString(block.Thinking)
	_, _ = f.signature.WriteString(block.Signature)
	c.blocks[index] = f
}

func (c *AnthropicSignedThinkingCollector) Delta(index int, delta *AnthropicDelta) bool {
	if delta == nil || (delta.Type != "thinking_delta" && delta.Type != "signature_delta") {
		return false
	}
	f := c.blocks[index]
	if f == nil {
		c.failed = true
		return true
	}
	value := delta.Thinking
	if delta.Type == "signature_delta" {
		value = delta.Signature
	}
	if f.text.Len()+f.signature.Len()+f.dataBytes+len(value) > maxAnthropicThinkingEnvelopeBytes {
		c.failed = true
		return true
	}
	if delta.Type == "thinking_delta" {
		_, _ = f.text.WriteString(value)
	} else {
		_, _ = f.signature.WriteString(value)
	}
	return true
}

// Apply reports a protocol error while allowing the application to retain and
// bill the usage already observed on the upstream response.
func (c *AnthropicSignedThinkingCollector) Apply(response *AnthropicResponse) *ResponsesError {
	for index, f := range c.blocks {
		if index < 0 || index >= len(response.Content) {
			c.failed = true
			continue
		}
		response.Content[index].Thinking = f.text.String()
		response.Content[index].Signature = f.signature.String()
	}
	if c.failed {
		return &ResponsesError{Code: "thinking_replay_limit", Message: "Signed thinking exceeds the replay limit or has invalid block indices"}
	}
	return nil
}
