# Repositories

本包实现 application 层定义的数据库、Redis、缓存和上游数据端口。package 名保持为 `repository`。

## 文件索引

`gateway_cache_integrity.go` 保存第一方 OAuth integrity 状态的共享读取与 Lua 原子 expected-state CAS。
`account_repo_lenovo_attribution.go` 保存账号归因安装身份、请求租约和结果的凭据/状态 CAS；普通编辑与通用 Extra 补丁保留其当前状态。
`account_repo_location.go` 原子固定账号首次位置偏移；`gateway_cache_location.go` 按固定 TTL 共享按请求生成的坐标和精度。
`gateway_cache_codex_session.go` 原子创建并续期 Codex 虚拟会话身份，保持 UUIDv7 的首次生成时间和跨实例缓存亲和性。

| 前缀 | 职责 |
| --- | --- |
| `account*`, `user*`, `group*`, `api_key*` | 核心身份和账号持久化 |
| `usage_log*`, `usage_billing*`, `billing_cache*` | 用量、计费队列和账务缓存 |
| `usage_billing_video.go` | 视频冻结的行锁事务，原子提交资金、额度、日志和终态 |
| `scheduler*`, `concurrency*`, `session_limit*`, `rpm_cache*` | 调度与并发状态 |
| `gateway_cache_live_billing.go` | Live 终止时长原子冻结、待结算索引和跨实例恢复 |
| `openai_oauth_gateway_rate_limit_cache.go` | OpenAI OAuth 按账号独立、同账号跨实例共享的请求令牌桶和逻辑请求去重 |
| `ops*`, `audit_log*`, `channel_monitor*` | 运维、审计和监控查询 |
| `ops_repo_latency_metrics.go` | TPS 精确分位数与现有 latency/TTFT 查询融合，供 raw/preagg/snapshot 共用，不新增扫描轮次 |
| `cloudflare_ingress*`, `cloudflare_waf*` | Cloudflare Access Rule/WAF 分片客户端、Redis 共享状态、异步同步、统计缓存与到期回收 |
| `email_cache*` | 注册/通知验证码的原子校验及密码重置链接的哈希和旧值兼容消费 |
| `payment*`, `subscription*`, `promo_code*`, `redeem_code*` | 商业对象持久化 |
| `chat*` | 在线客服会话、消息、素材、快捷回复及分批保留清理 |
| `cluster*` | 逻辑节点、runner 历史、共享任务租约与版本发布状态 |
| `*_oauth_*`, `http_upstream*`, `proxy*` | 外部凭据、代理池均衡/健康状态和网络访问实现 |
| `http_upstream_public.go`, `http_upstream_public_proxy.go` | 用户图片下载与显式标记的有界 HTTPS PUT 上传，逐跳公开 IP 校验和 DNS 固定，保留账号出口；上传不携带账号凭据或跟随重定向，HTTP/HTTPS 代理通过 CONNECT 访问固定 IP，SOCKS 代理接收固定 IP |
| `http_upstream_codex_tls.go` | req/v3 实验传输逐连接创建独立 ClientHello 和 key share，避免重连或并发握手复用 uTLS 的可变状态 |
| `oauth2_provider_store.go` | OAuth2 一次性 authorization code 与 access token 摘要的 Redis 存储 |
| `leader_lock_cache.go` | Redis 后台任务租约，按 owner 比较释放/续租，用于长期额度 WS owner |
| `wire.go` | repository provider 集合 |

`account_repo.go` 只保留仓储结构和构造器；账户持久化分别位于 `account_repo_crud.go`, `account_repo_list.go`, `account_repo_credentials.go`, `account_repo_oauth_401_delete.go`, `account_repo_scheduler_cache.go`, `account_repo_scheduling.go`, `account_repo_extra.go`, `account_repo_probe.go`, `account_repo_mapping.go` 和 `account_repo_quota.go`。新增账户查询或写入应进入对应职责文件，不再回填主文件。

SQL、Ent 和 Redis 细节只能停留在本层。复杂文件按 `query/command/cache/batch/recovery` 拆分，事务边界必须保持在同一公开方法内。

`affiliate_budget_lock.go` 串行化邀请人上限计算与发放；`affiliate_refund.go` 按来源订单和历史实际返利幂等追回。返利发放、解冻、转入和追回统一按 affiliate 行 → ledger 行锁顺序执行；解冻只计尚未追回的部分，内部负 quota 表示欠额，公开可用额度不返回负值。`admin_api_key_usage.go` 仅更新当前密钥的使用时间，避免写回旧权限快照。

审核配置写入与分组删除通过 `group_moderation_scope.go` 约定 group → settings 锁顺序；普通删除直接获取写锁，级联删除保留原事务和写锁。配置写入复核仍存在的分组，删除在同一事务清理 `group_ids`，不改写无效历史 JSON。

API Key 列表排序由 `api_key_repo_sort.go` 定义，在 SQL 分页前按第一绑定分组名称排序，无分组项在两个方向都置后，ID 为稳定次序；有序路由绑定和列表预加载继续由现有 owner 维护。

`usage_billing_account_quota.go` 为日/周额度生成有限 SQL 变体，并对被省略的维度校验数据库当前配置；旧提示自动回退完整语句，阈值跨越仍在同一事务发布 scheduler outbox。

`api_key_auth_bloom_cache.go` 用标准 Redis Lua/位图保存共享 API Key 成员预筛；`api_key_auth_bloom_repo.go` 按主键分页预热，并用 PostgreSQL 事务 advisory lock 协调重建与创建。位图固定约 2 MiB，随机无效凭据不新增 key 或写入位图；准备标记与位图同键、构建代次与租约校验避免缓存丢失后发布不完整快照。

`mcp_oauth*` owns durable management grants, digest credentials, transactional replay revocation and the single joined token-authority read. Migration 252 also revokes grants for old-node user mutations during upgrade/rollback.

`user_repo_email_blacklist.go` 为管理端执行单次有界邮箱探针查询；application 按当前邮箱身份规则复核。
批量禁用在同一事务更新普通用户状态并撤销 MCP 授权，再由 application 触发既有鉴权缓存失效。
