# 上游同步审查记录（2026-08-19）

本次审查以本项目 `origin/main` 的 `f2442f069` 为基线，将上游
`Wei-Shaw/sub2api` 的 `main` 最终冻结在
`82f7dd14f717bef480879f73cba288791b9b9663`。本批审查的合入 PR 为
#5782、#5749、#5676、#5780 和 #5794。

## 选择性同步

- #5782（中国区供应商账号测试路由）未重复移植。上游该修复依赖独立的
  `kimi`、`zhipu`、`deepseek` 账号平台，而本项目将 OpenAI 兼容供应商建模为
  `openai` API Key 加自定义 `base_url`。现有 `AccountTestService` 已按
  `openai_compat` 能力标记在默认和显式模式下路由 `/v1/chat/completions`，并有
  真实 SSE、4xx、超时和非 JSON 响应测试覆盖。
- #5749（Sora 残留清理）按本项目路径完成语义移植：移除三份 README 的失效说明、
  `deploy/config.example.yaml` 中不再生效的示例键，以及中英文未引用 i18n 键。
  历史 Sora 迁移文件保持不变。
- `PublicSettings.sora_client_enabled` 没有直接删除，保留恒为 `false` 的废弃字段，
  以维持滚动升级期间的 JSON 形状；它不再进入 SSR 注入，也没有前端消费方。
- #5676（OpenAI 容量降载恢复）移植缺失语义，没有替换本项目已有首输出实现：
  无 code 的 overload 文案与结构化 code 一样按请求级瞬时故障处理；原生 SSE、
  passthrough SSE 和 WS HTTP bridge 在首个语义输出前暂存生命周期帧，失败时丢弃，
  输出后禁止 replay。客户端收到的致命容量 code 会改写为可重试的 `server_error`。
- #5780 是上游渠道监控“配额模式”的后续审计修复。本项目没有引入该模式、相关
  `check_mode/account_id` 协议或 CN provider 独立账号平台，因此没有把补丁扩展为
  新功能；现有主动/被动渠道监控保持不变。
- #5794 的 Star History 可用地址已同步到三份 README，并继续指向本项目自己的
  `DR-lin-eng/sub2api-no2api`，没有带回上游推广或赞助内容。

## 性能与升级影响

#5676 仅在 OpenAI 流的首个语义输出前增加暂存；正常 token 解析与输出后的热路径不做
容量文案扫描。原生 SSE 复用 8 MiB 暂存器，超过 64 KiB 后写入权限为 `0600` 的
匿名临时文件；passthrough 同样复用该暂存器，WS bridge 使用 8 MiB 有界帧队列。
任何路径超限都在未提交语义输出时 failover，不允许无界堆内存增长。

#5782 的新增分支只发生在管理端账号测试，但本项目已有更通用的协议能力分流，因此
没有额外分支或分配。#5749 主要减少无效文档和前端资源；兼容字段保留避免旧客户端
在升级窗口出现协议形状变化。#5780 未进入本项目运行路径，因而不新增定时任务、账号
查询或前端请求。

## 复核入口

代码与测试仍是最终事实来源；完成同步后从仓库根目录运行 `make check-docs`，并按
`AGENTS.md` 选择 Docker 后端、前端和生产构建验证。发布时以推送提交的精确 SHA
独立核对 GitHub Actions 的 `CI`、`Security Scan` 和 `Docker Image` 结果。
