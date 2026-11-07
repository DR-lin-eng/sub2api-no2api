package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAIImagesNativeEndpointContextKey struct{}

func usesCodexDirectImages(model string) bool {
	switch strings.TrimSpace(model) {
	case "gpt-image-2.6-flare", "gpt-image-2.6-sunburst":
		return true
	}
	return false
}

// 正式转发与后台测试共用同一份端点选择和请求构造。
func buildOpenAIImagesOAuthPayload(parsed *OpenAIImagesRequest, model string) ([]byte, string, error) {
	if parsed == nil {
		return nil, "", fmt.Errorf("parsed images request is required")
	}
	if err := validateOpenAIImages26Options(parsed, model); err != nil {
		return nil, "", err
	}
	if !usesCodexDirectImages(model) {
		body, err := buildOpenAIImagesResponsesRequest(parsed, model)
		return body, chatgptCodexURL, err
	}
	if strings.TrimSpace(parsed.Prompt) == "" {
		return nil, "", fmt.Errorf("prompt is required")
	}
	// 只把已解析并校验的图片字段发送给上游，避免客户端注入任意 JSON 字段。
	payload := make(map[string]any, 16)
	payload["model"] = model
	prompt := parsed.Prompt
	if !parsed.Multipart && gjson.ValidBytes(parsed.Body) {
		if rawPrompt := gjson.GetBytes(parsed.Body, "prompt").String(); rawPrompt != "" {
			prompt = rawPrompt
		}
	}
	payload["prompt"] = prompt
	for _, field := range []struct {
		key   string
		value string
	}{
		{"size", parsed.Size}, {"quality", parsed.Quality}, {"complexity", parsed.Complexity},
		{"background", parsed.Background}, {"output_format", parsed.OutputFormat},
		{"moderation", parsed.Moderation}, {"input_fidelity", parsed.InputFidelity},
		{"style", parsed.Style},
		{"safety_identifier", parsed.SafetyIdentifier}, {"user", parsed.User},
	} {
		if value := strings.TrimSpace(field.value); value != "" {
			payload[field.key] = value
		}
	}
	if parsed.N > 1 {
		payload["n"] = parsed.N
	}
	if parsed.OutputCompression != nil {
		payload["output_compression"] = *parsed.OutputCompression
	}
	if parsed.PartialImages != nil {
		payload["partial_images"] = *parsed.PartialImages
	}
	if parsed.FrameCount != nil {
		payload["frame_count"] = *parsed.FrameCount
	}
	if parsed.FrameDurationMS != nil {
		payload["frame_duration_ms"] = *parsed.FrameDurationMS
	}
	if parsed.SVG != nil {
		payload["svg"] = parsed.SVG
	}
	if parsed.Stream {
		payload["stream"] = true
	}

	endpoint := "/images/generations"
	if parsed.IsEdits() {
		endpoint = "/images/edits"
		images := make([]map[string]string, 0, len(parsed.InputImageURLs)+len(parsed.Uploads))
		for _, imageURL := range parsed.InputImageURLs {
			if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
				images = append(images, map[string]string{"image_url": imageURL})
			}
		}
		for _, upload := range parsed.Uploads {
			url, err := openAIImageUploadToDataURL(upload)
			if err != nil {
				return nil, "", err
			}
			images = append(images, map[string]string{"image_url": url})
		}
		if len(images) == 0 {
			return nil, "", fmt.Errorf("image input is required")
		}
		payload["images"] = images
		mask := strings.TrimSpace(parsed.MaskImageURL)
		if parsed.MaskUpload != nil {
			var err error
			mask, err = openAIImageUploadToDataURL(*parsed.MaskUpload)
			if err != nil {
				return nil, "", err
			}
		}
		if mask != "" {
			payload["mask"] = map[string]string{"image_url": mask}
		}
		markup := strings.TrimSpace(parsed.MarkupImageURL)
		if parsed.MarkupUpload != nil {
			var err error
			markup, err = openAIImageUploadToDataURL(*parsed.MarkupUpload)
			if err != nil {
				return nil, "", err
			}
		}
		if markup != "" {
			payload["markup"] = map[string]string{"image_url": markup}
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", fmt.Errorf("marshal Codex Images request: %w", err)
	}
	return body, strings.TrimSuffix(chatgptCodexURL, "/responses") + endpoint, nil
}

