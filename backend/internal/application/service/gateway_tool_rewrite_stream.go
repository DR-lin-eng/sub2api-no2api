package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"unsafe"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/tidwall/gjson"
)

// toolNameStreamRestorer restores rewritten Claude tool names in SSE streams,
// including fake names split across adjacent delta chunks.
//
// Performance design over naive per-delta scanning:
//  1. Pre-flattened `fakes` slice plus 256-bit first/second-byte bitmaps
//     (`firstByteMask`, `secondByteMask`) reject impossible suffix holdbacks in
//     register operations without Go map iteration.
//  2. `requireUnderscore` detects when every active fake tool name contains '_'
//     (true for all static `cc_ses*_` and dynamic `<verb>_...` rewrites), allowing
//     a single SIMD `bytes.IndexByte(slice, '_')` pre-screen to skip all N tool
//     comparisons on ordinary prose tokens with zero heap allocations.
//  3. `gjson` zero-alloc field inspection in `RestoreAnthropicSSEBlock` avoids
//     per-event `encoding/json.Unmarshal` allocations on unchanged prose deltas.
type toolNameStreamRestorer struct {
	rw                *ToolNameRewrite
	fakes             []string
	maxHold           int
	firstByteMask     [4]uint64
	secondByteMask    [4]uint64
	requireUnderscore bool
	hasRules          bool

	pending   []byte
	scratch   []byte
	outputBuf []byte
	singleOut [1]string

	anthropicIndex int
	anthropicField string
	ccCallKey      int
	responsesKey   string
}

func newToolNameStreamRestorer(c interface {
	Get(string) (any, bool)
}) *toolNameStreamRestorer {
	rw := toolNameRewriteFromContext(c)
	capHint := len(staticToolNameRewrites)
	if rw != nil {
		capHint += len(rw.ReverseOrdered)
	}
	r := &toolNameStreamRestorer{
		rw:                rw,
		fakes:             make([]string, 0, capHint),
		requireUnderscore: true,
		anthropicIndex:    -1,
		ccCallKey:         -1,
	}
	addFake := func(fake, real string) {
		if fake == "" || fake == real {
			return
		}
		r.hasRules = true
		r.fakes = append(r.fakes, fake)
		if hold := len(fake) - 1; hold > r.maxHold {
			r.maxHold = hold
		}
		lead := fake[0]
		r.firstByteMask[lead>>6] |= uint64(1) << (lead & 63)
		if len(fake) > 1 {
			sec := fake[1]
			r.secondByteMask[sec>>6] |= uint64(1) << (sec & 63)
		}
		if strings.IndexByte(fake, '_') < 0 {
			r.requireUnderscore = false
		}
	}
	if rw != nil {
		for _, pair := range rw.ReverseOrdered {
			addFake(pair[0], pair[1])
		}
	}
	for realPrefix, fakePrefix := range staticToolNameRewrites {
		addFake(fakePrefix, realPrefix)
	}
	if !r.hasRules {
		r.requireUnderscore = false
	}
	return r
}

func (r *toolNameStreamRestorer) singleSlice(s string) []string {
	r.singleOut[0] = s
	return r.singleOut[:]
}

func (r *toolNameStreamRestorer) hasLeadByte(b byte) bool {
	return (r.firstByteMask[b>>6] & (uint64(1) << (b & 63))) != 0
}

func (r *toolNameStreamRestorer) hasSecondByte(b byte) bool {
	return (r.secondByteMask[b>>6] & (uint64(1) << (b & 63))) != 0
}

func (r *toolNameStreamRestorer) mayContainCompleteFake(data []byte) bool {
	if !r.hasRules || len(data) == 0 {
		return false
	}
	if r.requireUnderscore {
		return bytes.IndexByte(data, '_') >= 0
	}
	for _, b := range data {
		if r.hasLeadByte(b) {
			return true
		}
	}
	return false
}

func (r *toolNameStreamRestorer) restoreCompleteString(s string) string {
	if s == "" || !r.hasRules {
		return s
	}
	raw := unsafe.Slice(unsafe.StringData(s), len(s))
	if !r.mayContainCompleteFake(raw) {
		return s
	}
	if r.rw != nil {
		for _, pair := range r.rw.ReverseOrdered {
			fake, real := pair[0], pair[1]
			if fake == "" || fake == real || !strings.Contains(s, fake) {
				continue
			}
			s = strings.ReplaceAll(s, fake, real)
		}
	}
	for prefix, replacement := range staticToolNameRewrites {
		if strings.Contains(s, replacement) {
			s = strings.ReplaceAll(s, replacement, prefix)
		}
	}
	return s
}

