package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
)

// geminiContentSessionSeedPrefix 区分 Gemini 内容派生的会话种子与其他来源的种子。
const geminiContentSessionSeedPrefix = "gemini_cs_"

// GenerateGeminiContentSessionHash 生成 Gemini 原生请求的内容粘性会话哈希。
// 种子由 API Key、模型、tools、systemInstruction 与第一条 user contents 的 parts 组成，
// 后四项的取法与 OpenAI 内容兜底（deriveOpenAIContentSessionSeed）一致，API Key 用于隔离租户。
// 这些字段不变时，末尾追加、改写或替换 contents 都不改变哈希；字段全部相同的不同会话共享同一哈希。
// systemInstruction 与 system_instruction 同时存在时取前者。
// 找不到第一条 user contents（role 为 user 或省略），或其 parts 中没有有效 part 时返回空串，不建立粘性。
func GenerateGeminiContentSessionHash(apiKeyID int64, model string, body []byte) string {
	root := parseRawJSONView(body)
	if !root.IsObject() {
		return ""
	}
	firstUserParts := firstGeminiUserParts(root.Get("contents"))
	if !firstUserParts.Exists() {
		return ""
	}

	d := xxhash.New()
	_, _ = d.WriteString(geminiContentSessionSeedPrefix)
	_, _ = d.WriteString("key=")
	_, _ = d.WriteString(strconv.FormatInt(apiKeyID, 10))
	if model = strings.TrimSpace(model); model != "" {
		_, _ = d.WriteString("|model=")
		_, _ = d.WriteString(model)
	}
	if tools := root.Get("tools"); tools.IsArray() {
		if normalized, ok := normalizeGeminiSessionSeedJSON(tools); ok {
			_, _ = d.WriteString("|tools=")
			_, _ = d.WriteString(normalized)
		}
	}
	system := root.Get("systemInstruction")
	if !system.Exists() {
		system = root.Get("system_instruction")
	}
	if normalized, ok := normalizeGeminiSessionSeedJSON(system); ok {
		_, _ = d.WriteString("|system=")
		_, _ = d.WriteString(normalized)
	}
	_, _ = d.WriteString("|first_user=")
	_, _ = d.WriteString(canonicalGeminiSessionSeed(firstUserParts.Raw))
	return fmt.Sprintf("%016x", d.Sum64())
}

// firstGeminiUserParts 返回第一条 user contents（role 为 user 或省略）的 parts，跳过它之前的
// model 等其他角色；该条不是对象或 parts 中没有有效 part 时返回空结果。
func firstGeminiUserParts(contents gjson.Result) gjson.Result {
	var parts gjson.Result
	if !contents.IsArray() {
		return parts
	}
	contents.ForEach(func(_, content gjson.Result) bool {
		if content.IsObject() {
			if role := content.Get("role").String(); role != "" && role != "user" {
				return true
			}
			if p := content.Get("parts"); hasMeaningfulGeminiParts(p) {
				parts = p
			}
		}
		return false
	})
	return parts
}

// hasMeaningfulGeminiParts 判断 parts 中是否至少有一个有效 part：text 去除首尾空白后非空，
// 或是至少含一个字段的其他 part（如 inlineData、fileData）。
func hasMeaningfulGeminiParts(parts gjson.Result) bool {
	if !parts.IsArray() {
		return false
	}
	meaningful := false
	parts.ForEach(func(_, part gjson.Result) bool {
		if text := part.Get("text"); text.Exists() {
			meaningful = strings.TrimSpace(text.String()) != ""
		} else if part.IsObject() {
			part.ForEach(func(_, _ gjson.Result) bool {
				meaningful = true
				return false
			})
		}
		return !meaningful
	})
	return meaningful
}

func normalizeGeminiSessionSeedJSON(value gjson.Result) (string, bool) {
	if !value.Exists() || value.Type == gjson.Null {
		return "", false
	}
	normalized := canonicalGeminiSessionSeed(value.Raw)
	return normalized, normalized != "" && normalized != "{}" && normalized != "[]" && normalized != "null"
}
func canonicalGeminiSessionSeed(raw string) string {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return raw
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return string(normalized)
}
