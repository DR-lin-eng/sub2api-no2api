package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

type openAIResponsesImageAPIBridgeRequest struct {
	Body               []byte
	BodyReader         io.ReadCloser
	ContentType        string
	Parsed             *OpenAIImagesRequest
	RequestedModel     string
	UpstreamImageModel string
	CaptureOnly        bool
}

type openAIResponsesImageInput struct {
	ImageURL    string
	FileID      string
	Data        []byte
	ContentType string
	FileName    string
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

type OpenAIResponsesImageOutputItem = openAIResponsesImageAPIBridgeItem

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

// NormalizeOpenAIResponsesImageToolRequest injects the official hosted image
// tool when it is absent and returns the requested image count and model.
func NormalizeOpenAIResponsesImageToolRequest(body []byte) ([]byte, int, string, error) {
	if !gjson.ValidBytes(body) {
		return nil, 0, "", fmt.Errorf("failed to parse Responses image request")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil || document == nil {
		return nil, 0, "", fmt.Errorf("failed to parse Responses image request")
	}

	n := 1
	if rawN, exists := document["n"]; exists {
		number, ok := rawN.(json.Number)
		if !ok {
			return nil, 0, "", fmt.Errorf("n must be a positive integer")
		}
		parsed, err := strconv.Atoi(number.String())
		if err != nil || parsed <= 0 || parsed > 10 {
			return nil, 0, "", fmt.Errorf("n must be an integer between 1 and 10")
		}
		n = parsed
	}

	tools := make([]any, 0, 1)
	if rawTools, exists := document["tools"]; exists {
		var ok bool
		tools, ok = rawTools.([]any)
		if !ok {
			return nil, 0, "", fmt.Errorf("tools must be an array")
		}
	}
	imageModel := ""
	found := false
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if !ok || strings.TrimSpace(fmt.Sprint(tool["type"])) != "image_generation" {
			continue
		}
		found = true
		if rawModel, ok := tool["model"].(string); ok {
			imageModel = strings.TrimSpace(rawModel)
		}
		if imageModel == "" {
			imageModel = "gpt-image-2"
			tool["model"] = imageModel
		}
		break
	}
	if !found {
		imageModel = "gpt-image-2"
		tools = append(tools, map[string]any{"type": "image_generation", "model": imageModel})
		document["tools"] = tools
	}
	normalized, err := json.Marshal(document)
	if err != nil {
		return nil, 0, "", fmt.Errorf("encode normalized Responses image request: %w", err)
	}
	return normalized, n, imageModel, nil
}

// PrepareOpenAIResponsesImageChildRequest converts a parent request into one
// independently billable single-image child request.
func PrepareOpenAIResponsesImageChildRequest(body []byte, stream bool) ([]byte, error) {
	normalized, _, _, err := NormalizeOpenAIResponsesImageToolRequest(body)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(normalized))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	document["n"] = 1
	document["stream"] = stream
	return json.Marshal(document)
}

func (s *OpenAIGatewayService) buildOpenAIResponsesImageAPIBridgeRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	requestedModel string,
	upstreamModel string,
) (*openAIResponsesImageAPIBridgeRequest, error) {
	normalized, _, _, err := NormalizeOpenAIResponsesImageToolRequest(body)
	if err != nil {
		return nil, err
	}
	body = normalized
	root := gjson.ParseBytes(body)
	imageTool := firstOpenAIResponsesImageTool(root.Get("tools"))
	action := strings.ToLower(strings.TrimSpace(imageTool.Get("action").String()))
	if action != "" && action != "auto" && action != "generate" && action != "edit" {
		return nil, fmt.Errorf("image_generation action must be one of auto, generate, or edit")
	}
	imageInputs := collectOpenAIResponsesImageInputs(root.Get("input"))
	isEdit := action == "edit" || (action != "generate" && len(imageInputs) > 0)
	if isEdit && len(imageInputs) == 0 {
		return nil, fmt.Errorf("image_generation action=edit requires an input_image")
	}

	prompt := extractOpenAIResponsesImagePrompt(root)
	if prompt == "" {
		return nil, fmt.Errorf("input or prompt is required for image generation")
	}
	responseFormat := strings.ToLower(strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageTool, "response_format").String()))
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
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "input_fidelity")
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "style")
	copyOpenAIResponsesImageOption(upstream, root, imageTool, "partial_images")

	endpoint := openAIImagesGenerationsEndpoint
	contentType := "application/json"
	var upstreamBody []byte
	if isEdit {
		endpoint = openAIImagesEditsEndpoint
		upstreamBody, contentType, err = s.buildOpenAIResponsesImageEditMultipart(ctx, c, account, upstream, imageInputs, imageTool.Get("input_image_mask"))
	} else {
		upstreamBody, err = json.Marshal(upstream)
	}
	if err != nil {
		return nil, fmt.Errorf("encode forced Images API request: %w", err)
	}
	size := strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageTool, "size").String())
	parsed := &OpenAIImagesRequest{
		Endpoint:           endpoint,
		ContentType:        contentType,
		Multipart:          isEdit,
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
		ContentType:        contentType,
		Parsed:             parsed,
		RequestedModel:     strings.TrimSpace(requestedModel),
		UpstreamImageModel: strings.TrimSpace(upstreamModel),
	}, nil
}

