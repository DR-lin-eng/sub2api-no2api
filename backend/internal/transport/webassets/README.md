# Embedded Web Assets

本包负责把 `frontend` 构建产物嵌入发布版后端，并提供 HTML/静态文件缓存。

| 文件/目录 | 作用 |
| --- | --- |
| `embed_on.go` | release 构建嵌入 `dist/` |
| `embed_off.go` | 非嵌入构建的兼容实现 |
| `html_cache.go` | 注入公开设置后的 HTML 缓存 |
| `static_cache.go` | 静态资源缓存策略 |
| `dist/` | 前端构建输出，不手工编辑 |

前端输出目录由 `frontend/vite.config.ts` 指向本目录。
