package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

type OpenAIResponsesImagePlan struct {
	ImageModel         string
	N                  int
	Stream             bool
	PreviousResponseID string

	prompt         string
	action         string
	responseFormat string
	size           string
	quality        string
	background     string
	outputFormat   string
	options        map[string]any
	toolEcho       map[string]any
	inputs         []openAIResponsesImageInput
	inputItemIDs   []string
	mask           *openAIResponsesImageInput
	forceGenerate  bool

	prepareOnce sync.Once
	prepareErr  error
	fileCache   sync.Map
	fileGroup   singleflight.Group
}

type openAIResponsesImageResolvedInput struct {
	Data        []byte
	ContentType string
	Filename    string
}

type openAIResponsesImageFormField struct {
	Name  string
	Value string
}

func CompileOpenAIResponsesImagePlan(body []byte) (*OpenAIResponsesImagePlan, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("failed to parse Responses image request")
	}
	root := gjson.ParseBytes(body)
	n := 1
	if rawN := root.Get("n"); rawN.Exists() {
		if rawN.Type != gjson.Number {
			return nil, fmt.Errorf("n must be a positive integer")
		}
		parsed, err := strconv.Atoi(rawN.Raw)
		if err != nil || parsed <= 0 || parsed > 10 {
			return nil, fmt.Errorf("n must be an integer between 1 and 10")
		}
		n = parsed
	}
	stream := false
	if rawStream := root.Get("stream"); rawStream.Exists() {
		if rawStream.Type != gjson.True && rawStream.Type != gjson.False {
			return nil, fmt.Errorf("invalid stream field type")
		}
		stream = rawStream.Bool()
	}
	tools := root.Get("tools")
	if tools.Exists() && !tools.IsArray() {
		return nil, fmt.Errorf("tools must be an array")
	}
	imageToolResult := firstOpenAIResponsesImageTool(tools)
	imageTool := map[string]any{"type": "image_generation"}
	if imageToolResult.Exists() && imageToolResult.Raw != "" {
		if err := json.Unmarshal([]byte(imageToolResult.Raw), &imageTool); err != nil {
			return nil, fmt.Errorf("failed to parse image_generation tool")
		}
	}
	imageModel := strings.TrimSpace(imageToolResult.Get("model").String())
	if imageModel == "" {
		imageModel = "gpt-image-2"
		imageTool["model"] = imageModel
	}
	action := strings.ToLower(strings.TrimSpace(imageToolResult.Get("action").String()))
	if action != "" && action != "auto" && action != "generate" && action != "edit" {
		return nil, fmt.Errorf("image_generation action must be one of auto, generate, or edit")
	}
	forceGenerate := action == "generate"
	inputs := collectOpenAIResponsesImageInputs(root.Get("input"))
	inputItemIDs := collectOpenAIResponsesImageItemIDs(root.Get("input"))
	previousResponseID := strings.TrimSpace(root.Get("previous_response_id").String())
	hasInheritedInput := previousResponseID != "" || len(inputItemIDs) > 0
	isEdit := action == "edit" || (action != "generate" && (len(inputs) > 0 || hasInheritedInput))
	if isEdit && len(inputs) == 0 && !hasInheritedInput {
		return nil, fmt.Errorf("image_generation action=edit requires an input_image")
	}
	if isEdit {
		action = "edit"
	} else {
		action = "generate"
	}

	prompt := extractOpenAIResponsesImagePrompt(root)
	if prompt == "" {
		return nil, fmt.Errorf("input or prompt is required for image generation")
	}
	responseFormat := strings.ToLower(strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageToolResult, "response_format").String()))
	if err := validateOpenAIImagesResponseFormat(responseFormat); err != nil {
		return nil, err
	}

	options := make(map[string]any)
	for _, key := range []string{
		"size", "quality", "background", "output_format", "output_compression",
		"moderation", "input_fidelity", "style", "partial_images",
	} {
		if value := firstOpenAIResponsesImageOption(root, imageToolResult, key); value.Exists() {
			options[key] = value.Value()
		}
	}
	var mask *openAIResponsesImageInput
	if rawMask := imageToolResult.Get("input_image_mask"); rawMask.Exists() && rawMask.IsObject() {
		candidate := openAIResponsesImageInput{
			ImageURL: strings.TrimSpace(rawMask.Get("image_url").String()),
			FileID:   strings.TrimSpace(rawMask.Get("file_id").String()),
		}
		if candidate.ImageURL != "" || candidate.FileID != "" {
			mask = &candidate
		}
	}

	return &OpenAIResponsesImagePlan{
		ImageModel:         imageModel,
		N:                  n,
		Stream:             stream,
		PreviousResponseID: previousResponseID,
		prompt:             prompt,
		action:             action,
		responseFormat:     responseFormat,
		size:               strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageToolResult, "size").String()),
		quality:            strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageToolResult, "quality").String()),
		background:         strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageToolResult, "background").String()),
		outputFormat:       strings.TrimSpace(firstOpenAIResponsesImageOption(root, imageToolResult, "output_format").String()),
		options:            options,
		toolEcho:           imageTool,
		inputs:             inputs,
		inputItemIDs:       inputItemIDs,
		mask:               mask,
		forceGenerate:      forceGenerate,
	}, nil
}

