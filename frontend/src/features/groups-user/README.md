# User Groups

本 feature 只持有普通用户的分组查询，没有独立页面、Store 或管理员 CRUD。

- [data/datasources/groupsUserDatasource.ts](data/datasources/groupsUserDatasource.ts)：`/groups/available` 返回可绑定分组，`/groups/rates` 返回本人自定义倍率，空倍率响应归一化为空对象。
- 主要消费者是 [keys](../keys/README.md) 和共享分组选择组件；管理配置归属 [admin-groups](../admin-groups/README.md)。

标准分组是否公开或获授权、订阅分组是否有有效订阅，均由后端判定。列表选择不替代创建 Key 与网关请求时的权限复查。倍率是服务端数据，展示时应保留未设置与显式值的区别。

## 验证

当前目录没有独立 Vitest。已有消费者回归从 `frontend/` 执行：

```sh
pnpm exec vitest run src/features/keys src/common/widgets/data/__tests__/GroupOptionItem.spec.ts
```

协议变化时补充 datasource 用例，并验证后端 available-groups 与 API Key 鉴权。
