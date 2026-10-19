package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *GeminiMessagesCompatService) handleStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time, originalModel string) (*geminiStreamResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	writer := &geminiStreamWriteTracker{Writer: c.Writer}
	messageID := generateAnthropicMsgID()
	messageStart := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         originalModel,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	}
	writeSSE(writer, "message_start", messageStart)
	flusher.Flush()
	if writer.err != nil {
		return &geminiStreamResult{usage: &ClaudeUsage{}, clientDisconnect: true}, nil
	}

	var firstTokenMs *int
	var usage ClaudeUsage
	finishReason := ""
	finished := false
	sawToolUse := false

	nextBlockIndex := 0
	openBlockIndex := -1
	openBlockType := ""
	seenText := ""
	seenThought := ""
	openToolIndex := -1
	openToolID := ""
	openToolName := ""
	seenToolJSON := ""

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			if geminiStreamAbortedWithBillableOutput(c, err, &usage) {
				return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMs, clientDisconnect: true, finished: finished}, nil
			}
			return nil, fmt.Errorf("stream read error: %w", err)
		}

		if !strings.HasPrefix(line, "data:") {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			if errors.Is(err, io.EOF) {
				break
			}
			continue
		}

		unwrappedBytes, err := unwrapGeminiResponse([]byte(payload))
		if err != nil {
			continue
		}
		observer := upstreamResponseModelObserverFromContext(c)
		if observer == nil {
			observer = beginUpstreamResponseModelObservation(c)
		}
		observer.ObserveGemini(unwrappedBytes)
		observeGeminiImageOutputs(c, unwrappedBytes)

		var geminiResp map[string]any
		if err := json.Unmarshal(unwrappedBytes, &geminiResp); err != nil {
			continue
		}

		if fr := extractGeminiFinishReason(geminiResp); fr != "" {
			finishReason = fr
		}

		if u := extractGeminiUsage(unwrappedBytes); u != nil {
			usage = *u
		}

		parts := extractGeminiParts(geminiResp)
		for _, part := range parts {
			if text, ok := part["text"].(string); ok && isGeminiThoughtPart(part) {
				delta, newSeen := computeGeminiTextDelta(seenThought, text)
				seenThought = newSeen
				if delta == "" {
					continue
				}
				if openToolIndex >= 0 {
					writeSSE(writer, "content_block_stop", map[string]any{"type": "content_block_stop", "index": openToolIndex})
					openToolIndex = -1
					openToolName = ""
					seenToolJSON = ""
				}
				if openBlockType != "thinking" {
					if openBlockIndex >= 0 {
						writeSSE(writer, "content_block_stop", map[string]any{"type": "content_block_stop", "index": openBlockIndex})
					}
					openBlockType = "thinking"
					openBlockIndex = nextBlockIndex
					nextBlockIndex++
					writeSSE(writer, "content_block_start", map[string]any{
						"type": "content_block_start", "index": openBlockIndex,
						"content_block": map[string]any{"type": "thinking", "thinking": "", "signature": ""},
					})
				}
				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				writeSSE(writer, "content_block_delta", map[string]any{
					"type": "content_block_delta", "index": openBlockIndex,
					"delta": map[string]any{"type": "thinking_delta", "thinking": delta},
				})
				flusher.Flush()
				continue
			}
			if text, ok := part["text"].(string); ok && text != "" {
				// Close an open tool_use block before starting text, mirroring
				// the functionCall branch (which closes open text blocks) and
				// the chat-completions sibling's closeOpenTool(). Otherwise a
				// tool→text sequence keeps the tool_use block open while the
				// text block starts, emitting overlapping Anthropic content
				// blocks that violate the SSE contract.
				if openToolIndex >= 0 {
					writeSSE(writer, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": openToolIndex,
					})
					openToolIndex = -1
					openToolName = ""
					seenToolJSON = ""
				}

				delta, newSeen := computeGeminiTextDelta(seenText, text)
				seenText = newSeen
				if delta == "" {
					continue
				}

				if openBlockType != "text" {
					if openBlockIndex >= 0 {
						writeSSE(writer, "content_block_stop", map[string]any{
							"type":  "content_block_stop",
							"index": openBlockIndex,
						})
					}
					openBlockType = "text"
					openBlockIndex = nextBlockIndex
					nextBlockIndex++
					writeSSE(writer, "content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": openBlockIndex,
						"content_block": map[string]any{
							"type": "text",
							"text": "",
						},
					})
				}

				if firstTokenMs == nil {
					ms := int(time.Since(startTime).Milliseconds())
					firstTokenMs = &ms
				}
				writeSSE(writer, "content_block_delta", map[string]any{
					"type":  "content_block_delta",
					"index": openBlockIndex,
					"delta": map[string]any{
						"type": "text_delta",
						"text": delta,
					},
				})
				flusher.Flush()
				continue
			}

			if fc, ok := part["functionCall"].(map[string]any); ok && fc != nil {
				name, _ := fc["name"].(string)
				args := fc["args"]
				if strings.TrimSpace(name) == "" {
					name = "tool"
				}

				// Close any open text block before tool_use.
				if openBlockIndex >= 0 {
					writeSSE(writer, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": openBlockIndex,
					})
					openBlockIndex = -1
					openBlockType = ""
				}

				// If we receive streamed tool args in pieces, keep a single tool block open and emit deltas.
				if openToolIndex >= 0 && openToolName != name {
					writeSSE(writer, "content_block_stop", map[string]any{
						"type":  "content_block_stop",
						"index": openToolIndex,
					})
					openToolIndex = -1
					openToolName = ""
					seenToolJSON = ""
				}

				if openToolIndex < 0 {
					openToolID = "toolu_" + randomHex(8)
					openToolIndex = nextBlockIndex
					openToolName = name
					nextBlockIndex++
					sawToolUse = true

					writeSSE(writer, "content_block_start", map[string]any{
						"type":  "content_block_start",
						"index": openToolIndex,
						"content_block": map[string]any{
							"type":  "tool_use",
							"id":    openToolID,
							"name":  name,
							"input": map[string]any{},
						},
					})
				}

				argsJSONText := "{}"
				switch v := args.(type) {
				case nil:
					// keep default "{}"
				case string:
					if strings.TrimSpace(v) != "" {
						argsJSONText = v
					}
				default:
					if b, err := json.Marshal(args); err == nil && len(b) > 0 {
						argsJSONText = string(b)
					}
				}

				delta, newSeen := computeGeminiTextDelta(seenToolJSON, argsJSONText)
				seenToolJSON = newSeen
				if delta != "" {
					writeSSE(writer, "content_block_delta", map[string]any{
						"type":  "content_block_delta",
						"index": openToolIndex,
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": delta,
						},
					})
				}
				flusher.Flush()
			}
		}

		if writer.err != nil {
			return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMs, clientDisconnect: true, finished: finished}, nil
		}
		finished = finishReason != ""

		// Process the final unterminated line at EOF as well.
		if errors.Is(err, io.EOF) {
			break
		}
	}

	if openBlockIndex >= 0 {
		writeSSE(writer, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": openBlockIndex,
		})
	}
	if openToolIndex >= 0 {
		writeSSE(writer, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": openToolIndex,
		})
	}

	stopReason := mapGeminiFinishReasonToClaudeStopReason(finishReason)
	if sawToolUse {
		stopReason = "tool_use"
	}

	usageObj := map[string]any{
		"output_tokens": usage.OutputTokens,
	}
	if usage.InputTokens > 0 {
		usageObj["input_tokens"] = usage.InputTokens
	}
	writeSSE(writer, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": usageObj,
	})
	writeSSE(writer, "message_stop", map[string]any{
		"type": "message_stop",
	})
	flusher.Flush()

	return &geminiStreamResult{usage: &usage, firstTokenMs: firstTokenMs, clientDisconnect: writer.err != nil, finished: finished}, nil
}
