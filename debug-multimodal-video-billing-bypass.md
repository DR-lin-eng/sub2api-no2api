# Debug Session: multimodal-video-billing-bypass
- **Status**: [CLOSED]
- **progress_status**: verified
- **Issue**: `.bin/design/multimodal-billing-audit.md` 报告 multimodal 视频计费链路存在 P0 级绕过（只取片不轮询 → 30 分钟全额退款；绑定上游 id 失败静默；归属写 Redis 失败 → 轮询 404），需要以运行时证据确认并修复。
- **Evidence Source**: Go 单元测试输出（本地文件，不调用任何付费 API）
- **Evidence Directory**: `C:\Users\mkbk\AppData\Local\Temp\trae-debug-multimodal-video-billing-bypass`
- **Debug Server**: 不适用（Go 后端计费逻辑用单元测试复现，避免付费 API）
- **Log File**: `C:\Users\mkbk\AppData\Local\Temp\trae-debug-multimodal-video-billing-bypass\pre-fix.log` / `post-fix.log`

## Reproduction Steps
1. 用 Go 单元测试构造内存版 usageBillingRepo / 假上游响应，复现 P0-1：创建冻结 → 不轮询 status → sweeper 过期 → 冻结被全额释放（记录 `Released`，余额退回）。
2. 复现 P0-2：`BindBalanceHoldUpstreamRequest` 返回错误（或 0 行受影响）时，`BindMultimodalVideoBalanceUpstream` 只打 Warn 并返回，记录永远 pending。
3. 复现 P0-3：归属绑定失败时创建请求仍然成功，后续轮询无法恢复归属。
4. 修复后重复同一批测试，对比 pre-fix / post-fix 结果。

## Hypotheses & Verification
| ID | Hypothesis | Likelihood | Effort | Evidence |
|----|------------|------------|--------|----------|
| A | P0-1：视频结算只由 `video_status` 触发，`/content` 与 sweeper 过期释放共同造成 0 元取片 | High | Low | Confirmed（pre-fix: `TestReproP0_1` PASS=漏洞复现） |
| B | P0-2：`BindBalanceHoldUpstreamRequest` 0 行受影响仍返回 nil，内存记录被标记已绑定而库中无 upstream id | High | Low | Confirmed（pre-fix: `TestReproP0_2`） |
| C | P0-3：归属写 Redis 失败只 Warn，后续轮询 404，永不结算 | High | Low | Confirmed（pre-fix: `TestReproP0_3`） |
| D | P1-1：`CaptureBalance` 失败只 Warn 并 `return nil`，响应仍回显冻结额，记录保持 pending 后被 sweeper 退款 | Medium | Low | Confirmed（pre-fix: `TestReproP1_1`） |
| E | P1-2/P1-3/P1-4：图片/视频档位被上游输出尺寸改写或静默默认 2K/480p，导致免费或误拒 | Medium | Med | Confirmed（pre-fix: `TestReproP1_2/3/4`） |

## Log Evidence
- pre-fix.log：7 个复现用例全部 PASS（即 7 个缺陷全部按预期复现）
  - P0-1 取片不结算 → sweeper 全额释放，`CaptureBalance` 从未调用
  - P0-2 绑定 0 行仍把内存记录标记为已绑定，库中查不到 upstream id，过期退款
  - P0-3 归属写入失败 → 轮询解析失败 → 冻结退款
  - P1-1 落账失败返回 nil、响应回显冻结额、记录 pending → 过期退款
  - P1-2 请求 1K 输出 4K → 计费档位被改成 4K
  - P1-3 无 size/auto → 默认 2K
  - P1-4 `4k`/空分辨率 → 默认 480p
- 运行方式：Docker `golang:1.26.10` + `go test -overlay`（overlay 仅屏蔽仓库中与本任务无关的陈旧测试文件，未修改仓库源码）

## Post-Fix Evidence
- post-fix.log：`go test -run TestRepro -count=5 -v`，35 个用例全 PASS（7 个回归用例 × 5 轮），无真实上游请求、无真实扣费。
- 断言语义已从「复现缺陷」反转为「阻断缺陷」：
  - P0-1：`/content` 交付前调用 `settleMultimodalVideoContentBalance`，`CaptureBalance` 恰好 1 次，记录 `settled`，sweeper 不再释放。
  - P0-2：repo 仅 `RowsAffected == 1` 才返回 nil；命中 0 行 → `ErrBalanceHoldNotFound` → service 返回 `ErrBalanceHoldFailed`，内存记录不被标记已绑定。
  - P0-3：归属写缓存失败返回 `ErrBalanceHoldFailed`；缓存不可用时 `ResolveMultimodalVideoBalanceAccountID` 用冻结记录恢复归属，且校验 user/apiKey/未释放状态。
  - P1-1：落账失败返回错误、响应不含 `cost`、记录保持 pending；宽限期内 sweeper 不退款；下次轮询重试成功回显 `"cost":0.5`。
  - P1-2：请求尺寸可分类时 `ImageBillingSizeLocked=true`，`RecordUsage` 跳过 `ApplyOpenAIImageBillingResolution`；测试同时证明缺少锁定位时 1K 会被改写为 4K。
  - P1-3：请求未给可分类尺寸时不再猜 2K，只要求存在任一档位价；落账侧 `calculateOpenAIImageCost` 对无法分类尺寸直接报错。
  - P1-4：`ClassifyVideoBillingResolution` 严格分类，`ValidateMultimodalVideoResolution` 在提交上游前拒绝 `4k`/`1440p`，`GrokMediaEndpoint.IsVideoGenerationRequest()` 已补齐供 handler 使用。
- 回归范围：
  - `go build ./...` 通过。
  - `go test ./internal/infrastructure/repository/... ./internal/transport/http/...` 全 PASS（含 handler、routes、middleware）。
  - `go test ./internal/application/service/` 仅 `TestCompositeGroupSchedulerHasAllCanonicalPlatformBuckets` 失败，属基线问题：`scheduler_snapshot_service.go` 与 `composite_platform_test.go` 均未在本次改动中修改，`schedulerSnapshotPlatforms()` 已包含 `multimodal` 而测试期望列表未同步。与本任务无关，未改动。

## Verification Conclusion
7 个缺陷（P0-1/P0-2/P0-3、P1-1/P1-2/P1-3/P1-4）全部由「pre-fix 复现 PASS」转为「post-fix 回归 PASS」。修复点集中在：取片/轮询交付前结算、冻结绑定强一致、归属可回退到冻结记录、落账失败不冒充成功、图片档位锁定为请求档位、无法分类的尺寸/分辨率直接拒绝而非默认 2K/480p。

## Final Evidence Gate
- **Evidence Sources**: `pre-fix.log`（7/7 复现）、`post-fix.log`（35/35 回归）、`go build ./...`、repository/handler/routes 测试输出
- **progress_status**: verified
- **Hypothesis Conclusions**: A/B/C/D/E 全部 Confirmed，且已在 post-fix 中被阻断
