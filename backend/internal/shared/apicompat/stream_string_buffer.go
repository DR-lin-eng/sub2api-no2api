package apicompat

import "strings"

// appendStreamString changes only how an existing cumulative string is stored.
// Callers still publish its complete value after each delta. A caller-provided
// seed or replacement stays authoritative, including an explicit empty string.
// Builders are never copied or pooled: previously published String snapshots
// remain immutable when the next block starts or an external value replaces it.
func appendStreamString(buffer **strings.Builder, current, delta string) string {
	if delta == "" {
		return current
	}
	if *buffer == nil || (*buffer).String() != current {
		*buffer = &strings.Builder{}
		_, _ = (*buffer).WriteString(current)
	}
	_, _ = (*buffer).WriteString(delta)
	return (*buffer).String()
}

// streamStringBytes keeps matching bytes for APIs that require []byte without
// copying the growing public string on every call. The shared snapshot also
// detects sequentially diverging state copies: a mismatch creates a new buffer
// instead of overwriting another state's accumulated prefix.
type streamStringBytes struct {
	data     []byte
	snapshot string
}

func appendStreamStringBytes(buffer **streamStringBytes, previous, current, delta string) []byte {
	if *buffer == nil || (*buffer).snapshot != previous {
		*buffer = &streamStringBytes{data: append([]byte(nil), previous...)}
	}
	(*buffer).data = append((*buffer).data, delta...)
	(*buffer).snapshot = current
	return (*buffer).data
}

// streamJSONMayBeComplete is only a necessary condition for json.Valid. An
// object/array/string cannot be complete before its final delimiter arrives.
// Scalar forms keep the original validator path because their prefixes can
// already be valid JSON. Delimiters inside strings still use json.Valid.
func streamJSONMayBeComplete(value string) bool {
	value = strings.Trim(value, " \t\r\n")
	if len(value) == 0 {
		return false
	}
	switch value[0] {
	case '{':
		return value[len(value)-1] == '}'
	case '[':
		return value[len(value)-1] == ']'
	case '"':
		return len(value) > 1 && value[len(value)-1] == '"'
	default:
		return true
	}
}