func (r *toolNameStreamRestorer) restoreInto(dst, chunk []byte) []byte {
	if !r.hasRules || (len(r.pending) == 0 && len(chunk) == 0) {
		return dst
	}

	var work []byte
	if len(r.pending) == 0 {
		work = chunk
	} else {
		r.scratch = append(r.scratch[:0], r.pending...)
		r.scratch = append(r.scratch, chunk...)
		work = r.scratch
		r.pending = r.pending[:0]
	}

	hold := r.trailingPrefixHoldback(work)
	safe := len(work) - hold
	if hold > 0 {
		r.pending = append(r.pending[:0], work[safe:]...)
	}
	if safe <= 0 {
		return dst
	}
	safeSlice := work[:safe]
	if r.mayContainCompleteFake(safeSlice) {
		safeSlice = restoreToolNamesInBytes(safeSlice, r.rw)
	}
	return append(dst, safeSlice...)
}

func (r *toolNameStreamRestorer) flushInto(dst []byte) []byte {
	if len(r.pending) == 0 {
		return dst
	}
	rest := r.pending
	r.pending = r.pending[:0]
	if r.mayContainCompleteFake(rest) {
		rest = restoreToolNamesInBytes(rest, r.rw)
	}
	return append(dst, rest...)
}

func (r *toolNameStreamRestorer) restoreStringFragment(fragment string) string {
	if !r.hasRules || (len(r.pending) == 0 && fragment == "") {
		return fragment
	}
	raw := unsafe.Slice(unsafe.StringData(fragment), len(fragment))
	if len(r.pending) == 0 && r.trailingPrefixHoldback(raw) == 0 && !r.mayContainCompleteFake(raw) {
		return fragment
	}
	r.outputBuf = r.restoreInto(r.outputBuf[:0], raw)
	if len(r.outputBuf) == 0 {
		return ""
	}
	return string(r.outputBuf)
}

func (r *toolNameStreamRestorer) flushPendingString() string {
	r.outputBuf = r.flushInto(r.outputBuf[:0])
	if len(r.outputBuf) == 0 {
		return ""
	}
	return string(r.outputBuf)
}

func (r *toolNameStreamRestorer) trailingPrefixHoldback(data []byte) int {
	if len(data) == 0 || r.maxHold <= 0 {
		return 0
	}
	start := len(data) - r.maxHold
	if start < 0 {
		start = 0
	}
	for idx := start; idx < len(data); idx++ {
		if !r.hasLeadByte(data[idx]) {
			continue
		}
		if idx+1 < len(data) && !r.hasSecondByte(data[idx+1]) {
			continue
		}
		suffix := data[idx:]
		if r.isProperPrefixOfAnyFake(suffix) {
			return len(suffix)
		}
	}
	return 0
}

func (r *toolNameStreamRestorer) isProperPrefixOfAnyFake(candidate []byte) bool {
	n := len(candidate)
	if n == 0 || n > r.maxHold {
		return false
	}
	c0 := candidate[0]
	for _, fake := range r.fakes {
		if len(fake) > n && fake[0] == c0 && bytes.Equal(unsafe.Slice(unsafe.StringData(fake), n), candidate) {
			return true
		}
	}
	return false
}

