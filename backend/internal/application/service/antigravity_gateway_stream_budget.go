package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
)

const (
	antigravityMaxStreamEvents        = 100_000
	antigravityMaxStreamPayloadBytes  = 64 << 20
	antigravityMaxToolArgumentBytes   = 16 << 20
	antigravityMaxPendingStreamEvents = 256
	antigravityMaxPendingPayloadBytes = 1 << 20
)

type antigravityStreamLimits struct {
	maxEvents            int64
	maxPayloadBytes      int64
	maxToolArgumentBytes int64
	maxPendingEvents     int64
	maxPendingBytes      int64
}

func defaultAntigravityStreamLimits() antigravityStreamLimits {
	return antigravityStreamLimits{
		maxEvents:            antigravityMaxStreamEvents,
		maxPayloadBytes:      antigravityMaxStreamPayloadBytes,
		maxToolArgumentBytes: antigravityMaxToolArgumentBytes,
		maxPendingEvents:     antigravityMaxPendingStreamEvents,
		maxPendingBytes:      antigravityMaxPendingPayloadBytes,
	}
}

type antigravityStreamBudget struct {
	limits            antigravityStreamLimits
	events            int64
	payloadBytes      int64
	toolArgumentBytes int64
	pendingEvents     int64
	pendingBytes      int64
}

func newAntigravityStreamBudget(limits antigravityStreamLimits) *antigravityStreamBudget {
	return &antigravityStreamBudget{limits: limits}
}

func (b *antigravityStreamBudget) observeEvent(payloadBytes, toolArgumentBytes int, pending bool) error {
	if b == nil {
		return nil
	}
	b.events++
	b.payloadBytes += int64(payloadBytes)
	b.toolArgumentBytes += int64(toolArgumentBytes)
	if pending {
		b.pendingEvents++
		b.pendingBytes += int64(payloadBytes)
	}

	checks := []struct {
		name  string
		value int64
		limit int64
	}{
		{name: "event_count", value: b.events, limit: b.limits.maxEvents},
		{name: "payload_bytes", value: b.payloadBytes, limit: b.limits.maxPayloadBytes},
		{name: "tool_argument_bytes", value: b.toolArgumentBytes, limit: b.limits.maxToolArgumentBytes},
		{name: "pending_event_count", value: b.pendingEvents, limit: b.limits.maxPendingEvents},
		{name: "pending_payload_bytes", value: b.pendingBytes, limit: b.limits.maxPendingBytes},
	}
	for _, check := range checks {
		if check.limit > 0 && check.value > check.limit {
			return &antigravityStreamLimitError{kind: check.name, value: check.value, limit: check.limit}
		}
	}
	return nil
}

func (b *antigravityStreamBudget) releasePending() {
	if b == nil {
		return
	}
	b.pendingEvents = 0
	b.pendingBytes = 0
}

type antigravityStreamLimitError struct {
	kind  string
	value int64
	limit int64
}

func (e *antigravityStreamLimitError) Error() string {
	if e == nil {
		return "antigravity stream limit exceeded"
	}
	return fmt.Sprintf("antigravity stream %s exceeded: value=%d limit=%d", e.kind, e.value, e.limit)
}

func antigravityStreamLimitFailoverError(_ error) *UpstreamFailoverError {
	return &UpstreamFailoverError{
		StatusCode:             http.StatusBadGateway,
		ResponseBody:           []byte(`{"error":"upstream stream exceeded compatibility limits"}`),
		RetryableOnSameAccount: true,
	}
}

func antigravityAnthropicToolArgumentBytes(event *apicompat.AnthropicStreamEvent) int {
	if event == nil {
		return 0
	}
	bytes := 0
	if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
		bytes += len(event.ContentBlock.Input)
	}
	if event.Delta != nil && event.Delta.Type == "input_json_delta" {
		bytes += len(event.Delta.PartialJSON)
	}
	return bytes
}

func antigravityGeminiToolArgumentBytes(parts []map[string]any) int {
	total := 0
	for _, part := range parts {
		functionCall, ok := part["functionCall"].(map[string]any)
		if !ok {
			continue
		}
		args, exists := functionCall["args"]
		if !exists {
			continue
		}
		encoded, err := json.Marshal(args)
		if err == nil {
			total += len(encoded)
		}
	}
	return total
}