// buildOpenAIResponsesImageAPIBridgeRequest is retained for pure generation
// parsing tests; edit inputs require the service method so URLs/file IDs can be
// resolved with the selected account.
func buildOpenAIResponsesImageAPIBridgeRequest(body []byte, requestedModel, upstreamModel string) (*openAIResponsesImageAPIBridgeRequest, error) {
	return (&OpenAIGatewayService{}).buildOpenAIResponsesImageAPIBridgeRequest(context.Background(), nil, nil, body, requestedModel, upstreamModel)
}

func collectOpenAIResponsesImageInputs(value gjson.Result) []openAIResponsesImageInput {
	inputs := make([]openAIResponsesImageInput, 0, 1)
	var walk func(gjson.Result)
	walk = func(current gjson.Result) {
		switch {
		case current.IsArray():
			for _, child := range current.Array() {
				walk(child)
			}
		case current.IsObject():
			kind := strings.TrimSpace(current.Get("type").String())
			if kind == "input_image" || current.Get("image_url").Exists() {
				input := openAIResponsesImageInput{
					ImageURL: strings.TrimSpace(current.Get("image_url").String()),
					FileID:   strings.TrimSpace(current.Get("file_id").String()),
				}
				if input.ImageURL != "" || input.FileID != "" {
					inputs = append(inputs, input)
				}
			}
			if content := current.Get("content"); content.Exists() {
				walk(content)
			}
		}
	}
	walk(value)
	return inputs
}

func (s *OpenAIGatewayService) buildOpenAIResponsesImageEditMultipart(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	fields map[string]any,
	inputs []openAIResponsesImageInput,
	mask gjson.Result,
) ([]byte, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, value := range fields {
		if key == "response_format" && strings.TrimSpace(fmt.Sprint(value)) == "" {
			continue
		}
		if err := writer.WriteField(key, fmt.Sprint(value)); err != nil {
			return nil, "", err
		}
	}
	for i, input := range inputs {
		data, contentType, filename, err := s.resolveOpenAIResponsesImageInput(ctx, c, account, input)
		if err != nil {
			return nil, "", fmt.Errorf("resolve input image %d: %w", i, err)
		}
		fieldName := "image"
		if len(inputs) > 1 {
			fieldName = "image[]"
		}
		part, err := createOpenAIResponsesImageFormFile(writer, fieldName, filename, contentType)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(data); err != nil {
			return nil, "", err
		}
	}
	if mask.Exists() && mask.IsObject() {
		maskInput := openAIResponsesImageInput{
			ImageURL: strings.TrimSpace(mask.Get("image_url").String()),
			FileID:   strings.TrimSpace(mask.Get("file_id").String()),
		}
		if maskInput.ImageURL != "" || maskInput.FileID != "" {
			data, contentType, filename, err := s.resolveOpenAIResponsesImageInput(ctx, c, account, maskInput)
			if err != nil {
				return nil, "", fmt.Errorf("resolve input image mask: %w", err)
			}
			part, err := createOpenAIResponsesImageFormFile(writer, "mask", filename, contentType)
			if err != nil {
				return nil, "", err
			}
			if _, err := part.Write(data); err != nil {
				return nil, "", err
			}
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func createOpenAIResponsesImageFormFile(writer *multipart.Writer, fieldName, filename, contentType string) (io.Writer, error) {
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     fieldName,
		"filename": filename,
	}))
	header.Set("Content-Type", contentType)
	return writer.CreatePart(header)
}

