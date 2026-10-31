package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"regexp"
	"strconv"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
)

const (
	modelAttributeMaxFieldsPerModality = 64
	modelAttributeMaxEnumValues        = 64
	modelAttributeMaxSerializedBytes   = 32 << 10
	modelAttributeMaxFormValueBytes    = 4 << 10
)

// 模态渲染模式：replace 表示前端只渲染模板声明的字段，
// overlay 表示在硬编码参数之上用同名字段替换对应控件。
const (
	modelAttributeRenderModeReplace = "replace"
	modelAttributeRenderModeOverlay = "overlay"
)

var (
	modelAttributeKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	modelAttributeModalities = map[string]struct{}{
		"image": {},
		"video": {},
		"audio": {},
	}
)

// ModelAttributeField 描述模型在某个模态下接受的一个输入字段。
// 字段名、类型与取值全部来自配置，代码里不出现任何具体参数名。
type ModelAttributeField struct {
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	Values   []string `json:"values,omitempty"`
	Default  any      `json:"default,omitempty"`
	Required bool     `json:"required,omitempty"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	UI       string   `json:"ui,omitempty"`
	Label    string   `json:"label,omitempty"`
}

// normalizeModelAttributes 规范化整份 attributes，返回值可直接写库。
// nil 或空对象表示不约束。
func normalizeModelAttributes(raw map[string]any) (map[string]any, error) {
	normalized := make(map[string]any, len(raw))
	for key, value := range raw {
		modality := strings.ToLower(strings.TrimSpace(key))
		if _, ok := modelAttributeModalities[modality]; !ok {
			return nil, modelAttributesInvalid(fmt.Sprintf("unsupported modality %q", key))
		}
		if _, exists := normalized[modality]; exists {
			return nil, modelAttributesInvalid(fmt.Sprintf("duplicate modality %q", key))
		}
		block, ok := value.(map[string]any)
		if !ok {
			return nil, modelAttributesInvalid(fmt.Sprintf("modality %q must be an object", modality))
		}
		fields, err := normalizeModelAttributeFields(block, modality)
		if err != nil {
			return nil, err
		}
		hidden, err := normalizeModelAttributeHiddenKeys(block, modality)
		if err != nil {
			return nil, err
		}
		if len(fields) == 0 && len(hidden) == 0 {
			continue
		}
		plain := []any{}
		if len(fields) > 0 {
			encoded, err := json.Marshal(fields)
			if err != nil {
				return nil, modelAttributesInvalid(fmt.Sprintf("modality %q cannot be encoded", modality))
			}
			if err := json.Unmarshal(encoded, &plain); err != nil {
				return nil, modelAttributesInvalid(fmt.Sprintf("modality %q cannot be encoded", modality))
			}
		}
		normalizedBlock := map[string]any{
			"mode":   modelAttributeRenderMode(block),
			"fields": plain,
		}
		if len(hidden) > 0 {
			normalizedBlock["hide"] = hidden
		}
		normalized[modality] = normalizedBlock
	}
	if len(normalized) == 0 {
		return map[string]any{}, nil
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, modelAttributesInvalid("model attributes cannot be encoded")
	}
	if len(payload) > modelAttributeMaxSerializedBytes {
		return nil, modelAttributesInvalid(
			fmt.Sprintf("model attributes exceed %d bytes", modelAttributeMaxSerializedBytes),
		)
	}
	return normalized, nil
}

// modelAttributeFields 取出某模态的字段声明；未配置返回 nil。
func modelAttributeFields(attributes map[string]any, modality string) ([]ModelAttributeField, error) {
	block := modelAttributesForModality(attributes, modality)
	if len(block) == 0 {
		return nil, nil
	}
	return normalizeModelAttributeFields(block, strings.ToLower(strings.TrimSpace(modality)))
}

// modelAttributesForModality 返回某个模态的配置块，例如 {"mode": "replace", "fields": [...]}。
func modelAttributesForModality(attributes map[string]any, modality string) map[string]any {
	modality = strings.ToLower(strings.TrimSpace(modality))
	if modality == "" || len(attributes) == 0 {
		return nil
	}
	raw, ok := attributes[modality]
	if !ok {
		return nil
	}
	block, ok := raw.(map[string]any)
	if !ok || len(block) == 0 {
		return nil
	}
	return block
}

// modelAttributeRenderMode 归一化模态渲染模式；未配置或无法识别时回退为 overlay。
func modelAttributeRenderMode(block map[string]any) string {
	raw, ok := block["mode"]
	if !ok {
		return modelAttributeRenderModeOverlay
	}
	mode, _ := raw.(string)
	if strings.ToLower(strings.TrimSpace(mode)) == modelAttributeRenderModeReplace {
		return modelAttributeRenderModeReplace
	}
	return modelAttributeRenderModeOverlay
}

func normalizeModelAttributeFields(block map[string]any, modality string) ([]ModelAttributeField, error) {
	for key := range block {
		if key != "fields" && key != "mode" && key != "hide" {
			return nil, modelAttributesInvalid(
				fmt.Sprintf("modality %q only accepts \"mode\", \"fields\" and \"hide\"", modality),
			)
		}
	}
	if rawMode, ok := block["mode"]; ok {
		mode, _ := rawMode.(string)
		switch strings.ToLower(strings.TrimSpace(mode)) {
		case "", modelAttributeRenderModeReplace, modelAttributeRenderModeOverlay:
		default:
			return nil, modelAttributesInvalid(
				fmt.Sprintf("modality %q has unsupported render mode %v", modality, rawMode),
			)
		}
	}
	rawFields, ok := block["fields"]
	if !ok || rawFields == nil {
		return nil, nil
	}
	items, ok := rawFields.([]any)
	if !ok {
		return nil, modelAttributesInvalid(fmt.Sprintf("%s.fields must be an array", modality))
	}
	if len(items) > modelAttributeMaxFieldsPerModality {
		return nil, modelAttributesInvalid(
			fmt.Sprintf("%s.fields exceeds %d items", modality, modelAttributeMaxFieldsPerModality),
		)
	}
	fields := make([]ModelAttributeField, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, modelAttributesInvalid(
				fmt.Sprintf("%s.fields[%d] must be an object", modality, index),
			)
		}
		field, err := decodeModelAttributeField(object)
		if err != nil {
			return nil, err
		}
		field, err = field.normalize()
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[field.Key]; duplicate {
			return nil, modelAttributesInvalid(
				fmt.Sprintf("%s.fields contains duplicate key %q", modality, field.Key),
			)
		}
		seen[field.Key] = struct{}{}
		fields = append(fields, field)
	}
	return fields, nil
}

// normalizeModelAttributeHiddenKeys 归一化 hide 列表：只校验字段名合法性与去重，
// 具体隐藏哪些控件由前端按自身控件绑定解释。
func normalizeModelAttributeHiddenKeys(block map[string]any, modality string) ([]string, error) {
	raw, ok := block["hide"]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, modelAttributesInvalid(fmt.Sprintf("%s.hide must be an array", modality))
	}
	if len(items) > modelAttributeMaxFieldsPerModality {
		return nil, modelAttributesInvalid(
			fmt.Sprintf("%s.hide exceeds %d items", modality, modelAttributeMaxFieldsPerModality),
		)
	}
	keys := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		key, ok := item.(string)
		if !ok {
			return nil, modelAttributesInvalid(
				fmt.Sprintf("%s.hide[%d] must be a string", modality, index),
			)
		}
		key = strings.TrimSpace(key)
		if !modelAttributeKeyPattern.MatchString(key) {
			return nil, modelAttributesInvalid(
				fmt.Sprintf("%s.hide[%d] has invalid key %q", modality, index, key),
			)
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}

func decodeModelAttributeField(object map[string]any) (ModelAttributeField, error) {
	encoded, err := json.Marshal(object)
	if err != nil {
		return ModelAttributeField{}, modelAttributesInvalid("attribute field cannot be decoded")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var field ModelAttributeField
	if err := decoder.Decode(&field); err != nil {
		return ModelAttributeField{}, modelAttributesInvalid(
			fmt.Sprintf("attribute field is invalid: %v", err),
		)
	}
	return field, nil
}

func (f ModelAttributeField) normalize() (ModelAttributeField, error) {
	f.Key = strings.TrimSpace(f.Key)
	f.Type = strings.ToLower(strings.TrimSpace(f.Type))
	f.UI = strings.ToLower(strings.TrimSpace(f.UI))
	f.Label = strings.TrimSpace(f.Label)

	if !modelAttributeKeyPattern.MatchString(f.Key) {
		return f, modelAttributesInvalid(
			fmt.Sprintf("attribute key %q must match %s", f.Key, modelAttributeKeyPattern.String()),
		)
	}
	switch f.Type {
	case "string", "integer", "number", "boolean", "enum":
	default:
		return f, modelAttributesInvalid(
			fmt.Sprintf("attribute %q has unsupported type %q", f.Key, f.Type),
		)
	}
	switch f.UI {
	case "":
		f.UI = "auto"
	case "auto", "hidden":
	default:
		return f, modelAttributesInvalid(
			fmt.Sprintf("attribute %q has unsupported ui %q", f.Key, f.UI),
		)
	}
	if f.Label == "" {
		f.Label = f.Key
	}

	if f.Type == "enum" {
		if len(f.Values) == 0 {
			return f, modelAttributesInvalid(fmt.Sprintf("enum attribute %q requires values", f.Key))
		}
		if len(f.Values) > modelAttributeMaxEnumValues {
			return f, modelAttributesInvalid(
				fmt.Sprintf("enum attribute %q exceeds %d candidates", f.Key, modelAttributeMaxEnumValues),
			)
		}
		candidates := make([]string, 0, len(f.Values))
		seen := make(map[string]struct{}, len(f.Values))
		for _, value := range f.Values {
			value = strings.TrimSpace(value)
			if value == "" {
				return f, modelAttributesInvalid(
					fmt.Sprintf("enum attribute %q has an empty candidate", f.Key),
				)
			}
			if _, duplicate := seen[value]; duplicate {
				continue
			}
			seen[value] = struct{}{}
			candidates = append(candidates, value)
		}
		f.Values = candidates
	} else if len(f.Values) > 0 {
		return f, modelAttributesInvalid(
			fmt.Sprintf("attribute %q only allows values when type is enum", f.Key),
		)
	}

	if f.Min != nil || f.Max != nil {
		if f.Type != "integer" && f.Type != "number" {
			return f, modelAttributesInvalid(
				fmt.Sprintf("attribute %q only allows min/max when type is numeric", f.Key),
			)
		}
		if f.Min != nil && f.Max != nil && *f.Min > *f.Max {
			return f, modelAttributesInvalid(fmt.Sprintf("attribute %q has min greater than max", f.Key))
		}
	}

	if f.Default != nil {
		value, ok := normalizeModelAttributeValue(f.Type, f.Default)
		if !ok {
			return f, modelAttributesInvalid(
				fmt.Sprintf("attribute %q default does not match type %q", f.Key, f.Type),
			)
		}
		if err := f.validateValue(value); err != nil {
			return f, err
		}
		f.Default = value
	}
	return f, nil
}

// validateModelAttributeValues 按字段声明校验客户端提交值；未提交的键不参与校验。
func validateModelAttributeValues(fields []ModelAttributeField, values map[string]any) error {
	for _, field := range fields {
		value, present := values[field.Key]
		if !present || value == nil {
			if field.Required && field.Default == nil {
				return infraerrors.BadRequest(
					"MODEL_ATTRIBUTE_REQUIRED",
					fmt.Sprintf("model attribute %q is required", field.Key),
				)
			}
			continue
		}
		if err := field.validateValue(value); err != nil {
			return err
		}
	}
	return nil
}

func (f ModelAttributeField) validateValue(value any) error {
	normalized, ok := normalizeModelAttributeValue(f.Type, value)
	if !ok {
		return infraerrors.BadRequest(
			"MODEL_ATTRIBUTE_INVALID",
			fmt.Sprintf("model attribute %q expects %s", f.Key, f.Type),
		)
	}
	switch f.Type {
	case "enum":
		text, _ := normalized.(string)
		for _, candidate := range f.Values {
			if candidate == text {
				return nil
			}
		}
		return infraerrors.BadRequest(
			"MODEL_ATTRIBUTE_INVALID",
			fmt.Sprintf("model attribute %q must be one of %s", f.Key, strings.Join(f.Values, ", ")),
		)
	case "integer":
		number, _ := normalized.(int64)
		return f.validateRange(float64(number))
	case "number":
		number, _ := normalized.(float64)
		return f.validateRange(number)
	}
	return nil
}

func (f ModelAttributeField) validateRange(value float64) error {
	if f.Min != nil && value < *f.Min {
		return infraerrors.BadRequest(
			"MODEL_ATTRIBUTE_INVALID",
			fmt.Sprintf("model attribute %q must be >= %s", f.Key, formatModelAttributeNumber(*f.Min)),
		)
	}
	if f.Max != nil && value > *f.Max {
		return infraerrors.BadRequest(
			"MODEL_ATTRIBUTE_INVALID",
			fmt.Sprintf("model attribute %q must be <= %s", f.Key, formatModelAttributeNumber(*f.Max)),
		)
	}
	return nil
}

// injectModelAttributeDefaults 仅在 application/json 请求体里补写必填字段的默认值。
func injectModelAttributeDefaults(
	body []byte,
	contentType string,
	fields []ModelAttributeField,
) ([]byte, error) {
	if len(fields) == 0 || !isModelAttributeJSONContentType(contentType) {
		return body, nil
	}
	payload, ok := decodeModelAttributeJSONBody(body)
	if !ok {
		return body, nil
	}
	changed := false
	for _, field := range fields {
		if !field.Required || field.Default == nil {
			continue
		}
		if _, exists := payload[field.Key]; exists {
			continue
		}
		payload[field.Key] = field.Default
		changed = true
	}
	if !changed {
		return body, nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return body, nil
	}
	return encoded, nil
}

// applyModelAttributes 按模型属性校验请求并补写默认值；未配置属性时原样返回 body。
func applyModelAttributes(
	ctx context.Context,
	resolver CustomModelCapabilityResolver,
	modelName string,
	modality string,
	values map[string]any,
	body []byte,
	contentType string,
) ([]byte, error) {
	if resolver == nil {
		return body, nil
	}
	fields, err := resolveModelAttributeFields(ctx, resolver, modelName, modality)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return body, nil
	}
	if strings.TrimSpace(contentType) == "" {
		// 部分视频端点在转发前才归一化 Content-Type，未标注时按 JSON 处理。
		contentType = "application/json"
	}
	switch {
	case isModelAttributeJSONContentType(contentType):
		if decoded, ok := decodeModelAttributeJSONBody(body); ok {
			values = decoded
		}
	case isModelAttributeMultipartContentType(contentType):
		values = modelAttributeMultipartValues(body, contentType, fields, values)
	}
	if err := validateModelAttributeValues(fields, values); err != nil {
		return nil, err
	}
	return injectModelAttributeDefaults(body, contentType, fields)
}

func decodeModelAttributeJSONBody(body []byte) (map[string]any, bool) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, false
	}
	return payload, true
}

func modelAttributeMultipartValues(
	body []byte,
	contentType string,
	fields []ModelAttributeField,
	values map[string]any,
) map[string]any {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil || strings.TrimSpace(params["boundary"]) == "" {
		return values
	}
	declared := make(map[string]ModelAttributeField, len(fields))
	for _, field := range fields {
		declared[field.Key] = field
	}
	merged := make(map[string]any, len(values)+len(declared))
	for key, value := range values {
		merged[key] = value
	}
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		name := strings.TrimSpace(part.FormName())
		field, ok := declared[name]
		if !ok || part.FileName() != "" {
			_ = part.Close()
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(part, modelAttributeMaxFormValueBytes))
		_ = part.Close()
		if readErr != nil {
			break
		}
		if _, exists := merged[name]; exists {
			continue
		}
		merged[name] = parseModelAttributeFormValue(field.Type, string(data))
	}
	return merged
}

// parseModelAttributeFormValue 把 multipart 文本值转成字段声明的类型；
// 转换失败时保留原始字符串，交给后续校验报出明确的类型错误。
func parseModelAttributeFormValue(fieldType, raw string) any {
	switch fieldType {
	case "integer":
		if parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
			return parsed
		}
	case "number":
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
			return parsed
		}
	case "boolean":
		if parsed, err := strconv.ParseBool(strings.TrimSpace(raw)); err == nil {
			return parsed
		}
	}
	return raw
}

func normalizeModelAttributeValue(fieldType string, value any) (any, bool) {
	switch fieldType {
	case "string", "enum":
		text, ok := value.(string)
		return text, ok
	case "integer":
		switch typed := value.(type) {
		case int:
			return int64(typed), true
		case int32:
			return int64(typed), true
		case int64:
			return typed, true
		case float32:
			return modelAttributeInteger(float64(typed))
		case float64:
			return modelAttributeInteger(typed)
		case json.Number:
			parsed, err := typed.Int64()
			return parsed, err == nil
		}
	case "number":
		switch typed := value.(type) {
		case int:
			return float64(typed), true
		case int32:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case float32:
			return float64(typed), true
		case float64:
			return typed, true
		case json.Number:
			parsed, err := typed.Float64()
			return parsed, err == nil
		}
	case "boolean":
		flag, ok := value.(bool)
		return flag, ok
	}
	return nil, false
}

func modelAttributeInteger(value float64) (any, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) {
		return nil, false
	}
	return int64(value), true
}

func isModelAttributeJSONContentType(contentType string) bool {
	mediaType := modelAttributeMediaType(contentType)
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func isModelAttributeMultipartContentType(contentType string) bool {
	return modelAttributeMediaType(contentType) == "multipart/form-data"
}

func modelAttributeMediaType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func formatModelAttributeNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func modelAttributesInvalid(message string) error {
	return infraerrors.BadRequest("MODEL_ATTRIBUTES_INVALID", message)
}
