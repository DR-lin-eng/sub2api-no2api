// Package openaitiming reads optional OpenAI turn telemetry without changing
// stream progress, retry decisions, or locally measured latency.
package openaitiming

import (
	"math"
	"strings"

	"github.com/tidwall/gjson"
)

const EventType = "responsesapi.websocket_timing"

type Metrics struct {
	ResponseID                string   `json:"response_id,omitempty"`
	TimingScope               string   `json:"timing_scope,omitempty"`
	FirstSampledMessageTTFTMs *float64 `json:"first_sampled_message_ttft_ms,omitempty"`
	EngineServiceTTFTTotalMs  *float64 `json:"engine_service_ttft_total_ms,omitempty"`
	EngineQueueMaxMs          *float64 `json:"engine_queue_max_ms,omitempty"`
	TotalTurnTimeSeconds      *float64 `json:"total_turn_time_s,omitempty"`
	NumEngineCalls            *float64 `json:"num_engine_calls,omitempty"`
	ResponsesAPIDurationMs    *float64 `json:"responsesapi_duration_excl_client_tools_ms,omitempty"`
}

// FirstTokenMs uses the service boundary, not sampling alone. The reported
// total across multiple engine calls is not the first token of the turn.
func (m *Metrics) FirstTokenMs() *int {
	if m == nil || (m.NumEngineCalls != nil && *m.NumEngineCalls != 1) {
		return nil
	}
	return roundedMilliseconds(m.EngineServiceTTFTTotalMs, 1)
}

func (m *Metrics) DurationMs() *int {
	if m == nil {
		return nil
	}
	return roundedMilliseconds(m.TotalTurnTimeSeconds, 1000)
}

func roundedMilliseconds(value *float64, scale float64) *int {
	if value == nil || !validNumber(*value, math.MaxInt32/scale) {
		return nil
	}
	ms := int(math.Round(*value * scale))
	return &ms
}

func validNumber(value, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= maximum
}

func number(root gjson.Result, key string, maximum float64) *float64 {
	value := root.Get(key)
	if value.Type != gjson.Number || !validNumber(value.Float(), maximum) {
		return nil
	}
	n := value.Float()
	return &n
}

// Collector belongs to exactly one forwarding attempt or WebSocket turn.
// It accepts only matching response IDs and never waits for late telemetry.
type Collector struct {
	responseID string
	completed  bool
	metrics    *Metrics
}

func (c *Collector) Observe(payload []byte, eventType string) {
	if c == nil || c.completed {
		return
	}
	eventType = strings.TrimSpace(eventType)
	switch eventType {
	case "response.created", "response.in_progress", "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		id := gjson.GetBytes(payload, "response.id").String()
		if id != "" {
			if c.responseID != "" && c.responseID != id {
				return
			}
			c.responseID = id
			if c.metrics != nil && c.metrics.ResponseID != "" && c.metrics.ResponseID != id {
				c.metrics = nil
			}
		}
		if eventType != "response.created" && eventType != "response.in_progress" {
			c.completed = true
		}
	case EventType:
		if !gjson.ValidBytes(payload) {
			return
		}
		root := gjson.GetBytes(payload, "timing_metrics")
		if !root.IsObject() {
			return
		}
		id := root.Get("response_id").String()
		if len(id) > 200 {
			return
		}
		if c.responseID != "" && id != "" && id != c.responseID {
			return
		}
		scope := root.Get("timing_scope").String()
		if scope != "" && scope != "logical_turn" && scope != "response" {
			return
		}
		m := &Metrics{
			ResponseID: id, TimingScope: scope,
			FirstSampledMessageTTFTMs: number(root, "first_sampled_message_ttft_ms", math.MaxInt32),
			EngineServiceTTFTTotalMs:  number(root, "engine_service_ttft_total_ms", math.MaxInt32),
			EngineQueueMaxMs:          number(root, "engine_queue_max_ms", math.MaxInt32),
			TotalTurnTimeSeconds:      number(root, "total_turn_time_s", math.MaxInt32/1000.0),
			NumEngineCalls:            number(root, "num_engine_calls", math.MaxInt32),
			ResponsesAPIDurationMs:    number(root, "responsesapi_duration_excl_client_tools_ms", math.MaxInt32),
		}
		if m.FirstSampledMessageTTFTMs == nil && m.EngineServiceTTFTTotalMs == nil && m.EngineQueueMaxMs == nil && m.TotalTurnTimeSeconds == nil && m.ResponsesAPIDurationMs == nil {
			return
		}
		c.metrics = m
	}
}

func (c *Collector) Snapshot() *Metrics {
	if c == nil || c.metrics == nil {
		return nil
	}
	copy := *c.metrics
	return &copy
}
