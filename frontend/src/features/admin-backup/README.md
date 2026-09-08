# Admin Backup

本 feature 的 `BackupPage` 嵌入系统设置的备份页签，当前没有独立 `/admin/backup` 路由。

## 两套接口

| 入口 | 作用 |
| --- | --- |
| [adminBackupDatasource.ts](data/datasources/adminBackupDatasource.ts) | `/admin/backups`：S3 配置、定时计划、备份记录、下载、恢复和异步图片存储 |
| [dataManagementDatasource.ts](data/datasources/dataManagementDatasource.ts) | `/admin/data-management`：备份 Agent 健康、PostgreSQL/Redis 来源配置、S3 profile 和任务协议 |
| [BackupPage.vue](presentation/pages/BackupPage.vue) | 当前页面主要消费第一套接口；持有操作、进度轮询和恢复轮询 |
| [backupStatus.ts](backupStatus.ts) | 两类任务及恢复状态的展示映射 |

`completed` 与 `succeeded` 分属不同协议，`partial_succeeded` 也不能展示成全部完成。分卷下载返回 `parts` 时应保留序号和大小，不假定总有单一 `url`。

## 操作边界

先保存并测试 S3，再配置定时计划或创建备份；创建响应仅代表任务已受理，按记录状态确认最终完成。恢复由页面确认、密码输入、step-up 与后端共同控制；恢复状态独立于原备份状态。离开页面需停止轮询。

图片存储可选择复用备份 S3 的连接信息，但保留自己的 bucket/prefix；这不等同于把图片内容纳入数据库备份。后端入口见 [backup handler](../../../../backend/internal/transport/http/handler/admin/backup_handler.go) 和 [备份服务](../../../../backend/internal/application/service/backup_service.go)。

## 验证

从 `frontend/` 执行 `pnpm exec vitest run src/features/admin-backup`。覆盖分卷下载、状态映射、错误和恢复交互；真实备份或恢复验证另需独立数据库及对象存储环境。
