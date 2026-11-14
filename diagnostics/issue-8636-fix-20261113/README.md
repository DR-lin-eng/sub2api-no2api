# 保留 zstd 的 Codex HTTP 400 恢复

对应 [issue #8636](https://github.com/Wei-Shaw/sub2api/issues/8636) 的后续修复。每条新请求继续默认启用 zstd；收到精确的裸 400 或显式编码 415，且满足安全重放条件时，在同一账号上用未压缩的完整请求重试一次。该兼容重试不写入账号配置，下一条新请求仍使用 zstd。

- [VERIFICATION.md](VERIFICATION.md)：实现边界、原始代码对照、Docker 验证及升级回退。
- [RESULTS.json](RESULTS.json)：HTTP 场景、压缩状态、输入/身份摘要与数据库结算结果。
- [CHECKS.json](CHECKS.json)：单测、网关回归、静态检查和原始日志 SHA-256。
- `provider.go`：支持真实 zstd 解码并核对线上字节长度的可控上游，只有合成数据。

复用[原诊断脚本](../issue-8636-20261112/run.py)，从仓库根目录执行（Python 3.14、Docker、已有基础镜像和 Go 缓存）：

```sh
python3 diagnostics/issue-8636-20261112/run.py \
  --expect-recovery \
  --results /private/tmp/sub2api-issue-8636-fix-20261113 \
  --evidence-dir diagnostics/issue-8636-fix-20261113 \
  --provider-source diagnostics/issue-8636-fix-20261113/provider.go
```

脚本从当前源码构建后端及假上游，使用原有 `compose.yaml` 创建独立 PostgreSQL、Redis、应用和上游容器；Docker 内部网络不发布端口且不访问真实 OpenAI 服务。退出时清理本次容器、网络和临时数据卷。原始诊断的 [RESULTS.json](../issue-8636-20261112/RESULTS.json) 和 [CHECKS.json](../issue-8636-20261112/CHECKS.json) 保留。
