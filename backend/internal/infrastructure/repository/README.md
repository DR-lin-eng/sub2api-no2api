# Repositories

本包实现 application 层定义的数据库、Redis、缓存和上游数据端口。package 名保持为 `repository`。

## 文件索引

| 前缀 | 职责 |
| --- | --- |
| `account*`, `user*`, `group*`, `api_key*` | 核心身份和账号持久化 |
| `usage_log*`, `usage_billing*`, `billing_cache*` | 用量、计费队列和账务缓存 |
| `scheduler*`, `concurrency*`, `session_limit*`, `rpm_cache*` | 调度与并发状态 |
| `ops*`, `audit_log*`, `channel_monitor*` | 运维、审计和监控查询 |
| `payment*`, `subscription*`, `promo_code*`, `redeem_code*` | 商业对象持久化 |
| `*_oauth_*`, `http_upstream*`, `proxy*` | 外部凭据和网络访问实现 |
| `wire.go` | repository provider 集合 |

`account_repo.go` 只保留仓储结构和构造器；账户持久化分别位于 `account_repo_crud.go`, `account_repo_list.go`, `account_repo_credentials.go`, `account_repo_scheduler_cache.go`, `account_repo_scheduling.go`, `account_repo_extra.go`, `account_repo_probe.go`, `account_repo_mapping.go` 和 `account_repo_quota.go`。新增账户查询或写入应进入对应职责文件，不再回填主文件。

SQL、Ent 和 Redis 细节只能停留在本层。复杂文件按 `query/command/cache/batch/recovery` 拆分，事务边界必须保持在同一公开方法内。
