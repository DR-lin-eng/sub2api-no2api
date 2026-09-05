package apicompat

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	CodexBootstrapDelegation = "delegation"
	CodexBootstrapAutomation = "automation"
)

// NormalizeCodexCallOutputBootstrap converts Codex Desktop's synthetic,
// call-less bootstrap output into ordinary user input. Normal tool outputs are
// left untouched so their call_id pairing remains mandatory.
func NormalizeCodexCallOutputBootstrap(body []byte) ([]byte, string, bool) {
	if !bytes.Contains(body, []byte(`"function_call_output"`)) ||
		(!bytes.Contains(body, []byte(`"create_thread"`)) &&
			!bytes.Contains(body, []byte(`"send_message_to_thread"`)) &&
			!bytes.Contains(body, []byte(`"automation_update"`))) {
		return body, "", false
	}
	if !hasUniqueJSONMembers(body) {
		return body, "", false
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var request map[string]any
	if err := decoder.Decode(&request); err != nil {
		return body, "", false
	}
	input, ok := request["input"].([]any)
	if !ok {
		return body, "", false
	}

	bootstrapKind := ""
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind := codexCallOutputBootstrapKind(item)
		if kind != "" && bootstrapKind != "" && kind != bootstrapKind {
			return body, "", false
		}
		if kind != "" {
			bootstrapKind = kind
		}
	}
	if bootstrapKind == "" {
		return body, "", false
	}
	allowHistoricalContext := bootstrapKind == CodexBootstrapDelegation
	if previousResponseID, exists := request["previous_response_id"]; exists {
		value, ok := previousResponseID.(string)
		if !ok || (!allowHistoricalContext && strings.TrimSpace(value) != "") {
			return body, "", false
		}
	}

	// Delegation can wake an existing task and coexist with unambiguous history.
	// Automation remains bootstrap-only. Missing identifiers always leave the
	// synthetic output ambiguous, so retain ordinary validation in that case.
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if codexCallOutputBootstrapKind(item) == bootstrapKind {
			callIDValue, exists := item["call_id"]
			callID, isString := callIDValue.(string)
			if exists && (!isString || strings.TrimSpace(callID) != "") {
				return body, "", false
			}
			continue
		}
		typ := codexBootstrapStringField(item, "type")
		if typ == "item_reference" {
			if allowHistoricalContext && strings.TrimSpace(codexBootstrapStringField(item, "id")) != "" {
				continue
			}
			return body, "", false
		}
		if strings.HasSuffix(typ, "_call") || isResponsesCallOutputType(typ) {
			if allowHistoricalContext && strings.TrimSpace(codexBootstrapStringField(item, "call_id")) != "" {
				continue
			}
			return body, "", false
		}
	}

	changed := false
	for i, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok || codexCallOutputBootstrapKind(item) != bootstrapKind {
			continue
		}
		output, ok := item["output"].(string)
		if !ok {
			return body, "", false
		}
		input[i] = map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{map[string]any{
				"type": "input_text",
				"text": output,
			}},
		}
		changed = true
	}
	if !changed {
		return body, "", false
	}
	normalized, err := json.Marshal(request)
	if err != nil {
		return body, "", false
	}
	return normalized, bootstrapKind, true
}

func codexCallOutputBootstrapKind(item map[string]any) string {
	if codexBootstrapStringField(item, "type") != "function_call_output" {
		return ""
	}
	output, ok := item["output"].(string)
	if !ok {
		return ""
	}
	namespace := codexBootstrapStringField(item, "namespace")
	name := codexBootstrapStringField(item, "name")
	if (namespace == "codex_app" || namespace == "codex_tui") &&
		(name == "create_thread" || name == "send_message_to_thread") &&
		validCodexDelegationEnvelope(output) {
		return CodexBootstrapDelegation
	}
	if namespace == "codex_app" && name == "automation_update" && (validCodexAutomationBootstrap(output) || validCodexAutomationHeartbeat(output)) {
		return CodexBootstrapAutomation
	}
	return ""
}

