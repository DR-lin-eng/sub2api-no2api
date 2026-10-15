package apicompat

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

const anthropicThinkingEnvelopePrefix = "anthropic-thinking-v1:"
const maxAnthropicThinkingEnvelopeBytes = 8 << 20

// This is a wire envelope, not a signature issuer. The provider still verifies
// the opaque signature; foreign Responses ciphertext is never decoded here.
func encodeAnthropicThinking(block AnthropicContentBlock) string {
	if len(block.Thinking)+len(block.Signature)+len(block.Data) > maxAnthropicThinkingEnvelopeBytes {
		return ""
	}
	payload, err := json.Marshal(struct {
		Type      string `json:"type"`
		Thinking  string `json:"thinking,omitempty"`
		Signature string `json:"signature,omitempty"`
		Data      string `json:"data,omitempty"`
	}{block.Type, block.Thinking, block.Signature, block.Data})
	if err != nil || len(payload) > maxAnthropicThinkingEnvelopeBytes {
		return ""
	}
	return anthropicThinkingEnvelopePrefix + base64.RawStdEncoding.EncodeToString(payload)
}

func decodeAnthropicThinking(value string) (AnthropicContentBlock, bool, error) {
	var block AnthropicContentBlock
	encoded, matches := strings.CutPrefix(value, anthropicThinkingEnvelopePrefix)
	if !matches {
		return block, false, nil
	}
	if len(encoded) > base64.RawStdEncoding.EncodedLen(maxAnthropicThinkingEnvelopeBytes) {
		return block, true, fmt.Errorf("anthropic thinking envelope exceeds the replay limit")
	}
	raw, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return block, true, fmt.Errorf("invalid Anthropic thinking envelope")
	}
	if err := json.Unmarshal(raw, &block); err != nil {
		return block, true, fmt.Errorf("invalid Anthropic thinking block")
	}
	if (block.Type != "thinking" || strings.TrimSpace(block.Signature) == "" || block.Signature == "skip_thought_signature_validator") &&
		(block.Type != "redacted_thinking" || strings.TrimSpace(block.Data) == "") {
		return block, true, fmt.Errorf("invalid Anthropic signed thinking block")
	}
	// Only signed-block fields may cross this boundary.
	return AnthropicContentBlock{Type: block.Type, Thinking: block.Thinking, Signature: block.Signature, Data: block.Data}, true, nil
}
