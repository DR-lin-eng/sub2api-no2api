package service

import (
	"bufio"
	"encoding/json"
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

func (s *OpenAIGatewayService) handleOpenAIResponsesImageAPIJSON(
	resp *http.Response,
	c *gin.Context,
	bridge *openAIResponsesImageAPIBridgeRequest,
) (OpenAIUsage, []openAIResponsesImageAPIBridgeItem, json.RawMessage, bool, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, nil, nil, false, err
	}
	body = stripOpenAIImagesEmptySSEKeepalives(body)
	originalBody := body
	discardOutput := bridge.Parsed.Stream && c != nil && c.Request != nil && c.Request.Context().Err() != nil
	if discardOutput {
		// The upstream stream is deliberately detached for billing. Once the
		// downstream is gone, keep only enough output state to count the image.
	} else if bridge.Parsed.ResponseFormat == "url" {
		body, err = s.rewriteOpenAIImagesResponseURLsStrict(c, body)
		if err != nil {
			if bridge.Parsed.Stream && c != nil && c.Request != nil && c.Request.Context().Err() != nil {
				discardOutput = true
				body = originalBody
			} else {
				usage, items, usageRaw := openAIResponsesImagePostprocessFailure(originalBody)
				return usage, items, usageRaw, false, err
			}
		}
	} else {
		body, err = s.rewriteOpenAIImagesResponseURLsAsBase64(c, body)
		if err != nil {
			if bridge.Parsed.Stream && c != nil && c.Request != nil && c.Request.Context().Err() != nil {
				discardOutput = true
				body = originalBody
			} else {
				usage, items, usageRaw := openAIResponsesImagePostprocessFailure(originalBody)
				return usage, items, usageRaw, false, err
			}
		}
	}
	var envelope openAIImagesResponseFormatEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return OpenAIUsage{}, nil, nil, false, fmt.Errorf("decode forced Images API response: %w", err)
	}
	items, err := openAIResponsesItemsFromImagesEnvelope(envelope, bridge, discardOutput)
	if err != nil {
		return OpenAIUsage{}, nil, envelope.Usage, false, err
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	response := buildOpenAIResponsesImageAPIBridgeResponse(bridge, envelope.Created, items, envelope.Usage, true)
	if bridge.CaptureOnly {
		return usage, items, envelope.Usage, false, nil
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	if bridge.Parsed.Stream {
		clientDisconnected, writeErr := writeOpenAIResponsesImageAPISyntheticStream(c, response, items)
		return usage, items, envelope.Usage, clientDisconnected, writeErr
	}
	payload, err := json.Marshal(response)
	if err != nil {
		return OpenAIUsage{}, nil, envelope.Usage, false, err
	}
	c.Data(resp.StatusCode, "application/json; charset=utf-8", payload)
	return usage, items, envelope.Usage, false, nil
}

func openAIResponsesImagePostprocessFailure(body []byte) (OpenAIUsage, []openAIResponsesImageAPIBridgeItem, json.RawMessage) {
	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	var usageRaw json.RawMessage
	if raw := gjson.GetBytes(body, "usage"); raw.Exists() && raw.IsObject() {
		usageRaw = json.RawMessage(raw.Raw)
	}
	count := extractOpenAIImageCountFromJSONBytes(body)
	items := make([]openAIResponsesImageAPIBridgeItem, count)
	for i := range items {
		items[i] = openAIResponsesImageAPIBridgeItem{
			ID:     newOpenAIResponsesBridgeID("ig_"),
			Type:   "image_generation_call",
			Status: "completed",
			Result: "discarded",
		}
	}
	return usage, items, usageRaw
}

func openAIResponsesItemsFromImagesEnvelope(
	envelope openAIImagesResponseFormatEnvelope,
	bridge *openAIResponsesImageAPIBridgeRequest,
	discardOutput bool,
) ([]openAIResponsesImageAPIBridgeItem, error) {
	items := make([]openAIResponsesImageAPIBridgeItem, 0, len(envelope.Data))
	for _, image := range envelope.Data {
		result := ""
		if discardOutput && (strings.TrimSpace(image.B64JSON) != "" || strings.TrimSpace(image.URL) != "") {
			result = "discarded"
		} else if bridge.Parsed.ResponseFormat == "url" {
			result = strings.TrimSpace(image.URL)
		} else {
			result = openAIResponsesBridgeBase64(image.B64JSON)
			if result == "" {
				result = openAIResponsesBridgeBase64(image.URL)
			}
		}
		if result == "" {
			return nil, fmt.Errorf("forced Images API response did not contain the requested image result")
		}
		items = append(items, openAIResponsesImageAPIBridgeItem{
			ID:            newOpenAIResponsesBridgeID("ig_"),
			Type:          "image_generation_call",
			Status:        "completed",
			Result:        result,
			RevisedPrompt: image.RevisedPrompt,
			OutputFormat:  firstNonEmpty(envelope.OutputFormat, bridge.Parsed.OutputFormat, "png"),
			Size:          firstNonEmpty(envelope.Size, bridge.Parsed.Size),
			Quality:       firstNonEmpty(envelope.Quality, bridge.Parsed.Quality),
			Background:    firstNonEmpty(envelope.Background, bridge.Parsed.Background),
		})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("forced Images API response did not contain image output")
	}
	return items, nil
}

func openAIResponsesBridgeBase64(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		comma := strings.IndexByte(value, ',')
		if comma < 0 || comma+1 >= len(value) || !strings.Contains(strings.ToLower(value[:comma]), ";base64") {
			return ""
		}
		return strings.TrimSpace(value[comma+1:])
	}
	if strings.HasPrefix(strings.ToLower(value), "http://") || strings.HasPrefix(strings.ToLower(value), "https://") {
		return ""
	}
	return value
}

