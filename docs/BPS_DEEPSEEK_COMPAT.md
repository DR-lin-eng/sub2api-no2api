# Excel BPS 与 DeepSeek 兼容

本次仅接入账号级 Excel/Basispoints Responses 转发与 DeepSeek 专用 Responses / Chat Completions 兼容层。账号选择、OAuth 凭据刷新、并发槽位、上游代理和用量结算继续沿用现有网关。

## Excel BPS

管理员在普通 OpenAI OAuth 账号的编辑表单启用“Excel BPS”；持久化字段为 `accounts.extra.excel_bps_enabled`，默认关闭。读取兼容旧字段 `openai_excel_bps`，新字段的显式 `false` 优先于旧字段的 `true`。API Key、影子账号、PAT 与 Agent Identity 不进入 BPS 分支。

`/v1/responses` 入口按当前选中账号判断开关：启用且不是单独的 compact 路径时，`openai_gateway_forward.go` 转交 `openai_excel_bps.go`，用现有账号 HTTP 上游传输请求 Basispoints，并由 `internal/shared/basispoints` 转换文本、工具续接和图片附件。工具目录、重放与附件缓存只在存在明确会话键时启用，并按账号、API Key 与会话隔离。流式和非流式结果都回到现有用量/计费链路；桥接不支持的原生托管工具显式返回错误，不静默切换上游。独立 compact 路径仍由现有网关处理。桥接协议能力以 `internal/shared/basispoints/README.md` 与测试为准。

账号列表具有独立 `excel_bps=enabled|disabled` 过滤器。筛选只覆盖普通 OpenAI OAuth 账号，并在 repository 的 PostgreSQL JSONB 谓词中**先于分页**执行；与平台、类型、OAuth 额度、状态及搜索条件组合。当前筛选同样传入批量选择、批量编辑、导出和上游计费快照，不由前端对已分页结果再裁剪。

批量编辑中普通 OpenAI OAuth 账号提供“不修改 / 开启 / 关闭 BPS”三态选择，提交 `excel_bps_enabled` 布尔字段；筛选目标同时提交 `filters.excel_bps`，后端与列表共用数据库筛选谓词解析完整目标。写入前校验所有目标账号资格（影子、PAT、Agent Identity 等不适用时整批拒绝），更新时在 SQL 再次检查资格。关闭写入显式 `false`，覆盖旧字段的 `true`；不接受通过通用 `extra` 批量绕过资格校验。

## DeepSeek

`openai_deepseek_compat.go` 限定 DeepSeek 账号或匹配的上游模型/主机：原生 Responses 转发去除不适用的有状态字段并归一化图片输入；当 DeepSeek 原生 Responses 遇到 Lite `additional_tools` 或原生 compact trigger 时改走 Chat 桥，避免工具声明被忽略或 compact item 缺失；转到 Chat Completions 的路径补齐缺少的历史 assistant `reasoning_content`、处理不支持的 `json_schema` 请求格式，必要时将 compact 请求变成摘要回合，再包装为 Responses compaction 输出。非 DeepSeek 请求保持原链路。

## 验证入口

- 后端：`go test ./internal/shared/basispoints ./internal/application/service ./internal/infrastructure/repository ./internal/transport/http/handler/admin`。
- 前端：`pnpm run typecheck` 与 `pnpm exec vitest run src/features/admin-accounts`。
- 文档：仓库根目录 `make check-docs`。
