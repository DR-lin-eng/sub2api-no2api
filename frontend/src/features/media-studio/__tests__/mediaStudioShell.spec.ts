import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { useMediaStudioPreview } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const currentDir = dirname(fileURLToPath(import.meta.url))

describe('media studio shell', () => {
  it('enables image, video, and batch modes', () => {
    const { modes } = useMediaStudioPreview()
    expect(modes.map(mode => mode.id)).toEqual(['image', 'video', 'batch'])
    expect(modes.every(mode => mode.available)).toBe(true)
  })

  it('uses protected blob video previews and the existing batch workspace route', () => {
    const datasource = readFileSync(resolve(currentDir, '../data/datasources/mediaStudioDatasource.ts'), 'utf8')
    const canvas = readFileSync(resolve(currentDir, '../presentation/widgets/MediaStudioCanvas.vue'), 'utf8')
    const page = readFileSync(resolve(currentDir, '../presentation/pages/MediaStudioPage.vue'), 'utf8')

    expect(datasource).toContain('getVideoGenerationContent')
    expect(datasource).toContain("headers: authHeaders(apiKey)")
    expect(canvas).toContain('<video')
    expect(canvas).toContain('preload="metadata"')
    expect(page).toContain("router.push({ name: 'BatchImageGuide' })")
  })
})
