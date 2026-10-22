package apicompat

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// responsesCustomInputStream scans the growing function argument buffer once.
// Only the root input string is streamed; incomplete escapes and UTF-8 runes
// stay in the argument buffer until the next chunk completes them.
type responsesCustomInputStream struct {
	pos       int
	depth     int
	keyStart  int
	inString  bool
	escaped   bool
	wantKey   bool
	inputKey  bool
	wantValue bool
	inInput   bool
	finished  bool
	emitted   strings.Builder
}

func (s *responsesCustomInputStream) delta(arguments string) string {
	if s.finished {
		return ""
	}
	for !s.inInput && s.pos < len(arguments) {
		c := arguments[s.pos]
		s.pos++
		if s.depth == 0 && c != '{' && c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			// Preserve the existing raw-input fallback when an upstream ignores
			// the wrapper schema, even if its text contains a JSON object.
			s.finished = true
			return ""
		}
		if s.inString {
			if s.escaped {
				s.escaped = false
			} else if c == '\\' {
				s.escaped = true
			} else if c == '"' {
				s.inString = false
				if s.depth == 1 && s.wantKey {
					var key string
					if json.Unmarshal([]byte(arguments[s.keyStart:s.pos]), &key) != nil {
						s.finished = true
						return ""
					}
					s.inputKey, s.wantKey = key == "input", false
				}
			}
			continue
		}
		switch c {
		case '{', '[':
			s.depth++
			if s.depth == 1 {
				if c != '{' {
					s.finished = true
					return ""
				}
				s.wantKey = true
			}
		case '}', ']':
			s.depth--
			if s.depth <= 0 {
				s.finished = true
				return ""
			}
		case ',':
			if s.depth == 1 {
				s.wantKey, s.wantValue = true, false
			}
		case ':':
			if s.depth == 1 {
				s.wantValue = true
			}
		case '"':
			if s.depth == 1 && s.wantValue && s.inputKey {
				s.inInput = true
			} else {
				s.inString, s.keyStart = true, s.pos-1
			}
		}
	}
	if !s.inInput {
		return ""
	}
	start := s.pos
	for s.pos < len(arguments) {
		c := arguments[s.pos]
		if c == '"' {
			s.finished = true
			break
		}
		if c == '\\' {
			n := completeCustomInputEscape(arguments[s.pos:])
			if n == 0 {
				break
			}
			s.pos += n
			continue
		}
		if c >= utf8.RuneSelf {
			if !utf8.FullRuneInString(arguments[s.pos:]) {
				break
			}
			_, n := utf8.DecodeRuneInString(arguments[s.pos:])
			s.pos += n
		} else {
			s.pos++
		}
	}
	if start == s.pos {
		return ""
	}
	var delta string
	if json.Unmarshal([]byte(`"`+arguments[start:s.pos]+`"`), &delta) != nil {
		s.finished = true
		return ""
	}
	_, _ = s.emitted.WriteString(delta)
	return delta
}

func completeCustomInputEscape(value string) int {
	if len(value) < 2 {
		return 0
	}
	if value[1] != 'u' {
		return 2
	}
	if len(value) < 6 {
		return 0
	}
	r, err := strconv.ParseUint(value[2:6], 16, 16)
	if err != nil || !utf16.IsSurrogate(rune(r)) || r >= 0xdc00 {
		return 6
	}
	// Keep a possible surrogate pair in the same json.Unmarshal operation.
	if len(value) == 6 || (len(value) == 7 && value[6] == '\\') {
		return 0
	}
	if strings.HasPrefix(value[6:], `\u`) {
		if len(value) < 12 {
			return 0
		}
		low, err := strconv.ParseUint(value[8:12], 16, 16)
		if err == nil && low >= 0xdc00 && low <= 0xdfff {
			return 12
		}
	}
	return 6
}

// finish emits only the suffix absent from earlier deltas. The authoritative
// done payload also covers upstreams that omit deltas or ignore the schema.
func (s *responsesCustomInputStream) finish(input string) string {
	prefix := s.emitted.String()
	if !strings.HasPrefix(input, prefix) {
		return ""
	}
	suffix := input[len(prefix):]
	_, _ = s.emitted.WriteString(suffix)
	return suffix
}

func chatCustomInputStream(state *ChatCompletionsToResponsesStreamState, index int) *responsesCustomInputStream {
	if state.customInputStreams == nil {
		state.customInputStreams = make(map[int]*responsesCustomInputStream)
	}
	if state.customInputStreams[index] == nil {
		state.customInputStreams[index] = &responsesCustomInputStream{}
	}
	return state.customInputStreams[index]
}

func appendChatCustomInputDelta(state *ChatCompletionsToResponsesStreamState, index int, call *ChatToolCall) []ResponsesStreamEvent {
	delta := chatCustomInputStream(state, index).delta(call.Function.Arguments)
	if delta == "" {
		return nil
	}
	return []ResponsesStreamEvent{chatToResponsesEvent(state, "response.custom_tool_call_input.delta", &ResponsesStreamEvent{
		OutputIndex: state.ToolOutputIndex[index],
		ItemID:      state.ToolItemIDs[index],
		Delta:       delta,
	})}
}
