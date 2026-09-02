# Vue Router

本目录定义前端路由、访问元数据、导航守卫、页面标题和首次设置重定向。完整路由列表以 `index.ts` 为唯一事实源，不在 README 复制一份容易过期的清单。

## 文件索引

| 文件 | 作用 |
| --- | --- |
| `index.ts` | 路由记录、全局 guard、路径规范化、滚动恢复、预加载和 chunk 错误恢复 |
| `meta.d.ts` | `RouteMeta` 类型扩展 |
| `setupRedirect.ts` | setup 已完成时的目标路径选择 |
| `title.ts` | 站点名、i18n 和自定义菜单标题解析 |
| `__tests__/` | guard 辅助、标题和 setup 重定向测试 |

## 路由域

- `/setup`: 首次安装。
- 公共/认证：`/home`, `/login`, `/register`, OAuth 回调、密码重置、法律文档等。
- 用户：dashboard、API Key、用量、订阅、支付、渠道、个人设置等。
- 管理：dashboard、Ops、账号/分组/渠道、用户、支付、设置、审计和风控等。
- `/:pathMatch(.*)*`: 404。

查精确路径或组件时：

```sh
rg -n 'path:|name:|component:' frontend/src/core/routes/index.ts
```

## Route Meta

`meta.d.ts` 当前定义：

| 字段 | 含义 |
| --- | --- |
| `requiresAuth` | 是否登录；未设置时默认为 `true` |
| `requiresAdmin` | 是否要求管理员 |
| `requiresPayment` | 是否要求内部支付功能启用 |
| `requiresRiskControl` | 是否要求风控功能启用 |
| `requiresSupportChat` | 是否要求在线客服功能显式启用 |
| `requiresMediaStudio` | 是否要求媒体工坊功能显式启用 |
| `title`, `titleKey`, `descriptionKey` | 页面标题和 i18n 元数据 |
| `breadcrumbs`, `icon`, `hideInMenu` | 导航展示元数据 |

新增字段时同时修改 `meta.d.ts`、guard/标题消费者和测试。

## Guard 顺序

`beforeEach` 的主要顺序是：

1. 将包含大写字母的 pathname 替换为小写规范 URL，并保留 query/hash。
2. 启动导航 loading，并首次恢复内存 access token。
3. 根据站点设置和路由元数据生成标题。
4. 处理 `/setup` 已完成重定向。
5. 处理公共路由、登录态和 backend mode 公共白名单。
6. 验证登录和管理员角色。
7. 确保支付、风控、客服和媒体工坊所需的公开设置已加载，再应用功能开关。
8. 应用 simple mode 和 backend mode 的访问限制。
9. 导航完成后停止 loading 并触发空闲预加载。

Vue Router 默认允许大小写变体匹配；规范化必须在加载 route-scoped locale 之前执行，否则例如 `/ADMIN/dashboard` 会只加载 base 文案并显示原始 i18n key。

动态 import 在部署更新后可能失效。`router.onError` 对 chunk load error 做一次受控刷新；修改时必须保留防循环机制。

## 添加路由

1. 页面放入所属 `features/<domain>/presentation/pages/`；真正跨业务的公共页面才放入 `common/pages/`。
2. 使用 `() => import(...)` 懒加载。
3. 显式填写 `requiresAuth`、`requiresAdmin` 和功能开关 meta。
4. 添加 `titleKey`，并同步 locale。
5. 更新菜单可见性，但不要把菜单隐藏当作权限校验。
6. 为 guard 分支、setup/功能开关或复杂重定向添加测试。

后端仍必须执行对应 JWT、Admin 或 step-up 校验。前端 Router 只负责导航体验。
