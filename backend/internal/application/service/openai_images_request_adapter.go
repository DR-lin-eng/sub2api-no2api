package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

type customModelRequestAdapterDefinition struct {
	Version int `json:"version"`
	Match   struct {
		Endpoint string `json:"endpoint"`
	} `json:"match"`
	Upstream struct {
		Path        string `json:"path"`
		ContentType string `json:"content_type"`
	} `json:"upstream"`
	Headers struct {
		Set map[string]any `json:"set"`
	} `json:"headers"`
	Body struct {
		Mode  string         `json:"mode"`
		Value map[string]any `json:"value"`
	} `json:"body"`
}

type openAIImagesAdaptedRequest struct {
	Body        []byte
	ContentType string
	Endpoint    string
	Headers     map[string]string
}

const (
	maxCustomModelRequestAdapterBytes       = 64 << 10
	maxCustomModelAdaptedRequestOverhead    = 256 << 10
	maxCustomModelRenderedUpstreamPathBytes = 8 << 10
)

func validateCustomModelRequestAdapter(rawAdapter map[string]any) error {
	if len(rawAdapter) == 0 {
		return nil
	}
	encoded, err := json.Marshal(rawAdapter)
	if err != nil {
		return fmt.Errorf("%w: request adapter is not valid JSON", ErrCustomModelConfigInvalid)
	}
	if len(encoded) > maxCustomModelRequestAdapterBytes {
		return fmt.Errorf("%w: request adapter exceeds %d bytes", ErrCustomModelConfigInvalid, maxCustomModelRequestAdapterBytes)
	}
	var adapter customModelRequestAdapterDefinition
	if err := json.Unmarshal(encoded, &adapter); err != nil {
		return fmt.Errorf("%w: decode request adapter", ErrCustomModelConfigInvalid)
	}
	if adapter.Version != 0 && adapter.Version != 1 {
		return fmt.Errorf("%w: unsupported request adapter version %d", ErrCustomModelConfigInvalid, adapter.Version)
	}
	for field, path := range map[string]string{
		"match endpoint": adapter.Match.Endpoint,
		"upstream path":  adapter.Upstream.Path,
	} {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !isCustomModelAbsolutePath(path) {
			return fmt.Errorf("%w: %s must be an absolute URL path", ErrCustomModelConfigInvalid, field)
		}
		if err := validateCustomModelTemplateString(path); err != nil {
			return err
		}
	}
	contentType := strings.ToLower(strings.TrimSpace(adapter.Upstream.ContentType))
	switch contentType {
	case "", "preserve", "application/json", "multipart/form-data":
	default:
		return fmt.Errorf("%w: unsupported content type %q", ErrCustomModelConfigInvalid, contentType)
	}
	bodyMode := strings.ToLower(strings.TrimSpace(adapter.Body.Mode))
	switch bodyMode {
	case "", "off", "merge", "replace":
	default:
		return fmt.Errorf("%w: unsupported body mode %q", ErrCustomModelConfigInvalid, bodyMode)
	}
	if contentType == "multipart/form-data" && bodyMode != "" && bodyMode != "off" {
		return fmt.Errorf("%w: multipart request bodies only support off mode", ErrCustomModelConfigInvalid)
	}
	for name, rawValue := range adapter.Headers.Set {
		value, ok := rawValue.(string)
		if !ok {
			return fmt.Errorf("%w: header %q must be a string", ErrCustomModelConfigInvalid, name)
		}
		if _, _, err := normalizeHeaderOverrideEntry(name, value); err != nil {
			return fmt.Errorf("%w: %v", ErrCustomModelConfigInvalid, err)
		}
		if err := validateCustomModelTemplateString(value); err != nil {
			return err
		}
	}
	return validateCustomModelTemplateValue(adapter.Body.Value)
}

