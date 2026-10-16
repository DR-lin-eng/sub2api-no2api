# 上游主线增量同步审查（2026-10-16，后续批次）

本轮以本项目 main `eb130c485520fa27536e33064f39894972205362` 为基线，延续已关闭的上游边界 `b8cfb7884084393e08f0e2c4027afc3755357b98`，冻结至 `b0a02a3304a0a78cdd003ddda1dd23ba02c03084`。新增范围为 4 个合并 PR、一个重叠测试夹具提交和上游 VERSION；支付 PR #8122/#8133 已由本项目 #138 适配并关闭，本轮不重复处理。原链接工作树保持原始 HEAD 与字节，在其可写克隆执行和发布。

## 逐 PR 决定

| 上游 PR | 决定 | 当前 owner 与关闭内容 |
| --- | --- | --- |
| [#8114](https://github.com/Wei-Shaw/sub2api/pull/8114) | 适配并关闭冻结差异；配置写入做性能重构 | `gemini_compat_thinking.go` 和既有 Gemini compatibility service：2.5 使用有界 budget，3 及以上使用 level；图片/语音/向量/未知版本不添加档位，模型名已有档位时保留原优先级。流式摘要分别输出为 Anthropic thinking 与 Chat `reasoning_content`；历史推理不转为普通正文，签名重试保留客户端原始 thinking 意图。用量记录使用实际转发档位。 |
| [#8121](https://github.com/Wei-Shaw/sub2api/pull/8121) | 按现有映射与目录 owner 适配并关闭 | `openai_model_retirement.go`、account support、普通/高级调度、previous-response 粘性、compact 与 Codex 目录投影：ChatGPT OAuth/setup-token/影子账号拒绝实际上游目标 GPT-5.5，API-key 继续支持。管理员显式替代映射和渠道映射按现有单次映射链判断。目录在公开别名投影后过滤，保留 metadata、共享缓存和最终 ETag；明确空目录在缓存与普通/custom/allowlist 展示中保持为空。过滤后的普通 OpenAI 列表补齐 object/created/owned_by，并保留原有 type/display_name/created_at 展示字段。 |
| [#8080](https://github.com/Wei-Shaw/sub2api/pull/8080) | 复用现有搜索 owner，适配并关闭 | `gateway_websearch_emulation.go` 的共享 gate/writer 和 `OpenAIGatewayService.ForwardAsAnthropic`：仅 OpenAI API-key 的纯 WebSearch 请求允许跟随现有全局/渠道/账号开关；OAuth/setup-token 保持原生搜索。保留账号完整出口 context、搜索管理器的配额/代理策略及既有计费入口。创建/编辑字段和序列化归属原账号 feature。 |
| [#8140](https://github.com/Wei-Shaw/sub2api/pull/8140) | 补齐本项目未覆盖出口并关闭 | `openai_responses_created_at.go`、普通 HTTP Responses 流/JSON/SSE→JSON 与 HTTP→WS bridge：只对缺失/null 的 `created_at` 补值；同条流的补值使用固定时间戳，已有值保持原字节。apicompat 的合成响应与 compact failure 已在基线覆盖，不重复重构。原始 passthrough 模式继续保留其字节契约；本项目没有上游新引入的 bare-error→response.failed 合成分支，原有 error/failover 行为保持。 |

[机器台账](../diagnostics/upstream-sync-20261016-followup/UPSTREAM_PRS.json) 保存冻结 merge SHA、原始 diff 哈希、owner、子项与重开条件；[冻结输入](../diagnostics/upstream-sync-20261016-followup/UPSTREAM_REVIEW_INPUT.json) 固定历史台账哈希。旧的 covered/ported/intentional-local-divergence 和专项 deferred 决定沿用，只有新的上游语义或当前 owner 的实际回归才重开；tracking merge 登记祖先，不代表未实施的历史专项功能完成。上游 VERSION `0.2.17` 不覆盖本项目版本，根 README 的无赞助政策继续保留。

## 性能和兼容边界

Gemini thinkingConfig 原补丁每个字段调用一次 sjson，含档位和 includeThoughts 时复制整个历史两次。本项目先构造小配置，再一次插入；既有配置的非目标字段保留。Responses 时间戳检查使用调用内只读 string view，避免为读取字段复制完整终态 output；delta 没有 response 对象时不分配补丁。模型退休只使用已经加载的账号/映射和缓存，无数据库/注册表请求、后台 worker 或新增重试。

[PERFORMANCE.json](../diagnostics/upstream-sync-20261016-followup/PERFORMANCE.json) 记录相同 Docker Go 1.26.9、linux/arm64、2 CPU 下原始上游 reference 与适配版本的微基准样本；这些结果不代表完整网关吞吐。WebSearch 仅显式启用时调用既有第三方搜索，不新增模型生成请求。Gemini 客户端指定的思考档位现在真实转发，可能改变上游推理时间和实际 reasoning token；账单沿用实际 usage，并记录实际转发档位，不能把这项语义修复宣称为生成成本不变。

GPT-5.5 退休作用于 ChatGPT 凭据而非调用本站的 API Key。显式映射到可用新模型继续生效；不自动改写用户普通聊天模型，不删除历史价格。本站原有默认 compact 为 GPT-5.4，已经避开上游 GPT-5.5 默认值；显式 compact 目标与 fallback 另行按退休规则检查。

## 平滑升级和回退

本轮不新增 schema、迁移、依赖或必填配置。真实 Docker 同库 `BASELINE → MODIFIED → ROLLBACK → RESTORED` 验证 `/health`、`/ready`、嵌入前端、加密登录、API Key、订阅与迁移数量，持久快照保持相同。无新增全局启用开关；需要旧/新行为一致时，管理员等全部节点升级后再为 OpenAI API-key 配置搜索模拟，随后可通过原三态字段关闭。GPT-5.5 客户端需选择可用替代模型或支持该模型的 API-key 上游。

二进制回退仍受前轮支付约束：暂停退款管理并用新版结清 held/pending 退款，详见 [前轮支付同步](UPSTREAM_SYNC_20261016.md)。本轮没有修改退款实现，因此不重放支付补丁，也不取消既有回退条件。

## 验证与源码事务

[VERIFICATION.txt](../diagnostics/upstream-sync-20261016-followup/VERIFICATION.txt) 保存命令、输入、逐字 stdout/stderr、退出状态和错误修正；`records/` 保留完整机器记录。固定源探针在 BASELINE 中观察到 Responses JSON/SSE 无 created_at、OpenAI API-key 搜索未启用、OAuth GPT-5.5 可调度、Gemini 档位未转发；MODIFIED 补齐时间戳、允许配置搜索、拒绝订阅退休目标、真实转发 high；独立 ROLLBACK 恢复基线行为和原始哈希，再重施 DIFF 验证候选哈希。

四个角色为 [MODIFIED_FILE.tar.gz](../diagnostics/upstream-sync-20261016-followup/MODIFIED_FILE.tar.gz)、[DIFF_FILE.patch](../diagnostics/upstream-sync-20261016-followup/DIFF_FILE.patch)、上述验证账本与可执行 [ROLLBACK.sh](../diagnostics/upstream-sync-20261016-followup/ROLLBACK.sh)。回退只接受独立副本，通过 sibling baseline 还原本轮源码并删除新增路径；不修改用户原工作树。

现有 `payment_fulfillment_test.go` 1201 行导致 `make check-layout` 在精确基线和候选均退出 2；其字节未改，不扩大 allowlist。账号创建表单新增搜索字段时，仅将对应 extra 的纯转换移入已有 `accountFormPolicy.ts`，遵守模块行数门禁。后端完整 unit/integration、相关 race、lint、前端必需门禁、完整账号用例、生产镜像和最终四阶段运行记录分别取证。发布的 PR/head/main、Actions 和 GHCR revision/双架构清单在完成后单独保存，不能以本地构建代替发布证据。
