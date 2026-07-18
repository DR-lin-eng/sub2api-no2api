package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

type openAIResponsesImageAPIBridgeRequest struct {
	Body               []byte
	Parsed             *OpenAIImagesRequest
	RequestedModel     string
	UpstreamImageModel string
}

type openAIImagesResponseFormatEnvelope struct {
	Created      int64                                `json:"created"`
	Data         []openAIImagesResponseFormatDataItem `json:"data"`
	Usage        json.RawMessage                      `json:"usage,omitempty"`
	Background   string                               `json:"background,omitempty"`
	OutputFormat string                               `json:"output_format,omitempty"`
	Quality      string                               `json:"quality,omitempty"`
	Size         string                               `json:"size,omitempty"`
	Model        string                               `json:"model,omitempty"`
}

type openAIImagesResponseFormatDataItem struct {
	B64JSON       string `json:"b64_json,omitempty"`
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func firstOptionalString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

type openAIResponsesImageAPIBridgeItem struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Result        string `json:"result,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	Background    string `json:"background,omitempty"`
}

type openAIResponsesImageAPIBridgeResponse struct {
	ID          string                                  `json:"id"`
	Object      string                                  `json:"object"`
	CreatedAt   int64                                   `json:"created_at"`
	CompletedAt *int64                                  `json:"completed_at,omitempty"`
	Status      string                                  `json:"status"`
	Error       any                                     `json:"error"`
	Model       string                                  `json:"model,omitempty"`
	Output      []openAIResponsesImageAPIBridgeItem     `json:"output"`
	Usage       json.RawMessage                         `json:"usage,omitempty"`
	Metadata    map[string]string                       `json:"metadata"`
	Tools       []openAIResponsesImageAPIBridgeToolEcho `json:"tools,omitempty"`
}

type openAIResponsesImageAPIBridgeToolEcho struct {
	Type         string `json:"type"`
	Model        string `json:"model,omitempty"`
	Size         string `json:"size,omitempty"`
	Quality      string `json:"quality,omitempty"`
	Background   string `json:"background,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
}

type openAIResponsesImageAPIBridgeEvent struct {
	Type           string                                 `json:"type"`
	SequenceNumber int                                    `json:"sequence_number"`
	OutputIndex    *int                                   `json:"output_index,omitempty"`
	ItemID         string                                 `json:"item_id,omitempty"`
	PartialIndex   *int                                   `json:"partial_image_index,omitempty"`
	PartialImage   string                                 `json:"partial_image_b64,omitempty"`
	Item           *openAIResponsesImageAPIBridgeItem     `json:"item,omitempty"`
	Response       *openAIResponsesImageAPIBridgeResponse `json:"response,omitempty"`
}

func (a *Account) forcedOpenAIResponsesImageModel(requestModel string) (string, bool) {
	if !a.ForceOpenAIImageAPI() {
		return "", false
	}
	mapped := strings.TrimSpace(a.GetMappedModel(requestModel))
	return mapped, IsGPTImageGenerationModel(mapped)
}

