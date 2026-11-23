import { defineConfig } from 'vitepress'

// This site is served from a custom domain at the origin root.
// Keep local builds aligned with the production DOCS_BASE set by the workflow.
const base = normalizeBase(process.env.DOCS_BASE ?? '/')
const repo = 'https://github.com/DR-lin-eng/sub2api-no2api'

function normalizeBase(value: string): string {
  const trimmed = value.trim().replace(/^\/+|\/+$/g, '')
  return trimmed ? `/${trimmed}/` : '/'
}

export default defineConfig({
  lang: 'zh-CN',
  title: 'Sub2API 文档',
  description: 'Sub2API 网关 API 文档：OpenAI、Anthropic、Gemini 兼容接口，生图与视频，以及客户端接入指南。',
  base,
  cleanUrls: true,
  srcExclude: ['README.md'],
  lastUpdated: true,
  head: [['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}logo.svg` }]],
  themeConfig: {
    logo: '/logo.svg',
    siteTitle: 'Sub2API 文档',
    nav: [
      { text: '指南', link: '/guide/overview', activeMatch: '^/guide/' },
      { text: 'API 参考', link: '/api/', activeMatch: '^/api/' },
      { text: '客户端接入', link: '/clients/claude-code', activeMatch: '^/clients/' },
      { text: '管理 API', link: '/admin/', activeMatch: '^/admin/' }
    ],
    sidebar: [
      {
        text: '开始使用',
        items: [
          { text: '概览', link: '/guide/overview' },
          { text: '快速开始', link: '/guide/quickstart' },
          { text: '认证', link: '/guide/authentication' },
          { text: '流式响应', link: '/guide/streaming' },
          { text: '错误处理', link: '/guide/errors' }
        ]
      },
      {
        text: '文本 API',
        items: [
          { text: '端点总览', link: '/api/' },
          { text: 'Chat Completions', link: '/api/chat-completions' },
          { text: 'Responses', link: '/api/responses' },
          { text: 'Messages（Anthropic）', link: '/api/messages' },
          { text: 'Gemini 原生接口', link: '/api/gemini' },
          { text: 'Embeddings', link: '/api/embeddings' },
          { text: '模型列表', link: '/api/models' }
        ]
      },
      {
        text: '图片与视频 API',
        items: [
          { text: '图片生成与编辑', link: '/api/images' },
          { text: '异步图片任务', link: '/api/images-async' },
          { text: '批量图片', link: '/api/images-batch' },
          { text: '视频生成', link: '/api/videos' }
        ]
      },
      {
        text: 'Key 与用量',
        items: [{ text: '用量、计费与分组查询', link: '/api/usage' }]
      },
      {
        text: '客户端接入',
        items: [
          { text: 'Claude Code', link: '/clients/claude-code' },
          { text: 'Codex CLI', link: '/clients/codex' },
          { text: 'Gemini CLI', link: '/clients/gemini-cli' },
          { text: '官方 SDK', link: '/clients/sdks' }
        ]
      },
      {
        text: '管理端',
        items: [{ text: 'Admin API', link: '/admin/' }]
      }
    ],
    socialLinks: [{ icon: 'github', link: repo }],
    editLink: {
      pattern: `${repo}/edit/main/docs-site/:path`,
      text: '在 GitHub 上编辑此页'
    },
    search: {
      provider: 'local',
      options: {
        translations: {
          button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
          modal: {
            noResultsText: '没有找到相关结果',
            resetButtonTitle: '清除查询',
            footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' }
          }
        }
      }
    },
    outline: { level: [2, 3], label: '本页内容' },
    docFooter: { prev: '上一页', next: '下一页' },
    lastUpdated: { text: '最后更新' },
    darkModeSwitchLabel: '外观',
    lightModeSwitchTitle: '切换到浅色模式',
    darkModeSwitchTitle: '切换到深色模式',
    sidebarMenuLabel: '目录',
    returnToTopLabel: '返回顶部',
    footer: {
      message: '示例中的网关地址和 API Key 均为占位符，请替换为你自己站点的值。',
      copyright: 'Sub2API'
    }
  }
})
