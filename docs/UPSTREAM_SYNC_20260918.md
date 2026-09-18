# 上游主线同步审查记录（2026-09-18）

## 冻结点与选择性范围

| 项目 | 值 |
| --- | --- |
| 下游基线 | `4d1db9284b2b368db6fd476a45df3f7908774ec9` |
| 上一审查点 | `881f3202694c6bc932446931a30c27d9675178b9` |
| 本轮上游冻结点 | `efe9aab1e4ec89a42ba45e8dac20e882c5409a6a` |
| 新增范围 | 52 commits，26 个 first-parent merge PR |
| 分支 | `codex/upstream-sync-20260918` |

本轮不直接合并上游旧目录树。所有改动按当前 `application / infrastructure / shared / transport / features` owner 重新落位；不导入上游 VERSION、README 赞助内容或不兼容迁移。

## 已移植的上游行为

| 上游 PR / 提交 | 当前实现 | 升级与性能边界 |
| --- | --- | --- |
| #7246 / `881ab1b0c` | DeepSeek native Responses 的 `function_call_output` 图片移到后续 user message，保留 call pairing。 | 仅检测到 tool-output 与 image marker 才解码/重建；普通请求仍走原 sjson 快路径。 |
| #7256 / `be4a4990f` | Antigravity 只移除 system 开头的 Claude attribution 元数据，不修改 user/tool 同文案。 | 作用域限于 Antigravity transformer，原有 Anthropic 路径不变。 |
| #7224 / `18bfa4bf2` | 严格 Chat 上游将 `developer` 角色按目标主机/平台改为 `system`。 | 在账号和目的地选定后适配，原始 body 保留，重试其他账号不会污染。 |
| #7177 / `d7ee1ab6b` | 客户端取消后用 `WithoutCancel + 3s` 预算写入 Responses account affinity。 | 只影响终态 Redis 写入，不延长请求主链路。 |
| #7195 / `8e34ca5e3` | active 但手动暂停的 OAuth 账号继续后台刷新 token。 | 仍受状态、refresh token 和 retry cooldown 限制，不扩大 API-key 候选。 |
| #7207 / `18d483c2a` | 分组用量汇总先读水位，再把 `created_at >= $7` 作为参数传入 tail 查询。 | 使 PostgreSQL 能使用 `usage_logs.created_at` 索引，水位无效时回退 epoch 全量重算。 |
| #7234 / `1a32b91e` | 分页跳转兼容 number input 的数值模型。 | 仅 UI 输入归一化。 |
| #7183 / `0ed735d3b` | ProxySelector 批量测试复用单项 in-flight guard，避免重复请求。 | 并行度和单项错误展示保持原行为。 |
| #7185 / `6a4938bdf` | 公告批量已读使用 `allSettled`，成功项立即标记，失败项可重试。 | 新到达公告不会被错误标记。 |
| #7186 / `130ba634a` | Clipboard fallback 捕获 `execCommand` 异常并清理 textarea。 | 失败返回 false，不改变成功路径。 |
| #7239 / `fe36f4a9` | 注册页首屏使用注入的 public settings，禁用 promo 时不闪现输入框。 | 无注入值时 fail-closed，异步设置仍会覆盖最终状态。 |
| #7236 / `98321a05` | 充值金额输入拒绝非法字符后恢复上一次已接受文本。 | 保留小数编辑和清空语义。 |
| #7262 / `017e9e98` | 退款余额告警比较实际编辑的退款金额，而不是订单原金额。 | 部分退款和关闭余额扣除时行为不变。 |
| #7238 / `f8e5a0d9` | 模型标签输入只有存在待提交文本时才拦截 Tab。 | 空输入可正常离开控件。 |
| #7182 / `21532add` | 用户订单状态筛选切回第一页，手动刷新保留当前页。 | 只改变查询参数，不改变 API 协议。 |
| #7265 / `b51f0759` | 支付配置请求共享 in-flight Promise，失败后可重试。 | forced refresh 也不会产生重复并发请求。 |
| #7266 / `406be7c5` | 清空订阅状态立即重置 loading，旧请求完成不会恢复旧数据。 | 使用已有 generation guard。 |
| #7184 / `14636d2f`、#7235 / `ee9ac3e4` | TOTP 错误统一提取 API message，输入格受 store state 驱动。 | 不改变提交字段和步骤状态机。 |
| #7263 / `be313735` | BaseDialog 的标题 ID 计数器提升到 module scope。 | 保持多实例 aria-labelledby 唯一。 |

## 已覆盖、暂缓和不重复处理

- #7215 (`2f16e098`) 的 manifest 重复解析和精确 `models` 数组校验已存在于当前模块化 owner，本轮不重复改写。
- #7237 (`0838e0e6`) 的 platform quota 非负/有限数校验已在当前 admin handler 中存在，本轮只保留现有实现。
- #7212 兑换历史分页、#7173 Gemini native model listing 需要分别调整当前 feature/API owner，暂不把旧目录实现整体带入；列入下一专项。
- #7261 gRPC 依赖升级会连带大批 `go.mod/go.sum` 变化，功能代码本轮不混入；单独依赖升级后再跑 govulncheck。
- 上游 README/VERSION 及已撤回提交不导入，保持本项目版本和无赞助规范。

## 性能审查结论

1. 分组用量查询是本轮最大热点。原查询把运行期水位放在 CTE/CROSS JOIN 中，planner 无法把时间下界作为 index condition；新查询将水位读为参数，保留失效水位 epoch 回退，避免错误汇总。
2. DeepSeek 图片兼容只在两个 marker 同时出现时解码；无图片 native Responses 请求没有额外 JSON round-trip。
3. strict Chat role adaptation 只在受影响平台/主机执行，且每次账号尝试从原始 body 重建，避免 failover 累积变形。
4. OAuth 刷新仍是有界分页、provider gate 和 retry cooldown；暂停账号纳入不会引入全表扫描。

## 验证入口

完整命令、字面输出、退出码、回滚和 Docker 运行态记录在：

`artifacts/upstream-sync-20260918/VERIFICATION.txt`

四个必备工件：

- `artifacts/upstream-sync-20260918/MODIFIED_FILE`
- `artifacts/upstream-sync-20260918/DIFF_FILE`
- `artifacts/upstream-sync-20260918/VERIFICATION.txt`
- `artifacts/upstream-sync-20260918/ROLLBACK.sh`

候选镜像和隔离栈：

- `sub2api-upstream-sync-20260918:candidate`
- `diagnostics/upstream-sync-20260918/compose.yaml`

## 差异关闭与发布

完成本轮验证后，以 tree-preserving tracking merge 关闭已审查的 `881f3202..efe9aab1` ancestry，避免未来重复检查；不把未移植的 redemption/Gemini/dependency 专项伪装成已合并。分支推送后以最终 commit SHA 查询 GitHub Actions 的 Backend、Frontend、Security 和 Docker Image jobs；任何失败都以修复后的新 SHA 重跑。
