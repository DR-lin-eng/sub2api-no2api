# Codex 间歇性 400 与 zstd 请求压缩诊断

针对 [上游 issue #8636](https://github.com/Wei-Shaw/sub2api/issues/8636)，使用当前源码、合成 OAuth 凭据、PostgreSQL、Redis 和可控 HTTP 上游进行 Docker 对照。这里的假上游明确提供“接受 zstd”“只拒绝 zstd”“所有请求都拒绝”三种模式，用来验证故障机制与网关行为；它不能证明真实 ChatGPT 服务或某次生产故障的根因。

本目录的结果冻结在 `1d3e1ea2b` 的原始诊断。后续[保留压缩的修复与验证](../issue-8636-fix-20261113/README.md)使用同一脚本的 `--expect-recovery` 模式，在独立目录保存新结果；原始结果文件保留。

- [VERIFICATION.md](VERIFICATION.md)：讨论线索、源码结论、测试结果、现有缓解方法及边界。
- [RESULTS.json](RESULTS.json)：每个 HTTP 场景的状态、上游调用次数、压缩/解压字节数、输入摘要和计费检查。
- [CHECKS.json](CHECKS.json)：基础回归统计与原始日志、夹具的 SHA-256。
- `provider.go`：只处理合成数据的假上游。
- `run.py`、`compose.yaml`：可重跑的隔离环境；退出时清理本次容器、网络和数据卷。

从仓库根目录执行，要求 Docker、Python 3.14，以及本机已有的 Go 模块缓存和所列基础镜像：

```sh
python3 diagnostics/issue-8636-20261112/run.py
```

后端和假上游均在 Go 1.26.11 Linux 容器中从源码构建。默认构建阶段使用 `--network none`，测试阶段使用 Docker `internal` 网络，不发布宿主端口；HTTP 客户端通过 `docker exec` 在应用容器内发送请求。运行时镜像仅提供 Alpine 环境与依赖，`/app/sub2api` 被本次源码编译的二进制覆盖。API 测试不构建前端。

默认构建缓存为 `sub2api-go-mod-cache` 和 `sub2api-go-build-cache`；运行时基础镜像为 `sub2api:sync-candidate-20261112`。可在 `compose.yaml` 中替换为依赖兼容的本地运行时镜像。原始单测 JSONL、应用日志与二进制默认保存在 `/private/tmp/sub2api-issue-8636-20261112/`。`--skip-build` 复用该目录已有的后端二进制，仍从源码构建假上游。
