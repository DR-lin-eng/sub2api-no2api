package apicompat

import (
	"encoding/json"
	"strings"
)

const (
	literalThinkingOpenTag  = "<thinking>"
	literalThinkingCloseTag = "</thinking>"
	// Keep the undecided prefix bounded. A malformed upstream must not be able
	// to turn the stream bridge into an unbounded response buffer.
	literalThinkingMaxBufferedBytes = 64 * 1024
)

// normalizeLiteralThinkingText recognizes the narrow legacy format emitted by
// some Chat Completions fallbacks. Only a complete tag at byte zero is
// normalized; everything else is preserved verbatim.
func normalizeLiteralThinkingText(text string) (reasoning, visible string, normalized bool) {
	if !strings.HasPrefix(text, literalThinkingOpenTag) {
		return "", text, false
	}
	closeOffset := strings.Index(text[len(literalThinkingOpenTag):], literalThinkingCloseTag)
	if closeOffset < 0 {
		return "", text, false
	}
	closeOffset += len(literalThinkingOpenTag)
	reasoning = text[len(literalThinkingOpenTag):closeOffset]
	visible = text[closeOffset+len(literalThinkingCloseTag):]
	if reasoning == "" || strings.Contains(reasoning, literalThinkingOpenTag) ||
		strings.Contains(visible, literalThinkingOpenTag) || strings.Contains(visible, literalThinkingCloseTag) {
		return "", text, false
	}
	return reasoning, visible, true
}

func normalizeLiteralThinkingMessage(message ChatMessage) (ChatMessage, bool) {
	if message.Role != "assistant" {
		return message, false
	}
	if message.ReasoningContent != "" || message.Reasoning != "" {
		return message, false
	}
	raw := bytesTrimSpace(message.Content)
	if len(raw) == 0 || string(raw) == "null" {
		return message, false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return message, false
	}
	reasoning, visible, ok := normalizeLiteralThinkingText(text)
	if !ok {
		return message, false
	}
	if message.ReasoningContent == "" && message.Reasoning == "" {
		message.ReasoningContent = reasoning
	}
	message.Content, _ = json.Marshal(visible)
	return message, true
}

type literalThinkingStreamParser struct {
	enabled bool
	// undecided buffers only a possible leading tag. Once the prefix is
	// disproved or the input is malformed, it permanently becomes passthrough.
	undecided  bool
	normalized bool
	buffer     strings.Builder
}

func newLiteralThinkingStreamParser(enabled bool) literalThinkingStreamParser {
	return literalThinkingStreamParser{enabled: enabled, undecided: enabled}
}

// feed returns reasoning and visible-text segments which are safe to emit.
// Before the closing tag is observed, a possible prefix is deliberately held
// back so a chunk boundary cannot leak the literal tag to Codex.
func (p *literalThinkingStreamParser) feed(chunk string) (reasoning, visible string) {
	if p == nil || chunk == "" {
		return "", ""
	}
	if !p.enabled || !p.undecided {
		if p != nil && p.normalized {
			return "", chunk
		}
		return "", chunk
	}

	if p.buffer.Len()+len(chunk) > literalThinkingMaxBufferedBytes {
		p.undecided = false
		return "", p.buffer.String() + chunk
	}
	_, _ = p.buffer.WriteString(chunk)
	buffered := p.buffer.String()

	// A partial opening tag remains undecided. Any mismatch can be emitted
	// immediately, preserving the disabled-path streaming behavior.
	if len(buffered) < len(literalThinkingOpenTag) {
		if !strings.HasPrefix(literalThinkingOpenTag, buffered) {
			p.undecided = false
			return "", buffered
		}
		return "", ""
	}
	if !strings.HasPrefix(buffered, literalThinkingOpenTag) {
		p.undecided = false
		return "", buffered
	}

	if r, v, ok := normalizeLiteralThinkingText(buffered); ok {
		p.undecided = false
		p.normalized = true
		p.buffer.Reset()
		return r, v
	}
	// A complete close tag is present but the content was malformed. Do not
	// partially reinterpret it; emit the original bytes as assistant text.
	if strings.Contains(buffered[len(literalThinkingOpenTag):], literalThinkingCloseTag) {
		p.undecided = false
		p.buffer.Reset()
		return "", buffered
	}
	return "", ""
}

// flush returns an undecided prefix verbatim at stream end or before a tool
// call/native reasoning delta. A normalized parser has no pending bytes.
func (p *literalThinkingStreamParser) flush() string {
	if p == nil || !p.undecided || p.buffer.Len() == 0 {
		return ""
	}
	p.undecided = false
	text := p.buffer.String()
	p.buffer.Reset()
	return text
}
