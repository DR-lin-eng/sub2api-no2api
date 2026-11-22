package service

import (
	"bytes"
	"strings"
	"sync"
	"unsafe"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIRequestBodyIndexContextKey          = "openai_request_body_index"
	openAICodexBootstrapCheckedBodyContextKey = "openai_codex_bootstrap_checked_body"
)

// openAIRequestBodyIndex records where each top-level member of one request
// body starts. gjson.GetBytes(body, "field") has to skip every preceding
// member, so a field that follows a large input array costs a full body scan
// on every lookup; the index pays for that scan once per body.
//
// The index is read-only and tied to the exact byte slice it was built from.
// Request bodies are never modified in place: every rewrite produces a new
// slice, which no longer matches and gets its own index.
type openAIRequestBodyIndex struct {
	body   []byte
	object bool
	fields []openAIRequestBodyIndexField

	inputItemTypesOnce sync.Once
	inputItemTypes     []string
}

type openAIRequestBodyIndexField struct {
	key   string
	value gjson.Result
}

func newOpenAIRequestBodyIndex(body []byte) *openAIRequestBodyIndex {
	index := &openAIRequestBodyIndex{body: body}
	// Match gjson.GetBytes by starting at the first root container.
	rootOffset := -1
	for i := 0; i < len(body) && rootOffset < 0; i++ {
		switch body[i] {
		case '{':
			rootOffset = i
		case '[':
			return index
		}
	}
	if rootOffset < 0 {
		return index
	}
	index.object = true
	root := body[rootOffset:]
	nextKeyOffset := 1
	parseRawJSONView(root).ForEach(func(key, value gjson.Result) bool {
		if key.Index < nextKeyOffset || key.Index > len(root) {
			return false
		}
		// Result.ForEach can continue after the root '}' on malformed input.
		// The separator range excludes braces inside the preceding parsed value.
		if bytes.IndexByte(root[nextKeyOffset:key.Index], '}') >= 0 {
			return false
		}
		nextKeyOffset = value.Index + len(value.Raw)
		value.Index += rootOffset
		index.fields = append(index.fields, openAIRequestBodyIndexField{key: key.Str, value: value})
		return true
	})
	return index
}

func (x *openAIRequestBodyIndex) covers(body []byte) bool {
	return x != nil && len(x.body) == len(body) && unsafe.SliceData(x.body) == unsafe.SliceData(body)
}

// Lookup resolves a gjson path whose first segment is a plain top-level key.
// The result is a zero-copy view of the body: use it for checks that finish
// before the caller returns, and Get for values that are stored.
func (x *openAIRequestBodyIndex) Lookup(path string) gjson.Result {
	if x == nil || len(x.body) == 0 {
		return gjson.Result{}
	}
	head, rest, nested := strings.Cut(path, ".")
	if !x.object || head == "" || strings.ContainsAny(head, `*?#|@\!{}[]()`) {
		return gjson.Get(unsafe.String(unsafe.SliceData(x.body), len(x.body)), path)
	}
	for i := range x.fields {
		if x.fields[i].key != head {
			continue
		}
		value := x.fields[i].value
		if !nested {
			return value
		}
		// gjson keeps scanning later duplicate keys when a nested path misses.
		if value.Type != gjson.JSON {
			continue
		}
		if found := value.Get(rest); found.Exists() {
			return found
		}
	}
	return gjson.Result{}
}

// Get is Lookup with the result detached from the body, like gjson.GetBytes.
func (x *openAIRequestBodyIndex) Get(path string) gjson.Result {
	value := x.Lookup(path)
	if value.Type == gjson.String && len(value.Str) > 0 {
		value.Str = strings.Clone(value.Str)
	}
	value.Raw = strings.Clone(value.Raw)
	return value
}

// InputItemTypes lists the distinct "type" values of the input array items,
// as item.Get("type").String() reports them. It is computed once per body.
func (x *openAIRequestBodyIndex) InputItemTypes() []string {
	if x == nil {
		return nil
	}
	x.inputItemTypesOnce.Do(func() {
		input := x.Lookup("input")
		if !input.IsArray() {
			return
		}
		input.ForEach(func(_, item gjson.Result) bool {
			itemType := item.Get("type").String()
			for _, seen := range x.inputItemTypes {
				if seen == itemType {
					return true
				}
			}
			x.inputItemTypes = append(x.inputItemTypes, strings.Clone(itemType))
			return true
		})
	})
	return x.inputItemTypes
}