func collectOpenAIResponsesImageItemIDs(value gjson.Result) []string {
	ids := make([]string, 0, 1)
	seen := make(map[string]struct{})
	var walk func(gjson.Result)
	walk = func(current gjson.Result) {
		switch {
		case current.IsArray():
			for _, child := range current.Array() {
				walk(child)
			}
		case current.IsObject():
			if strings.TrimSpace(current.Get("type").String()) == "image_generation_call" {
				id := strings.TrimSpace(current.Get("id").String())
				if id != "" {
					if _, exists := seen[id]; !exists {
						seen[id] = struct{}{}
						ids = append(ids, id)
					}
				}
			}
			if content := current.Get("content"); content.Exists() {
				walk(content)
			}
		}
	}
	walk(value)
	return ids
}

func (p *OpenAIResponsesImagePlan) ToolEcho() map[string]any {
	if p == nil {
		return nil
	}
	return cloneOpenAIResponsesImageMap(p.toolEcho)
}

func cloneOpenAIResponsesImageMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (s *OpenAIGatewayService) PrepareOpenAIResponsesImagePlan(ctx context.Context, c *gin.Context, plan *OpenAIResponsesImagePlan) error {
	if plan == nil {
		return fmt.Errorf("responses image plan is required")
	}
	plan.prepareOnce.Do(func() {
		refsByURL := make(map[string][]*openAIResponsesImageInput)
		for i := range plan.inputs {
			if plan.inputs[i].FileID == "" {
				refsByURL[plan.inputs[i].ImageURL] = append(refsByURL[plan.inputs[i].ImageURL], &plan.inputs[i])
			}
		}
		if plan.mask != nil && plan.mask.FileID == "" {
			refsByURL[plan.mask.ImageURL] = append(refsByURL[plan.mask.ImageURL], plan.mask)
		}
		group, prepareCtx := errgroup.WithContext(ctx)
		group.SetLimit(generatedImageDownloadConcurrency)
		for rawURL, refs := range refsByURL {
			rawURL, refs := rawURL, refs
			group.Go(func() error {
				select {
				case generatedImageDownloadSlots <- struct{}{}:
					defer func() { <-generatedImageDownloadSlots }()
				case <-prepareCtx.Done():
					return prepareCtx.Err()
				}
				data, contentType, filename, err := s.resolveOpenAIResponsesImageInput(prepareCtx, c, nil, openAIResponsesImageInput{ImageURL: rawURL})
				if err != nil {
					return err
				}
				for _, ref := range refs {
					ref.Data = data
					ref.ContentType = contentType
					ref.FileName = filename
					ref.ImageURL = ""
				}
				return nil
			})
		}
		plan.prepareErr = group.Wait()
	})
	return plan.prepareErr
}

