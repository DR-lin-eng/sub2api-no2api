package service

import (
	"fmt"
	"mime"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// grokMediaClientEndpointPath 返回媒体端点对应的客户端 API 路径，
// 用于自定义模型 request_adapter 的 match.endpoint 判定与 {{request.endpoint}} 变量。
func grokMediaClientEndpointPath(endpoint GrokMediaEndpoint) string {
	switch endpoint {
	case GrokMediaEndpointImagesGenerations:
		return openAIImagesGenerationsEndpoint
	case GrokMediaEndpointImagesEdits:
		return openAIImagesEditsEndpoint
	case GrokMediaEndpointVideosGenerations:
		return "/v1/videos/generations"
	case GrokMediaEndpointVideosEdits:
		return "/v1/videos/edits"
	case GrokMediaEndpointVideosExtensions:
		return "/v1/videos/extensions"
	default:
		return ""
	}
}

// replaceGrokMediaURLPath 用自定义模板渲染出的路径替换上游 URL 的 path 部分。
func replaceGrokMediaURLPath(rawURL, path string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse grok media upstream url: %w", err)
	}
	parsed.Path = path
	parsed.RawPath = ""
	return parsed.String(), nil
}

// buildGrokMediaRequestVariables 构造媒体链路自定义模板的 {{request.*}} 变量表。
// 与图片链路保持同名变量（input_images/images/mask_image），模板写法可直接复用；
// 其余请求字段可通过 {{request.body.<key>}} 访问原始请求体。
func buildGrokMediaRequestVariables(
	info GrokMediaRequestInfo,
	endpoint string,
	body []byte,
	contentType string,
	upstreamModel string,
) map[string]any {
	resolution := info.Resolution
	duration := info.DurationSeconds
	if gjson.ValidBytes(body) {
		// 解析结果带有计费默认值，模板变量优先使用请求体里的原始值。
		resolution = strings.TrimSpace(gjson.GetBytes(body, "resolution").String())
		duration = int(gjson.GetBytes(body, "duration").Int())
	}
	inputImages := append([]string{}, info.InputImageURLs...)
	for _, upload := range info.Uploads {
		if dataURL := upload.ModerationDataURL(); dataURL != "" {
			inputImages = append(inputImages, dataURL)
		}
	}
	maskImage := info.MaskImageURL
	if maskImage == "" && info.MaskUpload != nil {
		maskImage = info.MaskUpload.ModerationDataURL()
	}
	values := map[string]any{
		"endpoint":         endpoint,
		"content_type":     contentType,
		"model":            info.Model,
		"upstream_model":   upstreamModel,
		"prompt":           info.Prompt,
		"n":                info.N,
		"size":             info.Size,
		"aspect_ratio":     info.AspectRatio,
		"image_resolution": info.ImageResolution,
		"resolution":       resolution,
		"duration":         duration,
		"input_images":     inputImages,
		"images":           inputImages,
		"mask_image":       maskImage,
	}
	if payload, err := customModelJSONAdapterBase(body, contentType); err == nil {
		values["body"] = payload
	}
	return map[string]any{"request": values}
}

func prepareGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if endpoint != GrokMediaEndpointImagesEdits || gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
		return body, contentType, nil
	}

	info := ParseGrokMediaRequest(contentType, body)
	payload := make(map[string]any)
	if info.Model != "" {
		payload["model"] = info.Model
	}
	if info.Prompt != "" {
		payload["prompt"] = info.Prompt
	}
	if info.N > 1 {
		payload["n"] = info.N
	}
	if info.Size != "" {
		payload["size"] = info.Size
	}
	if info.AspectRatio != "" {
		payload["aspect_ratio"] = info.AspectRatio
	}
	if info.ImageResolution != "" {
		payload["resolution"] = info.ImageResolution
	}

	images := make([]map[string]string, 0, len(info.InputImageURLs)+len(info.Uploads))
	for _, imageURL := range info.InputImageURLs {
		if imageURL = strings.TrimSpace(imageURL); imageURL != "" {
			images = append(images, map[string]string{"url": imageURL})
		}
	}
	for _, upload := range info.Uploads {
		dataURL, err := openAIImageUploadToDataURL(upload)
		if err != nil {
			return nil, "", err
		}
		images = append(images, map[string]string{"url": dataURL})
	}
	if len(images) > 0 {
		payload["image"] = images[0]
		if len(images) > 1 {
			payload["images"] = images
		}
	}

	maskImageURL := strings.TrimSpace(info.MaskImageURL)
	if info.MaskUpload != nil {
		dataURL, err := openAIImageUploadToDataURL(*info.MaskUpload)
		if err != nil {
			return nil, "", err
		}
		maskImageURL = dataURL
	}
	if maskImageURL != "" {
		payload["mask"] = map[string]string{"url": maskImageURL}
	}

	out, err := marshalOpenAIUpstreamJSON(payload)
	if err != nil {
		return nil, "", err
	}
	return out, "application/json", nil
}

func normalizeGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	var imageFields []string
	switch endpoint {
	case GrokMediaEndpointImagesEdits:
		imageFields = []string{"image", "images", "mask"}
	case GrokMediaEndpointVideosGenerations:
		imageFields = []string{"image", "images", "reference_images"}
	}
	var err error
	body, err = canonicalizeGrokMediaImageURLFields(body, imageFields...)
	if err != nil {
		return nil, "", err
	}
	info := ParseGrokMediaRequest(contentType, body)
	upstreamModel := NormalizeGrokMediaModelForEndpoint(endpoint, info.Model, info.HasInputImage())
	if upstreamModel == "" || upstreamModel == info.Model {
		return body, contentType, nil
	}
	out, err := sjson.SetBytes(body, "model", upstreamModel)
	if err != nil {
		return nil, "", fmt.Errorf("rewrite grok media model: %w", err)
	}
	return out, contentType, nil
}

func canonicalizeGrokMediaImageURLFields(body []byte, fields ...string) ([]byte, error) {
	out := body
	for _, field := range fields {
		value := gjson.GetBytes(out, field)
		if !value.Exists() {
			continue
		}
		if value.IsArray() {
			for index := range value.Array() {
				var err error
				out, err = canonicalizeGrokMediaImageURLObject(out, fmt.Sprintf("%s.%d", field, index))
				if err != nil {
					return nil, err
				}
			}
			continue
		}
		var err error
		out, err = canonicalizeGrokMediaImageURLObject(out, field)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func canonicalizeGrokMediaImageURLObject(body []byte, path string) ([]byte, error) {
	legacyPath := path + ".image_url"
	legacy := gjson.GetBytes(body, legacyPath)
	if !legacy.Exists() {
		return body, nil
	}

	out := body
	if strings.TrimSpace(gjson.GetBytes(out, path+".url").String()) == "" {
		var err error
		out, err = sjson.SetBytes(out, path+".url", legacy.Value())
		if err != nil {
			return nil, fmt.Errorf("normalize grok media image url: %w", err)
		}
	}
	out, err := sjson.DeleteBytes(out, legacyPath)
	if err != nil {
		return nil, fmt.Errorf("remove legacy grok media image url: %w", err)
	}
	return out, nil
}

func sanitizeGrokMediaForwardBody(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if !endpoint.RequiresRequestBody() || !gjson.ValidBytes(body) {
		return body, contentType, nil
	}
	switch endpoint {
	case GrokMediaEndpointImagesGenerations, GrokMediaEndpointImagesEdits:
		out, err := applyGrokImagineImageGeometry(body)
		if err != nil {
			return nil, "", fmt.Errorf("sanitize grok media image geometry: %w", err)
		}
		return out, contentType, nil
	default:
		return body, contentType, nil
	}
}
