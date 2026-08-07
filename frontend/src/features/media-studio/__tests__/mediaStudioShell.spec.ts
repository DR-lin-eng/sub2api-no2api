import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { useMediaStudioPreview } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const currentDir = dirname(fileURLToPath(import.meta.url))

describe('media studio shell', () => {
  it('defines the future media generation entry points', () => {
    const { modes, previewStages, getModeById } = useMediaStudioPreview()

    expect(modes.map((mode) => mode.id)).toEqual(['image', 'video', 'batch'])
    expect(modes.every((mode) => mode.available === false)).toBe(true)
    expect(previewStages.map((stage) => stage.id)).toEqual(['prompt', 'configure', 'generate', 'deliver'])
    expect(getModeById('video').id).toBe('video')
  })

  it('uses the app layout shell', () => {
    const pageSource = readFileSync(resolve(currentDir, '../presentation/pages/MediaStudioPage.vue'), 'utf8')

    expect(pageSource).toContain('<AppLayout>')
    expect(pageSource).toContain("import AppLayout from '@/common/widgets/layout/AppLayout.vue'")
  })

  it('registers route, navigation label, and Chinese copy', () => {
    const root = resolve(currentDir, '../../..')
    const routeSource = readFileSync(resolve(root, 'core/routes/index.ts'), 'utf8')
    const sidebarSource = readFileSync(resolve(root, 'common/widgets/layout/AppSidebar.vue'), 'utf8')
    const zhCommonSource = readFileSync(resolve(root, 'core/i18n/locales/zh/common.ts'), 'utf8')
    const zhMediaStudioSource = readFileSync(resolve(root, 'core/i18n/locales/zh/mediaStudio.ts'), 'utf8')

    expect(routeSource).toContain("path: '/media-studio'")
    expect(routeSource).toContain("titleKey: 'mediaStudio.title'")
    expect(routeSource).toContain('requiresMediaStudio: true')
    expect(sidebarSource).toContain("label: t('nav.mediaStudio')")
    expect(sidebarSource).toContain('featureFlag: flagMediaStudio')
    expect(zhCommonSource).toContain("mediaStudio: '媒体工坊'")
    expect(zhMediaStudioSource).toContain("title: '媒体工坊'")
  })
})
