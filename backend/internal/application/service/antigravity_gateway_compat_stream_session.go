package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
)

type antigravityCompatStreamSession struct {
	processor      *antigravity.StreamingProcessor
	adapter        antigravityCompatStreamAdapter
	writer         *antigravityClientWriter
	usage          *ClaudeUsage
	budget         *antigravityStreamBudget
	pendingEvents  []apicompat.AnthropicStreamEvent
	firstTokenMs   *int
	startTime      time.Time
	meaningfulData bool
}

func newAntigravityCompatStreamSession(
	model string,
	startTime time.Time,
	adapter antigravityCompatStreamAdapter,
	writer *antigravityClientWriter,
) *antigravityCompatStreamSession {
	return newAntigravityCompatStreamSessionWithLimits(model, startTime, adapter, writer, defaultAntigravityStreamLimits())
}

func newAntigravityCompatStreamSessionWithLimits(
	model string,
	startTime time.Time,
	adapter antigravityCompatStreamAdapter,
	writer *antigravityClientWriter,
	limits antigravityStreamLimits,
) *antigravityCompatStreamSession {
	return &antigravityCompatStreamSession{
		processor: antigravity.NewStreamingProcessor(model),
		adapter:   adapter,
		writer:    writer,
		usage:     &ClaudeUsage{},
		budget:    newAntigravityStreamBudget(limits),
		startTime: startTime,
	}
}

func (s *antigravityCompatStreamSession) consume(line string) error {
	return s.processor.ProcessLineEvents(strings.TrimRight(line, "\r\n"), s)
}

func (s *antigravityCompatStreamSession) hasMeaningfulData() bool {
	return s.meaningfulData
}

func (s *antigravityCompatStreamSession) finish() (*antigravityStreamResult, error) {
	usage, err := s.processor.FinishEvents(s)
	mergeAntigravityCompatUsage(s.usage, usage)
	if err != nil {
		return nil, err
	}
	if !s.hasMeaningfulData() && !s.writer.Disconnected() {
		return nil, antigravityCompatEmptyStreamError()
	}
	s.adapter.Finalize(s.writer)
	return s.result(s.writer.Disconnected()), nil
}

// ConsumeClaudeStreamEvent implements antigravity.ClaudeStreamEventSink.
func (s *antigravityCompatStreamSession) ConsumeClaudeStreamEvent(event antigravity.ClaudeStreamEvent) error {
	converted := convertAntigravityClaudeStreamEvent(event)
	eventMeaningful := isMeaningfulAntigravityCompatEvent(&converted)
	if err := s.budget.observeEvent(
		event.PayloadBytes,
		antigravityClaudeStreamToolArgumentBytes(event),
		!s.meaningfulData && !eventMeaningful,
	); err != nil {
		return err
	}
	if event.Usage != nil {
		mergeAntigravityCompatUsage(s.usage, event.Usage)
	}
	if event.Message != nil {
		mergeAntigravityCompatUsage(s.usage, &event.Message.Usage)
	}
	s.emitOrBuffer(converted)
	return nil
}

func convertAntigravityClaudeStreamEvent(event antigravity.ClaudeStreamEvent) apicompat.AnthropicStreamEvent {
	converted := apicompat.AnthropicStreamEvent{Type: event.Type, Index: event.Index}
	if event.Message != nil {
		converted.Message = &apicompat.AnthropicResponse{
			ID:           event.Message.ID,
			Type:         event.Message.Type,
			Role:         event.Message.Role,
			Content:      []apicompat.AnthropicContentBlock{},
			Model:        event.Message.Model,
			StopReason:   event.Message.StopReason,
			StopSequence: event.Message.StopSequence,
			Usage:        convertAntigravityClaudeUsage(event.Message.Usage),
		}
	}
	if event.ContentBlock != nil {
		block := event.ContentBlock
		converted.ContentBlock = &apicompat.AnthropicContentBlock{
			Type:      block.Type,
			Text:      block.Text,
			Thinking:  block.Thinking,
			Signature: block.Signature,
			ID:        block.ID,
			Name:      block.Name,
			Input:     block.Input,
		}
		if block.Source != nil {
			converted.ContentBlock.Source = &apicompat.AnthropicImageSource{
				Type:      block.Source.Type,
				MediaType: block.Source.MediaType,
				Data:      block.Source.Data,
			}
		}
	}
	if event.Delta != nil {
		delta := event.Delta
		converted.Delta = &apicompat.AnthropicDelta{
			Type:         delta.Type,
			Text:         delta.Text,
			Thinking:     delta.Thinking,
			Signature:    delta.Signature,
			PartialJSON:  delta.PartialJSON,
			StopReason:   delta.StopReason,
			StopSequence: delta.StopSequence,
		}
	}
	if event.Usage != nil {
		usage := convertAntigravityClaudeUsage(*event.Usage)
		converted.Usage = &usage
	}
	return converted
}