func buildOpenAIResponsesImageAPIBridgeResponse(
	bridge *openAIResponsesImageAPIBridgeRequest,
	createdAt int64,
	items []openAIResponsesImageAPIBridgeItem,
	usage json.RawMessage,
	completed bool,
) openAIResponsesImageAPIBridgeResponse {
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	status := "in_progress"
	var completedAt *int64
	if completed {
		status = "completed"
		finished := time.Now().Unix()
		completedAt = &finished
	}
	response := openAIResponsesImageAPIBridgeResponse{
		ID:          newOpenAIResponsesBridgeID("resp_"),
		Object:      "response",
		CreatedAt:   createdAt,
		CompletedAt: completedAt,
		Status:      status,
		Error:       nil,
		Model:       bridge.RequestedModel,
		Output:      items,
		Usage:       usage,
		Metadata:    map[string]string{},
		Tools: []openAIResponsesImageAPIBridgeToolEcho{{
			Type:         "image_generation",
			Model:        bridge.UpstreamImageModel,
			Size:         bridge.Parsed.Size,
			Quality:      bridge.Parsed.Quality,
			Background:   bridge.Parsed.Background,
			OutputFormat: bridge.Parsed.OutputFormat,
		}},
	}
	if !completed {
		response.Output = []openAIResponsesImageAPIBridgeItem{}
		response.Usage = nil
	}
	return response
}

func writeOpenAIResponsesImageAPISyntheticStream(
	c *gin.Context,
	response openAIResponsesImageAPIBridgeResponse,
	items []openAIResponsesImageAPIBridgeItem,
) (bool, error) {
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return false, fmt.Errorf("streaming is not supported by response writer")
	}
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	sequence := 0
	created := response
	created.Status = "in_progress"
	created.CompletedAt = nil
	created.Output = []openAIResponsesImageAPIBridgeItem{}
	created.Usage = nil
	if err := writeOpenAIResponsesImageAPIEvent(c, flusher, openAIResponsesImageAPIBridgeEvent{Type: "response.created", SequenceNumber: sequence, Response: &created}); err != nil {
		return true, err
	}
	sequence++
	for i := range items {
		index := i
		added := items[i]
		added.Status = "in_progress"
		added.Result = ""
		if err := writeOpenAIResponsesImageAPIEvent(c, flusher, openAIResponsesImageAPIBridgeEvent{Type: "response.output_item.added", SequenceNumber: sequence, OutputIndex: &index, Item: &added}); err != nil {
			return true, err
		}
		sequence++
		if err := writeOpenAIResponsesImageAPIEvent(c, flusher, openAIResponsesImageAPIBridgeEvent{Type: "response.output_item.done", SequenceNumber: sequence, OutputIndex: &index, Item: &items[i]}); err != nil {
			return true, err
		}
		sequence++
	}
	if err := writeOpenAIResponsesImageAPIEvent(c, flusher, openAIResponsesImageAPIBridgeEvent{Type: "response.completed", SequenceNumber: sequence, Response: &response}); err != nil {
		return true, err
	}
	return false, nil
}

