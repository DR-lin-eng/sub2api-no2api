# 权限组与客服账号使用教程

> 适用对象：站点管理员。本文介绍自定义人员权限组，不是模型分组、订阅套餐或 API Key 分组。

## 1. 功能与默认行为

在 **系统设置 → 权限组** 中，可以新增权限组、设置显示名称、勾选权限并独立保存。在 **用户管理 → 新建用户 / 编辑用户 → 角色** 中，为账号选择该权限组。

升级不会改变已有 `admin` 和 `user` 账号：完整管理员保留原有功能，普通用户不会自动成为客服。迁移 `238_permission_groups.sql` 只在配置不存在时新增默认组，不覆盖已保存的配置。

默认 **客服** 组（ID：`support`）包含：

| 权限 | 可以做什么 |
| --- | --- |
| `support.read` | 查看客服收件箱、会话、消息、素材与未读计数 |
| `support.write` | 回复、上传、撤回、标记已读/未读，管理快捷回复和客服素材 |
| `users.read_basic` | 从客服会话打开普通用户的基本资料 |

**默认客服不具备**余额转账、修改余额/配额、修改密码、查看 API Key、系统设置、上游账号管理、权限分配等权限。

基本资料包含用户 ID、邮箱、用户名、角色、状态、余额快照、并发/RPM、备注、调度等级与时间字段；不包含密钥、身份绑定和通知收件人。`users.read_basic` 不开放完整用户管理列表。

## 2. 开始前

1. 使用完整管理员（`role=admin`）登录。
2. 确认前后端均已更新到包含本功能的同一版本，应用启动迁移成功。
3. 保留至少一个完整管理员账号。修改员工权限应使用管理员自己的会话，不共享管理员账号。
4. 首次进入管理界面时，按现有页面完成管理员确认流程。

## 3. 启用在线客服

1. 打开 **系统设置 → 功能开关**。
2. 找到 **在线客服**，开启 **启用在线客服**。
3. 点击主设置页的 **保存设置**。

预期结果：用户端出现在线客服入口，有 `support.read` 的员工能访问客服收件箱。权限组和功能开关是独立配置：给账号分配“客服”不会自动打开在线客服。

## 4. 配置权限组

1. 打开 **系统设置 → 权限组**（也可以访问 `/admin/settings?tab=permission-groups`）。
2. 使用内置“客服”，或者点击 **新增权限组**。
3. 填写组名称，勾选所需权限。
4. 点击这个页签自己的 **保存权限组**。
5. 保存成功后重新打开页签，确认名称和权限已持久化。

规则：

- 名称为 1–50 个字符，名称不能重复。
- 每组至少一个权限，权限必须来自界面提供的目录。
- 内置客服组可以改名、调整权限，但不能删除。是否内置由服务端决定。
- ID 是用户 `role` 字段保存的稳定标识，不随显示名称变化。新建组由界面生成 ID；接口 ID 为 1–20 个小写字母、数字、下划线或连字符，以字母或数字开头，不能使用 `admin`、`user`。
- 保存失败时草稿仍保留，修正后可以再次保存；加载失败时点击重试。
- 权限组配置和给员工分配角色仅由完整管理员操作。`settings.manage` 不授予权限组编辑或管理员 API Key 管理能力。

### 常用配置示例

| 组名称 | 建议勾选 |
| --- | --- |
| 客服 | `support.read`、`support.write`、`users.read_basic` |
| 客服质检（只读） | `support.read`、`users.read_basic` |
| 客服财务 | 默认客服权限，再加 `support.balance_transfer` |
| 运营概览 | `dashboard.read` |
| 用户资料维护 | `users.manage`；如需改邮箱/密码，再加 `users.credentials`；如需改限额，再加 `users.billing` |

权限不会互相隐式补齐。例如单独授予 `support.write` 不会获得收件箱读取权限；需要使用客服界面时应同时授予 `support.read`。余额转账仍遵守原有业务余额检查和幂等约束。

## 5. 为员工分配客服角色

1. 管理员打开 **用户管理**。
2. 新建员工账号，或编辑一个已有普通账号。
3. 在 **角色** 下拉框中选择“客服”或自定义组的名称。
4. 保存用户。
5. 用独立浏览器会话登录该员工账号。

预期结果：在线客服开启时，客服登录后进入 **客服收件箱**，而不是无权限的管理仪表盘。关闭客服时，该员工转到个人资料页面，避免循环跳转。其他自定义组会进入可访问的管理页面；没有可进入页面的组落到个人资料。

后台按每次请求读取当前权限组；浏览器菜单依赖 `/auth/me` 的权限快照。角色或权限修改后，刷新登录会话以同步界面。已经建立的客服 WebSocket 连接沿用其握手鉴权直至断开或到期；如需立即收回访问，应禁用账号并结束现有连接。

## 6. 验收清单

使用一名普通用户与一名默认客服验证：

| 操作 | 默认客服的预期结果 |
| --- | --- |
| 用户发起会话，客服读取、回复 | 成功 |
| 在会话中打开普通用户资料 | 成功，只显示基本字段 |
| 查看管理员/其他员工的用户资料接口 | `403` |
| 直接访问 `/admin/settings`、`/admin/accounts` | 前端跳转至可访问页；直接 API 请求返回 `403` |
| 请求完整用户列表、用户 API Key、余额修改 | `403` |
| 客服余额转账 | 默认 `403`；增加转账权限后才开放 |
| 删除组内某个权限后重新发起对应请求 | 按新权限判断 |
| 普通用户直接调用 Admin API | `403` |

