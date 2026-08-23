# 账号级 IPv6 出口

本文说明账号级 IPv6 出口池的运行边界、数据模型、请求链路和 Linux
部署要求。实现事实源位于 `backend/internal/modules/egress/`、
`backend/internal/platform/egress/` 和
`backend/internal/infrastructure/repository/egress_repo.go`。

## 适用范围

该功能把运营商路由到单机 Linux 节点的 IPv6 前缀按账号分配为稳定源地址。
它不是 HTTP/SOCKS 代理，也不会为前缀内的每个地址创建代理记录。

启用条件：

- 部署节点是 Linux，且 `deployment.mode=standalone`。
- 运营商原生路由前缀，或 HE Tunnel Broker 通过 6in4 路由的前缀；单个
  SLAAC 地址仍不满足要求。
- `ipv6_egress.allocation_secret` 至少 32 个字符并跨重启保持不变。
- Docker 部署同时使用 `deploy/docker-compose.ipv6-egress.yml` 和宿主机路由脚本。

代码接受 `/120` 或更大的地址空间，生产环境推荐使用 `/64`，或从 `/56`
中为节点划分独立子前缀。多个应用实例、共享前缀和跨节点账号亲和不在当前
支持范围内；配置校验会拒绝多实例模式。

## 出口决策

账号加载时一并取得 `egress_mode` 和持久绑定，生成完整的 `egress.Route`。
优先级如下：

| 条件 | 实际出口 |
| --- | --- |
| 账号已有 `proxy_id` | 保持现有外部 HTTP/SOCKS 代理 |
| `egress_mode=ipv6_pool` | 使用指定池的账号绑定地址 |
| `egress_mode=direct` | 使用节点普通出口 |
| `egress_mode=inherit` 且功能启用 | 使用系统默认池的账号绑定地址 |
| `egress_mode=inherit` 且功能关闭 | 回滚到普通直连 |

显式 `ipv6_pool` 永远失败关闭。无 AAAA、缺少绑定、源地址绑定失败、路由错误
或功能关闭时都返回上游错误，不能回退到 IPv4。已有外部代理也保持失败关闭，
不能因代理对象无效而改成直连。

## 数据模型与分配

迁移 `backend/migrations/231_add_ipv6_egress_pools.sql` 增加：

- `ipv6_egress_pools`：名称、CIDR、节点、状态、默认标志和分配版本。
- `account_egress_bindings`：账号、池、唯一 IPv6、状态、绑定版本和轮换时间。
- `accounts.egress_mode`：`inherit`、`direct`、`external_proxy` 或 `ipv6_pool`。

分配器使用稳定密钥、池 ID、池分配版本、账号 ID 和绑定版本计算 HMAC 地址，
不枚举整个前缀。同一账号跨进程和容器重启保持地址不变；轮换只递增绑定版本。
数据库唯一约束处理地址冲突，池创建在 PostgreSQL advisory transaction lock 下
拒绝任何重叠 CIDR。

删除账号会级联释放绑定。仍有绑定的池不能删除。前缀变更应创建新池并逐步迁移，
不原地修改已有 CIDR。

## 请求链路

调度快照和账号 repository 已包含出口绑定。稳态网关请求使用已选账号携带的
`egress.Route`，不会为出口新增数据库查询。

```text
scheduler/account load
  -> Account.EgressRoute (proxy_id remains authoritative)
  -> HTTP / TLS fingerprint / WebSocket / req / shared client route
  -> IPv6-only resolver
  -> per-socket IPV6_FREEBIND source bind
  -> upstream AAAA address
```

连接池键包含模式、池 ID、源 IPv6 和绑定版本；IPv6 路由强制账号级隔离。
轮换后旧空闲连接会关闭，已运行的 SSE 或 WebSocket 不会被主动中断。

接入范围包括主 HTTP/1.1、HTTP/2、SSE、TLS 指纹、OpenAI Live/WebSocket、
账号测试、模型和额度查询、Token 刷新、图片/媒体下载、WebSearch、Gemini、
Vertex 和 Antigravity 客户端。新增账号级上游客户端时，必须接收已加载路由或
路由 context；不支持路由的旧 `HTTPUpstream` 实现遇到 IPv6 必须失败关闭。

## 生命周期与健康

`modules/egress.Service` 在每个启用功能的进程启动时运行本地出口预检，因为不同
网络命名空间的路由状态不能互相代表。只有 worker-enabled 进程补齐默认池中
`inherit` 账号的缺失绑定；worker-disabled 进程仍运行预检但不写入绑定。

池必须先绑定一个账号并通过 HTTPS 出口回显探测，才能设为默认。探测会验证上游
观察到的地址与绑定地址完全一致。池健康状态是进程内运行状态，重启后先显示
“未探测”，随后由默认池预检或管理员手动探测刷新；禁用池会清除旧健康状态。

管理员在 **系统设置 -> 功能开关** 启用“IPv6 出口”。该开关持久化在数据库，
是账号路由和 HE sidecar 的运行总开关，不需要再修改 `IPV6_EGRESS_ENABLED`。
关闭时继承模式回到普通直连，显式 IPv6 路由失败关闭；控制服务会向 HE sidecar
提交 remove，清理 SIT 接口和本地池路由。旧版本数据库中的
`ipv6_egress_ui_enabled` 键继续作为兼容存储键。

