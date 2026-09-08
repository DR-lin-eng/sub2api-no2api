# Activity Center

本模块持有活动配置、用户可见性、参与资格、抽奖、签到和兑换膨胀的领域规则。操作说明见 [活动中心](../../../../docs/ACTIVITY_CENTER.md)。

## 职责与入口

- [campaign.go](campaign.go)：实体、Repository/RewardGranter 端口、配置校验、奖池与库存、参与记录、膨胀规则。
- [campaign_state.go](campaign_state.go)：活动类型与状态的白名单。
- [checkin.go](checkin.go)：按活动时区计算签到日、连续签到与周期奖励、脱敏排行榜。
- [activity_center_rewards.go](../../application/service/activity_center_rewards.go)：奖励发放和兑换膨胀的应用层接入。
- [activity_center_repo.go](../../infrastructure/repository/activity_center_repo.go)：持久化、参与事务、并发锁与提交后回调。

## 不变量

活动必须处于 `active` 且在生效时间窗内，用户还必须满足对应分组资格。库存、每日限制和当日签到判定在事务内复查；奖励发放与参与记录使用相同事务，缓存等副作用在提交后执行。

`redeem` 是兼容旧数据的膨胀活动类型。签到 `monthly` 表示 30 天循环，不是自然月；中断连续签到后重新从第 1 天开始。用户 DTO 不应泄露未抽中的卡密库存或管理端配置。

## 验证

从 `backend/` 执行领域与奖励回归：

```sh
go test ./internal/modules/activitycenter
go test ./internal/application/service -run 'TestActivityCenter'
```

事务、并发和迁移还需验证 repository 中的 `activity_center_repo_integration_test.go` 与 `migrations_activity_center*`，按 [后端测试约定](../../../AGENTS.md) 使用数据库环境。
