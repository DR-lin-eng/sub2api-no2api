package service

// DeepSeek 官方 Anthropic 原生端点（/v1/messages）的 thinking 历史补位。
//
// DeepSeek 在 thinking 模式下要求工具续接请求把 assistant 历史里的 thinking
// 块完整回传；缺失时上游会按 tool-call ID 尝试自动恢复，恢复失败返回 400
// "The content[].thinking in the thinking mode must be passed back to the API"。
// 会话历史中由其他后端（如 Ollama 中转）生成的 tool_use 轮没有 thinking 块，
// 被 file-owner 固定到 DeepSeek 官方账号后就会触发该 400。
//
// Chat Completions 链已有同构先例 ensureDeepSeekChatReasoningPlaceholders
// （见 openai_gateway_responses_chat_fallback.go），这里为 Anthropic 原生直通链
// 做对应补位。

import (
	"strings"

	"github.com/tidwall/gjson"
)

// deepSeekAnthropicThinkingPlaceholderBlock 是插到 assistant 历史 content 首位的
// thinking 占位块。thinking 填单个空格而非空串：DeepSeek 可能把空串按缺失处理
// （同 Chat 链 ensureDeepSeekChatReasoningPlaceholders 的结论：DeepSeek 拒绝
// 空串，非空即可通过，LiteLLM 同样注入单个空格）。signature 可以省略：
// DeepSeek 官方文档说明 input 侧的 signature 不做校验。
const deepSeekAnthropicThinkingPlaceholderBlock = `{"type":"thinking","thinking":" "}`

// anthropicBodyThinkingEnabled 报告 Anthropic Messages 请求体是否处于 thinking
// 模式。thinking.type 为 enabled/adaptive 视为启用、disabled 视为关闭；没有
// type 时看 output_config.effort，none 关闭、其余（含缺失）视为启用——DeepSeek
// 对 deepseek-flash 默认启用 thinking。
func anthropicBodyThinkingEnabled(body []byte) bool {
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "thinking.type").String())) {
	case "enabled", "adaptive":
		return true
	case "disabled":
		return false
	}
	effort := strings.TrimSpace(gjson.GetBytes(body, "output_config.effort").String())
	return !strings.EqualFold(effort, "none")
}

// ensureDeepSeekAnthropicThinkingBlocks 给带 tool_use 但缺 thinking 的 assistant
// 历史消息补单个空格占位的 thinking 块（DeepSeek 可能把空串按缺失处理），插在
// content 数组最前面。已有 thinking / redacted_thinking 的消息、只有文本的消息
// 都不改写。
//
// Walk the original immutable JSON and splice only at qualifying array openings.
// Every source byte is copied at most once into the result.
func ensureDeepSeekAnthropicThinkingBlocks(body []byte) []byte {
	if !gjson.ValidBytes(body) {
		return body
	}
	messages := parseRawJSONView(body).Get("messages")
	if !messages.IsArray() {
		return body
	}
	var out []byte
	cursor := 0
	messages.ForEach(func(_, message gjson.Result) bool {
		if strings.TrimSpace(message.Get("role").String()) != "assistant" {
			return true
		}
		content := message.Get("content")
		if !content.IsArray() {
			return true
		}
		hasTool, hasThinking := false, false
		content.ForEach(func(_, block gjson.Result) bool {
			switch block.Get("type").String() {
			case "tool_use":
				hasTool = true
			case "thinking", "redacted_thinking":
				hasThinking = true
			}
			return !hasThinking
		})
		if hasTool && !hasThinking {
			if out == nil {
				out = make([]byte, 0, len(body)+len(deepSeekAnthropicThinkingPlaceholderBlock)+1)
			}
			pos := content.Index + 1
			out = append(out, body[cursor:pos]...)
			out = append(out, deepSeekAnthropicThinkingPlaceholderBlock...)
			out = append(out, ',')
			cursor = pos
		}
		return true
	})
	if out == nil {
		return body
	}
	return append(out, body[cursor:]...)
}

// shouldRepairDeepSeekAnthropicThinking 报告该请求是否需要做 DeepSeek Anthropic
// thinking 补位：仅 DeepSeek 官方上游账号（targetsDeepSeekAPIHost）且请求启用
// thinking 时生效，其他上游保持字节不变。
func shouldRepairDeepSeekAnthropicThinking(account *Account, body []byte) bool {
	if account == nil {
		return false
	}
	return isDeepSeekAPIHost(account.GetAnthropicProtocolBaseURL()) && anthropicBodyThinkingEnabled(body)
}