func (s *OpenAIGatewayService) resolveOpenAIResponsesImagePlanInputs(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	plan *OpenAIResponsesImagePlan,
) ([]openAIResponsesImageResolvedInput, *openAIResponsesImageResolvedInput, error) {
	if err := s.PrepareOpenAIResponsesImagePlan(ctx, c, plan); err != nil {
		return nil, nil, err
	}
	inputs := make([]openAIResponsesImageResolvedInput, len(plan.inputs))
	for i := range plan.inputs {
		resolved, err := s.resolveOpenAIResponsesImagePlanInputForAccount(ctx, account, plan, plan.inputs[i])
		if err != nil {
			return nil, nil, fmt.Errorf("resolve input image %d: %w", i, err)
		}
		inputs[i] = resolved
	}
	var mask *openAIResponsesImageResolvedInput
	if plan.mask != nil {
		resolved, err := s.resolveOpenAIResponsesImagePlanInputForAccount(ctx, account, plan, *plan.mask)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve input image mask: %w", err)
		}
		mask = &resolved
	}
	return inputs, mask, nil
}

func (s *OpenAIGatewayService) resolveOpenAIResponsesImagePlanInputForAccount(
	ctx context.Context,
	account *Account,
	plan *OpenAIResponsesImagePlan,
	input openAIResponsesImageInput,
) (openAIResponsesImageResolvedInput, error) {
	if input.FileID == "" {
		return openAIResponsesImageResolvedInput{Data: input.Data, ContentType: input.ContentType, Filename: input.FileName}, nil
	}
	if account == nil {
		return openAIResponsesImageResolvedInput{}, fmt.Errorf("selected account is required for file_id")
	}
	key := strconv.FormatInt(account.ID, 10) + "|" + input.FileID
	if cached, ok := plan.fileCache.Load(key); ok {
		if resolved, typeOK := cached.(openAIResponsesImageResolvedInput); typeOK {
			return resolved, nil
		}
		plan.fileCache.Delete(key)
	}
	value, err, _ := plan.fileGroup.Do(key, func() (any, error) {
		if cached, ok := plan.fileCache.Load(key); ok {
			if resolved, typeOK := cached.(openAIResponsesImageResolvedInput); typeOK {
				return resolved, nil
			}
			plan.fileCache.Delete(key)
		}
		data, contentType, filename, err := s.downloadOpenAIFileImage(ctx, account, input.FileID)
		if err != nil {
			return nil, err
		}
		resolved := openAIResponsesImageResolvedInput{Data: data, ContentType: contentType, Filename: filename}
		plan.fileCache.Store(key, resolved)
		return resolved, nil
	})
	if err != nil {
		return openAIResponsesImageResolvedInput{}, err
	}
	resolved, ok := value.(openAIResponsesImageResolvedInput)
	if !ok {
		return openAIResponsesImageResolvedInput{}, fmt.Errorf("cached file image has an invalid type")
	}
	return resolved, nil
}

func (p *OpenAIResponsesImagePlan) upstreamFields(upstreamModel string, n int, stream bool) ([]openAIResponsesImageFormField, map[string]any) {
	fields := []openAIResponsesImageFormField{
		{Name: "model", Value: upstreamModel},
		{Name: "prompt", Value: p.prompt},
	}
	jsonBody := map[string]any{"model": upstreamModel, "prompt": p.prompt}
	if p.responseFormat != "" {
		fields = append(fields, openAIResponsesImageFormField{Name: "response_format", Value: p.responseFormat})
		jsonBody["response_format"] = p.responseFormat
	}
	if n != 1 {
		value := strconv.Itoa(n)
		fields = append(fields, openAIResponsesImageFormField{Name: "n", Value: value})
		jsonBody["n"] = n
	}
	if stream {
		fields = append(fields, openAIResponsesImageFormField{Name: "stream", Value: "true"})
		jsonBody["stream"] = true
	}
	for _, key := range []string{
		"size", "quality", "background", "output_format", "output_compression",
		"moderation", "input_fidelity", "style", "partial_images",
	} {
		value, exists := p.options[key]
		if !exists {
			continue
		}
		fields = append(fields, openAIResponsesImageFormField{Name: key, Value: fmt.Sprint(value)})
		jsonBody[key] = value
	}
	return fields, jsonBody
}

