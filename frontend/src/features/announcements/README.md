# Announcements

本 feature 持有 `/admin/announcements` 的公告管理，以及应用壳层中的用户公告列表、弹窗队列和已读状态；没有独立用户公告路由。

- [data/datasources](data/datasources/)：管理员 CRUD/阅读统计与用户可见公告/标为已读接口。
- [presentation/pages/AnnouncementsPage.vue](presentation/pages/AnnouncementsPage.vue)：管理页面。
- [presentation/widgets](presentation/widgets/)：目标用户条件编辑和阅读状态查看。
- [presentation/stores/announcementsStore.ts](presentation/stores/announcementsStore.ts)：用户列表、未读数、弹窗排队与当前会话内去重。

可见性与定向条件由 [announcement_service.go](../../../../backend/internal/application/service/announcement_service.go) 决定。Store 普通拉取按 20 分钟节流，失败后允许重试；缓存最多保留本次返回的前 20 条，不等于后台公告总数。退出或切换用户应清空公告与弹窗会话状态。

从 `frontend/` 执行 `pnpm exec vitest run src/features/announcements`；定向规则变更还需后端 announcement targeting 回归，弹窗生命周期变更应补充 Store 测试。
