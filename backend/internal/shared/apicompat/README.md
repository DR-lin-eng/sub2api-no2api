# apicompat

Anthropic、Chat Completions 与 Responses 的双向协议转换。`*_to_*` 负责转换，`*_bridge*` 负责兼容桥接，`responses_stream_*` 负责流事件，`codex_bootstrap*` 负责识别 Codex Desktop 的合成首轮输入，`types.go` 定义公共结构。

Chat 兼容桥的 `EffectiveResponsesTools` 同时处理 additional_tools 和完成的 tool_search_output；只把相关发现项交给既有冲突/去重 helper，不再次解码完整历史。heartbeat 与 delegation 均由共享 bootstrap owner 严格验证。

`responses_client_tools*` 负责 function 兼容上游的客户端工具降级与恢复。流恢复器与 Chat Completions 回退桥共用增量解码器，逐片解码根级 `input` 字符串，保留跨片的 JSON 转义、Unicode 代理对和 UTF-8 字符；完成事件仅补发尚未输出的后缀。原生 OpenAI Responses 透传直接保留 custom 工具协议。

`reasoning_usage.go` 统一读取上游 reasoning/thoughts 计数，区分未报告与明确的 0，供管理员用量日志使用；它不改变输出 token 总量或计费。
