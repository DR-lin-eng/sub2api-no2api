# Backend Scripts

| 文件 | 作用 |
| --- | --- |
| `resolve-version.sh` | 解析构建版本 |
| `check-source-layout.sh` | 检查目录边界和超长文件基线 |
| `finalize-ingress-reject-cleanup.sql` | 入口拒绝日志清理收尾 SQL |
| `test-responses-image-ssrf.sh`, `responses-image-ssrf-baseline-overlay.json` | 在无外部出口的 Docker 网络中对照复现修复前后的 Responses 图片编辑 SSRF；固定基线 ref，保留测试日志并自动清理容器/网络 |

脚本必须支持从任意工作目录调用，并在失败时返回非零状态。

从仓库根目录执行 `bash backend/scripts/test-responses-image-ssrf.sh`。测试依赖已缓存的 `golang:1.26.9-bookworm` 镜像、Go 模块及构建缓存；默认使用 `sub2api-go-mod-cache` 和 `sub2api-go-build-cache` Docker 卷，可通过 `SUB2API_SSRF_GOMOD_VOLUME` / `SUB2API_SSRF_GOBUILD_VOLUME` 指定。`SUB2API_SSRF_BASELINE_REF` 可替换已确认含漏洞的基线提交。测试使用 `ssrf_docker` tag，与 PostgreSQL/Redis Testcontainers 的 `integration` 入口独立。