func validateCustomModelTemplateValue(value any) error {
	switch typed := value.(type) {
	case string:
		return validateCustomModelTemplateString(typed)
	case map[string]any:
		for _, item := range typed {
			if err := validateCustomModelTemplateValue(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := validateCustomModelTemplateValue(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateCustomModelTemplateString(value string) error {
	remaining := value
	for {
		start := strings.Index(remaining, "{{")
		if start < 0 {
			if strings.Contains(remaining, "}}") {
				return fmt.Errorf("%w: unmatched template delimiter", ErrCustomModelConfigInvalid)
			}
			return nil
		}
		endOffset := strings.Index(remaining[start+2:], "}}")
		if endOffset < 0 {
			return fmt.Errorf("%w: unterminated template variable", ErrCustomModelConfigInvalid)
		}
		path := strings.TrimSpace(remaining[start+2 : start+2+endOffset])
		if !strings.HasPrefix(path, "request.") || len(path) <= len("request.") || strings.ContainsAny(path, "{} \t\r\n") {
			return fmt.Errorf("%w: invalid template variable %q", ErrCustomModelConfigInvalid, path)
		}
		remaining = remaining[start+2+endOffset+2:]
	}
}

func applyOpenAIImagesRequestAdapter(
	body []byte,
	contentType string,
	parsed *OpenAIImagesRequest,
	upstreamModel string,
	rawAdapter map[string]any,
) (*openAIImagesAdaptedRequest, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	if err := validateCustomModelRequestAdapter(rawAdapter); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(rawAdapter)
	if err != nil {
		return nil, fmt.Errorf("marshal custom model request adapter: %w", err)
	}
	var adapter customModelRequestAdapterDefinition
	if err := json.Unmarshal(encoded, &adapter); err != nil {
		return nil, fmt.Errorf("decode custom model request adapter: %w", err)
	}
	if adapter.Version != 0 && adapter.Version != 1 {
		return nil, fmt.Errorf("unsupported custom model request adapter version: %d", adapter.Version)
	}

	matchEndpoint := strings.TrimSpace(adapter.Match.Endpoint)
	if matchEndpoint != "" && matchEndpoint != parsed.Endpoint {
		return &openAIImagesAdaptedRequest{
			Body:        body,
			ContentType: contentType,
			Endpoint:    parsed.Endpoint,
		}, nil
	}

	endpoint := parsed.Endpoint
	if targetPath := strings.TrimSpace(adapter.Upstream.Path); targetPath != "" {
		if !strings.HasPrefix(targetPath, "/") || strings.Contains(targetPath, "://") {
			return nil, fmt.Errorf("custom model upstream path must be a relative absolute-path")
		}
		endpoint = targetPath
	}

	targetContentType := strings.TrimSpace(adapter.Upstream.ContentType)
	if targetContentType == "" || targetContentType == "preserve" {
		targetContentType = contentType
	}

	variables := buildCustomModelRequestVariables(parsed, body, contentType, upstreamModel)
	renderedTargetPath, err := renderCustomModelRequestString(
		endpoint,
		variables,
		maxCustomModelRenderedUpstreamPathBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("render custom model upstream path: %w", err)
	}
	if !isCustomModelAbsolutePath(renderedTargetPath) || customModelPathHasTraversal(renderedTargetPath) {
		return nil, fmt.Errorf("custom model upstream path must be an absolute URL path")
	}
	endpoint = renderedTargetPath
	renderedContentType, err := renderCustomModelRequestString(
		targetContentType,
		variables,
		maxHeaderOverrideValueLength,
	)
	if err != nil {
		return nil, fmt.Errorf("render custom model content type: %w", err)
	}
	targetContentType = renderedContentType

	adaptedBody := body
	bodyMode := strings.ToLower(strings.TrimSpace(adapter.Body.Mode))
	if bodyMode == "" {
		bodyMode = "off"
	}
	adaptedBodyLimit := customModelAdaptedRequestBodyLimit(len(body))
	renderedBodyValue, err := renderCustomModelRequestValue(adapter.Body.Value, variables, adaptedBodyLimit)
	if err != nil {
		return nil, fmt.Errorf("render custom model request body: %w", err)
	}
	bodyValue, ok := renderedBodyValue.(map[string]any)
	if !ok {
		bodyValue = map[string]any{}
	}
	switch targetContentType {
	case "application/json":
		payload, payloadErr := openAIImagesJSONAdapterBase(body, contentType)
		if payloadErr != nil {
			return nil, payloadErr
		}
		switch bodyMode {
		case "off":
		case "merge":
			payload = mergeCustomModelRequestBody(payload, bodyValue)
		case "replace":
			payload = cloneCustomModelRequestBody(bodyValue)
		default:
			return nil, fmt.Errorf("unsupported custom model body mode: %s", bodyMode)
		}
		adaptedBody, err = marshalCustomModelAdaptedRequestBody(payload, adaptedBodyLimit)
		if err != nil {
			return nil, err
		}
	case "multipart/form-data":
		mediaType, _, parseErr := mime.ParseMediaType(contentType)
		if parseErr != nil || !strings.EqualFold(mediaType, "multipart/form-data") {
			return nil, fmt.Errorf("conversion to multipart/form-data is not supported")
		}
		if bodyMode != "off" {
			return nil, fmt.Errorf("multipart request body only supports off mode")
		}
	default:
		mediaType, _, parseErr := mime.ParseMediaType(targetContentType)
		if parseErr != nil || !strings.EqualFold(mediaType, "application/json") {
			return nil, fmt.Errorf("unsupported custom model content type: %s", targetContentType)
		}
	}

	headers := make(map[string]string, len(adapter.Headers.Set))
	renderedHeaders, err := renderCustomModelRequestValue(
		adapter.Headers.Set,
		variables,
		maxHeaderOverrideValueLength,
	)
	if err != nil {
		return nil, fmt.Errorf("render custom model request headers: %w", err)
	}
	headerValues, ok := renderedHeaders.(map[string]any)
	if !ok {
		headerValues = map[string]any{}
	}
	for name, rawValue := range headerValues {
		value, ok := rawValue.(string)
		if !ok {
			return nil, fmt.Errorf("custom model header %q must resolve to a string", name)
		}
		normalizedName, normalizedValue, normalizeErr := normalizeHeaderOverrideEntry(name, value)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if normalizedName != "" && normalizedValue != "" {
			headers[normalizedName] = normalizedValue
		}
	}
	return &openAIImagesAdaptedRequest{
		Body:        adaptedBody,
		ContentType: targetContentType,
		Endpoint:    endpoint,
		Headers:     headers,
	}, nil
}

func isCustomModelAbsolutePath(path string) bool {
	return strings.HasPrefix(path, "/") &&
		!strings.HasPrefix(path, "//") &&
		!strings.Contains(path, "://") &&
		!strings.ContainsAny(path, "\\?#\r\n")
}

func customModelPathHasTraversal(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if segment == "." || segment == ".." {
			return true
		}
	}
	return false
}

func buildCustomModelRequestVariables(
	parsed *OpenAIImagesRequest,
	body []byte,
	contentType string,
	upstreamModel string,
) map[string]any {
	values := map[string]any{
		"endpoint":        parsed.Endpoint,
		"content_type":    contentType,
		"model":           parsed.Model,
		"upstream_model":  upstreamModel,
		"prompt":          parsed.Prompt,
		"size":            parsed.Size,
		"quality":         parsed.Quality,
		"background":      parsed.Background,
		"output_format":   parsed.OutputFormat,
		"moderation":      parsed.Moderation,
		"input_fidelity":  parsed.InputFidelity,
		"style":           parsed.Style,
		"n":               parsed.N,
		"stream":          parsed.Stream,
		"response_format": parsed.ResponseFormat,
	}
	if payload, err := openAIImagesJSONAdapterBase(body, contentType); err == nil {
		values["body"] = payload
	}
	inputImages := append([]string{}, parsed.InputImageURLs...)
	for _, upload := range parsed.Uploads {
		if dataURL := upload.ModerationDataURL(); dataURL != "" {
			inputImages = append(inputImages, dataURL)
		}
	}
	maskImage := parsed.MaskImageURL
	if maskImage == "" && parsed.MaskUpload != nil {
		maskImage = parsed.MaskUpload.ModerationDataURL()
	}
	values["input_images"] = inputImages
	values["images"] = inputImages
	values["mask_image"] = maskImage
	return map[string]any{"request": values}
}

func renderCustomModelRequestValue(value any, variables map[string]any, maxStringBytes int64) (any, error) {
	switch typed := value.(type) {
	case string:
		return renderCustomModelRequestStringValue(typed, variables, maxStringBytes)
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			rendered, err := renderCustomModelRequestValue(item, variables, maxStringBytes)
			if err != nil {
				return nil, err
			}
			out[key] = rendered
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			rendered, err := renderCustomModelRequestValue(item, variables, maxStringBytes)
			if err != nil {
				return nil, err
			}
			out[i] = rendered
		}
		return out, nil
	default:
		return value, nil
	}
}

func renderCustomModelRequestString(
	value string,
	variables map[string]any,
	maxStringBytes int64,
) (string, error) {
	rendered, err := renderCustomModelRequestStringValue(value, variables, maxStringBytes)
	if err != nil {
		return "", err
	}
	stringValue, ok := rendered.(string)
	if !ok {
		return "", fmt.Errorf("value must resolve to a string")
	}
	return stringValue, nil
}

func renderCustomModelRequestStringValue(value string, variables map[string]any, maxStringBytes int64) (any, error) {
	const prefix = "{{"
	const suffix = "}}"
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, prefix) && strings.HasSuffix(trimmed, suffix) &&
		strings.Count(trimmed, prefix) == 1 && strings.Count(trimmed, suffix) == 1 {
		path := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, prefix), suffix))
		resolved, ok := lookupCustomModelRequestVariable(path, variables)
		if !ok {
			return nil, fmt.Errorf("unknown request template variable %q", path)
		}
		if stringValue, ok := resolved.(string); ok && int64(len(stringValue)) > maxStringBytes {
			return nil, fmt.Errorf("rendered request template value exceeds %d bytes", maxStringBytes)
		}
		return resolved, nil
	}

	var result strings.Builder
	if int64(len(value)) <= maxStringBytes {
		result.Grow(len(value))
	}
	remaining := value
	for {
		start := strings.Index(remaining, prefix)
		if start < 0 {
			if err := appendCustomModelRenderedString(&result, remaining, maxStringBytes); err != nil {
				return nil, err
			}
			return result.String(), nil
		}
		if err := appendCustomModelRenderedString(&result, remaining[:start], maxStringBytes); err != nil {
			return nil, err
		}
		endOffset := strings.Index(remaining[start+len(prefix):], suffix)
		if endOffset < 0 {
			return nil, fmt.Errorf("unterminated request template variable")
		}
		end := start + len(prefix) + endOffset
		path := strings.TrimSpace(remaining[start+len(prefix) : end])
		resolved, ok := lookupCustomModelRequestVariable(path, variables)
		if !ok {
			return nil, fmt.Errorf("unknown request template variable %q", path)
		}
		available := maxStringBytes - int64(result.Len())
		rendered, err := customModelRequestVariableString(resolved, available)
		if err != nil {
			return nil, err
		}
		if err := appendCustomModelRenderedString(&result, rendered, maxStringBytes); err != nil {
			return nil, err
		}
		remaining = remaining[end+len(suffix):]
	}
}