func streamOpenAIResponsesImageMultipart(
	fields []openAIResponsesImageFormField,
	inputs []openAIResponsesImageResolvedInput,
	mask *openAIResponsesImageResolvedInput,
) (io.ReadCloser, string) {
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	contentType := multipartWriter.FormDataContentType()
	go func() {
		var writeErr error
		for _, field := range fields {
			if writeErr = multipartWriter.WriteField(field.Name, field.Value); writeErr != nil {
				break
			}
		}
		if writeErr == nil {
			for i := range inputs {
				fieldName := "image"
				if len(inputs) > 1 {
					fieldName = "image[]"
				}
				part, err := createOpenAIResponsesImageFormFile(multipartWriter, fieldName, inputs[i].Filename, inputs[i].ContentType)
				if err != nil {
					writeErr = err
					break
				}
				if _, err := io.Copy(part, bytes.NewReader(inputs[i].Data)); err != nil {
					writeErr = err
					break
				}
			}
		}
		if writeErr == nil && mask != nil {
			part, err := createOpenAIResponsesImageFormFile(multipartWriter, "mask", mask.Filename, mask.ContentType)
			if err != nil {
				writeErr = err
			} else if _, err := io.Copy(part, bytes.NewReader(mask.Data)); err != nil {
				writeErr = err
			}
		}
		if writeErr == nil {
			writeErr = multipartWriter.Close()
		}
		_ = writer.CloseWithError(writeErr)
	}()
	return reader, contentType
}

func (s *OpenAIGatewayService) buildOpenAIResponsesImageAPIBridgePlanRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	plan *OpenAIResponsesImagePlan,
	requestedModel string,
	upstreamModel string,
	n int,
	stream bool,
	captureOnly bool,
) (*openAIResponsesImageAPIBridgeRequest, error) {
	if plan == nil {
		return nil, fmt.Errorf("responses image plan is required")
	}
	if n <= 0 {
		return nil, fmt.Errorf("image count must be positive")
	}
	if err := validateOpenAIImagesModel(upstreamModel); err != nil {
		return nil, err
	}
	if err := s.PrepareOpenAIResponsesImagePlan(ctx, c, plan); err != nil {
		return nil, err
	}
	fields, jsonBody := plan.upstreamFields(upstreamModel, n, stream)
	endpoint := openAIImagesGenerationsEndpoint
	contentType := "application/json"
	var bodyReader io.ReadCloser
	if plan.action == "edit" {
		endpoint = openAIImagesEditsEndpoint
		inputs, mask, err := s.resolveOpenAIResponsesImagePlanInputs(ctx, c, account, plan)
		if err != nil {
			return nil, err
		}
		bodyReader, contentType = streamOpenAIResponsesImageMultipart(fields, inputs, mask)
	} else {
		body, err := json.Marshal(jsonBody)
		if err != nil {
			return nil, fmt.Errorf("encode forced Images API request: %w", err)
		}
		bodyReader = io.NopCloser(bytes.NewReader(body))
	}
	parsed := &OpenAIImagesRequest{
		Endpoint:           endpoint,
		ContentType:        contentType,
		Multipart:          plan.action == "edit",
		Model:              upstreamModel,
		ExplicitModel:      true,
		Prompt:             plan.prompt,
		Stream:             stream,
		N:                  n,
		Size:               plan.size,
		ExplicitSize:       plan.size != "",
		SizeTier:           normalizeOpenAIImageSizeTier(plan.size),
		ResponseFormat:     plan.responseFormat,
		Quality:            plan.quality,
		Background:         plan.background,
		OutputFormat:       plan.outputFormat,
		RequiredCapability: OpenAIImagesCapabilityNative,
	}
	return &openAIResponsesImageAPIBridgeRequest{
		BodyReader:         bodyReader,
		ContentType:        contentType,
		Parsed:             parsed,
		RequestedModel:     strings.TrimSpace(requestedModel),
		UpstreamImageModel: strings.TrimSpace(upstreamModel),
		CaptureOnly:        captureOnly,
	}, nil
}