func codexBootstrapStringField(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func isResponsesCallOutputType(typ string) bool {
	return strings.HasSuffix(typ, "_call_output") || typ == "tool_search_output"
}

func validCodexDelegationEnvelope(value string) bool {
	decoder := xml.NewDecoder(strings.NewReader(value))
	var rootSeen, sourceSeen, inputSeen bool
	var childName string
	var childText bytes.Buffer
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return rootSeen && depth == 0 && sourceSeen && inputSeen
		}
		if err != nil {
			return false
		}
		switch current := token.(type) {
		case xml.StartElement:
			depth++
			if current.Name.Space != "" || len(current.Attr) != 0 ||
				(depth == 1 && current.Name.Local != "codex_delegation") || depth > 2 {
				return false
			}
			if depth == 1 {
				if rootSeen {
					return false
				}
				rootSeen = true
				continue
			}
			if current.Name.Local != "source_thread_id" && current.Name.Local != "input" {
				return false
			}
			childName = current.Name.Local
			childText.Reset()
		case xml.EndElement:
			if current.Name.Space != "" {
				return false
			}
			if depth == 2 {
				if current.Name.Local != childName || strings.TrimSpace(childText.String()) == "" {
					return false
				}
				if childName == "source_thread_id" {
					if sourceSeen {
						return false
					}
					sourceSeen = true
				} else {
					if inputSeen {
						return false
					}
					inputSeen = true
				}
				childName = ""
			}
			depth--
			if depth < 0 {
				return false
			}
		case xml.CharData:
			if depth == 2 {
				_, _ = childText.Write(current)
			} else if len(bytes.TrimSpace(current)) != 0 {
				return false
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			return false
		}
	}
}

func validCodexAutomationBootstrap(value string) bool {
	normalized := strings.ReplaceAll(value, "\r\n", "\n")
	if strings.ContainsRune(normalized, '\r') {
		return false
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) < 6 {
		return false
	}
	if _, ok := codexAutomationHeaderValue(lines[0], "Automation: "); !ok {
		return false
	}
	automationID, ok := codexAutomationHeaderValue(lines[1], "Automation ID: ")
	if !ok || !validCodexAutomationID(automationID) {
		return false
	}
	if lines[2] != "Automation memory: $CODEX_HOME/automations/"+automationID+"/memory.md" {
		return false
	}
	lastRun, ok := codexAutomationHeaderValue(lines[3], "Last run: ")
	if !ok || !validCodexAutomationLastRun(lastRun) || lines[4] != "" {
		return false
	}
	return strings.TrimSpace(strings.Join(lines[5:], "\n")) != ""
}

func codexAutomationHeaderValue(line, prefix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	value := strings.TrimPrefix(line, prefix)
	return value, value != "" && strings.TrimSpace(value) == value
}

func validCodexAutomationID(value string) bool {
	if len(value) == 0 || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			continue
		}
		return false
	}
	return true
}

func validCodexAutomationLastRun(value string) bool {
	if value == "never" {
		return true
	}
	separator := strings.LastIndex(value, " (")
	if separator <= 0 || !strings.HasSuffix(value, ")") {
		return false
	}
	runAt, err := time.Parse(time.RFC3339Nano, value[:separator])
	if err != nil {
		return false
	}
	epochMillis, err := strconv.ParseInt(value[separator+2:len(value)-1], 10, 64)
	return err == nil && runAt.UnixMilli() == epochMillis
}

func hasUniqueJSONMembers(body []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if !consumeUniqueJSONValue(decoder) {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}

func consumeUniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}

	switch delim {
	case '{':
		members := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return false
			}
			key, ok := keyToken.(string)
			if !ok {
				return false
			}
			if _, duplicate := members[key]; duplicate {
				return false
			}
			members[key] = struct{}{}
			if !consumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !consumeUniqueJSONValue(decoder) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

func validCodexAutomationHeartbeat(value string) bool {
	decoder := xml.NewDecoder(strings.NewReader(value))
	var rootSeen, automationIDSeen bool
	var automationID bytes.Buffer
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			id := automationID.String()
			return rootSeen && automationIDSeen && depth == 0 &&
				strings.TrimSpace(id) == id && validCodexAutomationID(id)
		}
		if err != nil {
			return false
		}
		switch current := token.(type) {
		case xml.StartElement:
			depth++
			if current.Name.Space != "" || len(current.Attr) != 0 || depth > 2 {
				return false
			}
			if depth == 1 {
				if rootSeen || current.Name.Local != "heartbeat" {
					return false
				}
				rootSeen = true
			} else if automationIDSeen || current.Name.Local != "automation_id" {
				return false
			}
			automationIDSeen = depth == 2
		case xml.EndElement:
			if current.Name.Space != "" {
				return false
			}
			depth--
			if depth < 0 {
				return false
			}
		case xml.CharData:
			if depth == 2 {
				_, _ = automationID.Write(current)
			} else if len(bytes.TrimSpace(current)) != 0 {
				return false
			}
		case xml.Comment, xml.ProcInst, xml.Directive:
			return false
		}
	}
}