// RestoreAnthropicSSEBlock restores rewritten tool names in one Anthropic SSE
// event block (`event: ...\ndata: ...\n\n`), buffering trailing partial fake
// prefixes across `text_delta` and `input_json_delta` chunks on the same content
// block and flushing any remainder before block/message boundaries.
func (r *toolNameStreamRestorer) RestoreAnthropicSSEBlock(block string) []string {
	if r == nil || block == "" {
		return nil
	}
	if !r.hasRules {
		return r.singleSlice(block)
	}

	eventName, dataLine, hasData := parseSingleSSEBlock(block)
	if !hasData || dataLine == "" || dataLine == "[DONE]" {
		if len(r.pending) > 0 && (dataLine == "[DONE]" || eventName == "message_stop" || eventName == "error") {
			flushed := r.FlushPending()
			return append(flushed, block)
		}
		return r.singleSlice(block)
	}

	isDeltaCandidate := strings.Contains(dataLine, `"text_delta"`) || strings.Contains(dataLine, `"input_json_delta"`)
	if !isDeltaCandidate {
		var prefix []string
		if len(r.pending) > 0 && isAnthropicBoundaryEvent(eventName, dataLine) {
			prefix = r.FlushPending()
		}
		restoredBlock := r.restoreCompleteString(block)
		if len(prefix) == 0 {
			return r.singleSlice(restoredBlock)
		}
		return append(prefix, restoredBlock)
	}

	if gjson.Get(dataLine, "type").Str != "content_block_delta" {
		return r.singleSlice(r.restoreCompleteString(block))
	}
	idxRes := gjson.Get(dataLine, "index")
	if idxRes.Type != gjson.Number {
		return r.singleSlice(r.restoreCompleteString(block))
	}
	blockIndex := int(idxRes.Int())
	deltaType := gjson.Get(dataLine, "delta.type").Str

	var field string
	var valRes gjson.Result
	switch deltaType {
	case "text_delta":
		valRes = gjson.Get(dataLine, "delta.text")
		if valRes.Type != gjson.String {
			return r.singleSlice(r.restoreCompleteString(block))
		}
		field = "text"
	case "input_json_delta":
		valRes = gjson.Get(dataLine, "delta.partial_json")
		if valRes.Type != gjson.String {
			return r.singleSlice(r.restoreCompleteString(block))
		}
		field = "partial_json"
	default:
		return r.singleSlice(r.restoreCompleteString(block))
	}
	value := valRes.Str

	var out []string
	if len(r.pending) > 0 && (blockIndex != r.anthropicIndex || field != r.anthropicField) {
		out = append(out, r.FlushPending()...)
	}
	r.anthropicIndex = blockIndex
	r.anthropicField = field

	if value == "" && len(r.pending) == 0 {
		if len(out) == 0 {
			return r.singleSlice(block)
		}
		return append(out, block)
	}

	hadPending := len(r.pending) > 0
	restoredValue := r.restoreStringFragment(value)
	if restoredValue == "" && len(r.pending) > 0 {
		// Entire fragment held back awaiting next delta.
		return out
	}
	if !hadPending && len(r.pending) == 0 && restoredValue == value {
		if len(out) == 0 {
			return r.singleSlice(block)
		}
		return append(out, block)
	}
	rebuilt, ok := buildAnthropicDeltaSSEBlock(eventName, blockIndex, deltaType, field, restoredValue)
	if !ok {
		return append(out, r.restoreCompleteString(block))
	}
	return append(out, rebuilt)
}

// FlushPending flushes any buffered Anthropic delta prefix as a synthetic delta
// event on the active content block.
func (r *toolNameStreamRestorer) FlushPending() []string {
	if r == nil || len(r.pending) == 0 {
		return nil
	}
	idx := r.anthropicIndex
	field := r.anthropicField
	r.anthropicIndex = -1
	r.anthropicField = ""
	text := r.flushPendingString()
	if text == "" || idx < 0 || field == "" {
		return nil
	}
	deltaType := "text_delta"
	if field == "partial_json" {
		deltaType = "input_json_delta"
	}
	block, ok := buildAnthropicDeltaSSEBlock("content_block_delta", idx, deltaType, field, text)
	if !ok {
		return nil
	}
	return []string{block}
}

func buildAnthropicDeltaSSEBlock(eventName string, index int, deltaType, field, value string) (string, bool) {
	delta := map[string]any{
		"type": deltaType,
		field:  value,
	}
	payload, err := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": index,
		"delta": delta,
	})
	if err != nil {
		return "", false
	}
	if eventName == "" {
		eventName = "content_block_delta"
	}
	return "event: " + eventName + "\ndata: " + string(payload) + "\n\n", true
}

func parseSingleSSEBlock(block string) (eventName, dataLine string, hasData bool) {
	rem := block
	for len(rem) > 0 {
		var line string
		if nl := strings.IndexByte(rem, '\n'); nl >= 0 {
			line = rem[:nl]
			rem = rem[nl+1:]
		} else {
			line = rem
			rem = ""
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			continue
		}
		if !hasData && strings.HasPrefix(trimmed, "data:") {
			dataLine = strings.TrimLeft(strings.TrimPrefix(trimmed, "data:"), " \t\r")
			hasData = true
		}
	}
	return eventName, dataLine, hasData
}

func isAnthropicBoundaryEvent(eventName, dataLine string) bool {
	switch eventName {
	case "content_block_start", "content_block_stop", "message_delta", "message_stop", "error":
		return true
	}
	return strings.Contains(dataLine, `"content_block_stop"`) ||
		strings.Contains(dataLine, `"content_block_start"`) ||
		strings.Contains(dataLine, `"message_delta"`) ||
		strings.Contains(dataLine, `"message_stop"`)
}

