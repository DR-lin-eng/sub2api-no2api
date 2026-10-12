# xai

xAI/Grok OAuth、SSO、模型、额度和账单协议。文件按 `oauth`, `sso_device`, `models`, `quota`, `billing`, `cli_identity` 组织。

CLI 网关、账号测试和账单探测共用固定交互身份；`XAI_GROK_CLI_VERSION` 只接受不低于最低支持版本的规范 SemVer，官方 API 域名不注入 CLI 专属头。
