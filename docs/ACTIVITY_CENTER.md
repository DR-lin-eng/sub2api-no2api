# 活动中心

活动中心面向已登录用户提供活动列表、详情、参与记录和奖励结果；管理员在 `/admin/activity-center/campaigns` 创建和维护活动。用户入口由 `activity_center_enabled` 控制，但资格、时间窗、库存、频率和奖励发放始终由后端校验。

## 活动类型

| 类型 | 行为 |
| --- | --- |
| `lottery` | 用户选择或使用默认奖池后进行加权抽奖；奖品可以是余额、订阅、卡密或其他已实现奖励类型。 |
| `checkin` | 按活动时区判断自然日签到，按连续天数发放周期奖励，并提供脱敏排行榜。 |
| `inflate` | 余额充值/兑换等已接入金额按活动配置增加到账金额，并记录原始金额、到账金额和膨胀比例。 |
| `redeem` | 兼容旧数据的兑换膨胀类型，新配置应优先使用明确的 `inflate` 类型。 |
| `custom` | 由管理端配置展示内容和引用 ID；是否有可执行奖励取决于对应业务接入。 |

活动状态为 `draft`、`active` 或 `archived`。只有 `active` 且当前时间处于 `starts_at`（含）到 `ends_at`（不含）之间的活动对用户可见。管理员列表可以看到非活动状态，用户列表不能据此推断管理配置。

## 抽奖与签到

抽奖奖池可以配置用户分组资格、每日参与上限、奖品权重、总库存和奖品代码。服务端在参与事务中重新加载活动、判断资格和每日限制，再按密码学随机数进行加权选择；库存为零的奖品不进入候选。中奖后奖励和参与记录在同一事务内落库，提交后的通知或缓存操作不改变事务结果。

签到配置包含活动时区、`weekly`/`biweekly`/`monthly` 周期、必需分组、每日奖励和连续签到模式。`monthly` 当前按 30 天计算；断签后连续天数从 1 重新开始。重复签到返回明确错误，不会再次发放奖励。排行榜只返回脱敏姓名或邮箱，不返回可识别的完整邮箱。

膨胀活动在已接入的兑换/余额流程中解析有效活动，生成膨胀后的到账金额并单独记录原始值。活动规则不应在前端复制；前端只显示服务端返回的结果。

## 入口与事实源

- 用户：`GET /api/v1/activity-center/campaigns`、`GET /api/v1/activity-center/campaigns/:id`、`POST /participate`、`GET/POST /checkin/*`、`GET /records`。
- 管理员：`/api/v1/admin/activity-center/campaigns` 的分页、详情、创建、更新、删除，以及 `/records` 查询。
- 前端 owner：[activity-center](../frontend/src/features/activity-center/README.md)。
- 领域规则：[activitycenter module](../backend/internal/modules/activitycenter/README.md)；HTTP 映射在 `backend/internal/transport/http/handler/activity_center_handler.go`。

接口的完整参数和响应以 routes、handler DTO 与 datasource 为准。不要把活动中心记录当作完整财务流水；奖励落库后仍由相应余额、订阅或卡密模块维护其事实。

## 验证

从仓库根目录执行：

```sh
cd backend && go test ./internal/modules/activitycenter
cd backend && go test ./internal/application/service -run 'TestActivityCenter'
cd frontend && pnpm exec vitest run src/features/activity-center
```

涉及迁移、库存并发或奖励事务时，还要运行 activity center repository integration tests。
