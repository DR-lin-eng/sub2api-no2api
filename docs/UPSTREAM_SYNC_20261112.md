# 上游主线同步审查（2026-11-12）

本项目基线为 `0f1836b5a3db2ccdf8d4ab985d199215e45f39a2`；冻结审查范围为
`6b4b37db43443b5e88e424fe3c69602cc3810dfb..530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484`。
验证期间本项目主线仅增加 `e367b7ce6` 的 VERSION 自动提交，已快进纳入；运行态对照仍使用上述冻结源码。
共审查 **33 个新增合并 PR**：30 项已关闭，3 项保留明确的未完成子契约。所有行为都适配进当前
application、infrastructure、shared、modules 和 frontend feature owner；未恢复上游旧目录。

[机器台账](../diagnostics/upstream-sync-20261112/UPSTREAM_PRS.json) 保存每项 merge/head SHA、原始 merge diff
SHA-256、当前 owner、决定和重开条件。已覆盖或有意保留的差异不会在下次同步时再次移植。
tracking merge 仅登记冻结的审查边界，暂缓能力仍以台账为准。上游赞助内容和版本元数据保持本项目独立管理。

## 逐项决定

| PR | 状态 | 当前项目的处理 |
| --- | --- | --- |
| [#8610](https://github.com/Wei-Shaw/sub2api/pull/8610) | 关闭 | 本项目未启用其依赖的 request_body_memory_budget_bytes，因而没有固定 1 GiB 的 503 回归。共享 ingress/patch/SSE 预算继续继承专项，不单独增加无消费者的自适应配置或 sysutil API。 |
| [#8607](https://github.com/Wei-Shaw/sub2api/pull/8607) | 关闭 | 文档按本项目已经验证的 App/legacy/native 图片路径说明实际返回 MIME/格式边界；不恢复上游另一套图片说明入口。 |
| [#8603](https://github.com/Wei-Shaw/sub2api/pull/8603) | 关闭 | 上游被隔离的 allocation 测试 owner 在本项目不存在，当前实现已重构，不恢复平行文件。保留本地已有性能测试并运行相关门禁。 |
| [#8590](https://github.com/Wei-Shaw/sub2api/pull/8590) | 关闭 | 单账号手动查询 force 跳过 Antigravity 两层缓存检查，继续同一 singleflight 和已有出口；复制倒计时字段后重算，避免并发修改缓存；成功查询清除旧传输错误。 |
| [#8606](https://github.com/Wei-Shaw/sub2api/pull/8606) | 部分/专项暂缓 | 当前可靠结算仅有聚合写入及 Anthropic 5m/1h，继续原价，不把 12h 误塞旧字段或只移植新默认价。新 30m/12h 用量、四类运营者价卡及其区间、持久队列/救援重放、管理员 DTO 和旧节点结算需作为同一专项适配/迁移；不采用与本项目冲突的上游 250 编号。 |
| [#8381](https://github.com/Wei-Shaw/sub2api/pull/8381) | 关闭 | Sonnet 5.5 的 fallback 与捆绑快照读取价改为 .10 USD/MTok，同步相邻费用断言；不覆盖用户显式价卡。 |
| [#8384](https://github.com/Wei-Shaw/sub2api/pull/8384) | 关闭 | hash 不一致时保留实际下载正文 hash 作为同步锚点，使下一次重试；原校验告警/合法正文装载及自定义目录优先级保留。 |
| [#8634](https://github.com/Wei-Shaw/sub2api/pull/8634) | 关闭 | 标准 Fable/Mythos 5.2 固定价卡：输入 8、输出 40、5m/1h 写入 10/16、读取 .20 USD/MTok；管理员价格和显式别名优先，缺失条目不回退到旧 Claude 家族。 |
| [#8564](https://github.com/Wei-Shaw/sub2api/pull/8564) | 关闭 | 原生 ingress 后续轮次的普通 failover 包装成不可重放当前轮次错误，防止 handler 重放原始首帧；现有 HTTP bridge 的安全当前轮次恢复仍由原 owner 管理。 |
| [#8358](https://github.com/Wei-Shaw/sub2api/pull/8358) | 部分/专项暂缓 | 现有历史 search 声明复用原 owner；不引入上游为切换 search 而长期保存全部输出并自动重放的新 owner。需先对完整历史证明、字节上限、取消、首轮/后续轮次、槽位和断连可靠计费做专项验收。 |
| [#8565](https://github.com/Wei-Shaw/sub2api/pull/8565) | 关闭 | 透传 WS 使用当前 payload-aware TTFT 分类；保持既有 Codex 本地首事件优先、逐轮独立计时和 semantic-output 的重试边界。 |
| [#8576](https://github.com/Wei-Shaw/sub2api/pull/8576) | 关闭 | 当前已将两个 codex metadata 事件归为 replay-safe preamble，保留本项目本地首事件 TTFT，不重复移植。 |
| [#8579](https://github.com/Wei-Shaw/sub2api/pull/8579) | 关闭 | 容量 shedding 默认保持请求级保护；只对真实错误字段匹配显式 model scope，复用当前规则 owner。命中后停止当前尝试的同账号重放；持久化失败仍只换当前请求，不扩大整账号惩罚。实际模型映射仅应用一次。 |
| [#8585](https://github.com/Wei-Shaw/sub2api/pull/8585) | 关闭 | 保留 namespace 与原始 encrypted_function_args（含 null/[]），覆盖 added、迟到 done 和空 terminal 回填；继续使用当前 builder。 |
| [#8386](https://github.com/Wei-Shaw/sub2api/pull/8386) | 关闭 | 跳过 ccswitch-openai-reasoning-v1 外部 JSON envelope；真实提供方 ciphertext 和可见 thinking 不被改造。 |
| [#8589](https://github.com/Wei-Shaw/sub2api/pull/8589) | 关闭 | Responses 与 Chat 一样仅使用首个会话项前的 system/developer 前缀；工具循环后的提醒不改变 seed，不重做本地解析/哈希 owner。 |
| [#8385](https://github.com/Wei-Shaw/sub2api/pull/8385) | 关闭 | 兼容路径对非默认 failover 状态也执行当前账号错误策略一次；余额不足处理顺序、映射模型和已有错误投影保留。 |
| [#8584](https://github.com/Wei-Shaw/sub2api/pull/8584) | 关闭 | 非流式 thought part 不混入正文；原 usage/thought token 及图片 Markdown 行为保留。 |
| [#8621](https://github.com/Wei-Shaw/sub2api/pull/8621) | 关闭 | 旧模型工具结果先连续输出全部 functionResponse，再追加媒体；Gemini 3+ 继续 functionResponse.parts，保留当前单次解码 owner。 |
| [#8568](https://github.com/Wei-Shaw/sub2api/pull/8568) | 关闭 | Pro Argon 使用独立 family scope，写入与读取一致；Ultra/未知计划保持共享 Gemini scope；图片不进入文本额度池。 |
| [#8566](https://github.com/Wei-Shaw/sub2api/pull/8566) | 关闭 | 每实例 refundSuccessCode 默认 1、显式可选 0；数值严格相等，禁止 1.5/0.5 截断为成功；订单查询仍使用原 1 码，不按成功码尝试重复退款。 |
| [#8631](https://github.com/Wei-Shaw/sub2api/pull/8631) | 关闭 | 当前 owner 已在 hydrate 失败时对 acquired 槽位释放，CPA/粘性/失败排除/等待计划逻辑保留，不重复修复。 |
| [#8637](https://github.com/Wei-Shaw/sub2api/pull/8637) | 关闭 | 校验后 Lua 原子消费，错误时拒绝签发；不改变 Redis key/JSON/TTL 或 session binding。并发读取仅一个 winner，缓存不可用不签发。 |
| [#8161](https://github.com/Wei-Shaw/sub2api/pull/8161) | 关闭 | 本项目保留显式 effort 才绑定分组策略、Sol/Astra none→low 与 Terra minimal→none 的既有规范化。没有上游提前 ValidateGPT61 拒绝及 none mapping 契约，不引入平行拒绝/映射策略改变旧默认。 |
| [#8346](https://github.com/Wei-Shaw/sub2api/pull/8346) | 关闭 | API Key 页面已异步分批加载用量，不重复实现。按当前 SQL 同时汇总 token 的契约增加覆盖列，迁移使用本项目下一个编号 257，并恢复 invalid index 后重跑。 |
| [#8581](https://github.com/Wei-Shaw/sub2api/pull/8581) | 关闭 | 刷新期间保留旧行与已知额度，行操作暂时 inert，首次加载仍用 skeleton；查询状态使用 aria-busy。 |
| [#8642](https://github.com/Wei-Shaw/sub2api/pull/8642) | 关闭 | 独立的六位倍率验证/持久化/展示，保留币种结算精度、订单快照及旧值；补齐大有限浮点缩放溢出及极小正值不得落零的保护。 |
| [#8367](https://github.com/Wei-Shaw/sub2api/pull/8367) | 关闭 | 保存区改为页内底部 sticky，沿用原表单、权限、禁用和 step-up。 |
| [#8622](https://github.com/Wei-Shaw/sub2api/pull/8622) | 关闭 | page snapshot 严格核对 namespace/name/call_id/shape 后转换成不可信 user text；HTTP/WS 共用 owner，保留数值精度、重复键拒绝和历史。普通请求复用原 FCO 负向预检。 |
| [#8379](https://github.com/Wei-Shaw/sub2api/pull/8379) | 关闭 | Codex 原生 Images 的 X-Codex-Image-Turn-Id 请求不提前提交非流式 200；其他心跳、流式图片和可靠结算保留。 |
| [#8575](https://github.com/Wei-Shaw/sub2api/pull/8575) | 关闭 | 在 JSON/compact 变换和准入前拒绝非法 UTF-8，保留协议 400。 |
| [#8357](https://github.com/Wei-Shaw/sub2api/pull/8357) | 关闭 | 持久 publish 成功后唤醒本进程 worker；改为轮询选择一个配置中的 worker、有界信号合并，避免唤醒全部 worker 的空查询放大。跨实例/到期重试继续 500ms polling。 |
| [#8639](https://github.com/Wei-Shaw/sub2api/pull/8639) | 部分/专项暂缓 | 移植 Anthropic/Bedrock 目录、adaptive/签名重放、校验和 OpenCode。保留本项目 CLI 指纹 pin 及源码目录的当前元数据 owner；原生 speed=fast 计费与上游另一个 Codex descriptor owner 不夹带启用。标准价卡由 #8634 覆盖。 |

## 性能与正确性

审计队列继续先持久发布，再发送本进程唤醒信号。按配置中的 worker 轮转唤醒一个，信号有界合并，
避免上游每次入队唤醒全部 worker 造成空数据库查询；500ms 轮询和跨实例恢复仍在原 owner 中。
刷新令牌增加一次 Redis 原子消费，失败时不签发；不新增网络重试或无界排队。

API Key 用量索引迁移采用本项目编号 257 和 `CREATE INDEX CONCURRENTLY`，包含当前 SQL 实际读取的
cost 和四类 token 列；不能仅照搬上游 cost 覆盖列。恢复分支先移除同名 invalid index 再重跑。
索引会增加磁盘和写入维护成本，构建期间会消耗 I/O；没有增加表字段或改写历史用量。

Codex page context 复用共享 bootstrap 的 `function_call_output` 负向预检。普通大请求不会先做一遍
完整 JSON 解码；有效 page snapshot 以不可信 user 文本传递。并行工具媒体继续使用单次解码，
新增 encrypted_function_args 使用原始 JSON 并进入已有 builder，保持 null/空数组和 terminal 补全。
会话种子只读取首个会话项之前的 system/developer 前缀，后续工具循环提醒不改变粘性身份。

OpenAI 容量错误仅匹配实际错误字段及显式 `scope=model` 规则。命中后沿用现有规则 owner 的当前请求换号契约；
即使持久化失败，也不扩大为整号惩罚，后续请求仍按实际持久状态调度。
缺省/账号 scope 不把请求级容量错误变成整号惩罚，Grok 不进入该特例。已经输出内容后只更新模型状态，
不回放请求。TTFT 保留本项目 Codex 首事件策略，payload-aware 分类不改变语义输出及重试边界。

验证结果和固定夹具性能数据见 [验证记录](../diagnostics/upstream-sync-20261112/VERIFICATION.md)。

## 滚动升级与回退

本轮新增一条兼容旧二进制的索引 migration；Ent schema、Wire、账务命令、Redis key/JSON/TTL 和
会话绑定格式不变。先由一个迁移节点完成索引构建并确认 readiness，再逐个替换节点。
四阶段验证保留同一 PostgreSQL/Redis，不执行 Down SQL；回退后保留已建索引和历史结算。

以下新行为须在所有相关节点升级后再启用：

- 原子 refresh 消费只有在全部鉴权节点升级后才能保证并发单次兑换；混合版本期间旧节点仍可能非原子消费。
- Claude 5.2 目录和新规则只应在对应网关及结算节点就绪后开放。无白名单分组可先设只含旧模型的显式白名单，
  升级后再开放；空白名单表示不限模型。回退前撤下新模型并停用仅新节点理解的策略，已有模型冷却需等待到期。
- 六位充值倍率在所有支付节点升级后再保存。旧节点保存设置会按原两位精度重写；回退前先停止新充值，
  恢复已验证的旧精度配置，再恢复下单。已有订单继续按其冻结快照结算，不重算旧订单。
- EasyPay 缺省退款成功码保持 1。需要成功码 0 的实例只在所有退款 worker 升级后启用。
  回退前暂停这些实例的退款，改用验证过的兼容通道后再恢复；不得因成功码不匹配盲目重发退款。

索引构建时长及实际业务负载不由本地夹具保证；运行态、性能、CI 和镜像发布证据分别记录，不能互相替代。

## 继续保留的专项

- **#8606**：OpenAI 30m/12h cache-write 计数、独立价格、分区价卡、结算持久队列/救援重放、DTO 与
  混合版本读取必须一起适配。目前仍按已验证的聚合计费，不能借用 Anthropic 5m/1h 字段。
- **#8358**：OAuth pooled WS search activation 自动重连必须证明完整历史并有界保留全部输出。
  本轮不新增另一套历史状态源或自动回放；原工具 replay owner 保留。
- **#8639**：原生 `speed=fast` 的完整账务、未验证的 Antigravity 5.2 映射和上游另一套 Codex descriptor
  owner 不在本轮启用。原生/Bedrock 目录、adaptive、签名重放、输入校验、标准价卡和 OpenCode 子项已适配。

继承此前的 #8484 共享请求/传输预算、#8006/#8369 显式 Gemini cachedContents 和
#8319/#8476 资源级管理员 Key 专项。除非冻结补丁行为改变、当前 owner 回归或本地契约明确变更，不重开关闭子项。

本轮使用隔离 Docker 数据库及假上游，不调用真实模型账号；生产部署需要按上述顺序执行。
