# Backend Scripts

| 文件 | 作用 |
| --- | --- |
| `resolve-version.sh` | 解析构建版本 |
| `check-source-layout.sh` | 检查目录边界和超长文件基线 |
| `finalize-ingress-reject-cleanup.sql` | 入口拒绝日志清理收尾 SQL |

脚本必须支持从任意工作目录调用，并在失败时返回非零状态。
