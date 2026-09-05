# apicompat

Anthropic、Chat Completions 与 Responses 的双向协议转换。`*_to_*` 负责转换，`*_bridge*` 负责兼容桥接，`responses_stream_*` 负责流事件，`codex_bootstrap*` 负责识别 Codex Desktop 的合成首轮输入，`types.go` 定义公共结构。

Chat 兼容桥的 `EffectiveResponsesTools` 同时处理 additional_tools 和完成的 tool_search_output；只把相关发现项交给既有冲突/去重 helper，不再次解码完整历史。heartbeat 与 delegation 均由共享 bootstrap owner 严格验证。
