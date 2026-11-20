package service

import (
	"github.com/DR-lin-eng/sub2api-no2api/internal/domain"
	"github.com/tidwall/gjson"
)

// gatewayRequestRootFields 保存一次根对象遍历取到的原始值视图。全部零拷贝，
// 只在本次解析内使用，body 被改写后必须重新扫描。
//
// 嵌套字段按 gjson.Get 的语义取值：路径的每一段都取「第一个能解析出子键的同名键」，
// 而不是简单取第一个同名键。例如 {"metadata":"x","metadata":{"user_id":"u"}} 在
// gjson.Get("metadata.user_id") 下取到 "u"，这里以 Exists() 作为「还没取到」的判据
// 复现该重试行为。
type gatewayRequestRootFields struct {
	model          gjson.Result
	stream         gjson.Result
	maxTokens      gjson.Result
	metadataUID    gjson.Result
	thinkingType   gjson.Result
	outputEffort   gjson.Result
	system         gjson.Result
	messages       gjson.Result
	input          gjson.Result
	contents       gjson.Result
	geminiSysParts gjson.Result
}

func scanGatewayRequestRoot(body []byte) gatewayRequestRootFields {
	var f gatewayRequestRootFields
	parseRawJSONView(body).ForEach(func(key, value gjson.Result) bool {
		switch key.Str {
		case "model":
			if !f.model.Exists() {
				f.model = value
			}
		case "stream":
			if !f.stream.Exists() {
				f.stream = value
			}
		case "max_tokens":
			if !f.maxTokens.Exists() {
				f.maxTokens = value
			}
		case "metadata":
			if !f.metadataUID.Exists() {
				f.metadataUID = value.Get("user_id")
			}
		case "thinking":
			if !f.thinkingType.Exists() {
				f.thinkingType = value.Get("type")
			}
		case "output_config":
			if !f.outputEffort.Exists() {
				f.outputEffort = value.Get("effort")
			}
		case "system":
			if !f.system.Exists() {
				f.system = value
			}
		case "messages":
			if !f.messages.Exists() {
				f.messages = value
			}
		case "input":
			if !f.input.Exists() {
				f.input = value
			}
		case "contents":
			if !f.contents.Exists() {
				f.contents = value
			}
		case "systemInstruction":
			if !f.geminiSysParts.Exists() {
				f.geminiSysParts = value.Get("parts")
			}
		}
		return true
	})
	return f
}

func setGatewayRequestRangesFromFields(parsed *ParsedRequest, protocol string, fields gatewayRequestRootFields) {
	if parsed == nil {
		return
	}
	switch protocol {
	case domain.PlatformGemini:
		if fields.geminiSysParts.Exists() && fields.geminiSysParts.IsArray() {
			parsed.systemRange = rangeFromResult(fields.geminiSysParts)
		}
		if fields.contents.Exists() && fields.contents.IsArray() {
			parsed.messagesRange = rangeFromResult(fields.contents)
		}
	default:
		if fields.system.Exists() {
			parsed.HasSystem = true
			parsed.systemRange = rangeFromResult(fields.system)
		}
		if fields.messages.Exists() && fields.messages.IsArray() {
			parsed.messagesRange = rangeFromResult(fields.messages)
		}
		if protocol == "responses" {
			if fields.input.Exists() {
				parsed.inputRange = rangeFromResult(fields.input)
			}
		}
	}
}
