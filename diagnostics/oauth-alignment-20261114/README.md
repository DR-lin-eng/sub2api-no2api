# 2026-11-14 OAuth 请求与压缩对齐

本目录对照用户提供的 20261112 桌面逆向报告，覆盖 OpenAI OAuth Responses、compact、自动透传和 ChatGPT App 图片请求的身份连续性与压缩边界。

实现要点：

- 每条新 OAuth HTTP 请求仍使用完整 zstd level 3 帧；只有精确裸 400 或明确 zstd/`Content-Encoding` 的 415 才允许在同一账号、同一出口做一次 identity 重发。
- 重发前读取实际发送快照，核验压缩字节长度、完整 zstd 帧、UTF-8 JSON 对象和原始 JSON hash；请求转换、账号压缩设置和身份字段保持不变。
- Responses 上游 `deflate` 响应同时识别标准 zlib 封装和历史 raw DEFLATE；解压后移除旧编码和长度头。
- ChatGPT App 图片请求在 prepare、上传、生成、恢复和下载发送点复核账号、凭据主体和用户/套餐类别；主体变化返回 `account_changed`，不把当前生成重放到新主体。

`run.py` 使用 Docker 内部网络、合成 OAuth 账号、PostgreSQL、Redis 和可控上游执行 BASELINE→MODIFIED→ROLLBACK→RESTORED。它不会访问真实 OpenAI、账号、token 或 cookie。上游捕获只保存 wire hash、JSON hash、字节数、编码和脱敏身份摘要。

```sh
python3 diagnostics/oauth-alignment-20261114/run.py --baseline-ref HEAD
```

本轮结果见 [RESULTS.json](RESULTS.json)：四阶段各 52 个场景；基线/回滚 34 个成功结算，修改/恢复 40 个；所有阶段没有重复结算或失败请求用量。完整包回归在 Docker 中运行：

```text
go test ./internal/application/service ./internal/infrastructure/repository ./internal/shared/httputil
```

报告结论仍只适用于合成上游和当前源码；它不能证明真实 OAuth 部署接受 zstd，也不能替代真实账号验证。
