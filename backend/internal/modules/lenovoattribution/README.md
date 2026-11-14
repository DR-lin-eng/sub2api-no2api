# Lenovo attribution

`state.go` 保存独立归因 POST 的 JSON、204 响应头规则、7 天/6 小时检查间隔和失败退避。
`State` 由账号仓储原子保存在 `accounts.extra.openai_lenovo_attribution`，包含首次生成的安装 UUID、身份摘要、结果、重试时间和有界租约，不保存 token 或硬件序列号。

应用层只在管理员打开独立网关开关、账号实际使用且到期时启动有上限的后台获取。它是网关主动启用的兼容功能，没有实现 App 的 Windows/Lenovo partner 检测；未配置真实序列号摘要时发送 App 支持的 installation-only body。
`X-Lenovo-Device-Recorded` 只从归因 POST 的 204 响应消费。它不是普通模型请求头，`204` 本身也不证明设备已记录。

验证：在 `backend/` 运行 `go test ./internal/modules/lenovoattribution`；账号隔离、请求和持久化测试分别在 application/service 与 infrastructure/repository。
