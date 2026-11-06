# openai

OpenAI/Codex 客户端识别、OAuth、请求身份和内置 instructions。文件按 `allowed_client`, `oauth`, `request`, `engine_fingerprint_signal`, `instructions*` 组织。

`siwc.go` 保存 Sign in with ChatGPT 的 direct grant 校验及 OIDC audience 兼容解析；授权常量和会话位于 `oauth.go`。流程入口见 [SIWC 说明](../../../../docs/OPENAI_SIWC.md)。
