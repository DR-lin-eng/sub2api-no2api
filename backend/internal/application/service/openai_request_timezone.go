package service

import (
	"bytes"
	"context"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unsafe"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/gin-gonic/gin"
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
func (s *OpenAIGatewayService) normalizeOpenAIRequestTimezone(ctx context.Context, c *gin.Context, account *Account, body []byte, _ string) []byte {
	if account == nil || !account.IsOpenAI() || len(body) == 0 {
		return body
	}
	target := s.openAIRequestTimezone()
	if s != nil && s.settingService != nil {
		enabled, configured := s.settingService.GetOpenAIRequestTimezoneSettings(ctx)
		if !enabled {
			return body
		}
		target = configured
	}
	return normalizeOpenAIRequestTimezoneForRequest(c, body, target)
}

func normalizeOpenAIRequestTimezone(body []byte, target string) []byte {
	return normalizeOpenAIRequestTimezoneForRequest(nil, body, target)
}

// openAIRequestTextMayHoldEnvironment reports whether a JSON string value can
// decode to text containing an environment_context block. Only a \u escape can
// spell those letters differently, so everything else is decided on the raw
// bytes without unescaping a large prompt.
func openAIRequestTextMayHoldEnvironment(value gjson.Result) bool {
	return strings.Contains(value.Raw, "environment_context") || strings.Contains(value.Raw, `\u`)
}

// normalizeOpenAIRequestTimezoneForRequest walks read-only views of the
// original body. Rewrites replace one string value each and leave every array
// index in place, so the views and their paths stay valid for the whole walk.
func normalizeOpenAIRequestTimezoneForRequest(c *gin.Context, body []byte, target string) []byte {
	original := body
	// No user text can hold an environment_context block unless the body spells
	// it out or carries a \u escape, so most bodies skip the input walk.
	mayHoldEnvironment := bytes.Contains(body, []byte(`\u`)) || bytes.Contains(body, []byte("environment_context"))
	if !mayHoldEnvironment {
		// Nothing to rewrite in input.
	} else if input := openAIRequestBodyFieldView(c, original, "input"); input.IsArray() {
		inputIndex := -1
		input.ForEach(func(_, item gjson.Result) bool {
			inputIndex++
			if item.Get("role").String() != "user" {
				return true
			}
			content := item.Get("content")
			if content.Type == gjson.String {
				if openAIRequestTextMayHoldEnvironment(content) {
					body = rewriteOpenAIRequestText(body, "input."+strconv.Itoa(inputIndex)+".content", content.String(), target)
				}
				return true
			}
			if !content.IsArray() {
				return true
			}
			contentIndex := -1
			content.ForEach(func(_, part gjson.Result) bool {
				contentIndex++
				if part.Get("type").String() != "input_text" {
					return true
				}
				text := part.Get("text")
				if text.Type != gjson.String || !openAIRequestTextMayHoldEnvironment(text) {
					return true
				}
				path := "input." + strconv.Itoa(inputIndex) + ".content." + strconv.Itoa(contentIndex) + ".text"
				body = rewriteOpenAIRequestText(body, path, text.String(), target)
				return true
			})
			return true
		})
	}

	var tools gjson.Result
	if unsafe.SliceData(body) == unsafe.SliceData(original) {
		tools = openAIRequestBodyFieldView(c, original, "tools")
	} else {
		tools = gjson.GetBytes(body, "tools")
	}
	if !tools.IsArray() {
		return body
	}
	toolIndex := -1
	var timezonePaths []string
	tools.ForEach(func(_, tool gjson.Result) bool {
		toolIndex++
		kind := tool.Get("type").String()
		if kind != "web_search" && !strings.HasPrefix(kind, "web_search_") {
			return true
		}
		value := tool.Get("user_location.timezone")
		if value.Type != gjson.String || value.String() == target {
			return true
		}
		timezonePaths = append(timezonePaths, "tools."+strconv.Itoa(toolIndex)+".user_location.timezone")
		return true
	})
	for _, path := range timezonePaths {
		if updated, err := sjson.SetBytes(body, path, target); err == nil {
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
