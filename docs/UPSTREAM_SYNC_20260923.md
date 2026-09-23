# 上游主线选择性同步审查记录（2026-09-23）

## 冻结范围与处理原则

- 下游基线 `f2284c586e8d182423b37eacf664d28b39cb41f5`；上一关闭点 `efe9aab1e4ec89a42ba45e8dac20e882c5409a6a`；本轮冻结上游 `7c0a2a556c440836c54b3f3135f033755f15ecb7`。
- 范围为 98 个 first-parent 提交（其中 94 个合并 PR、其余为版本/CI/测试直接提交），仅按最终净差异和本项目 `application / infrastructure / transport / features` owner 判断；不复制旧目录、上游 VERSION、赞助 README 或大规模新功能。
- 本轮按热路径成本和升级面优先落地 #7481；按管理员切换用户的数据隔离落地 #7500。已覆盖的 #7402、#7311、#7488 不重复更改。其余 89 项在下表明确保持“专项暂缓，未移植”：未经过当前项目兼容性/性能端到端验收，不能误称已合并其功能。

## 性能与兼容性审查

1. #7481 原选号路径对分组调用 `GetByID`，会在已有 `GetByIDLite` 之上再做账号计数聚合。四个调用点只读取隐私调度配置，不读取账号数；改为 Lite 不更改调度候选、粘性、并发或错误契约；单个选号/失败诊断由“分组行 + 账号计数”缩为“分组行”。已有仓储集成测试 `TestGetByIDLite_DoesNotUseAccountCount` 证明 Lite 不进行计数，单测断言已同步。
2. #7311 所需 `base_rpm`、`rpm_strategy`、`rpm_sticky_buffer` 已由当前 `scheduler_cache.go` 投影保留；#7402 的 OpenAI HTTP/2 PING 仍为 15s/15s；#7488 的输入 item ID 长度上限 64 已覆盖更多 item 类型。以上关闭重复差异，不增加热路径分配/调用。
3. #7500 仅在管理员切换用户或卸载组件时递增请求版本、清除旧字段；过期响应和过期错误不提交、不覆盖新用户。每次请求为常数时间检查，没有新网络请求；后端/协议/持久数据不变。
4. #7303 的所有上游响应体加锁及上下文取消、#6473 提前终止 SSE、#7276 语义心跳、#7412 计费倍率、#7160 日志滚动保留均涉及网关吞吐、结算或数据库成本，保持专项暂缓，不从旧 `service`/`repository` 路径整包合入。#7351/#7397 插件宿主、#7247 视频任务、#7387 外部审计引擎涉及新增端口、凭据、迁移和运行面，须单独设计和压测。

## PR 差异台账

“专项暂缓”只表示本轮未合入，后续按具体 owner 和独立测试重开；“已覆盖”与“已移植”才关闭对应的重复代码差异。表中 owner 是由 PR 文件路径映射到本项目职责层的定位，不是照搬建议。

