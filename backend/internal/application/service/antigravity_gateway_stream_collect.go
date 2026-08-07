package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
)

// collectClaudeStreamResponse collects a Gemini stream for a non-streaming
// Claude-compatible client while keeping accumulated payload and tool state bounded.
func (s *AntigravityGatewayService) collectClaudeStreamResponse(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
) ([]byte, *antigravityStreamResult, error) {
	return s.collectClaudeStreamResponseWithLimitsAndObserver(c, resp, startTime, originalModel, defaultAntigravityStreamLimits())
}

func (s *AntigravityGatewayService) collectClaudeStreamResponseWithLimits(
	resp *http.Response,
	startTime time.Time,
	originalModel string,
	limits antigravityStreamLimits,
) ([]byte, *antigravityStreamResult, error) {
	return s.collectClaudeStreamResponseWithLimitsAndObserver(nil, resp, startTime, originalModel, limits)
}

func (s *AntigravityGatewayService) collectClaudeStreamResponseWithLimitsAndObserver(
	c *gin.Context,
	resp *http.Response,
	startTime time.Time,
	originalModel string,
	limits antigravityStreamLimits,
) ([]byte, *antigravityStreamResult, error) {
	var observer *upstreamResponseModelObserver
	if c != nil {
		observer = upstreamResponseModelObserverFromContext(c)
		if observer == nil {
			observer = beginUpstreamResponseModelObservation(c)
		}
	}
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.settingService.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)

	var firstTokenMs *int
	var last map[string]any
	var lastWithParts map[string]any
	var collectedParts []map[string]any
	var meaningfulResponse bool
	budget := newAntigravityStreamBudget(limits)

	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	sendEvent := func(event scanEvent) bool {
		select {
		case events <- event:
			return true
		case <-done:
			return false
		}
	}

	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}(scanBuf)
	defer close(done)

	streamInterval := time.Duration(0)
	if s.settingService != nil && s.settingService.cfg != nil && s.settingService.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.settingService.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	for {
		select {
		case event, open := <-events:
			if !open {
				return s.finishCollectedClaudeStream(last, lastWithParts, collectedParts, meaningfulResponse, firstTokenMs, originalModel)
			}
			if event.err != nil {
				if errors.Is(event.err, bufio.ErrTooLong) {
					logger.LegacyPrintf("service.antigravity_gateway", "SSE line too long (antigravity claude non-stream): max_size=%d error=%v", maxLineSize, event.err)
				}
				return nil, nil, event.err
			}

			trimmed := strings.TrimRight(event.line, "\r\n")
			if !strings.HasPrefix(trimmed, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if payload == "" || payload == "[DONE]" {
				continue
			}
			inner, parseErr := s.unwrapV1InternalResponse([]byte(payload))
			if parseErr != nil {
				continue
			}
			observer.ObserveGemini(inner)

			var parsed map[string]any
			if err := json.Unmarshal(inner, &parsed); err != nil {
				continue
			}
			parts := extractGeminiParts(parsed)
			eventMeaningful := len(parts) > 0 || strings.TrimSpace(extractGeminiFinishReason(parsed)) != ""
			if err := budget.observeEvent(
				len(inner),
				antigravityGeminiToolArgumentBytes(parts),
				!meaningfulResponse && !eventMeaningful,
			); err != nil {
				return nil, nil, antigravityStreamLimitFailoverError(err)
			}

			last = parsed
			if len(parts) > 0 {
				lastWithParts = parsed
				collectedParts = append(collectedParts, parts...)
			}
			if eventMeaningful {
				meaningfulResponse = true
				budget.releasePending()
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
			}

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			logger.LegacyPrintf("service.antigravity_gateway", "Stream data interval timeout (antigravity claude non-stream)")
			return nil, nil, fmt.Errorf("stream data interval timeout")
		}
	}
}

func (s *AntigravityGatewayService) finishCollectedClaudeStream(
	last map[string]any,
	lastWithParts map[string]any,
	collectedParts []map[string]any,
	meaningfulResponse bool,
	firstTokenMs *int,
	originalModel string,
) ([]byte, *antigravityStreamResult, error) {
	if !meaningfulResponse {
		logger.LegacyPrintf("service.antigravity_gateway", "[antigravity-Forward] warning: empty stream response (claude non-stream), triggering failover")
		return nil, nil, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			ResponseBody:           []byte(`{"error":"empty stream response from upstream"}`),
			RetryableOnSameAccount: true,
		}
	}

	finalResponse := pickGeminiCollectResult(last, lastWithParts)
	if len(collectedParts) > 0 {
		finalResponse = mergeCollectedPartsToResponse(finalResponse, collectedParts)
	}
	geminiBody, err := json.Marshal(finalResponse)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal gemini response: %w", err)
	}
	claudeResp, antigravityUsage, err := antigravity.TransformGeminiToClaude(geminiBody, originalModel)
	if err != nil {
		logger.LegacyPrintf("service.antigravity_gateway", "[antigravity-Forward] transform_error error=%v body=%s", err, string(geminiBody))
		return nil, nil, fmt.Errorf("failed to parse upstream response: %w", err)
	}
	usage := &ClaudeUsage{
		InputTokens:              antigravityUsage.InputTokens,
		OutputTokens:             antigravityUsage.OutputTokens,
		CacheCreationInputTokens: antigravityUsage.CacheCreationInputTokens,
		CacheReadInputTokens:     antigravityUsage.CacheReadInputTokens,
		ImageOutputTokens:        antigravityUsage.ImageOutputTokens,
	}
	return claudeResp, &antigravityStreamResult{usage: usage, firstTokenMs: firstTokenMs}, nil
}
