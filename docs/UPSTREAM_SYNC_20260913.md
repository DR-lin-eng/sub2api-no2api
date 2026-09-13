# 上游主线同步审查记录（2026-09-13）

本记录冻结本轮上游审查边界、选择性移植和已关闭差异，避免后续重复检查。源码、测试和 CI 结果是最终事实源。

## 冻结点与审查范围

| 项目 | 值 |
| --- | --- |
| 下游基线 | `origin/main` @ `464446a805dff18113069e8508b5164f398696ff` |
| 上游冻结 | `upstream/main` @ `bdb42e22f81fcb633ff0a060961211dd2bcb515b` |
| 已关闭的上游边界 | `98d86915becae9fe9491a91ffc6defd5235c8d2b..bdb42e22f81fcb633ff0a060961211dd2bcb515b` |
| 分支 | `codex/upstream-sync-20260913` |
| 审查方法 | first-parent 合并提交、PR diff、模块 owner、回归测试、Docker 构建 |

范围内共 141 个提交、56 个 first-parent 合并 PR。open PR 不纳入本次冻结；只有已经进入上游 main 的内容才进入下表。

## 选择性移植

| PR | 结果 | 下游实现与边界 |
| --- | --- | --- |
| #6783 | 已移植并重构 | `httputil` 分块读取；Responses 输入 ID 原始 JSON 视图只在最终替换时复制一次，保留大整数、图片 data URL、重复 key/坏 JSON 兼容路径。 |
| #6965/#7049/#7064 | 已移植并保守化 | WS pool 常驻 reader loop、控制帧 ping、脏空闲连接淘汰、拓扑变更唤醒等待者、立即 abort；保留本项目显式 factor 和 `1.0` 生产默认，不引入上游 `5.0` 默认。 |
| #6964 | 已移植 | `agent_message` 在 Responses→Chat bridge 中按原顺序转为 user 文本，保留 `input_text`、`text`、`encrypted_content`。 |
| #7022 | 已移植 | `Accept-Encoding` 使用 canonical wire casing；HTTP/1.1 与 HTTP/2 均增加单字段回归测试。 |
| #7052 | 已移植 | JWT/Admin 用户查询仅将明确的 `ErrUserNotFound` 映射 401；临时存储错误映射 500，浏览器刷新失败保留会话并返回 `TOKEN_REFRESH_UNAVAILABLE`。 |
| #6917 | 已移植 | channel monitor endpoint 允许路径前缀，按完整 escaped path 合并，仍拒绝 query/fragment 和私网地址。 |
| #7012 | 已移植 | EasyPay `upstreamType` 允许点号；后端、前端校验和中英文文案同步。 |
| #6943 | 已移植 | Claude OAuth 保留客户端 `system[*].cache_control`，在 count_tokens 出口统一执行四断点上限。 |
| #6945 | 已移植 | `/v1/models/:model` 与 `/models/:model` 复用可见模型目录，返回完整单模型对象和稳定 `model_not_found` 错误。 |
| #6929 | 已移植并适配 | Gemini 原生 2xx 正文/SSE 检测错误 envelope、prompt/content filter、空响应；客户端字节不变，Ops 区分请求级与 provider 级结果。 |
| #6995 | 已移植 | 新增 message-level `mid-conversation-output-config-2026-07-01` 能力令牌和按 beta 的字段净化；顶层 `output_config` 不受影响。 |
| #6869 | 已移植 | Codex automation heartbeat 接受完整 `current_time_iso` + `instructions` envelope，并严格校验 RFC3339、重复字段和未知字段。 |
| #7010 | 已移植并隔离 | 仅 privacy/account metadata 请求使用 Firefox impersonation，识别 `cf-mitigated: challenge`，不改变普通 OAuth client。 |
| #6575 | 已移植 | Antigravity Gemini 3.7/3.8 Flash 及 thinking tiers、目录映射和 fallback 价格；tier 共享基础价格卡。 |
| #6661 | 已移植 | Grok media 在 eligibility/owner 校验前接管并释放 scheduler slot；视频查询固定原始任务 owner，避免泄漏槽位或跨账号读取。 |

## 已覆盖或暂缓的差异（已关闭）

以下条目已逐项检查并登记结论；后续同步不应重复导入同一 legacy 路径。