func newOpenAIResponsesBridgeID(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func buildOpenAIResponsesImageAPIBridgeRequest(body []byte, requestedModel, upstreamModel string) (*openAIResponsesImageAPIBridgeRequest, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("failed to parse Responses image request")
	}
	root := gjson.ParseBytes(body)
	if root.Get("previous_response_id").Exists() {
		return nil, fmt.Errorf("previous_response_id is not supported when the account forces the Images API")
	}
	if openAIResponsesInputContainsImage(root.Get("input")) {
		return nil, fmt.Errorf("image input is not supported by the forced generation bridge; use /v1/images/edits")
	}

	imageTool := firstOpenAIResponsesImageTool(root.Get("tools"))
	action := strings.ToLower(strings.TrimSpace(imageTool.Get("action").String()))
	if action == "edit" {
		return nil, fmt.Errorf("image_generation action=edit is not supported by /v1/images/generations")
	}

	prompt := extractOpenAIResponsesImagePrompt(root)
	if prompt == "" {
		return nil, fmt.Errorf("input or prompt is required for image generation")
	}
	responseFormat := strings.ToLower(strings.TrimSpace(root.Get("response_format").String()))
	if err := validateOpenAIImagesResponseFormat(responseFormat); err != nil {
		return nil, err
	}

	stream := root.Get("stream").Bool()
	n := 1
	if value := root.Get("n"); value.Exists() {
		if value.Type != gjson.Number || value.Int() <= 0 {
			return nil, fmt.Errorf("n must be a positive integer")
		}
		n = int(value.Int())
	}
	if stream && n != 1 {
		return nil, fmt.Errorf("streaming forced Images API requests currently require n=1")
	}

	upstream := map[string]any{
		"model":  upstreamModel,
		"prompt": prompt,
	}
	if responseFormat != "" {
		upstream["response_format"] = responseFormat
	}
	if n != 1 {
		upstream["n"] = n
	}
	if stream {
		upstream["stream"] = true
	}
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "size")
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "quality")
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "background")
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "output_format")
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "output_compression")
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "moderation")
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "partial_images")

	upstreamBody, err := json.Marshal(upstream)
	if err != nil {
		return nil, fmt.Errorf("encode forced Images API request: %w", err)
	}
	size := strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageTool, "size").String())
	parsed := &OpenAIImagesRequest{
		Endpoint:           openAIImagesGenerationsEndpoint,
		ContentType:        "application/json",
		Model:              upstreamModel,
		ExplicitModel:      true,
		Prompt:             prompt,
		Stream:             stream,
		N:                  n,
		Size:               size,
		ExplicitSize:       size != "",
		SizeTier:           normalizeOpenAIImageSizeTier(size),
		ResponseFormat:     responseFormat,
		Quality:            strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageTool, "quality").String()),
		Background:         strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageTool, "background").String()),
		OutputFormat:       strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageTool, "output_format").String()),
		RequiredCapability: OpenAIImagesCapabilityNative,
		Body:               upstreamBody,
	}
	return &openAIResponsesImageAPIBridgeRequest{
		Body:               upstreamBody,
		Parsed:             parsed,
		RequestedModel:     strings.TrimSpace(requestedModel),
		UpstreamImageModel: strings.TrimSpace(upstreamModel),
	}, nil
}

func firstOpenAIResponsesImageTool(tools gjson.Result) gjson.Result {
	if !tools.IsArray() {
		return gjson.Result{}
	}
	for _, tool := range tools.Array() {
		if strings.TrimSpace(tool.Get("type").String()) == "image_generation" {
			return tool
		}
	}
	return gjson.Result{}
}

func firstOpenAIResponsesImageOption(root, tool gjson.Result, key string) gjson.Result {
	if value := tool.Get(key); value.Exists() {
		return value
	}
	return root.Get(key)
}

func copyOpenAIResponsesImageOption(dst map[string]any, root, tool gjson.Result, key string) {
	if value := firstOpenAIResponsesImageOption(root, tool, key); value.Exists() {
		dst[key] = value.Value()
	}
}