func appendCustomModelRenderedString(builder *strings.Builder, value string, maxBytes int64) error {
	if maxBytes < 0 || int64(len(value)) > maxBytes || int64(builder.Len()) > maxBytes-int64(len(value)) {
		return fmt.Errorf("rendered request template value exceeds %d bytes", maxBytes)
	}
	_, _ = builder.WriteString(value)
	return nil
}

func lookupCustomModelRequestVariable(path string, variables map[string]any) (any, bool) {
	if !strings.HasPrefix(path, "request.") {
		return nil, false
	}
	parts := strings.Split(strings.TrimPrefix(path, "request."), ".")
	current := variables["request"]
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = typed[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		case []string:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func customModelRequestVariableString(value any, maxBytes int64) (string, error) {
	switch typed := value.(type) {
	case string:
		if int64(len(typed)) > maxBytes {
			return "", fmt.Errorf("rendered request template value exceeds %d bytes", maxBytes)
		}
		return typed, nil
	case nil:
		return "", nil
	default:
		fits, err := customModelJSONValueFitsWithinLimit(typed, maxBytes)
		if err != nil {
			return "", err
		}
		if !fits {
			return "", fmt.Errorf("rendered request template value exceeds %d bytes", maxBytes)
		}
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", fmt.Errorf("marshal request template variable: %w", err)
		}
		if int64(len(encoded)) > maxBytes {
			return "", fmt.Errorf("rendered request template value exceeds %d bytes", maxBytes)
		}
		return string(encoded), nil
	}
}

func customModelAdaptedRequestBodyLimit(inputBytes int) int64 {
	inputSize := int64(inputBytes)
	encodedSize := int64(base64.StdEncoding.EncodedLen(inputBytes))
	if encodedSize < inputSize {
		encodedSize = inputSize
	}
	maxInt64 := int64(^uint64(0) >> 1)
	if encodedSize > maxInt64-maxCustomModelAdaptedRequestOverhead {
		return maxInt64
	}
	return encodedSize + maxCustomModelAdaptedRequestOverhead
}

func marshalCustomModelAdaptedRequestBody(value any, maxBytes int64) ([]byte, error) {
	fits, err := customModelJSONValueFitsWithinLimit(value, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("size adapted image request body: %w", err)
	}
	if !fits {
		return nil, fmt.Errorf("adapted image request body exceeds %d bytes", maxBytes)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal adapted image request body: %w", err)
	}
	if int64(len(encoded)) > maxBytes {
		return nil, fmt.Errorf("adapted image request body exceeds %d bytes", maxBytes)
	}
	return encoded, nil
}

type customModelJSONSizeCounter struct {
	remaining int64
	exceeded  bool
}

func customModelJSONValueFitsWithinLimit(value any, maxBytes int64) (bool, error) {
	if maxBytes < 0 {
		return false, nil
	}
	counter := &customModelJSONSizeCounter{remaining: maxBytes}
	if err := counter.addValue(value); err != nil {
		return false, err
	}
	return !counter.exceeded, nil
}

func (c *customModelJSONSizeCounter) consume(size int64) {
	if c.exceeded {
		return
	}
	if size < 0 || size > c.remaining {
		c.exceeded = true
		return
	}
	c.remaining -= size
}

func (c *customModelJSONSizeCounter) addValue(value any) error {
	if c.exceeded {
		return nil
	}
	switch typed := value.(type) {
	case nil:
		c.consume(4)
	case bool:
		if typed {
			c.consume(4)
		} else {
			c.consume(5)
		}
	case string:
		c.addString(typed)
	case float64:
		if math.IsInf(typed, 0) || math.IsNaN(typed) {
			return fmt.Errorf("unsupported JSON number")
		}
		c.consume(int64(len(strconv.FormatFloat(typed, 'g', -1, 64))))
	case float32:
		if math.IsInf(float64(typed), 0) || math.IsNaN(float64(typed)) {
			return fmt.Errorf("unsupported JSON number")
		}
		c.consume(int64(len(strconv.FormatFloat(float64(typed), 'g', -1, 32))))
	case int:
		c.consume(int64(len(strconv.FormatInt(int64(typed), 10))))
	case int8:
		c.consume(int64(len(strconv.FormatInt(int64(typed), 10))))
	case int16:
		c.consume(int64(len(strconv.FormatInt(int64(typed), 10))))
	case int32:
		c.consume(int64(len(strconv.FormatInt(int64(typed), 10))))
	case int64:
		c.consume(int64(len(strconv.FormatInt(typed, 10))))
	case uint:
		c.consume(int64(len(strconv.FormatUint(uint64(typed), 10))))
	case uint8:
		c.consume(int64(len(strconv.FormatUint(uint64(typed), 10))))
	case uint16:
		c.consume(int64(len(strconv.FormatUint(uint64(typed), 10))))
	case uint32:
		c.consume(int64(len(strconv.FormatUint(uint64(typed), 10))))
	case uint64:
		c.consume(int64(len(strconv.FormatUint(typed, 10))))
	case map[string]any:
		return c.addMap(typed)
	case map[string]string:
		c.consume(2)
		index := 0
		for key, item := range typed {
			if index > 0 {
				c.consume(1)
			}
			c.addString(key)
			c.consume(1)
			c.addString(item)
			index++
		}
	case []any:
		return c.addSlice(typed)
	case []string:
		c.consume(2)
		for index, item := range typed {
			if index > 0 {
				c.consume(1)
			}
			c.addString(item)
		}
	default:
		return fmt.Errorf("unsupported request adapter JSON value %T", value)
	}
	return nil
}

func (c *customModelJSONSizeCounter) addMap(value map[string]any) error {
	c.consume(2)
	index := 0
	for key, item := range value {
		if index > 0 {
			c.consume(1)
		}
		c.addString(key)
		c.consume(1)
		if err := c.addValue(item); err != nil {
			return err
		}
		index++
	}
	return nil
}

func (c *customModelJSONSizeCounter) addSlice(value []any) error {
	c.consume(2)
	for index, item := range value {
		if index > 0 {
			c.consume(1)
		}
		if err := c.addValue(item); err != nil {
			return err
		}
	}
	return nil
}

func (c *customModelJSONSizeCounter) addString(value string) {
	c.consume(2)
	for index := 0; index < len(value) && !c.exceeded; {
		current := value[index]
		if current < utf8.RuneSelf {
			switch current {
			case '\\', '"', '\b', '\f', '\n', '\r', '\t':
				c.consume(2)
			case '<', '>', '&':
				c.consume(6)
			default:
				if current < 0x20 {
					c.consume(6)
				} else {
					c.consume(1)
				}
			}
			index++
			continue
		}
		runeValue, size := utf8.DecodeRuneInString(value[index:])
		switch {
		case runeValue == utf8.RuneError && size == 1:
			c.consume(3)
		case runeValue == '\u2028' || runeValue == '\u2029':
			c.consume(6)
		default:
			c.consume(int64(size))
		}
		index += size
	}
}

func openAIImagesJSONAdapterBase(body []byte, contentType string) (map[string]any, error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		return openAIImagesMultipartToJSON(body, params["boundary"])
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("custom model JSON adapter requires an object request body: %w", err)
	}
	return payload, nil
}