| PR | 上游合并 | 主题 | 当前 owner | 处理 |
| --- | --- | --- | --- | --- |
| #7351 | `4dbcdce43` | feat(plugin): generic host services + read-only status bridge channel | frontend/features 或 common、infrastructure/repository、application/service、transport/http/handler、transport/http/server | 专项暂缓，未移植 |
| #7313 | `4275047d1` | fix(openai): pass DeepSeek thinking-mode reasoning_content on chat fallback | application/service | 专项暂缓，未移植 |
| #7345 | `d5cea617a` | fix(apicompat): 摊平 Responses→Anthropic 工具 schema 顶层联合，修复 Codex 内置工具导致的 400 | shared、application/service | 专项暂缓，未移植 |
| #7340 | `1b45d2132` | fix(ratelimit): CN coding-plan 账号配额耗尽的 403 按限流暂停调度，而非永久禁用 | application/service | 专项暂缓，未移植 |
| #7294 | `09f88865e` | ui(header): 手机竖屏顶栏保留模型广场图标入口 | frontend/features 或 common | 专项暂缓，未移植 |
| #7278 | `0c9e83b59` | fix(antigravity): Gemini 原生流对 go-genai / python-genai 客户端不发 SSE 注释心跳 | application/service | 专项暂缓，未移植 |
| #7258 | `74431ffad` | fix(antigravity): Gemini 原生请求的裸模型名按 thinkingConfig 解析到 -low/-medium/-high 变体 | application/service | 专项暂缓，未移植 |
| #7247 | `aea725f2e` | feat: 支持 Seedance / Ark 原生视频任务 API | frontend/features 或 common、application/service、transport/http/handler、transport/http/server | 专项暂缓，未移植 |
| #7397 | `19794bc46` | feat(pluginapi): HostService 账号目录返回结构化只读元数据 | application/service | 专项暂缓，未移植 |
| #7387 | `f28adb6dd` | feat: 内容审计接入 TypeSafe AI，支持双引擎切换与独立配置 | frontend/features 或 common、infrastructure/repository、shared、application/service、transport/http/handler、schema/migrations | 专项暂缓，未移植 |
| #7362 | `8bb48622a` | fix(accounts): preserve refresh errors during usage queries | application/service | 专项暂缓，未移植 |
| #7349 | `53b4bbe73` | fix(settings): preserve client version on invalid user agent | application/service | 专项暂缓，未移植 |
| #7314 | `7403a0117` | fix(moderation): prevent reminder tags from bypassing keyword checks | application/service | 专项暂缓，未移植 |
| #7304 | `bbdcfbac0` | fix(gateway): restore public response model aliases | application/service | 专项暂缓，未移植 |
| #7402 | `fbb9006ad` | fix(upstream): 恢复 OpenAI HTTP/2 保活容错期限 | infrastructure/repository | 已覆盖：OpenAI H2 15s/15s 保活 |
| #7401 | `acdf3c54d` | feat(openai): display Codex credits and support referral invitations | frontend/features 或 common、infrastructure/repository、application/service、transport/http/handler、transport/http/server | 专项暂缓，未移植 |
| #7413 | `87957a608` | ci: parallelize release builds across platform runners | 构建/文档 | 专项暂缓，未移植 |
| #7412 | `1c0a69c0c` | feat: 支持自定义思考等级倍率计费 | frontend/features 或 common、infrastructure/repository、application/service、transport/http/handler、schema/migrations | 专项暂缓，未移植 |
| #7446 | `5295bd822` | fix(usage): export missing reasoning effort cleanly | frontend/features 或 common | 专项暂缓，未移植 |
| #7369 | `a266e50e8` | ui(users): 禁用/启用用户后原地更新该行 | frontend/features 或 common | 专项暂缓，未移植 |
| #7481 | `d8eeb0989` | perf: 选号读取分组时不再聚合账号数 | application/service | 已移植：请求时 GetByIDLite |
| #7476 | `a46493724` | fix(subscriptions): lock negative redemption updates | infrastructure/repository、application/service | 专项暂缓，未移植 |
| #7477 | `405727805` | fix(subscriptions): preserve partial days during redemption deductions | infrastructure/repository、application/service | 专项暂缓，未移植 |
| #7475 | `fcd07ba0a` | fix(proxies): invalidate billing probe on fallback restore | infrastructure/repository | 专项暂缓，未移植 |
| #7448 | `9d9e90960` | fix(gemini): 传输层错误转 failover,不再同账号退避重试 | application/service | 专项暂缓，未移植 |
| #7432 | `3442a6a53` | fix(antigravity): 裸 Gemini 模型名在所有转发入口解析到思考变体，修复 404/502 | application/service | 专项暂缓，未移植 |
| #7473 | `b6c6f1add` | fix(codex): include OpenCode Go in model capability resolution | application/service | 专项暂缓，未移植 |
| #7361 | `483cd5692` | feat(simple): make startup default group creation optional | infrastructure/repository | 专项暂缓，未移植 |
| #7466 | `1716e9915` | feat(grok):add grok 4.7 support | frontend/features 或 common、shared、application/service、transport/http/handler | 专项暂缓，未移植 |
| #7419 | `23dc073e6` | fix(monitor): preserve the saved auto-refresh preference | frontend/features 或 common | 专项暂缓，未移植 |
| #7363 | `322cc5e5f` | fix(users): keep numeric custom attribute values as strings | frontend/features 或 common | 专项暂缓，未移植 |
| #7372 | `c0b23ff79` | fix(settings): allow retrying failed initial admin settings loads | frontend/features 或 common | 专项暂缓，未移植 |
| #7264 | `e7d348868` | fix(accounts): retry default mappings after a failed fetch | frontend/features 或 common | 专项暂缓，未移植 |
| #7364 | `9e175bd49` | fix(users): show errors when group replacement fails | frontend/features 或 common | 专项暂缓，未移植 |
| #7366 | `e4f97a53e` | fix(users): persist cleared custom attribute hints | frontend/features 或 common | 专项暂缓，未移植 |
| #7374 | `c96d36ec9` | fix(search): wait for committed IME input before searching | frontend/features 或 common | 专项暂缓，未移植 |
| #7420 | `f2e55bf4d` | fix(usage): ignore superseded error detail requests | frontend/features 或 common | 专项暂缓，未移植 |
| #7426 | `680a992fc` | fix(openai): 改写 Codex turn metadata 时保留非 ASCII 转义 | application/service | 专项暂缓，未移植 |
| #7376 | `ca47fa352` | fix(groups): reject negative custom rates before adding | frontend/features 或 common | 专项暂缓，未移植 |
| #7367 | `a765b3c17` | fix(groups): require integer RPM overrides before adding | frontend/features 或 common | 专项暂缓，未移植 |
| #7373 | `023b38354` | fix(users): wait for group configuration before enabling save | frontend/features 或 common | 专项暂缓，未移植 |
| #7375 | `1ea00258d` | fix(users): ignore superseded balance history requests | frontend/features 或 common | 专项暂缓，未移植 |
| #7365 | `62a8919a1` | fix(monitor): ignore stale template picker responses | frontend/features 或 common | 专项暂缓，未移植 |
| #7422 | `5af29a3b9` | fix(users): keep API key results scoped to the selected user | frontend/features 或 common | 专项暂缓，未移植 |
| #7442 | `738ef1c73` | fix(images): preserve insufficient balance failures | application/service、transport/http/handler | 专项暂缓，未移植 |
| #7354 | `a5015e1fc` | fix(openai): alias DeepSeek Responses input_image with url | application/service | 专项暂缓，未移植 |
| #7417 | `c1319917c` | fix(payment): trim callback base URL trailing slashes | frontend/features 或 common | 专项暂缓，未移植 |
| #7418 | `5fd346114` | fix(pricing): parse scientific notation in token bounds | frontend/features 或 common | 专项暂缓，未移植 |
| #7395 | `2448a38b5` | fix(frontend): ensure Codex base URLs include /v1 | frontend/features 或 common | 专项暂缓，未移植 |
| #7404 | `c619ff847` | fix(account): preserve model mapping on OAuth reauth | transport/http/handler | 专项暂缓，未移植 |
| #7289 | `b717ac7f0` | fix(admin): align platform quota editor with supported platforms | frontend/features 或 common | 专项暂缓，未移植 |
| #7341 | `ed2360d7e` | fix(proxies): preserve renewals during expiry sweeps | infrastructure/repository | 专项暂缓，未移植 |
| #7342 | `a079a6596` | fix(proxies): exclude inactive fallback targets | infrastructure/repository、application/service | 专项暂缓，未移植 |
| #7339 | `231e72fdd` | fix(openai): honor error.status when classifying in-stream failures | application/service | 专项暂缓，未移植 |
| #7394 | `55108ee87` | fix(gateway): recognize Baseten reasoning budget errors | application/service | 专项暂缓，未移植 |
| #5443 | `de6388aec` | fix(gemini): honor RetryInfo on 429 and stop cooling Vertex service accounts until PST midnight | application/service | 专项暂缓，未移植 |
| #7479 | `65f086d2d` | fix(gateway): prevent Cloudflare 1010 from disabling OpenCode and Command Code accounts | application/service | 专项暂缓，未移植 |
| #7427 | `b0e32fe5b` | fix(openai): 非高级调度补齐选号决策并按 previous_response 路由 | application/service | 专项暂缓，未移植 |
| #7488 | `7be1628a7` | Strip oversized input item IDs | application/service | 已覆盖：全部输入 item ID 长度上限 64 |
| #7487 | `bb1f40e35` | Handle trailing system messages in moderation input | application/service | 专项暂缓，未移植 |
| #7323 | `7b13cb410` | fix(announcements): ignore superseded fetch results | frontend/features 或 common | 专项暂缓，未移植 |
| #7322 | `ba5737fe1` | fix(ccswitch): trim trailing slashes before Antigravity path | frontend/features 或 common | 专项暂缓，未移植 |
| #7321 | `4b0adb686` | fix(proxies): label newly expired proxies correctly | frontend/features 或 common | 专项暂缓，未移植 |
| #7320 | `7865051fd` | fix(upload): cancel superseded image reads | frontend/features 或 common | 专项暂缓，未移植 |
| #7319 | `2b12de14e` | fix(dialog): keep body locked while another dialog is open | frontend/features 或 common | 专项暂缓，未移植 |
| #7272 | `51f73840a` | fix(apicompat): 修复 Anthropic 转 Responses 流中 function_call_arguments.done 缺少 arguments 参数的问题 | shared | 专项暂缓，未移植 |
| #7169 | `fa9f104a9` | fix(gateway): list mixed Antigravity models for Gemini groups | application/service、transport/http/handler | 专项暂缓，未移植 |
| #7142 | `ea34a2356` | fix(ui): reset select highlight when search results change | frontend/features 或 common | 专项暂缓，未移植 |
| #7109 | `d27fbb3b7` | fix(ui): discard unapplied date ranges on dismissal | frontend/features 或 common | 专项暂缓，未移植 |
| #7108 | `974a819ba` | fix(ui): focus non-searchable select dropdowns | frontend/features 或 common | 专项暂缓，未移植 |
| #7097 | `b12a187d4` | fix(streaming): drop Anthropic ping keepalives before OpenAI conversion | application/service | 专项暂缓，未移植 |
| #7311 | `5cc6ca6f5` | fix(scheduler): keep account RPM config in the scheduler projection | infrastructure/repository | 已覆盖：调度投影 RPM 三字段 |
| #7303 | `583398d18` | fix(upstream): prevent EOF stalls when closing response bodies | infrastructure/repository | 专项暂缓，未移植 |
| #6473 | `e26abaef7` | fix(openai): end streaming at terminal event instead of waiting for u… | application/service | 专项暂缓，未移植 |
| #7276 | `13be6ca27` | fix(openai): keep transport heartbeats outside semantic output | application/service | 专项暂缓，未移植 |
| #7038 | `02d9901b4` | fix(openai): remove Responses Lite markers after OAuth mapping to GPT-5.5 | application/service | 专项暂缓，未移植 |
| #7077 | `da25b18db` | fix(antigravity): handle prefixItems and ensure array schema has valid items fallback | shared | 专项暂缓，未移植 |
| #6920 | `20a94fbb5` | fix(grok): allow quota queries during account cooldowns | application/service | 专项暂缓，未移植 |
| #7501 | `a81547f24` | fix(profile): recognize DingTalk profile sources | frontend/features 或 common | 专项暂缓，未移植 |
| #7500 | `5e4968492` | fix(users): discard stale user attribute responses | frontend/features 或 common | 已移植：用户切换请求隔离 |
| #7499 | `0eaa7c3c8` | fix(date-picker): refresh relative dates after midnight | frontend/features 或 common | 专项暂缓，未移植 |
| #7498 | `fc4465f78` | fix(channels): preserve model tags during IME composition | frontend/features 或 common | 专项暂缓，未移植 |
| #7496 | `9e0e14698` | fix(profile): avoid starting TOTP cooldowns after unmount | frontend/features 或 common | 专项暂缓，未移植 |
| #7495 | `d3d0f653e` | fix(subscriptions): use calendar dates for expiry labels | frontend/features 或 common | 专项暂缓，未移植 |
| #7494 | `d7e4bba0e` | fix(users): show normalized API errors in the balance dialog | frontend/features 或 common | 专项暂缓，未移植 |
| #7492 | `7d5615998` | fix(channel-monitor): ignore stale detail requests | frontend/features 或 common | 专项暂缓，未移植 |
| #7489 | `fa3f52678` | fix(tools): 清洗工具 Schema 里非法的 null required，修复 xAI / Moonshot 400 | application/service | 专项暂缓，未移植 |
| #7411 | `d6e8b44bc` | fix(antigravity): neutralize Claude Agent SDK identity in system prompts | shared | 专项暂缓，未移植 |
| #7308 | `60b9bf755` | fix(codex-models): 非 GPT 模型的 Codex 提示词模板去掉 GPT 身份声明，避免 Antigravity 429 | application/service | 专项暂缓，未移植 |
| #7300 | `5ad7cf4fb` | fix(responses): emit protocol-correct stream errors once | application/service、transport/http/handler | 专项暂缓，未移植 |
| #7493 | `24872fda7` | fix(accounts): keep temporary unschedulable status scoped to the current account | frontend/features 或 common | 专项暂缓，未移植 |
| #7139 | `b350e079f` | fix: keep API-key accounts available for unknown models | application/service | 专项暂缓，未移植 |
| #7060 | `0dd71286f` | fix(images): allow compatible Gemini models on API-key upstreams | application/service、transport/http/handler、transport/http/server | 专项暂缓，未移植 |
| #7160 | `7c0a2a556` | feat: 支持运维与请求日志的滚动保留设置 | frontend/features 或 common、infrastructure/repository、application/service | 专项暂缓，未移植 |

## 验证、回滚及发布

基线与候选的字面命令/输出/退出码、Docker 构建及运行、前端与仓储检查、可执行的独立副本回滚记录在 `diagnostics/upstream-sync-20260923/VERIFICATION.txt`。四件工件：`MODIFIED_FILE`、`DIFF_FILE`、`VERIFICATION.txt`、`ROLLBACK.sh`；主源码副本保持修改状态。

仅在完成本轮冻结范围的台账后，用 tree-preserving tracking merge 记录上游边界；这只关闭重复 *审查范围*，不表示 89 个暂缓项的功能已移植。后续以此文件的 PR 编号重开，不能依据 Git 祖先关系推断功能覆盖。发布后以最终下游 merge SHA 核验 CI / Docker Image / Security Scan，所有未完成或失败的检查须如实记录。
