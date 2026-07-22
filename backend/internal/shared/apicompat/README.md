# apicompat

Anthropic、Chat Completions 与 Responses 的双向协议转换。`*_to_*` 负责转换，`*_bridge*` 负责兼容桥接，`responses_stream_*` 负责流事件，`types.go` 定义公共结构。