| PR/主题 | 结论 |
| --- | --- |
| #6904 scheduler threshold metadata | 当前 `scheduler_cache` 已保留 `account_scheduling_threshold`、session/passive quota metadata，等价覆盖。 |
| #6850 channel monitor v2 UTC `date_bin` | 当前 fork 没有上游 `channel_monitor_v2` owner/目录；不移植不存在的 legacy 路径。 |
| #6499 setup bootstrap | `internal/bootstrap/setup` 已使用配置 DSN 并有测试，关闭重复同步。 |
| #6924 Codex UA | identity owner 已有 originator/UA 严格配对；本轮仅补 control-byte 拒绝。 |
| #6913/#6916/#6847/#6845/#6843/#6902/#6912/#6915 | 主要是上游旧 `frontend/src/views` 或旧 handler；当前 feature owner 已有等价实现，关闭整树迁移。 |
| #6960 WS execution scope/preemption | 当前 continuation/turn owner 与上游不同；整 PR 会破坏 affinity，暂缓并保留现有 session 语义。 |
| #7043 | 上游将 OAuth/API-key max-conns factor 默认改为 5.0，会在滚动升级中放大连接和内存；保留 1.0 默认，仅显式配置生效。 |
| #7011 | 不改本项目 `gpt-5.4` compact 默认，避免改变既有请求语义。 |
| #6974 | 不直接改 DeepSeek V4.1 真实账单价格；需要独立价格发布和历史账单对账。 |
| #6890 | 本项目已有独立 Responses image plan/bridge，不导入上游 legacy direct image owner。 |
| #7057 | 当前没有同构 `UpstreamModelMetadata/MaxContextWindow` 快照；暂缓 schema 设计。 |
| #7062 | MiniMax monitor adapter 与本项目 monitor contract 不同，暂缓。 |
| #6747 | OpenCode session/config compatibility 已存在，不引入上游大平台迁移。 |
| #7001/#6971/#6988/#6989/#6968/#6254/#7010 其余 legacy UI/manifest 差异 | 已 grep 对应 feature/DTO/telemetry；无同构缺口或由本轮专门修复覆盖，关闭重复检查。 |

## 性能与平滑升级评估

* 大请求：分块 reader 避免 `bytes.Buffer` 多轮扩容复制；已用 69 MiB benchmark 验证结果为 `57.646 ms`、`144,707,040 B/op`、`81 allocs/op`（Apple M4，单次基准）。正常无变化路径保持原切片。
* WS pool：reader loop channel 容量固定为 1；控制帧不进入应用队列，应用数据在空闲期会把连接标记为 unusable；等待者在连接释放/淘汰/新建时被唤醒，避免队头阻塞。后台探活使用 10 s 宽限，但淘汰前重新取得 lease token，避免关闭已借出连接。
* Gemini 信号：先用三类字段的 `bytes.Contains` probe，再做 JSON 解析；只在每个实际 data event 上检测，不复制下游 payload。过滤类结果标记为请求级业务结果，错误 envelope/空响应才计 provider SLA。
* 模型目录：集合和单模型共用一次编码；单模型只遍历当前可见目录，不触发额外上游请求。
* 配置兼容：没有新增数据库迁移；连接 factor、compact 默认和既有 API JSON/SSE/WS 形状保持不变。新字段均为可选，滚动升级时旧节点可继续处理旧请求。

## 验证入口

执行目录均已写入验证 artifact；关键命令如下：

```sh
cd backend && GOCACHE=/private/tmp/go-cache-sub2 go test -tags=unit ./...
cd backend && GOCACHE=/private/tmp/go-cache-sub2 go test -race -tags=unit ./internal/application/service ./internal/shared/apicompat ./internal/shared/httputil ./internal/transport/http/server/middleware ./internal/transport/http/handler
cd frontend && pnpm run lint:check && pnpm run typecheck
cd frontend && pnpm exec vitest run src/core/networks/__tests__/client.spec.ts src/features/billing/__tests__/PaymentProviderDialog.spec.ts
```

Docker 使用 Go `1.26.6-alpine` 运行同一组后端 focused tests，并构建候选镜像 `sub2api-upstream-sync-20260913:candidate`。命令、原始输出、退出码和 rollback 结果见 [`diagnostics/upstream-sync-20260913/VERIFICATION.txt`](../diagnostics/upstream-sync-20260913/VERIFICATION.txt)。

## 回退策略

代码回退以本分支提交为单位；不回退数据库、不清理用户工作区。连接池新增状态全部在进程内，重启旧版本会自然丢弃。若需验证单文件回退，执行 artifact 中的可执行 `ROLLBACK.sh`，它只覆盖指定的独立副本；主 `MODIFIED_FILE` 保持 changed 以供审计。

## 发布跟踪

提交和推送必须使用 `codex/upstream-sync-20260913`，创建 PR 后以 PR 合并产生的 exact SHA 查询 `gh run list --commit <SHA>`，不得沿用合并前 SHA 的 CI 结果。CI 状态、PR URL、workflow run id 和最终 SHA 在发布后回填本节及验证 artifact。