func (s *OpenAIGatewayService) resolveOpenAIResponsesImageInput(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	input openAIResponsesImageInput,
) ([]byte, string, string, error) {
	if input.FileID != "" {
		return s.downloadOpenAIFileImage(ctx, account, input.FileID)
	}
	rawURL := strings.TrimSpace(input.ImageURL)
	if rawURL == "" {
		return nil, "", "", fmt.Errorf("image_url or file_id is required")
	}
	if strings.HasPrefix(strings.ToLower(rawURL), "data:") {
		comma := strings.IndexByte(rawURL, ',')
		if comma < 0 || !strings.Contains(strings.ToLower(rawURL[:comma]), ";base64") {
			return nil, "", "", fmt.Errorf("input image must be a base64 data URL")
		}
		encoded := strings.TrimSpace(rawURL[comma+1:])
		if base64.StdEncoding.DecodedLen(len(encoded)) > openAIImageMaxDownloadBytes {
			return nil, "", "", fmt.Errorf("input image exceeds %d bytes", openAIImageMaxDownloadBytes)
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, "", "", fmt.Errorf("decode input image: %w", err)
		}
		if len(data) > openAIImageMaxDownloadBytes {
			return nil, "", "", fmt.Errorf("input image exceeds %d bytes", openAIImageMaxDownloadBytes)
		}
		contentType := strings.TrimSpace(strings.Split(strings.TrimPrefix(rawURL[:comma], "data:"), ";")[0])
		if !strings.HasPrefix(strings.ToLower(contentType), "image/") && !strings.HasPrefix(strings.ToLower(http.DetectContentType(data)), "image/") {
			return nil, "", "", fmt.Errorf("input data URL is not an image")
		}
		return data, contentType, "input" + generatedImageExtension("."+strings.TrimPrefix(contentType, "image/")), nil
	}
	parsed, err := url.Parse(rawURL)
	if err == nil && isLocalGeneratedImageInput(c, parsed) {
		resp, openErr := s.OpenGeneratedImage(ctx, path.Base(parsed.Path), nil)
		if openErr != nil {
			return nil, "", "", openErr
		}
		return readOpenAIResponsesImageBody(resp, path.Base(parsed.Path))
	}
	b64, err := s.downloadGeneratedImageBase64(ctx, rawURL)
	if err != nil {
		return nil, "", "", err
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, "", "", err
	}
	rawPath := ""
	if parsed != nil {
		rawPath = parsed.Path
	}
	return data, http.DetectContentType(data), "input" + generatedImageExtension(rawPath), nil
}

func isLocalGeneratedImageInput(c *gin.Context, parsed *url.URL) bool {
	if parsed == nil || !strings.HasPrefix(parsed.Path, "/generated/") {
		return false
	}
	if parsed.Scheme == "" && parsed.Host == "" {
		return true
	}
	localBase, err := url.Parse(generatedImageLocalBaseURL(c))
	return err == nil && localBase.Host != "" && strings.EqualFold(localBase.Host, parsed.Host)
}

func readOpenAIResponsesImageBody(resp *http.Response, filename string) ([]byte, string, string, error) {
	if resp == nil || resp.Body == nil {
		return nil, "", "", fmt.Errorf("image response is empty")
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes+1))
	if err != nil {
		return nil, "", "", err
	}
	if len(data) > openAIImageMaxDownloadBytes {
		return nil, "", "", fmt.Errorf("input image exceeds %d bytes", openAIImageMaxDownloadBytes)
	}
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("input image is empty")
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return nil, "", "", fmt.Errorf("input image response returned non-image content")
	}
	return data, contentType, filename, nil
}

