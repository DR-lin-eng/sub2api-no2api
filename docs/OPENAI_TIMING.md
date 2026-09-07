# OpenAI 请求计时

## 口径与文档

OpenAI 官方 [Codex Metrics](https://developers.openai.com/codex/config-advanced/#metrics)
分别列出 engine IAPI TTFT、engine service TTFT、turn TTFT 和端到端耗时。
这些指标的边界不同。本次查询没有找到 `responsesapi.websocket_timing` 四项字段的
公开完整契约，因此实现将其作为可选上游遥测，不依赖其存在，也不推导未提供的耗时。

提供的返回样本中，计时来自独立事件 `responsesapi.websocket_timing.timing_metrics`，
位于 `response.completed` 之前，而不是 usage 字段或响应头。

| 字段 | 样本原值 | 使用方式 |
| --- | --- | --- |
| `first_sampled_message_ttft_ms` | 470 | 管理员查看采样首字，不与服务层 TTFT 相加 |
| `engine_service_ttft_total_ms` | 690.87897 | 用户明细首字优先值，按毫秒四舍五入为 691 |
| `engine_queue_max_ms` | 74 | 管理员查看排队耗时，不再次计入首字 |
| `total_turn_time_s` | 1.047620254 | 用户明细耗时优先值，乘 1000 后四舍五入为 1048 ms |
| `responsesapi_duration_excl_client_tools_ms` | 1413.618964 | 管理员额外参考 API 层耗时 |

上游轮次耗时不等于客户端实际等待时间。首字和总耗时之间的差值也不能单独证明网络问题：
两套计时包含的排队、处理、重试、缓冲和传输边界不同。管理员必须结合本地请求链路排查。

## 存储与展示

- `usage_logs.first_token_ms` / `duration_ms` 继续保存原有本地观测值。
- 新增可空 JSONB 列 `openai_timing`，只保存明确允许的计时字段，不保存完整响应、提示词或凭据。
- 用量明细 DTO 的 `first_token_ms` / `duration_ms` 优先使用有效上游值，分别缺失时分别回退到本地值。
- 普通用户页面、详情和明细导出保持原来的首字/总耗时展示；用户 DTO 不增加来源或诊断字段。
- 管理员 DTO 另外返回 `local_first_token_ms`、`local_duration_ms`、`first_token_source`、
  `duration_source` 和 `openai_timing`。列表并列显示本地与引擎首字，详情显示完整对比与差值。
- Ops、账号聚合、统计摘要、调度、首输出超时、重试和计费继续使用既有本地口径，不受展示投影影响。
- 历史记录和其他平台没有遥测时保持原值；图片/视频不会因为上游遥测重新获得 TTFT。

## 采集边界

采集覆盖 Responses HTTP/SSE、HTTP 透传、Responses 到 Chat/Messages 的流式和缓冲转换、
WS 转发、WS/HTTP bridge 和 WS v2 passthrough。每个 attempt 或 turn 独立持有 collector。
按 `response_id` 匹配遥测，未知或已结束的 WS 轮次不会由计时事件创建；不为等待遥测延迟响应结束。
支持数值 0，拒绝负值、非数值、非有限值和超出毫秒整数范围的值；字段独立缺失时不补零。
当 `num_engine_calls` 明确不是 1 时，累计 engine service TTFT 不作为整轮首字，保留本地首字。

事实源：

- [`openaitiming`](../backend/internal/shared/openaitiming/)：结构化解析、校验和单位转换。
- [`openai_timing.go`](../backend/internal/application/service/openai_timing.go)：HTTP attempt 采集。
- [`passthrough_relay.go`](../backend/internal/application/service/openai_ws_v2/passthrough_relay.go)：WS 按轮次关联。
- [`openai_gateway_usage.go`](../backend/internal/application/service/openai_gateway_usage.go)：本地与上游数据双写。
- [`mappers.go`](../backend/internal/transport/http/handler/dto/mappers.go)：用户投影与管理员字段边界。
- [`UsageDetailDialog.vue`](../frontend/src/features/admin-usage/presentation/widgets/UsageDetailDialog.vue)：管理员对比。

## 升级与验证

迁移 `239_add_usage_openai_timing.sql` 只增加可空列，不回填或覆写历史计时。
旧版本代码可以忽略新增列。代码回滚时保留该列和数据；不要通过删除遥测列回滚应用。

从 `backend/` 执行相关单元测试：

```sh
go test ./internal/shared/openaitiming ./internal/application/service/... ./internal/transport/http/handler/dto ./internal/infrastructure/repository
go test -tags integration ./internal/infrastructure/repository -run 'TestOpenAITimingPersistence|TestUsageLogRepoSuite|TestUsageLogRepositoryCreate'
```

从 `frontend/` 执行：

```sh
pnpm exec vitest run src/features/admin-usage src/features/usage src/core/i18n/__tests__/routeLocaleCoverage.spec.ts
pnpm run typecheck
pnpm run lint:check
pnpm run build
```

数据库集成测试使用独立 Docker PostgreSQL/Redis，验证单条、批量、best-effort 和无返回值写入路径，
并确认回读保留本地计时、上游小数精度及原有 token 数。