管理端 `/admin/egress` 显示：

- Linux/free-bind/密钥/探测配置就绪状态。
- 池 CIDR、可用容量、分配数、默认状态和最后一次路由健康结果。
- 账号实际出口、绑定地址、池和绑定版本。
- 分配补齐、账号探测、策略切换和有 step-up 保护的地址轮换。

池和 HE 表单提供“探测可用前缀”操作。后端只读取当前 Linux/Docker 网络命名空间
中的 IPv6 interface address，并将没有路由掩码的裸地址按显式 `/64` 约定归一；
`/48`--`/63` 的宽前缀会按实际地址收窄到包含它的 `/64`，不会扩大路由范围，
`/64`--`/120` 则保持原掩码（单独的 `/128` 主机地址会被拒绝）。`he-*`、
`sit*` 和 `ip6tnl*` 等点对点隧道接口只作为诊断返回，不会被建议为账号池；HE
账号池必须来自单独 Routed 前缀。
探测结果仍需经过池重叠、容量、健康探测和管理员确认，系统不会自动把未知地址写入
路由表。

所有管理端变更经过统一审计中间件。错误日志和 API 错误保留无 AAAA、绑定、
路由和探测失败类型；不得把完整 IPv6 或账号 ID 作为低基数指标标签。

## Docker 网络

原生路由前缀使用 `docker-compose.ipv6-egress.yml`。该 override 只给应用容器分配
固定 ULA 下一跳，不把公网池地址逐个加入容器。宿主机脚本执行两项路由初始化：

1. 宿主机把公网池前缀路由到应用容器固定 ULA。
2. 通过 `nsenter` 在应用容器网络命名空间加入 `local <pool> dev lo`。

第二项让返回包被交给绑定非本地源地址的 socket；只有宿主机初始化脚本需要权限。
主应用容器不授予 `NET_ADMIN`，只使用每 socket `IPV6_FREEBIND`。容器重建、网络
重建或宿主机重启后必须重新执行脚本，`check` 模式会同时校验宿主机路由、容器
local 路由、IPv6 forwarding、外部 IPv6 路由和应用容器 capabilities。

具体命令和环境变量见 [部署索引](../deploy/README.md)。

### 无原生 IPv6 的 HE 6in4 接入

HE 模式使用独立的 `docker-compose.ipv6-egress-he.yml`。`sub2api` 主容器保持
无特权；HE sidecar 通过 `network_mode: service:sub2api` 加入同一容器网络命名
空间，只有 sidecar 获得容器内 `NET_ADMIN` 和 `NET_RAW`。它不使用 host network、
host PID、Docker socket 或宿主机文件系统挂载。

启用管理员运行开关后，管理员在 **IPv6 出口 -> HE 隧道** 保存参数并提交
`apply/check/remove`。后端把
严格校验后的期望配置与动作写入共享控制卷，sidecar 在同一网络命名空间创建
SIT/6in4、IPv6 默认路由和 `local <Routed prefix> dev lo`，再回写在线状态和结果。
update key 不通过 GET API 返回。

HE 页面中的 Tunnel `/64` 只用于 `HE_TUNNEL_CLIENT_IPV6` 和
`HE_TUNNEL_SERVER_IPV6`；账号池必须使用 HE 单独提供的 **Routed /64**。
二者混用会被后端拒绝。sidecar 会验证 HE 对端及外部 IPv6；Docker 宿主机和上游
防火墙仍须允许 IP protocol 41。CGNAT 和普通 TCP/UDP 端口转发无法承载 6in4。

公网 IPv4 会变化时，可在前端使用 HE tunnel ID、账号名和 tunnel update key
开启动态端点更新。sidecar 支持幂等应用和端点变化时的受限重建；移除操作只删除
sidecar 管理的 SIT 接口与 local pool 路由。

## 回滚与验证

移除 Compose override 或关闭管理员 IPv6 开关后，继承模式账号恢复普通直连；
显式 IPv6 账号继续失败关闭，直到管理员切换其模式。绑定数据保留，恢复功能时可
继续使用，不需要删除池。

从仓库根目录运行重点验证：

```sh
docker run --rm -v "$PWD/backend:/app" -w /app golang:1.26.6-alpine \
  go test ./internal/modules/egress ./internal/platform/egress ./internal/infrastructure/repository

sh deploy/tests/ipv6-egress-docker.sh
sh deploy/tests/he-ipv6-tunnel-docker.sh
sh deploy/tests/ipv6-egress-sidecar-docker.sh
docker compose -f deploy/docker-compose.local.yml \
  -f deploy/docker-compose.ipv6-egress-he.yml config
```

Docker 集成测试使用独立的 6in4 与路由前缀，验证主应用无网络管理 capability 时，
sidecar 仍能让两个未分配账号源地址完成稳定、轮换和回显检查。

该功能只能提供账号出口隔离。大量地址仍属于同一运营商前缀，上游可以按 `/64`
等前缀聚合，不能据此承诺独立信誉、无限并发或规避上游风控。