// openAIRequestBodyIndexFor returns the index of body, reusing the one cached
// on the request context when it was built from the same slice. Only the most
// recent body is kept, so at most one superseded body stays reachable.
func openAIRequestBodyIndexFor(c *gin.Context, body []byte) *openAIRequestBodyIndex {
	if c == nil {
		return newOpenAIRequestBodyIndex(body)
	}
	if cached, ok := c.Get(openAIRequestBodyIndexContextKey); ok {
		if index, ok := cached.(*openAIRequestBodyIndex); ok && index.covers(body) {
			return index
		}
	}
	index := newOpenAIRequestBodyIndex(body)
	c.Set(openAIRequestBodyIndexContextKey, index)
	return index
}

// OpenAIRequestBodyField is gjson.GetBytes(body, path) for a path that starts
// with a top-level key, served from the per-request body index. Without a
// request context there is nowhere to keep the index, so it reads body directly.
func OpenAIRequestBodyField(c *gin.Context, body []byte, path string) gjson.Result {
	if c == nil {
		return gjson.GetBytes(body, path)
	}
	return openAIRequestBodyIndexFor(c, body).Get(path)
}

// openAIRequestBodyFieldView is OpenAIRequestBodyField without the copy. The
// result aliases body and must not outlive the caller's use of it.
func openAIRequestBodyFieldView(c *gin.Context, body []byte, path string) gjson.Result {
	if c == nil {
		return gjson.Get(unsafe.String(unsafe.SliceData(body), len(body)), path)
	}
	return openAIRequestBodyIndexFor(c, body).Lookup(path)
}

// openAIRequestBodyInputItemTypeMatches is a pre-check for scans over
// gjson.GetBytes(body, "input"): false means match rejects the "type" of every
// item. Without a request context it returns true and the caller scans.
func openAIRequestBodyInputItemTypeMatches(c *gin.Context, body []byte, match func(string) bool) bool {
	if c == nil {
		return true
	}
	for _, itemType := range openAIRequestBodyIndexFor(c, body).InputItemTypes() {
		if match(itemType) {
			return true
		}
	}
	return false
}

// OpenAIRequestBodyMayHaveInputItemType is a cheap pre-check for scans that
// look for one input item type. False means no input item has that exact
// type, whether the caller reads input with gjson.GetBytes, a parsed view or
// an "input.#(type==...)" query. True means the caller must run its own scan:
// bodies whose shape makes those readers disagree are never ruled out.
func OpenAIRequestBodyMayHaveInputItemType(c *gin.Context, body []byte, itemType string) bool {
	if c == nil {
		return true
	}
	index := openAIRequestBodyIndexFor(c, body)
	if !index.singleInputObject() {
		return true
	}
	for _, seen := range index.InputItemTypes() {
		if seen == itemType {
			return true
		}
	}
	return false
}

// singleInputObject reports whether body is an object that starts at its
// first non-space byte and holds at most one top-level "input" member.
func (x *openAIRequestBodyIndex) singleInputObject() bool {
	if x == nil || !x.object {
		return false
	}
	for _, b := range x.body {
		if b == '{' {
			break
		}
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	inputs := 0
	for i := range x.fields {
		if x.fields[i].key == "input" {
			inputs++
		}
	}
	return inputs <= 1
}

// HasTopLevelKey reports whether any top-level member name satisfies match.
func (x *openAIRequestBodyIndex) HasTopLevelKey(match func(string) bool) bool {
	if x == nil {
		return false
	}
	for i := range x.fields {
		if match(x.fields[i].key) {
			return true
		}
	}
	return false
}

func sameOpenAIRequestBody(a, b []byte) bool {
	return len(a) == len(b) && unsafe.SliceData(a) == unsafe.SliceData(b)
}

// NormalizeCodexCallOutputBootstrapForRequest is
// apicompat.NormalizeCodexCallOutputBootstrap with the "nothing to normalize"
// answer remembered for the exact body slice, so the handler and Forward do
// not both search an unchanged body.
func NormalizeCodexCallOutputBootstrapForRequest(c *gin.Context, body []byte) ([]byte, string, bool) {
	if c != nil {
		if cached, ok := c.Get(openAICodexBootstrapCheckedBodyContextKey); ok {
			if checked, ok := cached.([]byte); ok && sameOpenAIRequestBody(checked, body) {
				return body, "", false
			}
		}
	}
	normalized, kind, changed := apicompat.NormalizeCodexCallOutputBootstrap(body)
	if !changed && c != nil {
		c.Set(openAICodexBootstrapCheckedBodyContextKey, body)
	}
	return normalized, kind, changed
}
