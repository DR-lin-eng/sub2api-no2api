package service

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openAIEnvironmentFieldStart matches one XML-like environment field while
// leaving opaque field contents untouched. Codex clients send this structure
// inside user input text, so a full XML parser would be both too strict and
// more destructive than the targeted rewrite needs to be.
var openAIEnvironmentFieldStart = regexp.MustCompile(`^<([A-Za-z_][A-Za-z0-9_.:-]*)(?:[ \t\r\n]+(?:[^<>"']|"[^"]*"|'[^']*')*)?[ \t\r\n]*/?>`)

// findOpenAIEnvironmentFieldEnd finds a matching closing tag for one field.
// Only the field name is interpreted; nested and unescaped content remains
// opaque so client-provided network, filesystem, and subagent text survives.
func findOpenAIEnvironmentFieldEnd(text, name string) (start, end int) {
	prefix := "</" + name
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], prefix)
		if index < 0 {
			break
		}
		start = offset + index
		offset = start + len(prefix)
		rest := strings.TrimLeft(text[offset:], " \t\r\n")
		if strings.HasPrefix(rest, ">") {
			return start, len(text) - len(rest) + 1
		}
	}
	return -1, -1
}

// rewriteOpenAIRequestEnvironment changes only the timezone value in a
// complete environment_context block. Ambiguous or malformed blocks are
// returned byte-for-byte unchanged.
func rewriteOpenAIRequestEnvironment(text, timezone string) (rewritten, previous, reason string) {
	trimmed := strings.TrimSpace(text)
	root := openAIEnvironmentFieldStart.FindStringSubmatchIndex(trimmed)
	if root == nil || trimmed[root[2]:root[3]] != "environment_context" {
		return text, "", "no_environment_context"
	}
	rootCloseStart, rootCloseEnd := findOpenAIEnvironmentFieldEnd(trimmed[root[1]:], "environment_context")
	if strings.HasSuffix(trimmed[:root[1]], "/>") || rootCloseEnd != len(trimmed)-root[1] {
		return text, "", "invalid_environment_context"
	}
	content := trimmed[root[1] : root[1]+rootCloseStart]
	valueStart, valueEnd := -1, -1
	for offset := 0; offset < len(content); {
		rest := strings.TrimLeftFunc(content[offset:], unicode.IsSpace)
		offset = len(content) - len(rest)
		if rest == "" {
			break
		}
		field := openAIEnvironmentFieldStart.FindStringSubmatchIndex(rest)
		if field == nil {
			return text, "", "invalid_environment_context"
		}
		name := rest[field[2]:field[3]]
		if name == "environment_context" {
			return text, "", "invalid_environment_context"
		}
		selfClosing := strings.HasSuffix(rest[:field[1]], "/>")
		if name == "timezone" && (valueStart >= 0 || selfClosing) {
			return text, "", "invalid_environment_context"
		}
		offset += field[1]
		if selfClosing {
			continue
		}
		end, closeEnd := findOpenAIEnvironmentFieldEnd(content[offset:], name)
		if end < 0 {
			return text, "", "invalid_environment_context"
		}
		if name == "timezone" {
			value := content[offset : offset+end]
			previous = strings.TrimSpace(value)
			if previous == "" || strings.Contains(value, "<") {
				return text, "", "invalid_environment_context"
			}
			valueStart = offset + len(value) - len(strings.TrimLeftFunc(value, unicode.IsSpace))
			valueEnd = offset + len(strings.TrimRightFunc(value, unicode.IsSpace))
		}
		offset += closeEnd
	}
	if valueStart < 0 {
		return text, "", "no_timezone"
	}
	if previous == timezone {
		return text, previous, "already_target"
	}
	// Patch only the value, preserving the original tags and whitespace.
	base := len(text) - len(strings.TrimLeftFunc(text, unicode.IsSpace)) + root[1]
	return text[:base+valueStart] + timezone + text[base+valueEnd:], previous, "replaced"
}

// openAIRequestTimezone returns the gateway-wide target. The service fallback
// keeps unit and embedding constructors safe; normal application startup
// supplies the validated config default.
func (s *OpenAIGatewayService) openAIRequestTimezone() string {
	if s != nil && s.cfg != nil {
		if timezone := strings.TrimSpace(s.cfg.Gateway.OpenAIRequestTimezone); timezone != "" {
			return timezone
		}
	}
	return config.DefaultOpenAIRequestTimezone
}

// normalizeOpenAIRequestTimezone applies the gateway-wide, startup-validated
// timezone to OpenAI Responses requests. It is deliberately account-agnostic:
// every OpenAI account behind this gateway receives the same projection.
func (s *OpenAIGatewayService) normalizeOpenAIRequestTimezone(account *Account, body []byte, _ string) []byte {
	if account == nil || !account.IsOpenAI() || len(body) == 0 {
		return body
	}
	return normalizeOpenAIRequestTimezone(body, s.openAIRequestTimezone())
}

func normalizeOpenAIRequestTimezone(body []byte, target string) []byte {
	input := gjson.GetBytes(body, "input")
	if input.IsArray() {
		for inputIndex, item := range input.Array() {
			if item.Get("role").String() != "user" {
				continue
			}
			content := item.Get("content")
			path := "input." + strconv.Itoa(inputIndex) + ".content"
			if content.Type == gjson.String {
				body = rewriteOpenAIRequestText(body, path, content.String(), target)
				continue
			}
			if !content.IsArray() {
				continue
			}
			for contentIndex, part := range content.Array() {
				if part.Get("type").String() != "input_text" || part.Get("text").Type != gjson.String {
					continue
				}
				body = rewriteOpenAIRequestText(body, path+"."+strconv.Itoa(contentIndex)+".text", part.Get("text").String(), target)
			}
		}
	}

	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}
	for index, tool := range tools.Array() {
		kind := tool.Get("type").String()
		if kind != "web_search" && !strings.HasPrefix(kind, "web_search_") {
			continue
		}
		value := tool.Get("user_location.timezone")
		if value.Type != gjson.String || value.String() == target {
			continue
		}
		if updated, err := sjson.SetBytes(body, "tools."+strconv.Itoa(index)+".user_location.timezone", target); err == nil {
			body = updated
		}
	}
	return body
}

func rewriteOpenAIRequestText(body []byte, path, text, target string) []byte {
	next, _, reason := rewriteOpenAIRequestEnvironment(text, target)
	if reason != "replaced" {
		return body
	}
	updated, err := sjson.SetBytes(body, path, next)
	if err != nil {
		return body
	}
	return updated
}
