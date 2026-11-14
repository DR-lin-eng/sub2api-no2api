# HTTP 200 中的 Responses 失败与断流恢复

对应客户端 `stream disconnected before completion: Upstream request failed` 的代码修复与 Docker 验证。HTTP 200 只确认传输已经建立；内部 `error` / `response.failed` 或没有终止事件的 EOF 仍属于失败。

- [VERIFICATION.md](VERIFICATION.md)：触发路径、实现、安全边界、实际验证结果与回退。
- [RESULTS.json](RESULTS.json)：逐场景上游尝试、正文/身份摘要、客户端终止事件及 PostgreSQL 结算核对。
- [CHECKS.json](CHECKS.json)：基础失败回放、专项/完整回归与静态检查记录、原始日志摘要。
- `provider.go`：可以产生一次性 200 错误、持续失败、输出后错误、正常 EOF 和网络截断的合成上游。

从仓库根目录执行，要求 Python 3.14、Docker、现有 Go 缓存和[基础运行镜像](../issue-8636-20261112/compose.yaml)：

```sh
python3 diagnostics/issue-8636-20261112/run.py \
  --expect-recovery --expect-http200-recovery \
  --results /private/tmp/sub2api-http200-stream-20261113 \
  --evidence-dir diagnostics/http200-stream-20261113 \
  --provider-source diagnostics/http200-stream-20261113/provider.go
```

构建与执行均在 Docker 中完成，源码只读；运行环境使用内部网络，只有合成 OAuth 凭据，不访问真实 OpenAI。退出时删除本次容器、网络和临时数据库。原始日志、二进制和详细测试 JSONL 保存在上述临时目录。
