# Admin Settings

系统设置 feature 负责设置读取、编辑、敏感保存与后台配置对话框。

- `adminSettingsStore.ts`: 跨 feature 读取运维开关时使用的稳定 Store 公开出口；实现仍归属本 feature。
- `data/dtos/adminSettingsDtos.ts`: 独立设置子域的请求/响应 DTO 与面板限流兼容归一化。
- `data/dtos/systemSettingsDtos.ts`: 主设置协议与注册、平台限额、支付、微信兼容归一化规则。
- `data/datasources/adminSettingsQueries.ts`: 已迁移设置子域的只读请求 owner。
- `data/datasources/adminSettingsActions.ts`: 已迁移设置子域的保存、恢复和预览请求 owner。
- `data/datasources/adminSettingsDatasource.ts`: 迁移期纯兼容 facade；新调用直接进入明确 DTO、Query 或 Action owner。
- `presentation/pages/`: 路由级加载、保存、step-up 与对话框编排。
- `presentation/widgets/settings-tabs/`: 按设置领域拆分的 tab 和 panel。
- `presentation/widgets/settings-tabs/gateway-resilience/`: Codex A/B、临时不可调度、冷却、流超时、请求修正与策略设置卡片；直接复用页面 context，由网关韧性 panel 按原顺序装配。Codex 查询失败时不得显示默认关闭状态，独立强制恢复 action 必须保持可用。
- `presentation/widgets/settings-tabs/identity-providers/`: LinuxDo、邮箱 OAuth、微信、钉钉与 OIDC 静态设置卡片；直接复用页面 context，由身份源 panel 按原顺序装配。
- `presentation/composables/`: 页面局部控制器、表单初始化和纯转换。
- `presentation/composables/settingsSavePreparation.ts`: 按页面既有顺序完成统一保存前的归一化与校验。
- `presentation/composables/settingsSavePayload.ts`: 按设置领域组装兼容 payload；新增字段放入所属 builder，不改变统一保存请求。
- `presentation/composables/settingsSaveResponse.ts`: 回填保存响应并清理敏感输入，保持后续缓存刷新与通知顺序。

新增设置项时，先确定所属 tab 和 datasource 字段，再把交互放入对应 controller。feature 内组件使用静态 import；不要把页面 context 提升为全局 Store，也不要通过 `@/api` 或 `@/stores` 兼容 barrel 新增依赖。保留单次设置加载、统一保存、敏感操作 step-up 和按需挂载语义。

邮件模板、面板限流、管理员 API Key、独立网关策略、Web Search、SMTP 测试及主设置均已迁入明确 owner。`adminSettingsDatasource.ts` 仅服务旧 `src/api/admin` 兼容出口；新调用不得继续扩展 `settingsAPI`。

验证入口：

```sh
pnpm exec vitest run src/features/admin-settings
pnpm run typecheck
```