func openAIImagesMultipartToJSON(body []byte, boundary string) (map[string]any, error) {
	boundary = strings.TrimSpace(boundary)
	if boundary == "" {
		return nil, fmt.Errorf("multipart boundary is required")
	}
	payload := make(map[string]any)
	fileValues := make(map[string][]string)
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	partCount := 0
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read multipart adapter field: %w", err)
		}
		partCount++
		if partCount > openAIImageMaxMultipartParts {
			_ = part.Close()
			return nil, fmt.Errorf("multipart request exceeds %d parts", openAIImageMaxMultipartParts)
		}
		name := strings.TrimSpace(part.FormName())
		data, readErr := readOpenAIImageMultipartPart(part, name)
		_ = part.Close()
		if readErr != nil {
			return nil, readErr
		}
		if name == "" {
			continue
		}
		if strings.TrimSpace(part.FileName()) != "" {
			contentType := strings.TrimSpace(part.Header.Get("Content-Type"))
			if contentType == "" || strings.EqualFold(contentType, "application/octet-stream") {
				contentType = http.DetectContentType(data)
			}
			fieldName := name
			if strings.HasPrefix(fieldName, "image[") {
				fieldName = "image"
			}
			fileValues[fieldName] = append(fileValues[fieldName],
				"data:"+contentType+";base64,"+base64.StdEncoding.EncodeToString(data))
			continue
		}
		payload[name] = parseCustomModelMultipartScalar(name, strings.TrimSpace(string(data)))
	}
	for name, values := range fileValues {
		payload[name] = values
	}
	return payload, nil
}

func parseCustomModelMultipartScalar(name, value string) any {
	switch name {
	case "n", "output_compression", "partial_images":
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	case "stream", "return_base64":
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return value
}

func mergeCustomModelRequestBody(base, patch map[string]any) map[string]any {
	out := cloneCustomModelRequestBody(base)
	for key, value := range patch {
		if value == nil {
			delete(out, key)
			continue
		}
		patchMap, patchIsMap := value.(map[string]any)
		baseMap, baseIsMap := out[key].(map[string]any)
		if patchIsMap && baseIsMap {
			out[key] = mergeCustomModelRequestBody(baseMap, patchMap)
			continue
		}
		if patchIsMap {
			out[key] = mergeCustomModelRequestBody(map[string]any{}, patchMap)
			continue
		}
		out[key] = value
	}
	return out
}

func cloneCustomModelRequestBody(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		if nested, ok := value.(map[string]any); ok {
			out[key] = cloneCustomModelRequestBody(nested)
			continue
		}
		out[key] = value
	}
	return out
}