func convertAntigravityClaudeUsage(usage antigravity.ClaudeUsage) apicompat.AnthropicUsage {
	return apicompat.AnthropicUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
		CacheReadInputTokens:     usage.CacheReadInputTokens,
	}
}

func antigravityClaudeStreamToolArgumentBytes(event antigravity.ClaudeStreamEvent) int {
	bytes := 0
	if event.ContentBlock != nil && event.ContentBlock.Type == "tool_use" {
		bytes += len(event.ContentBlock.Input)
	}
	if event.Delta != nil && event.Delta.Type == "input_json_delta" {
		bytes += len(event.Delta.PartialJSON)
	}
	return bytes
}

func (s *antigravityCompatStreamSession) collectResult(clientDisconnect bool) *antigravityStreamResult {
	_, usage := s.processor.Finish()
	mergeAntigravityCompatUsage(s.usage, usage)
	return s.result(clientDisconnect)
}

func (s *antigravityCompatStreamSession) result(clientDisconnect bool) *antigravityStreamResult {
	return &antigravityStreamResult{
		usage:            s.usage,
		firstTokenMs:     s.firstTokenMs,
		clientDisconnect: clientDisconnect,
	}
}

func (s *antigravityCompatStreamSession) consumeClaudeEvents(data []byte) error {
	var eventType []byte
	for len(data) > 0 {
		line := data
		if index := bytes.IndexByte(data, '\n'); index >= 0 {
			line = data[:index]
			data = data[index+1:]
		} else {
			data = nil
		}
		line = bytes.TrimSpace(line)
		switch {
		case bytes.HasPrefix(line, []byte("event:")):
			eventType = bytes.TrimSpace(line[len("event:"):])
		case bytes.HasPrefix(line, []byte("data:")):
			payload := bytes.TrimSpace(line[len("data:"):])
			if err := s.consumeClaudeData(eventType, payload); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *antigravityCompatStreamSession) consumeClaudeData(eventType, payload []byte) error {
	var event apicompat.AnthropicStreamEvent
	if json.Unmarshal(payload, &event) != nil {
		return nil
	}
	if event.Type == "" {
		event.Type = string(eventType)
	}
	eventMeaningful := isMeaningfulAntigravityCompatEvent(&event)
	if err := s.budget.observeEvent(
		len(payload),
		antigravityAnthropicToolArgumentBytes(&event),
		!s.meaningfulData && !eventMeaningful,
	); err != nil {
		return err
	}
	if event.Usage != nil {
		mergeAnthropicUsage(s.usage, *event.Usage)
	}
	if event.Message != nil {
		mergeAnthropicUsage(s.usage, event.Message.Usage)
	}
	s.emitOrBuffer(event)
	return nil
}

func (s *antigravityCompatStreamSession) emitOrBuffer(event apicompat.AnthropicStreamEvent) {
	if s.meaningfulData {
		s.adapter.Emit(&event, s.writer)
		return
	}

	s.pendingEvents = append(s.pendingEvents, event)
	if !isMeaningfulAntigravityCompatEvent(&event) {
		return
	}

	s.meaningfulData = true
	ms := int(time.Since(s.startTime).Milliseconds())
	s.firstTokenMs = &ms
	for i := range s.pendingEvents {
		s.adapter.Emit(&s.pendingEvents[i], s.writer)
	}
	s.pendingEvents = nil
	s.budget.releasePending()
}

func isMeaningfulAntigravityCompatEvent(event *apicompat.AnthropicStreamEvent) bool {
	if event == nil {
		return false
	}
	if event.ContentBlock != nil {
		block := event.ContentBlock
		return block.Type == "tool_use" ||
			block.Text != "" ||
			block.Thinking != "" ||
			block.Source != nil
	}
	if event.Delta != nil {
		delta := event.Delta
		return delta.Text != "" ||
			delta.PartialJSON != "" ||
			delta.Thinking != ""
	}
	return false
}

func mergeAntigravityCompatUsage(dst *ClaudeUsage, src *antigravity.ClaudeUsage) {
	if dst == nil || src == nil {
		return
	}
	dst.InputTokens = src.InputTokens
	dst.OutputTokens = src.OutputTokens
	dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	dst.CacheReadInputTokens = src.CacheReadInputTokens
	dst.ImageOutputTokens = src.ImageOutputTokens
}