func extractOpenAIResponsesImagePrompt(root gjson.Result) string {
	parts := make([]string, 0, 4)
	if instructions := strings.TrimSpace(root.Get("instructions").String()); instructions != "" {
		parts = append(parts, instructions)
	}
	input := root.Get("input")
	if input.Exists() {
		collectOpenAIResponsesInputText(input, &parts)
	} else if prompt := strings.TrimSpace(root.Get("prompt").String()); prompt != "" {
		parts = append(parts, prompt)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func collectOpenAIResponsesInputText(value gjson.Result, parts *[]string) {
	switch {
	case value.Type == gjson.String:
		if text := strings.TrimSpace(value.String()); text != "" {
			*parts = append(*parts, text)
		}
	case value.IsArray():
		for _, child := range value.Array() {
			collectOpenAIResponsesInputText(child, parts)
		}
	case value.IsObject():
		kind := strings.TrimSpace(value.Get("type").String())
		if kind == "input_text" || kind == "output_text" {
			if text := strings.TrimSpace(value.Get("text").String()); text != "" {
				*parts = append(*parts, text)
			}
			return
		}
		if content := value.Get("content"); content.Exists() {
			collectOpenAIResponsesInputText(content, parts)
		}
	}
}

func openAIResponsesInputContainsImage(value gjson.Result) bool {
	switch {
	case value.IsArray():
		for _, child := range value.Array() {
			if openAIResponsesInputContainsImage(child) {
				return true
			}
		}
	case value.IsObject():
		if strings.TrimSpace(value.Get("type").String()) == "input_image" || value.Get("image_url").Exists() {
			return true
		}
		if content := value.Get("content"); content.Exists() {
			return openAIResponsesInputContainsImage(content)
		}
	}
	return false
}

func (s *OpenAIGatewayService) forwardOpenAIResponsesViaImagesAPI(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	requestedModel string,
	upstreamImageModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	bridge, err := buildOpenAIResponsesImageAPIBridgeRequest(body, requestedModel, upstreamImageModel)
	if err != nil {
		setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type": "invalid_request_error", "message": err.Error(),
		}})
		return nil, err
	}

	keepaliveInterval := time.Duration(0)
	if s.cfg != nil {
		if bridge.Parsed.Stream {
			keepaliveInterval = time.Duration(s.cfg.Gateway.ImageStreamKeepaliveInterval) * time.Second
		} else {
			keepaliveInterval = time.Duration(s.cfg.Gateway.ImageNonstreamKeepaliveInterval) * time.Second
		}
	}
	stopKeepalive := func() {}
	if bridge.Parsed.Stream {
		stopKeepalive = StartOpenAIResponsesImageSSEKeepalive(c, keepaliveInterval)
	} else {
		stopKeepalive = StartOpenAIImagesJSONKeepalive(c, keepaliveInterval)
	}
	defer stopKeepalive()

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, bridge.Parsed.Stream)
	defer releaseUpstreamCtx()
	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIImagesRequest(
		upstreamCtx, c, account, bridge.Body, "application/json", token, openAIImagesGenerationsEndpoint,
	)
	if err != nil {
		return nil, err
	}
	if bridge.Parsed.Stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMessage := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMessage, respBody) {
			s.handleFailoverSideEffects(upstreamCtx, resp, account, respBody, upstreamImageModel)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				ResponseHeaders:        resp.Header.Clone(),
				RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleOpenAIImagesErrorResponse(upstreamCtx, resp, c, account, upstreamImageModel)
	}
	defer func() { _ = resp.Body.Close() }()

	var usage OpenAIUsage
	var items []openAIResponsesImageAPIBridgeItem
	var usageRaw json.RawMessage
	var firstTokenMs *int
	clientDisconnected := false
	if bridge.Parsed.Stream && isEventStreamResponse(resp.Header) {
		usage, items, usageRaw, firstTokenMs, clientDisconnected, err = s.handleOpenAIResponsesImageAPIStream(resp, c, bridge, startTime)
	} else {
		usage, items, usageRaw, clientDisconnected, err = s.handleOpenAIResponsesImageAPIJSON(resp, c, bridge)
	}
	if err != nil {
		if len(items) == 0 {
			return nil, err
		}
	}

	imageCount := len(items)
	if imageCount <= 0 {
		imageCount = bridge.Parsed.N
	}
	result := &OpenAIForwardResult{
		RequestID:        resp.Header.Get("x-request-id"),
		Usage:            usage,
		Model:            bridge.RequestedModel,
		BillingModel:     bridge.UpstreamImageModel,
		UpstreamModel:    bridge.UpstreamImageModel,
		UpstreamEndpoint: openAIImagesGenerationsEndpoint,
		Stream:           bridge.Parsed.Stream,
		ResponseHeaders:  resp.Header.Clone(),
		Duration:         time.Since(startTime),
		FirstTokenMs:     firstTokenMs,
		ClientDisconnect: clientDisconnected,
		ImageCount:       imageCount,
		ImageSize:        bridge.Parsed.SizeTier,
		ImageInputSize:   bridge.Parsed.Size,
	}
	_ = usageRaw
	if err != nil {
		return result, err
	}
	return result, nil
}

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
				return OpenAIUsage{}, nil, nil, false, err
			}
		}
	} else {
		body, err = s.rewriteOpenAIImagesResponseURLsAsBase64(c, body)
		if err != nil {
			if bridge.Parsed.Stream && c != nil && c.Request != nil && c.Request.Context().Err() != nil {
				discardOutput = true
				body = originalBody
			} else {
				return OpenAIUsage{}, nil, nil, false, err
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
		writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.failed", SequenceNumber: sequence, Response: &failed})
		return fmt.Errorf("upstream response failed: %s", message)
	}
	writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.created", SequenceNumber: sequence, Response: &created})
	sequence++

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
				writeEvent(openAIResponsesImageAPIBridgeEvent{Type: "response.completed", SequenceNumber: sequence, Response: &completed})
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
