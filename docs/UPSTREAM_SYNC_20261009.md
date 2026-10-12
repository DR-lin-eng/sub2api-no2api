# 上游主线同步审查（2026-10-09）

本轮从下游 `03a9207f932378207bf07ad09ce57aa154b5e0cd` 出发，延续已审查边界 `5fc0e486c3f6a8a191b8bd140f39b60457f611cf`，冻结至 `3a6fd1c9db07203ca308aaba69e502bc1f35b307`。范围为 2 个主线合并 PR 及 1 个上游 VERSION 提交。原工作区不改动，在独立可写克隆执行；原链接工作树的 FETCH_HEAD 不可写，不能把抓取错误当成“没有增量”。

## 差异关闭与专项边界

| 上游 PR | 本轮结果 | 当前 owner 与后续范围 |
| --- | --- | --- |
| [#7939](https://github.com/Wei-Shaw/sub2api/pull/7939) | 完整适配、性能重构；关闭该冻结差异 | `application/service/openai_oauth_web_search_history.go`；HTTP transform/passthrough 与普通/透传 WS 首轮及后续 turn。 |
| [#7618](https://github.com/Wei-Shaw/sub2api/pull/7618) | 安全工具链及当前模型修复适配；保留明确专项暂缓 | Responses→Anthropic 计费模型去除首尾空白；Go 安全修补保持 1.26 分支。已有 11 平台额度和 Zen qwen3.8-max 规则保持原 owner，不重复重构。平台/profile 清单、协议集合目录、Command Code 和 Cline 尚未实现，不标记完全关闭。 |

[机器台账](../diagnostics/upstream-sync-20261009/UPSTREAM_PRS.json) 记录 merge SHA、原始 diff SHA-256、逐子项关闭/重开条件。此前所有关闭和独立暂缓台账原样保留；tracking merge 只登记审查祖先，不代表专项功能完成。上游 VERSION `0.2.15` 不覆盖本项目版本线，三语 README 的无赞助政策保留。

#7618 共 134 个文件、13,053 行新增，包含旧 `service/handler/components/views` owner、删除两个数据库平台 CHECK 约束、原编号 242 迁移、新供应商及协议集合字段。直接覆盖会丢失本项目 `transport/application/domain` 和 feature owner、TypeSafe 原生隔离、账号完整出口和既有数据库防御约束。本轮保留现有 CN/OpenCode 路由契约；这不表示平台/profile 抽象已经实现。

新 `protocols` 集合的旧节点只理解单个 `protocol`，可能将原本可直通的入站转成有损协议；上游冷模型目录可在路由首次请求等待 2 秒。后续专项须采用有界后台刷新/快照、跨账号缓存隔离、代理/IPv6/TLS 一致的账号出口，验证旧/新 writer、身份、计费、失败限额和显式开启顺序，再注册新平台。Cline 还需要验证订阅/积分/免费模型各自冷却，不可退化为账号全局停调。本轮没有引入新平台，没有删除 CHECK、修改 Ent/Wire 或新增迁移；不能宣称这些新功能已经同步。

## OAuth 搜索历史和兼容性

历史包含 hosted `web_search_call` 而请求未声明搜索工具时，ChatGPT Codex 可能返回 `response protection is unavailable`；本轮移植声明机制，不删除历史。标准 Responses 补顶层 `{"type":"web_search","external_web_access":false}`；Lite 改放首个 `input.additional_tools`，没有载体时插在最后 `compaction_trigger` 之前。调用方没有任何工具且 choice 缺省/auto/none 时固定 none；已有工具及其他显式 choice 保留。已声明 `web_search*` 的请求保持字节原样。

旧 unary `/responses/compact`、API Key、其他平台和 BPS 独立链路不注入。真实 HTTP 四条转发路径及 WS ctx_pool/passthrough × OAuth/API-key × standard/Lite × 两轮 fixture 验证声明位置、choice、终态、trigger 和未改动 API-key 语义。兼容代码不修改身份、调度、可靠结算和失败重放规则。

#7618 的模型名问题在 `openai_gateway_responses_anthropic_native.go` 直接归一化请求模型，避免将裸模型字符串的首尾空白写入 `BillingModel`；不把分组默认映射泛化到普通 Responses。Kimi 与 OpenCode 的流式/非流式真实 Forward fixture 使用同一最终模型，保留用量输出。`Forward` 的既有 decoded OAuth 与 Lite 准备逻辑按职责移入 `openai_gateway_forward_prepare.go`，复用原 decoded map、调用顺序和身份对象；主文件从基线 1233 行降到 1200 行，allowlist 没有扩大。

## 安全工具链

当前 `govulncheck` 在基线 Go 1.26.6 检出 10 项可达标准库漏洞，并提示 x/net v0.57.0 的相关修补版本。官方 Go 发布目录确认 `1.26.9` 修补分支；x/net v0.60.0 的最低 Go 为 1.26.0，因此选择 Go **1.26.9** 与 **x/net v0.60.0**，无需跳到上游 Go 1.27.2、改变 Ent 导出格式或升级 lint 大版本。

Docker builder、backend/go.mod、CI/Security/Release Go 版本检查、部署 Dockerfile、当前三语技术栈和开发说明一致更新。传递依赖按 x/net 依赖图更新，保留既有工具依赖和旧 checksum；不修改 Node/pnpm、支付状态、数据库 schema 或公开必填协议。x/net v0.60 将现有 HTTP/2 适配 API 标记 deprecated；依上游 #7618 策略，仅对已验证的 idle-PING、H2C 和 typed GOAWAY owner 增加路径/符号都受限的 SA1019 例外，保留代理、连接健康探测和 failover 语义，不全局停用该规则。对应 keepalive/H2C 回归继续执行。修补后的 [安全记录](../diagnostics/upstream-sync-20261009/records/govulncheck-go1269.stdout) 显示 0 项可达漏洞；仍有 1 项模块级但未被代码调用的提示，不把它描述成所有依赖绝无风险。

## 性能

原始上游 helper 先复制 `input`，即使已有工具声明也分配整段历史。本项目改为调用内只读 string view 和 `ForEach`，提前检查顶层声明；Lite 新载体通过一次 span 插入，保留未知字段、数字精度、转义及非目标字节。map 版移除逐项类型临时数组，复用既有 decoded map。

相同 Docker Go 1.26.6、linux/arm64、GOMAXPROCS=2，8 MiB 历史、每轮 10 次、3 轮：已有顶层声明中位数 `5.73ms / 8.40MB / 5 allocs` 降至 `1.54ms / 0B / 0 allocs`；已有 additional_tools 声明分配降为 0，延迟约 4.4ms，未宣称该分支更快；Lite 插入中位数 `12.31ms / 50.38MB / 44 allocs` 降至 `8.79ms / 16.79MB / 4 allocs`。无标记检查两版均为零分配。该测试不代表全网关吞吐量，原始数据和 frozen upstream test-only reference 位于 [性能记录](../diagnostics/upstream-sync-20261009/records/reviewed-compat-and-benchmark.stdout)。

## 验证与回退

四个角色为 [MODIFIED_FILE](../diagnostics/upstream-sync-20261009/MODIFIED_FILE)（原生源码 tar.gz）、[DIFF_FILE](../diagnostics/upstream-sync-20261009/DIFF_FILE)（重建 patch）、[VERIFICATION.txt](../diagnostics/upstream-sync-20261009/VERIFICATION.txt) 和可执行 [ROLLBACK.sh](../diagnostics/upstream-sync-20261009/ROLLBACK.sh)。每个修改路径与原始 SHA 保存于 SOURCE_HASHES.json。回退脚本只接受独立副本，通过 sibling BASELINE_FILE 还原源字节并删除新增路径；不操作原工作区。

同一个 Forward fixture：基线四路径无搜索声明且 choice=auto，回归退出 1；候选四路径有缓存搜索声明且 choice=none，退出 0；独立回退恢复基线行为，退出 1。还原 hash 逐项一致，重施 DIFF_FILE 后 modified hash 一致。records 保存每次命令、输入、逐字 stdout/stderr、退出码和错误修正，没有把编译资源错误记为基线行为。

当前 Docker 单元、集成、race、lint、生产镜像和 PostgreSQL/Redis 同库升级/回退结果以 VERIFICATION.txt 的最终记录为准。源布局剩余 `payment_fulfillment_test.go` 1201 行、`pricing_service.go` 1238 行均与精确基线字节相同，不重复修复或扩大 allowlist。前端使用冻结依赖执行原必需门禁，既有可选失败须单独与基线核对。

发布的 head/main SHA、CI/Security/Docker 和 GHCR 多架构清单单独核对，不把本地测试当作远端发布完成。