// 原生 JSON 响应与后台图片预览使用同一份校验，防止空 data 被计为成功。
func parseCodexDirectImagesResponse(body []byte) ([]openAIResponsesImageResult, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("invalid Images API JSON response")
	}
	if upstreamErr := openAIImagesUpstreamErrorFromSSEPayload(body); upstreamErr != nil {
		return nil, upstreamErr
	}
	root := gjson.ParseBytes(body)
	if upstreamErr := openAIImagesUpstreamErrorFromGJSON(root.Get("error"), ""); upstreamErr != nil {
		return nil, upstreamErr
	}
	var results []openAIResponsesImageResult
	for _, item := range root.Get("data").Array() {
		value := item.Get("b64_json")
		if value.Type != gjson.String {
			continue
		}
		result := strings.TrimSpace(value.String())
		if result == "" {
			continue
		}
		meta := func(key string) string {
			if value := item.Get(key).String(); value != "" {
				return value
			}
			return root.Get(key).String()
		}
		results = append(results, openAIResponsesImageResult{
			Result: result, RevisedPrompt: item.Get("revised_prompt").String(),
			OutputFormat: meta("output_format"), Size: meta("size"),
			Quality: meta("quality"), Background: meta("background"), Model: meta("model"),
		})
	}
	if len(results) == 0 {
		return nil, &OpenAIImagesUpstreamError{StatusCode: 502, ErrorType: "upstream_error", Message: "Images API returned no image output"}
	}
	reconcileOpenAIResponsesImageResultSizes(results, nil)
	return results, nil
}

// Images 端点只输出图片；未提供输出分类时，output_tokens 全部是图片 token。
// 缓存图片数量只采信明确明细，不根据总缓存量猜测图文占比。
func codexDirectImagesUsage(body []byte) (OpenAIUsage, bool) {
	value := gjson.GetBytes(body, "usage")
	usage, ok := openAIUsageFromGJSON(value)
	if !ok {
		// Compatible Images providers may retain a Responses-style usage wrapper.
		// Keep that legacy extraction without treating mixed output as all images.
		usage, ok = extractOpenAIUsageFromJSONBytes(body)
		usage.SVGUsage = parseOpenAIImagesSVGUsage(gjson.GetBytes(body, "svg_usage"))
		return usage, ok || usage.SVGUsage != nil
	}
	usage.InputTokens = max(usage.InputTokens, 0)
	usage.OutputTokens = max(usage.OutputTokens, 0)
	usage.ImageInputTokens = min(max(usage.ImageInputTokens, 0), usage.InputTokens)
	if !value.Get("output_tokens_details.image_tokens").Exists() && !value.Get("output_tokens_details.text_tokens").Exists() {
		usage.ImageOutputTokens = usage.OutputTokens
	}
	usage.ImageOutputTokens = min(max(usage.ImageOutputTokens, 0), usage.OutputTokens)
	usage.CacheReadInputTokens = min(max(usage.CacheReadInputTokens, 0), usage.InputTokens)
	cached := value.Get("input_tokens_details.cached_tokens_details")
	if !value.Get("input_tokens_details.cached_tokens").Exists() && cached.IsObject() {
		imageTokens, _ := boundedJSONNonNegativeInt(cached.Get("image_tokens"))
		textTokens, _ := boundedJSONNonNegativeInt(cached.Get("text_tokens"))
		usage.CacheReadInputTokens = min(imageTokens, max(usage.InputTokens, 0))
		usage.CacheReadInputTokens += min(textTokens, max(usage.InputTokens-usage.CacheReadInputTokens, 0))
	}
	imageCached, _ := boundedJSONNonNegativeInt(cached.Get("image_tokens"))
	usage.ImageCacheReadTokens = min(imageCached, max(usage.ImageInputTokens, 0), max(usage.CacheReadInputTokens, 0))
	usage.SVGUsage = parseOpenAIImagesSVGUsage(gjson.GetBytes(body, "svg_usage"))
	return usage, true
}

