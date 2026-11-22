package service

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

const openAIResponsesOutputTextDeltaEventType = "response.output_text.delta"

// openAIResponsesPlainOutputTextDelta recognizes the Responses text delta that
// makes up almost all of a stream: an object holding only the documented
// scalar members of response.output_text.delta, each at most once. Such an
// event has no response, item, error, usage or model member, so the per-event
// normalizers, restorers and accounting observers have nothing to act on and
// the caller can skip them. Anything else, however slightly different, is not
// plain and takes the full path. data must already be known to be valid JSON.
func openAIResponsesPlainOutputTextDelta(data string) (delta string, ok bool) {
	const prefix = `{"type":"` + openAIResponsesOutputTextDeltaEventType + `"`
	if len(data) < len(prefix)+1 || data[len(data)-1] != '}' || !strings.HasPrefix(data, prefix) {
		return "", false
	}
	const (
		typeMember = 1 << iota
		sequenceNumberMember
		itemIDMember
		outputIndexMember
		contentIndexMember
		deltaMember
		logprobsMember
		obfuscationMember
	)
	var seen uint
	var deltaValue gjson.Result
	plain := true
	gjson.Parse(data).ForEach(func(key, value gjson.Result) bool {
		var member uint
		valid := false
		if len(key.Raw) == len(key.Str)+2 {
			switch key.Str {
			case "type":
				member, valid = typeMember, value.Raw == `"`+openAIResponsesOutputTextDeltaEventType+`"`
			case "sequence_number":
				member, valid = sequenceNumberMember, isSmallJSONNonNegativeInteger(value.Raw)
			case "output_index":
				member, valid = outputIndexMember, isSmallJSONNonNegativeInteger(value.Raw)
			case "content_index":
				member, valid = contentIndexMember, isSmallJSONNonNegativeInteger(value.Raw)
			case "item_id":
				member, valid = itemIDMember, value.Type == gjson.String
			case "obfuscation":
				member, valid = obfuscationMember, value.Type == gjson.String
			case "delta":
				member, valid = deltaMember, value.Type == gjson.String
				deltaValue = value
			case "logprobs":
				member, valid = logprobsMember, value.Raw == "[]" || value.Raw == "null"
			}
		}
		if !valid || seen&member != 0 {
			plain = false
			return false
		}
		seen |= member
		return true
	})
	if !plain || seen&typeMember == 0 {
		return "", false
	}
	if seen&deltaMember == 0 {
		return "", true
	}
	raw := deltaValue.Raw
	if strings.IndexByte(raw, '\\') < 0 && utf8.ValidString(raw) {
		return raw[1 : len(raw)-1], true
	}
	// Decode exactly as encoding/json does for ResponsesStreamEvent.Delta.
	if err := json.Unmarshal([]byte(raw), &delta); err != nil {
		return "", false
	}
	return delta, true
}

func isSmallJSONNonNegativeInteger(raw string) bool {
	if len(raw) == 0 || len(raw) > 9 || (len(raw) > 1 && raw[0] == '0') {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return false
		}
	}
	return true
}