// RestoreChatChunk restores rewritten tool names in bridged OpenAI Chat Completions
// streaming chunks, carrying partial fake prefixes across content, reasoning, and
// tool argument deltas.
func (r *toolNameStreamRestorer) RestoreChatChunk(chunk apicompat.ChatCompletionsChunk) []apicompat.ChatCompletionsChunk {
	if r == nil || !r.hasRules || len(chunk.Choices) == 0 {
		return []apicompat.ChatCompletionsChunk{chunk}
	}

	var out []apicompat.ChatCompletionsChunk
	choice := &chunk.Choices[0]
	delta := &choice.Delta

	// Flush pending if switching stream target (including empty finish-only chunks where targetKey == -1).
	targetKey := chatChunkTargetKey(delta)
	if len(r.pending) > 0 && targetKey != r.ccCallKey {
		if flushed, ok := r.flushChatChunk(chunk.ID, chunk.Object, chunk.Created, chunk.Model); ok {
			out = append(out, flushed)
		}
	}

	switch {
	case delta.Content != nil:
		r.ccCallKey = -2
		val := r.restoreStringFragment(*delta.Content)
		if choice.FinishReason != nil && len(r.pending) > 0 {
			val += r.flushPendingString()
			r.ccCallKey = -1
		}
		if val == "" && len(r.pending) > 0 && len(delta.ToolCalls) == 0 && choice.FinishReason == nil && chunk.Usage == nil {
			return out
		}
		cp := val
		delta.Content = &cp
	case delta.ReasoningContent != nil:
		r.ccCallKey = -3
		val := r.restoreStringFragment(*delta.ReasoningContent)
		if choice.FinishReason != nil && len(r.pending) > 0 {
			val += r.flushPendingString()
			r.ccCallKey = -1
		}
		if val == "" && len(r.pending) > 0 && len(delta.ToolCalls) == 0 && choice.FinishReason == nil && chunk.Usage == nil {
			return out
		}
		cp := val
		delta.ReasoningContent = &cp
	case len(delta.ToolCalls) == 1:
		tc := &delta.ToolCalls[0]
		fnCopy := tc.Function
		if fnCopy.Name != "" {
			fnCopy.Name = r.restoreCompleteString(fnCopy.Name)
		}
		if fnCopy.Arguments != "" || len(r.pending) > 0 {
			r.ccCallKey = chatToolCallIndex(tc.Index)
			val := r.restoreStringFragment(fnCopy.Arguments)
			if choice.FinishReason != nil && len(r.pending) > 0 {
				val += r.flushPendingString()
				r.ccCallKey = -1
			}
			if val == "" && len(r.pending) > 0 && fnCopy.Name == "" && tc.ID == "" && choice.FinishReason == nil && chunk.Usage == nil {
				return out
			}
			fnCopy.Arguments = val
		}
		tc.Function = fnCopy
	default:
		for i := range delta.ToolCalls {
			if delta.ToolCalls[i].Function.Name != "" {
				delta.ToolCalls[i].Function.Name = r.restoreCompleteString(delta.ToolCalls[i].Function.Name)
			}
		}
	}

	if choice.FinishReason != nil && len(r.pending) > 0 {
		if flushed, ok := r.flushChatChunk(chunk.ID, chunk.Object, chunk.Created, chunk.Model); ok {
			out = append(out, flushed)
		}
	}
	return append(out, chunk)
}

func chatToolCallIndex(idx *int) int {
	if idx == nil || *idx < 0 {
		return 0
	}
	return *idx
}

func chatChunkTargetKey(delta *apicompat.ChatDelta) int {
	if delta == nil {
		return -1
	}
	if delta.Content != nil {
		return -2
	}
	if delta.ReasoningContent != nil {
		return -3
	}
	if len(delta.ToolCalls) == 1 && delta.ToolCalls[0].Function.Arguments != "" {
		return chatToolCallIndex(delta.ToolCalls[0].Index)
	}
	return -1
}

func (r *toolNameStreamRestorer) flushChatChunk(id, object string, created int64, model string) (apicompat.ChatCompletionsChunk, bool) {
	key := r.ccCallKey
	r.ccCallKey = -1
	text := r.flushPendingString()
	if text == "" {
		return apicompat.ChatCompletionsChunk{}, false
	}
	var delta apicompat.ChatDelta
	switch key {
	case -2:
		delta.Content = &text
	case -3:
		delta.ReasoningContent = &text
	default:
		if key < 0 {
			return apicompat.ChatCompletionsChunk{}, false
		}
		idx := key
		delta.ToolCalls = []apicompat.ChatToolCall{{
			Index:    &idx,
			Function: apicompat.ChatFunctionCall{Arguments: text},
		}}
	}
	return apicompat.ChatCompletionsChunk{
		ID:      id,
		Object:  object,
		Created: created,
		Model:   model,
		Choices: []apicompat.ChatChunkChoice{{Index: 0, Delta: delta}},
	}, true
}