自定义角色对未纳入权限目录的后台业务默认拒绝。例如 `dashboard.read` 不开放 Ops，`users.manage` 不开放订阅/卡密，`settings.manage` 不开放审计/支付页面。部分管理页面的关联面板需要额外业务权限；遇到 `403` 时依据接口对应权限核对，不要直接改成完整管理员。

## 7. 其他可配置权限与边界

| 权限 | 范围 |
| --- | --- |
| `users.manage` | 普通用户列表、创建、编辑、删除与用户属性；不允许管理员工或分配员工角色 |
| `users.credentials` | 普通用户 API Key 读取、登录身份绑定；更新邮箱/密码还须具备用户编辑权限 |
| `users.billing` | 普通用户余额、并发/RPM、调度等级、模型分组、配额；批量操作需显式选择不超过 500 个普通用户，员工不能使用 `all=true` |
| `settings.manage` | 系统设置接口；权限组及管理员密钥接口仍仅限完整管理员。设置包含认证等高权限配置，应只交给受信任的配置维护人员 |
| `groups.manage` | 模型分组及路由，不是人员权限分配 |
| `accounts.manage` | 上游账号、巡检、代理和出口；现有二次验证要求保持不变 |

Admin API Key 的既有 scope 与人员权限组是两套机制。本功能不把员工角色自动转换成管理员 API Key，也不取消已有 scope 校验。

## 8. 常见问题

### 看不到“权限组”页签

检查是否为完整 `admin` 账号，并确认浏览器已加载新前端。普通员工即使获得 `settings.manage`，也不显示权限组管理页签。

### 分配客服后看不到收件箱

先检查在线客服开关，再检查该组是否有 `support.read`，最后刷新登录会话。只分配 `users.read_basic` 不产生独立用户列表入口。

### 保存报名称重复、空权限或非法 ID

保留草稿并修正对应项，再点击“保存权限组”。通过 API 保存时提交的是**完整组列表**，不是追加操作；请先 GET 当前配置并保留内置客服组。

### 移除自定义组后，原成员会怎样

原成员的 `role` ID 保留，但未知组不会得到后台权限。先把成员改分配到其他组再删除旧组。不要在 API 中复用已删除的组 ID：旧成员仍引用该 ID，复用会让它们获得新组权限。

### 如何查看技术接口

参见 [Admin API：权限组与客服角色](ADMIN_API.md#权限组与客服角色)。接口权限与数据字段以服务端为准。

## 9. Docker 回归方法

在独立测试数据库和 Redis 上验证，不复用生产数据。仓库根目录执行：

```sh
# 运行后端相关单元测试（含 unit build tag）
docker run --rm -v "$PWD":/workspace -w /workspace/backend \
  golang:1.26.6-alpine sh -c \
  'go test -tags=unit -count=1 ./internal/application/service ./internal/transport/http/server/middleware ./internal/transport/http/handler/admin ./internal/transport/http/server/routes'

# 使用独立依赖目录运行前端检查
# 首次执行会下载锁文件中指定的依赖。
docker run --rm -v "$PWD":/workspace -v permission-groups-node-modules:/workspace/frontend/node_modules \
  -w /workspace/frontend -e CI=true node:24-alpine sh -c \
  'npm install -g pnpm@11.17.0 && pnpm install --frozen-lockfile && pnpm run typecheck && pnpm run lint:check && pnpm exec vitest run src/core/routes/__tests__ src/features/admin-settings/__tests__ src/features/auth/__tests__/authStore.spec.ts src/features/support-chat/__tests__'

# 构建实际运行镜像（需要 BuildKit/buildx）
docker buildx build --load -t sub2api-permission-groups:local .
make check-docs
```

本次验证的实际命令、字面输出、退出码和完整补丁见 [VERIFICATION.txt](../artifacts/permission-groups/VERIFICATION.txt)。运行时测试使用随机生成的测试凭据，不在文档中保存访问 token。

## 10. 升级与回退

### 仅撤销人员授权

1. 管理员将员工角色改回普通用户，或调整所属组的权限。
2. 刷新员工会话并验证 API 拒绝原有后台操作。
3. 停用不用的员工账号，保留完整管理员。

### 回退到旧版本程序

1. 先导出权限组配置和员工 ID/角色对应关系，并按部署流程备份数据库。
2. 将自定义角色成员转为 `user`；不要为了兼容旧版把客服改为 `admin`。
3. 恢复前后端同一旧版本镜像，验证完整管理员登录和普通用户行为。
4. 保留迁移记录与 `permission_groups` 配置。旧程序不读取此新配置，不需要删除迁移或修改历史校验和。

[ROLLBACK.sh](../artifacts/permission-groups/ROLLBACK.sh) 用于在**独立源码副本**上反向应用本次补丁并核对原始文件哈希；它不回滚数据库，也不重启部署。脚本拒绝在本任务的发布工作区执行，且遇到额外源码改动时停止，以免覆盖后续工作。
