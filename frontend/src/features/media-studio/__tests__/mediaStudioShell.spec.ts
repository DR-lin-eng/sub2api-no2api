import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import { useMediaStudioPreview } from '@/features/media-studio/presentation/composables/useMediaStudioPreview'

const currentDir = dirname(fileURLToPath(import.meta.url))

describe('media studio shell', () => {
  it('enables image and video modes', () => {
    const { modes } = useMediaStudioPreview()
    expect(modes.map(mode => mode.id)).toEqual(['image', 'video'])
    expect(modes.every(mode => mode.available)).toBe(true)
  })

  it('uses protected blob video previews', () => {
    const datasource = readFileSync(resolve(currentDir, '../data/datasources/mediaStudioDatasource.ts'), 'utf8')
    const canvas = readFileSync(resolve(currentDir, '../presentation/widgets/MediaStudioCanvas.vue'), 'utf8')

    expect(datasource).toContain('getVideoGenerationContent')
    expect(datasource).toContain("headers: authHeaders(apiKey)")
    expect(canvas).toContain('<video')
    expect(canvas).toContain('preload="metadata"')
  })
})
