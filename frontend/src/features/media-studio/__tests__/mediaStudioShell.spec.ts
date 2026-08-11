import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { useMediaStudioPreview } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const currentDir = dirname(fileURLToPath(import.meta.url))

describe('media studio shell', () => {
  it('defines image, video, and batch studio metadata without API calls', () => {
    const { modes, getModeById } = useMediaStudioPreview()

    expect(modes.map((mode) => mode.id)).toEqual(['image', 'video', 'batch'])
    expect(modes.map((mode) => mode.iconName)).toEqual(['grid', 'play', 'copy'])
    expect(getModeById('image').available).toBe(true)
    expect(getModeById('video').available).toBe(false)
    expect(getModeById('batch').available).toBe(false)
    expect(getModeById('video').id).toBe('video')
  })

  it('uses the app layout, closed composer menu, and SVG icons', () => {
    const pageSource = readFileSync(resolve(currentDir, '../presentation/pages/MediaStudioPage.vue'), 'utf8')
    const canvasSource = readFileSync(resolve(currentDir, '../presentation/widgets/MediaStudioCanvas.vue'), 'utf8')

    expect(pageSource).toContain('<AppLayout>')
    expect(pageSource).toContain("import AppLayout from '@/common/widgets/layout/AppLayout.vue'")
    expect(pageSource).toContain('MediaStudioCanvas')
    expect(canvasSource).toContain('typeMenuOpen = !typeMenuOpen')
    expect(canvasSource).toContain('v-if="typeMenuOpen"')
    expect(canvasSource).toContain('import Icon from')
    expect(canvasSource).not.toContain("{{ selectedMode.icon }}")
    expect(pageSource).toContain('media-studio-page bg-transparent')
  })

  it('keeps image execution in the media studio data layer', () => {
    const datasourceSource = readFileSync(resolve(currentDir, '../data/datasources/mediaStudioDatasource.ts'), 'utf8')
    const controllerSource = readFileSync(resolve(currentDir, '../presentation/composables/useMediaStudioController.ts'), 'utf8')

    expect(datasourceSource).toContain("buildGatewayUrl('/v1/images/generations')")
    expect(datasourceSource).toContain("buildGatewayUrl('/v1/images/generations/async')")
    expect(datasourceSource).toContain("buildGatewayUrl(`/v1/images/tasks/${encodeURIComponent(taskId)}`)")
    expect(datasourceSource).toContain("buildGatewayUrl('/v1/models')")
    expect(controllerSource).toContain('submitImageGeneration')
    expect(controllerSource).toContain('localStorage')
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
    expect(zhMediaStudioSource).toContain("greeting: '你好，想创作什么？'")
  })
})
