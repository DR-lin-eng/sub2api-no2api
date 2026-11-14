package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/chatgptimages"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

func (s *OpenAIGatewayService) useChatGPTAppImages(ctx context.Context, c *gin.Context, account *Account) bool {
	legacy, exists := c.Get("openai_images_legacy_snapshot")
	if !exists {
		legacy = s.settingService.OpenAIImagesLegacyEnabled(ctx)
		c.Set("openai_images_legacy_snapshot", legacy)
	}
	return account.IsOpenAIOAuth() && !account.IsOpenAISIWC() && !account.IsOpenAIPersonalAccessToken() && !account.IsOpenAIAgentIdentity() && legacy != true
}

func (s *OpenAIGatewayService) forwardChatGPTAppImages(ctx context.Context, c *gin.Context, account *Account, parsed *OpenAIImagesRequest, mapped string) (*OpenAIForwardResult, error) {
	result, err := s.forwardChatGPTAppImagesAttempt(ctx, c, account, parsed, mapped)
	if err != nil && (result == nil || result.ImageCount == 0) {
		var failover *UpstreamFailoverError
		if errors.As(err, &failover) {
			return result, err
		}
		err = chatGPTAppImageFailure(err)
		var imageError *OpenAIImagesUpstreamError
		if errors.As(err, &imageError) {
			writeOpenAIImagesUpstreamErrorResponse(c, imageError)
		}
	}
	return result, err
}

func (s *OpenAIGatewayService) forwardChatGPTAppImagesAttempt(ctx context.Context, c *gin.Context, account *Account, parsed *OpenAIImagesRequest, mapped string) (*OpenAIForwardResult, error) {
	start := time.Now()
	model := strings.TrimSpace(parsed.Model)
	if mapped != "" {
		model = mapped
	}
	if err := s.validateOpenAIImagesModel(ctx, model); err != nil {
		return nil, err
	}
	if err := validateChatGPTAppImageOptions(parsed); err != nil {
		return nil, err
	}
	if apiKey := getAPIKeyFromContext(c); apiKey != nil && s.resolver != nil {
		if price := s.resolveOpenAIChannelPricing(ctx, model, apiKey); price != nil && price.Mode == BillingModeToken && !imageCountBillingApplies(apiKey, true) {
			return nil, &OpenAIImagesUpstreamError{StatusCode: 400, ErrorType: "invalid_request_error", Message: "ChatGPT App images require image-count pricing because the App does not guarantee image token usage. Use legacy images for token pricing."}
		}
	}
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	upstreamCtx, cancel := context.WithTimeout(upstreamCtx, 15*time.Minute)
	defer cancel()
	session, err := s.newChatGPTImageSession(upstreamCtx, c, account)
	if err != nil {
		return nil, err
	}
	session.model = model
	attachments, err := session.uploadInputs(upstreamCtx, c, parsed)
	if err != nil {
		return nil, err
	}
	var results []openAIResponsesImageResult
	var usage OpenAIUsage
	knownCount := 0
	var knownSizes []string
	var knownFrames []int
	conversationID := ""
	var generationErr error
	actualModel := ""
	if parsed.Stream && !OpenAIImagesSSEKeepalivePresent(c) {
		defer StartOpenAIImagesSSEKeepalive(c, 10*time.Second)()
	}
	count := max(parsed.N, 1)
	prompt := parsed.Prompt
	if !parsed.Multipart && gjson.ValidBytes(parsed.Body) {
		if raw := gjson.GetBytes(parsed.Body, "prompt"); raw.Type == gjson.String {
			prompt = raw.String()
		}
	}
	prompt = chatGPTAppImagePrompt(parsed, prompt)
	for i := 0; i < count; i++ {
		if len(results) >= count {
			break
		}
		timezone := "UTC"
		if enabled, configured := s.settingService.GetOpenAIRequestTimezoneSettings(upstreamCtx); enabled && configured != "" {
			timezone = configured
		}
		payload := chatgptimages.Conversation(prompt, openAIImagesResponsesMainModelValue(), uuid.NewString(), uuid.NewString(), uuid.NewString(), timezone, attachments)
		decoder := &chatgptimages.Decoder{}
		generationErr = session.generate(upstreamCtx, payload, decoder)
		if decoder.ConversationID != "" {
			conversationID = decoder.ConversationID
		}
		knownCount += len(decoder.Images)
		generationSizes := make([]string, len(decoder.Images))
		generationFrames := make([]int, len(decoder.Images))
		requestedFrames, _ := chatGPTAppGIFOptions(parsed)
		for index, image := range decoder.Images {
			generationFrames[index] = requestedFrames
			if image.Width > 0 && image.Height > 0 {
				generationSizes[index] = fmt.Sprintf("%dx%d", image.Width, image.Height)
			}
		}
		actualModel = decoder.Model
		if len(decoder.Usage) > 0 {
			u, _ := openAIUsageFromGJSON(gjson.ParseBytes(decoder.Usage))
			mergeChatGPTImageUsage(&usage, u)
		}
		if generationErr != nil && len(decoder.Images) == 0 {
			break
		}
		images, err := session.materialize(upstreamCtx, decoder)
		for index, image := range images {
			generationSizes[index] = image.Size
			generationFrames[index] = 1
			if image.OutputFormat == "gif" {
				if frames, scanErr := openAIImagesGIFFrames(image.Result); scanErr == nil {
					generationFrames[index] = frames
				}
			}
		}
		knownSizes = append(knownSizes, generationSizes...)
		knownFrames = append(knownFrames, generationFrames...)
		for _, image := range images {
			if outputErr := validateChatGPTAppImageOutput(parsed, image); outputErr != nil {
				err = outputErr
				break
			}
			results = append(results, image)
		}
		if err != nil {
			generationErr = err
			break
		}
		if len(images) == 0 {
			generationErr = fmt.Errorf("ChatGPT completed without image output")
			break
		}
		if generationErr != nil {
			break
		}
	}
	if knownCount == 0 {
		return nil, chatGPTAppImageFailure(generationErr)
	}
	reconcileOpenAIResponsesImageResultSizes(results, nil)
	result := &OpenAIForwardResult{RequestID: session.requestID, ResponseID: conversationID, Model: model, UpstreamModel: actualModel, Stream: parsed.Stream, Usage: usage, Duration: time.Since(start), ImageEndpoint: true, ImageCount: knownCount, ImageSize: parsed.SizeTier, ImageInputSize: parsed.Size, ImageOutputSizes: knownSizes}
	if parsed.OutputFormat == "gif" && imageCountBillingEnabled(getAPIKeyFromContext(c)) {
		frames, _ := chatGPTAppGIFOptions(parsed)
		result.ImageCount, result.ImageOutputSizes = expandOpenAIImagesFrameOutputs(knownCount, knownSizes, frames, knownFrames)
	}
	result.ImageSize = ResolveImageBillingSize(parsed.Size, result.ImageOutputSizes).BillingSize
	if len(results) == 0 {
		err := chatGPTAppImageFailure(generationErr)
		var imageError *OpenAIImagesUpstreamError
		if errors.As(err, &imageError) {
			writeOpenAIImagesUpstreamErrorResponse(c, imageError)
		}
		return result, err
	}
	// If a later requested image fails, retain completed output and usage. The
	// handler's existing partial-success path settles it without regenerating it.
	if parsed.Stream {
		output := make([]map[string]any, 0, len(results))
		for _, image := range results {
			output = append(output, map[string]any{"type": "image_generation_call", "status": "completed", "result": image.Result, "output_format": image.OutputFormat, "size": image.Size, "revised_prompt": image.RevisedPrompt})
		}
		event, _ := json.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"created_at": time.Now().Unix(), "output": output}})
		body := append(append([]byte("data: "), event...), []byte("\n\ndata: [DONE]\n\n")...)
		response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(bytes.NewReader(body))}
		_, _, _, first, writeErr := s.handleOpenAIImagesOAuthStreamingResponse(response, c, start, parsed.ResponseFormat, openAIImagesStreamPrefix(parsed), model)
		result.FirstTokenMs = first
		if writeErr != nil {
			return result, writeErr
		}
	} else {
		body, err := buildOpenAIImagesAPIResponse(results, time.Now().Unix(), nil, results[0], parsed.ResponseFormat)
		if err != nil {
			return result, err
		}
		StopOpenAIImagesJSONKeepaliveCommitted(c)
		c.Data(http.StatusOK, "application/json", body)
	}
	return result, generationErr
}

