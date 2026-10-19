package service

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// handleNativeStreamingResponse preserves the legacy helper signature.
func (s *GeminiMessagesCompatService) handleNativeStreamingResponse(c *gin.Context, resp *http.Response, startTime time.Time, isOAuth bool) (*geminiNativeStreamResult, error) {
	return s.handleNativeStreamingResponseObserved(c, resp, startTime, isOAuth, nil, "")
}

func (s *GeminiMessagesCompatService) handleNativeStreamingResponseObserved(c *gin.Context, resp *http.Response, startTime time.Time, isOAuth bool, account *Account, upstreamRequestID string) (*geminiNativeStreamResult, error) {
	if s.cfg != nil && s.cfg.Gateway.GeminiDebugResponseHeaders {
		logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] ========== Streaming Response Headers ==========")
		for key, values := range resp.Header {
			if strings.HasPrefix(strings.ToLower(key), "x-ratelimit") {
				logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] %s: %v", key, values)
			}
		}
		logger.LegacyPrintf("service.gemini_messages_compat", "[GeminiAPI] ====================================================")
	}

	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}

	c.Status(resp.StatusCode)
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// Native streaming is SSE by contract. Do not propagate an upstream
	// text/plain (or other stale) content type to downstream SDKs.
	c.Header("Content-Type", "text/event-stream; charset=utf-8")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	reader := bufio.NewReader(resp.Body)
	usage := &ClaudeUsage{}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	var firstTokenMs *int
	var best geminiResponseSignal
	sawDataEvent := false
	fallback := &geminiSSEFallbackBody{}
	clientDisconnect, finished := false, false
	writer := &geminiStreamWriteTracker{Writer: c.Writer}

	for {
		line, err := reader.ReadString('\n')
		// A non-EOF read error leaves an incomplete line; never observe or forward it.
		if len(line) > 0 && (err == nil || errors.Is(err, io.EOF)) {
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
				// Keepalive / done markers
				if payload == "" || payload == "[DONE]" {
					_, _ = io.WriteString(writer, line)
					flusher.Flush()
				} else {
					var rawToWrite string
					rawToWrite = payload

					var rawBytes []byte
					if isOAuth {
						innerBytes, err := unwrapGeminiResponse([]byte(payload))
						if err == nil {
							rawToWrite = string(innerBytes)
							rawBytes = innerBytes
						}
					} else {
						rawBytes = []byte(payload)
					}
					sawDataEvent = true
					if sig, ok := detectGeminiResponseSignal(rawBytes); ok && sig.Kind > best.Kind {
						best = sig
					}

					if u := extractGeminiUsage(rawBytes); u != nil {
						usage = u
					}
					observer.ObserveGemini(rawBytes)

					if firstTokenMs == nil {
						ms := int(time.Since(startTime).Milliseconds())
						firstTokenMs = &ms
					}

					if isOAuth {
						// SSE format requires double newline (\n\n) to separate events
						_, _ = fmt.Fprintf(writer, "data: %s\n\n", rawToWrite)
					} else {
						// Pass-through for AI Studio responses.
						_, _ = io.WriteString(writer, line)
					}
					flusher.Flush()
					if writer.err == nil {
						observeGeminiImageOutputs(c, rawBytes)
						if gjson.GetBytes(rawBytes, "candidates.0.finishReason").String() != "" {
							finished = true
						}
					}
				}
			} else {
				if !sawDataEvent {
					fallback.AddLine(trimmed)
				}
				_, _ = io.WriteString(writer, line)
				flusher.Flush()
			}
		}

		if writer.err != nil {
			clientDisconnect = true
			break
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if !geminiStreamAbortedWithBillableOutput(c, err, usage) {
				return nil, err
			}
			clientDisconnect = true
			break
		}
	}

	s.finalizeGeminiSSESignal(c, account, true, upstreamRequestID, best, sawDataEvent, fallback)

	return &geminiNativeStreamResult{usage: usage, firstTokenMs: firstTokenMs, clientDisconnect: clientDisconnect, finished: finished}, nil
}