func (s *OpenAIGatewayService) downloadOpenAIFileImage(ctx context.Context, account *Account, fileID string) ([]byte, string, string, error) {
	if account == nil || strings.TrimSpace(fileID) == "" {
		return nil, "", "", fmt.Errorf("file_id is required")
	}
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, "", "", err
	}
	baseURL := strings.TrimSpace(account.GetOpenAIBaseURL())
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	} else if baseURL, err = s.validateUpstreamBaseURL(baseURL); err != nil {
		return nil, "", "", err
	}
	targetURL := buildOpenAIEndpointURL(baseURL, "/v1/files/"+url.PathEscape(fileID)+"/content")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, "", "", err
	}
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, "", "", err
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	account.ApplyHeaderOverrides(req.Header)
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		return nil, "", "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = resp.Body.Close() }()
		return nil, "", "", fmt.Errorf("file content request returned status %d", resp.StatusCode)
	}
	return readOpenAIResponsesImageBody(resp, fileID+".png")
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

// ForwardOpenAIResponsesViaImagesAPI executes one Responses image child request
// against the selected API-key account's independent Images endpoint.
func (s *OpenAIGatewayService) ForwardOpenAIResponsesViaImagesAPI(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	requestedModel string,
	upstreamImageModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	plan, err := CompileOpenAIResponsesImagePlan(body)
	if err != nil {
		setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type": "invalid_request_error", "message": err.Error(),
		}})
		return nil, err
	}
	return s.ForwardOpenAIResponsesImagePlan(ctx, c, account, plan, requestedModel, upstreamImageModel, plan.N, plan.Stream, false, startTime)
}

func (s *OpenAIGatewayService) ForwardOpenAIResponsesImagePlan(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	plan *OpenAIResponsesImagePlan,
	requestedModel string,
	upstreamImageModel string,
	n int,
	stream bool,
	captureOnly bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("responses image plan is required")
	}

	keepaliveInterval := time.Duration(0)
	if s.cfg != nil {
		if stream {
			keepaliveInterval = time.Duration(s.cfg.Gateway.ImageStreamKeepaliveInterval) * time.Second
		} else {
			keepaliveInterval = time.Duration(s.cfg.Gateway.ImageNonstreamKeepaliveInterval) * time.Second
		}
	}
	stopKeepalive := func() {}
	if stream && !captureOnly {
		stopKeepalive = StartOpenAIResponsesImageSSEKeepalive(c, keepaliveInterval)
	} else if !captureOnly {
		stopKeepalive = StartOpenAIImagesJSONKeepalive(c, keepaliveInterval)
	}
	defer stopKeepalive()
	bridge, err := s.buildOpenAIResponsesImageAPIBridgePlanRequest(
		ctx, c, account, plan, requestedModel, upstreamImageModel, n, stream, captureOnly,
	)
	if err != nil {
		return nil, err
	}
	if bridge.BodyReader != nil {
		defer func() { _ = bridge.BodyReader.Close() }()
	}

	upstreamCtx, releaseUpstreamCtx := detachStreamUpstreamContext(ctx, stream)
	defer releaseUpstreamCtx()
	token, _, err := s.GetAccessToken(upstreamCtx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIImagesRequestReader(
		upstreamCtx, c, account, bridge.BodyReader, bridge.ContentType, token, bridge.Parsed.Endpoint,
	)
	if err != nil {
		return nil, err
	}
	if stream {
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
			return nil, newOpenAIUpstreamFailoverError(
				resp.StatusCode,
				resp.Header,
				respBody,
				upstreamMessage,
				account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
			)
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
		RequestID:           resp.Header.Get("x-request-id"),
		Usage:               usage,
		Model:               bridge.RequestedModel,
		BillingModel:        bridge.UpstreamImageModel,
		UpstreamModel:       bridge.UpstreamImageModel,
		UpstreamEndpoint:    bridge.Parsed.Endpoint,
		Stream:              bridge.Parsed.Stream,
		ResponseHeaders:     resp.Header.Clone(),
		Duration:            time.Since(startTime),
		FirstTokenMs:        firstTokenMs,
		ClientDisconnect:    clientDisconnected,
		ImageCount:          imageCount,
		ImageSize:           bridge.Parsed.SizeTier,
		ImageInputSize:      bridge.Parsed.Size,
		ResponsesImageItems: append([]OpenAIResponsesImageOutputItem(nil), items...),
		ResponsesImageUsage: append(json.RawMessage(nil), usageRaw...),
	}
	if err != nil {
		return result, err
	}
	return result, nil
}