func (s *OpenAIGatewayService) handleCodexDirectImagesNonStreamingResponse(resp *http.Response, c *gin.Context, parsed *OpenAIImagesRequest) (OpenAIUsage, int, []string, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if shouldClassifyOpenAIUpstreamStreamReadError(err, c.Request.Context()) {
			err = newOpenAIUpstreamStreamReadError(err)
		}
		return OpenAIUsage{}, 0, nil, err
	}
	results, err := parseCodexDirectImagesResponse(body)
	if err != nil {
		return OpenAIUsage{}, 0, nil, err
	}
	usage, _ := codexDirectImagesUsage(body)
	body, err = rewriteCodexDirectImagesResponse(body, parsed)
	if err != nil {
		return usage, 0, nil, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := strings.TrimSpace(resp.Header.Get("Content-Type")); upstreamType != "" {
			contentType = upstreamType
		}
	}
	c.Data(resp.StatusCode, contentType, body)
	return usage, len(results), openAIResponsesImageResultSizes(results), nil
}

// Rewrite each data item once, preserving unknown response fields. Whole-body
// sjson edits per image repeatedly copy every base64 result in a multi-image response.
func rewriteCodexDirectImagesResponse(body []byte, parsed *OpenAIImagesRequest) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(root["data"], &items); err != nil {
		return nil, err
	}
	var outputFormat string
	_ = json.Unmarshal(root["output_format"], &outputFormat)
	for i, item := range items {
		if item == nil {
			continue
		}
		if model := strings.TrimSpace(parsed.Model); model != "" {
			item["model"], _ = json.Marshal(model)
		}
		var image string
		_ = json.Unmarshal(item["b64_json"], &image)
		if actual := detectOpenAIImageResultSize(image); actual != "" {
			item["size"], _ = json.Marshal(actual)
			if i == 0 {
				root["size"] = item["size"]
			}
		}
		if parsed.ResponseFormat == "url" && image != "" {
			var format string
			_ = json.Unmarshal(item["output_format"], &format)
			if format == "" {
				format = outputFormat
			}
			if format == "" {
				format = parsed.OutputFormat
			}
			item["url"], _ = json.Marshal("data:" + openAIImageOutputMIMEType(format) + ";base64," + image)
			delete(item, "b64_json")
		}
	}
	var err error
	root["data"], err = json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return json.Marshal(root)
}

// Match the existing image stream owner: later empty usage must not erase
// counters already observed before completion or downstream disconnect.
func mergeCodexDirectImagesUsage(dst *OpenAIUsage, src OpenAIUsage) {
	if src.InputTokens > 0 {
		dst.InputTokens = src.InputTokens
	}
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if src.CacheReadInputTokens > 0 {
		dst.CacheReadInputTokens = src.CacheReadInputTokens
	}
	if src.CacheCreationInputTokens > 0 {
		dst.CacheCreationInputTokens = src.CacheCreationInputTokens
	}
	if src.ImageInputTokens > 0 {
		dst.ImageInputTokens = src.ImageInputTokens
	}
	if src.ImageOutputTokens > 0 {
		dst.ImageOutputTokens = src.ImageOutputTokens
	}
	if src.ImageCacheReadTokens > 0 {
		dst.ImageCacheReadTokens = src.ImageCacheReadTokens
	}
	if src.SVGUsage != nil {
		dst.SVGUsage = src.SVGUsage
	}
}