func validateChatGPTAppImageOptions(parsed *OpenAIImagesRequest) error {
	if err := validateOpenAIImages26Options(parsed, parsed.Model); err != nil {
		return &OpenAIImagesUpstreamError{StatusCode: 400, ErrorType: "invalid_request_error", Message: err.Error()}
	}
	if parsed.N > 10 {
		return &OpenAIImagesUpstreamError{StatusCode: 400, ErrorType: "invalid_request_error", Message: "ChatGPT App image count must not exceed 10"}
	}
	// The recovered App Images page has no native model-specific controls.
	// Reject unsupported guarantees before creating a conversation or uploading.
	if openAIImagesRequiresNativeProtocol(parsed) {
		return &OpenAIImagesUpstreamError{StatusCode: 400, ErrorType: "invalid_request_error", Message: "The selected image generation options could not be processed."}
	}
	return nil
}

func chatGPTAppImageFailure(err error) error {
	if err == nil {
		err = fmt.Errorf("ChatGPT returned no image output")
	}
	if _, ok := err.(*OpenAIImagesUpstreamError); ok {
		return err
	}
	if _, ok := err.(*UpstreamFailoverError); ok {
		return err
	}
	return &OpenAIImagesUpstreamError{StatusCode: 502, ErrorType: "upstream_error", Message: sanitizeUpstreamErrorMessage(err.Error())}
}

func mergeChatGPTImageUsage(dst *OpenAIUsage, src OpenAIUsage) {
	if src.ReasoningTokens != nil {
		if dst.ReasoningTokens == nil {
			n := *src.ReasoningTokens
			dst.ReasoningTokens = &n
		} else {
			*dst.ReasoningTokens += *src.ReasoningTokens
		}
	}
	dst.InputTokens += src.InputTokens
	dst.OutputTokens += src.OutputTokens
	dst.CacheReadInputTokens += src.CacheReadInputTokens
	dst.ImageInputTokens += src.ImageInputTokens
	dst.ImageOutputTokens += src.ImageOutputTokens
	dst.ImageCacheReadTokens += src.ImageCacheReadTokens
}