// RestoreResponsesEvent restores rewritten tool names across bridged OpenAI
// Responses SSE events (`output_text.delta`, `function_call_arguments.delta`,
// `reasoning_summary_text.delta`, and finalized item/response payloads).
func (r *toolNameStreamRestorer) RestoreResponsesEvent(evt apicompat.ResponsesStreamEvent) []apicompat.ResponsesStreamEvent {
	if r == nil || !r.hasRules {
		return []apicompat.ResponsesStreamEvent{evt}
	}

	var out []apicompat.ResponsesStreamEvent
	isStreamingDelta := evt.Type == "response.output_text.delta" ||
		evt.Type == "response.function_call_arguments.delta" ||
		evt.Type == "response.reasoning_summary_text.delta"

	key := ""
	if isStreamingDelta {
		key = evt.Type + ":" + evt.ItemID
	}
	if len(r.pending) > 0 && key != r.responsesKey {
		if flushed, ok := r.flushResponsesEvent(); ok {
			out = append(out, flushed)
		}
	}

	if isStreamingDelta {
		r.responsesKey = key
		val := r.restoreStringFragment(evt.Delta)
		if val == "" && len(r.pending) > 0 {
			return out
		}
		evt.Delta = val
		return append(out, evt)
	}

	if evt.Name != "" {
		evt.Name = r.restoreCompleteString(evt.Name)
	}
	if evt.Text != "" {
		evt.Text = r.restoreCompleteString(evt.Text)
	}
	if evt.Arguments != "" {
		evt.Arguments = r.restoreCompleteString(evt.Arguments)
	}
	if evt.Input != "" {
		evt.Input = r.restoreCompleteString(evt.Input)
	}
	if evt.Part != nil && (evt.Part.Text != "" || evt.Part.Refusal != "") {
		partCopy := *evt.Part
		if partCopy.Text != "" {
			partCopy.Text = r.restoreCompleteString(partCopy.Text)
		}
		if partCopy.Refusal != "" {
			partCopy.Refusal = r.restoreCompleteString(partCopy.Refusal)
		}
		evt.Part = &partCopy
	}
	if evt.Item != nil {
		itemCopy := r.restoreResponsesOutput(*evt.Item)
		evt.Item = &itemCopy
	}
	if evt.Response != nil && len(evt.Response.Output) > 0 {
		respCopy := *evt.Response
		outItems := make([]apicompat.ResponsesOutput, len(respCopy.Output))
		for i := range respCopy.Output {
			outItems[i] = r.restoreResponsesOutput(respCopy.Output[i])
		}
		respCopy.Output = outItems
		evt.Response = &respCopy
	}
	return append(out, evt)
}

func (r *toolNameStreamRestorer) restoreResponsesOutput(item apicompat.ResponsesOutput) apicompat.ResponsesOutput {
	if item.Name != "" {
		item.Name = r.restoreCompleteString(item.Name)
	}
	if item.Arguments != "" {
		item.Arguments = r.restoreCompleteString(item.Arguments)
	}
	if item.Input != "" {
		item.Input = r.restoreCompleteString(item.Input)
	}
	if len(item.Content) > 0 {
		contentCopy := make([]apicompat.ResponsesContentPart, len(item.Content))
		copy(contentCopy, item.Content)
		for i := range contentCopy {
			if contentCopy[i].Text != "" {
				contentCopy[i].Text = r.restoreCompleteString(contentCopy[i].Text)
			}
			if contentCopy[i].Refusal != "" {
				contentCopy[i].Refusal = r.restoreCompleteString(contentCopy[i].Refusal)
			}
		}
		item.Content = contentCopy
	}
	if len(item.Summary) > 0 {
		summaryCopy := make([]apicompat.ResponsesSummary, len(item.Summary))
		copy(summaryCopy, item.Summary)
		for i := range summaryCopy {
			if summaryCopy[i].Text != "" {
				summaryCopy[i].Text = r.restoreCompleteString(summaryCopy[i].Text)
			}
		}
		item.Summary = summaryCopy
	}
	return item
}

func (r *toolNameStreamRestorer) flushResponsesEvent() (apicompat.ResponsesStreamEvent, bool) {
	key := r.responsesKey
	r.responsesKey = ""
	text := r.flushPendingString()
	if text == "" || key == "" {
		return apicompat.ResponsesStreamEvent{}, false
	}
	eventType, itemID, _ := strings.Cut(key, ":")
	return apicompat.ResponsesStreamEvent{
		Type:   eventType,
		ItemID: itemID,
		Delta:  text,
	}, true
}
