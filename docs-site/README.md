# Sub2API 文档站

面向 API 调用方的公开文档站，独立于后端和内嵌前端。站点由 [VitePress](https://vitepress.dev/) 生成，经 GitHub Actions 发布到 GitHub Pages：

<https://dr-lin-eng.github.io/sub2api-no2api/>

它不会被编译进 Sub2API 二进制或 Docker 镜像。

## 目录

| 路径 | 内容 |
| --- | --- |
| `index.md` | 首页 |
| `guide/` | 概览、快速开始、认证、流式响应、错误处理 |
| `api/` | 网关端点参考 |
| `clients/` | Claude Code、Codex CLI、Gemini CLI 和官方 SDK 的接入方式 |
| `admin/` | Admin API 概要 |
| `.vitepress/config.ts` | 导航、侧边栏、搜索和站点路径 |
| `.vitepress/theme/` | 主题样式、端点标签组件和网关地址替换 |

## 本地预览与构建

```sh
pnpm --dir docs-site install
pnpm --dir docs-site run dev      # 本地预览
pnpm --dir docs-site run build    # 输出到 docs-site/.vitepress/dist
```

也可以从仓库根目录执行 `make build-docs-site`。

## 发布

`.github/workflows/docs-site.yml` 在以下情况构建并发布：

- 推送到 `main` 且改动了 `docs-site/` 或该 workflow。
- 发布 `v*` 版本标签。
- 在 Actions 页面手动运行。

Pull Request 只构建，不发布。

首次使用前，在仓库的 **Settings → Pages → Build and deployment** 把 **Source** 设为 **GitHub Actions**。

站点路径由构建时的 `DOCS_BASE` 决定，workflow 会从 Pages 配置自动取值。绑定自定义域名后无需改动代码。

## 编写约定

- 示例中的网关地址统一写 `https://api.example.com`。读者在侧边栏填入自己的地址后，页面上的这个占位符会被替换。链接后加 `?base_url=https://你的域名` 可以预先填好。来自链接的地址只在当前标签页有效，且只接受不含 shell 特殊字符的 http(s) 地址。
- 示例中的 Key 写 `sk-your-api-key` 或环境变量，不要写入真实凭据。
- 端点行为以 `backend/internal/transport/http/server/routes/gateway.go` 和对应 handler 为准。路由或响应格式变化时，在同一个改动里更新这里的页面。
- 本站只写调用方需要的内容。部署、架构和运维专题留在 `docs/`，需要时从页面链接过去。
- 新增页面后在 `.vitepress/config.ts` 的 `sidebar` 中登记。
