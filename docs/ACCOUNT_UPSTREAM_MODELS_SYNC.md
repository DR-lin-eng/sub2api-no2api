# 批量同步账号上游模型

管理员在账号列表勾选账号（支持“选择全部结果”），点击“同步上游模型”。页面按每批 20 个账号顺序同步，显示进度及每个账号的上游模型数、新增数、成功/失败/跳过原因。成功账号自动保存，失败或跳过账号保持选中，便于修正配置后重试。

后端入口为 `POST /api/v1/admin/accounts/models/sync-upstream-batch`，请求体为 `{"account_ids":[1,2]}`；每次接受 1–100 个正整数 ID，重复 ID 只处理一次，返回 `total/success/failed/skipped/results`。结果顺序与去重后的输入一致。接口继承管理端鉴权、权限及审计链路。

## 同步规则

- 复用 `upstream_models.go` 的真实模型查询：OpenAI OAuth 使用 Codex manifest；OpenAI 兼容 API Key 使用 `/models`；Anthropic OAuth、setup-token、API Key、Gemini API Key/受支持 OAuth、Antigravity OAuth/兼容 API Key、Grok OAuth/API Key 及兼容国产渠道使用各自现有适配器。
- 只将未配置的上游模型新增为 `credentials.model_mapping` 的同名映射。保留已有白名单、别名、自定义映射及其它凭据，不删除上游已移除的旧模型。与单账号白名单编辑器的“同步上游”一致。
- 保存使用账号行锁和 JSONB 键级合并，保留同步过程中新增的自定义映射。账号凭据、平台、类型或代理在查询后变化时拒绝保存并提示重试，避免把旧上游结果写入新账号配置。
- 更新与 scheduler outbox 写入在同一 SQL 语句中提交；成功后刷新账号调度快照及模型聚合缓存。无新增模型的重复同步仍返回成功、新增数 0。
- 每请求并发 4 个账号，每账号最多 20 秒，总预算 2 分钟。查询失败、空模型列表或保存失败不清空原配置；一个账号失败不影响其它账号。
- Gemini Code Assist、Bedrock/service account、没有模型列表适配器的平台会返回失败或跳过原因。凭据影子账号提示同步母账号。不使用静态目录冒充查询结果。
- OAuth 账号手动保存显式映射后，后台 OAuth 能力同步仍遵循原有规则，不覆盖显式映射；可再次手动同步以增加后续上游模型。

批次请求中断时，页面无法确认该批已提交的账号，保留选中并提示重试。新增映射的合并是幂等的，重试不会覆盖已有映射。查询接口原始错误和凭据不进入批量结果。

## 代码入口与验证

后端用例为 `internal/application/service/upstream_models_batch.go`，持久化为 `internal/infrastructure/repository/account_repo_models.go`，HTTP 入口为 `internal/transport/http/handler/admin/account_models_batch.go`。前端流程归属 `admin-accounts` 的 `useAccountUpstreamModelSync.ts` 和 `UpstreamModelSyncResultsDialog.vue`。

从后端目录运行相关 service、handler/routes 与 repository 测试；从前端目录运行账号 feature 的 Vitest、typecheck、lint 和生产构建。真实 PostgreSQL 测试验证 JSONB 合并、并发凭据变更及 outbox 原子性；Docker 回放对比基线、修改版本及回滚版本的同一批量请求。
