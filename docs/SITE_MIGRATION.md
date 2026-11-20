# 双站迁移

两站都升级到包含 `sub2api migrate` 的版本后，可以从任一站点的应用容器发起迁移。发起方自动读取本站的数据库配置。对方只需提供 HTTPS 域名或 IP 和一个带迁移权限的管理员 Key。IP 直连需要证书包含该 IP。使用私有 CA 时，把 CA 加入发起站容器的信任库。反向代理须覆盖客户端传入的来源地址和协议头，发送实际的 `X-Forwarded-For`、`X-Forwarded-Proto: https`，并在后端配置精确的受信代理地址。

| 用途 | `--scope` | 写入位置和处理方式 |
| --- | --- | --- |
| 两个运营中的站点合并 | `all`（默认） | 接收站现有业务库。先提示已有数据，再导入无冲突记录，重新分配数字 ID 并核对关系 |
| 只迁普通用户登录 | `login` | 邮箱、用户名、密码哈希、账号状态、第三方身份、Passkey 和 TOTP。源站筛选字段 |
| 只迁登录和额度 | `login-quota` | 在登录资料基础上加入余额、累计充值、并发和 RPM、分平台限额与使用窗口。数据库保留金额精度 |
| 完整搬站 | `clone` | 接收端建立独立数据库，保留用户及业务 ID、站点设置、管理员、凭据、账务和历史关联，并恢复自定义页面文件。完成后手动切换 |

在**对方站点**进入「系统设置 → 安全 → 管理员 API Key」创建有有效期的独立 Key。按方向选择一项权限。只同步用户时，可将权限限制到用户资料，即便拿到这个 Key 也无法改选完整克隆。启用了管理员二次验证的站点，授予、延期或轮换迁移权限会校验二次验证。旧的 legacy Key、普通管理读写权限和用户 API Key 都不能访问迁移端点。

| 本机命令 | 对方要授予的权限 | 只允许用户资料的权限 |
| --- | --- | --- |
| `push` 推送到对方 | `admin.migration.import` | `admin.migration.users.import` |
| `pull` 从对方拉取 | `admin.migration.export` | `admin.migration.users.export` |

在发起站的 Compose 部署目录执行其中一条命令，按提示输入对方的 Key。CSV 路径指向容器内可写位置。每个任务使用新的文件名。

```sh
docker compose exec --user sub2api sub2api /app/sub2api migrate push --peer new.example.com --scope clone --report /app/data/clone-errors.csv
docker compose exec --user sub2api sub2api /app/sub2api migrate pull --peer old.example.com --scope all --report /app/data/merge-errors.csv
docker compose exec --user sub2api sub2api /app/sub2api migrate push --peer new.example.com --scope login-quota --report /app/data/user-errors.csv
docker compose cp sub2api:/app/data/user-errors.csv ./user-errors.csv
```

每次启动会打印任务 UUID。网络波动时程序会有限重试。进程退出后，在同一发起站使用原方向、地址、选择范围、Key 和任务 UUID 再次执行。默认报告位于发起站的 `pricing.data_dir`，文件名为 `migration-errors-<UUID>.csv`，本地文件以仅当前用户可读写的权限创建，已有同名报告受保护。

```sh
docker compose exec --user sub2api sub2api /app/sub2api migrate push --peer new.example.com --scope login-quota --resume <任务UUID> --report /app/data/user-errors.csv
```

两端在经过证书校验的 TLS 1.3 连接上交换临时 X25519 公钥。HKDF 结合共享秘密、管理员 Key、双方公钥、任务和资料范围派生 AES-256-GCM 密钥。每个密文块都绑定任务、种类、序号和结束标志，CSV 回程也加密。数据库保存密文和受保护的临时私钥。每个远端请求重新检查当前 Key、方向和权限。终止 TLS 的代理可见管理员认证头，应由站点控制。进度可在任务创建后 24 小时内恢复，成功落盘报告后清理两端检查点。超期数据在下次创建任务时清理。

**在现有站点合并时**，大小写不同的同邮箱和 Gmail 别名视为冲突。目标用户的密码、余额和角色保留，源用户及依赖该用户的业务记录按归属逐项处理。相同第三方身份或 Passkey、业务唯一键冲突、未结冻结和无法重建的归属关系也会进入报告。CSV 含数据表、源记录 ID、邮箱和中文原因，适合筛选核对。保护性暂停的上游账号也会列入报告。CSV 含个人信息，应按用户资料管理。单次中断使用原 UUID 续传，另起任务会重新判定冲突。两个独立任务之间不会仅凭源站自报 ID 认领上次迁入的用户。

合并时只允许安全的站名、样式显示等少数设置。接收站继续控制管理员权限、登录提供方、支付商户、SMTP、备份与外发地址、模型请求模板。第三方登录须在接收站配置相同的身份提供方；WebAuthn 的 RP ID 变化通常需要重新登记。源 TOTP 秘密通过加密通道传输，在接收站重新加密，目标总开关未启用时该用户列入冲突。普通合并不会续办未完成的支付或冻结任务。合并前让源站完成排队计费；导入 API Key 前请在所有接收节点关闭 `api_key_auth_cache.bloom.enabled`。合并在一个事务内写入，最终阶段需要锁住选中的业务表，安排在维护窗口。

**完整切换时**，应在建立最终快照前停止源站新业务写入，排空在途计费、队列和支付回调，并保持迁移管理入口可达。源数据库是同一事务视图，自定义页面文件要在快照期间保持稳定。每个任务最多传输 32 GiB 序列化快照，超限会失败。发起导出的一端需要相应大小的临时磁盘和数据库空间，接收端还要预留密文、恢复暂存、新库和 PostgreSQL WAL 空间。现有业务库的迁移任务记录与独立克隆库存储分开。

接收数据库角色需要 `CREATEDB` 才能创建独立克隆库。命令输出实际数据库名 `sub2api_clone_<UUID去除连字符>` 和接收节点的页面路径。复制或挂载该任务的 `pages` 目录到新部署的 `pricing.data_dir/pages`。用新部署连接输出的数据库名和独立的空 Redis，检查登录、余额、已有 Key、账单、页面及对象访问后再切流。多节点部署要让各节点看到同一份页面文件。克隆继承源站 JWT/TOTP 根密钥、管理员 Key、OAuth 与支付配置。两个站点属于同一凭据信任域，切换后的旧站应停止对外和后台处理。回退时先处理切流后产生的新订单和余额变动，再切回原库。

克隆覆盖 Sub2API 的 PostgreSQL 业务数据和 `pricing.data_dir/pages`。Redis 中的短期会话、锁、调度缓存以及节点心跳在新部署重建。S3 等外部对象保留原引用，变更存储桶时需要另迁对象。容器环境变量、TLS 证书、外部提供方回调地址和用户浏览器里的媒体文件由各自的部署或浏览器持有。