func writeOpenAIResponsesImageAPIEvent(c *gin.Context, flusher http.Flusher, event openAIResponsesImageAPIBridgeEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *OpenAIGatewayService) handleOpenAIResponsesImageAPIStream(
	resp *http.Response,
	c *gin.Context,
	bridge *openAIResponsesImageAPIBridgeRequest,
	startTime time.Time,
) (OpenAIUsage, []openAIResponsesImageAPIBridgeItem, json.RawMessage, *int, bool, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{}, nil, nil, nil, false, fmt.Errorf("streaming is not supported by response writer")
	}

	createdAt := time.Now().Unix()
	responseID := newOpenAIResponsesBridgeID("resp_")
	sequence := 0
	created := buildOpenAIResponsesImageAPIBridgeResponse(bridge, createdAt, nil, nil, false)
	created.ID = responseID
	clientDisconnected := false
	lastWriteAt := time.Now()
	usage := OpenAIUsage{}
	var usageRaw json.RawMessage
	items := make([]openAIResponsesImageAPIBridgeItem, 0, 1)
	itemAdded := false
	var firstTokenMs *int
	var processErr error
	writeEvent := func(event openAIResponsesImageAPIBridgeEvent) {
		if clientDisconnected {
			return
		}
		if err := writeOpenAIResponsesImageAPIEvent(c, flusher, event); err != nil {
			clientDisconnected = true
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Forced Images API Responses client disconnected, continue draining upstream for billing")
			return
		}
		lastWriteAt = time.Now()
	}
	failStream := func(cause error) error {
		message := "Upstream image generation failed"
		if cause != nil {
			message = sanitizeUpstreamErrorMessage(strings.TrimSpace(cause.Error()))
			message = strings.TrimSpace(strings.TrimPrefix(message, "upstream response failed:"))
		}
		failed := openAIResponsesImageAPIBridgeResponse{
			ID:        responseID,
			Object:    "response",
			CreatedAt: createdAt,
			Status:    "failed",
			Error: map[string]string{
				"code":    "upstream_error",
				"message": message,
			},
			Model:    bridge.RequestedModel,
			Output:   items,
			Metadata: map[string]string{},
		}
		if !bridge.CaptureOnly {
			writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.failed", SequenceNumber: sequence, Response: &failed})
		}
		return fmt.Errorf("upstream response failed: %s", message)
	}
	if !bridge.CaptureOnly {
		writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.created", SequenceNumber: sequence, Response: &created})
		sequence++
	}

	processPayload := func(payload []byte) {
		if processErr != nil || len(payload) == 0 {
			return
		}
		if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
			clientDisconnected = true
		}
		if clientDisconnected {
			// Continue parsing usage and terminal output for billing, but avoid
			// downloads/Redis work whose result can no longer reach the client.
		} else if bridge.Parsed.ResponseFormat == "url" {
			rewritten, err := s.rewriteOpenAIImagesResponseURLsStrictPayload(c, payload)
			if err != nil {
				if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
					clientDisconnected = true
				} else {
					processErr = err
					return
				}
			} else {
				payload = rewritten
			}
		} else {
			stopDownloadKeepalive := func() {}
			if containsGeneratedImageURLField(payload) {
				stopDownloadKeepalive = StartOpenAIResponsesImageSSEKeepalive(c, s.openAIImageStreamKeepaliveInterval())
			}
			rewritten, err := s.rewriteOpenAIImagesResponseURLsAsBase64(c, payload)
			stopDownloadKeepalive()
			if err != nil {
				if c != nil && c.Request != nil && c.Request.Context().Err() != nil {
					clientDisconnected = true
				} else {
					processErr = err
					return
				}
			} else {
				payload = rewritten
			}
		}
		eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
		mergeOpenAIUsage(&usage, payload)
		if rawUsage := gjson.GetBytes(payload, "usage"); rawUsage.Exists() && rawUsage.IsObject() {
			usageRaw = append(usageRaw[:0], rawUsage.Raw...)
		}
		switch eventType {
		case "image_generation.partial_image":
			b64 := strings.TrimSpace(gjson.GetBytes(payload, "b64_json").String())
			if b64 == "" {
				return
			}
			if firstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			item := openAIResponsesImageAPIBridgeItem{ID: newOpenAIResponsesBridgeID("ig_"), Type: "image_generation_call", Status: "in_progress"}
			if len(items) == 0 {
				items = append(items, item)
			} else {
				item = items[0]
			}
			if !itemAdded {
				index := 0
				writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.output_item.added", SequenceNumber: sequence, OutputIndex: &index, Item: &item})
				sequence++
				itemAdded = true
			}
			index := 0
			partialIndex := int(gjson.GetBytes(payload, "partial_image_index").Int())
			writeEvent(openAIResponsesImageAPIBridgeEvent{
				Type: "response.image_generation_call.partial_image", SequenceNumber: sequence,
				OutputIndex: &index, ItemID: item.ID, PartialIndex: &partialIndex, PartialImage: b64,
			})
			sequence++
		case "image_generation.completed":
			result := openAIResponsesBridgeBase64(gjson.GetBytes(payload, "b64_json").String())
			if clientDisconnected && result == "" && strings.TrimSpace(gjson.GetBytes(payload, "url").String()) != "" {
				result = "discarded"
			} else if bridge.Parsed.ResponseFormat == "url" {
				result = strings.TrimSpace(gjson.GetBytes(payload, "url").String())
			}
			if result == "" {
				processErr = fmt.Errorf("forced Images API stream completed without the requested image result")
				return
			}
			if firstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			item := openAIResponsesImageAPIBridgeItem{
				ID: newOpenAIResponsesBridgeID("ig_"), Type: "image_generation_call", Status: "completed", Result: result,
				OutputFormat: firstNonEmpty(strings.TrimSpace(gjson.GetBytes(payload, "output_format").String()), bridge.Parsed.OutputFormat, "png"),
				Size:         firstNonEmpty(strings.TrimSpace(gjson.GetBytes(payload, "size").String()), bridge.Parsed.Size),
				Quality:      firstNonEmpty(strings.TrimSpace(gjson.GetBytes(payload, "quality").String()), bridge.Parsed.Quality),
				Background:   firstNonEmpty(strings.TrimSpace(gjson.GetBytes(payload, "background").String()), bridge.Parsed.Background),
			}
			if len(items) == 0 {
				items = append(items, item)
			} else {
				item.ID = items[0].ID
				items[0] = item
			}
			if !itemAdded {
				added := item
				added.Status = "in_progress"
				added.Result = ""
				index := 0
				writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.output_item.added", SequenceNumber: sequence, OutputIndex: &index, Item: &added})
				sequence++
				itemAdded = true
			}
			index := 0
			writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.output_item.done", SequenceNumber: sequence, OutputIndex: &index, Item: &items[0]})
			sequence++
		case "error", "image_generation.failed":
			message := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(payload)))
			if message == "" {
				message = "Upstream image generation failed"
			}
			processErr = fmt.Errorf("upstream response failed: %s", message)
		}
	}

	type readEvent struct {
		line []byte
		err  error
	}
	events := make(chan readEvent, 16)
	done := make(chan struct{})
	go func() {
		defer close(events)
		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				select {
				case events <- readEvent{line: line}:
				case <-done:
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					select {
					case events <- readEvent{err: err}:
					case <-done:
					}
				}
				return
			}
		}
	}()
	defer close(done)

	keepaliveInterval := s.openAIImageStreamKeepaliveInterval()
	if bridge.CaptureOnly {
		keepaliveInterval = 0
	}
	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveC <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveC = keepaliveTicker.C
	}
	var sseData openAISSEDataAccumulator
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				sseData.Flush(processPayload)
				if processErr != nil {
					return usage, items, usageRaw, firstTokenMs, clientDisconnected, failStream(processErr)
				}
				if len(items) == 0 || items[0].Result == "" {
					return usage, items, usageRaw, firstTokenMs, clientDisconnected, failStream(fmt.Errorf("forced Images API stream ended without image output"))
				}
				completed := buildOpenAIResponsesImageAPIBridgeResponse(bridge, createdAt, items, usageRaw, true)
				completed.ID = responseID
				if !bridge.CaptureOnly {
					writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.completed", SequenceNumber: sequence, Response: &completed})
				}
				return usage, items, usageRaw, firstTokenMs, clientDisconnected, nil
			}
			if ev.err != nil {
				return usage, items, usageRaw, firstTokenMs, clientDisconnected, failStream(ev.err)
			}
			line := strings.TrimRight(string(ev.line), "\r\n")
			sseData.AddLine(line, processPayload)
		case <-keepaliveC:
			if clientDisconnected || time.Since(lastWriteAt) < keepaliveInterval {
				continue
			}
			if _, err := io.WriteString(c.Writer, ": keepalive\n\n"); err != nil {
				clientDisconnected = true
				continue
			}
			flusher.Flush()
			lastWriteAt = time.Now()
		}
	}
}
