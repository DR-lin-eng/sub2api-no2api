import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const frontendRoot = resolve(__dirname, '../../..')

function readFrontendFile(path: string): string {
  return readFileSync(resolve(frontendRoot, path), 'utf8')
}

describe('initial asset loading contract', () => {
  it('preloads the above-the-fold logo at high priority', () => {
    const html = readFrontendFile('index.html')

    expect(html).toContain('id="app-logo-preload"')
    expect(html).toContain('rel="preload" as="image"')
    expect(html).toContain('fetchpriority="high"')
  })

  it('keeps HTTP and route-only heavy libraries out of the misc fallback chunk', () => {
    const config = readFrontendFile('vite.config.ts')
    const fallback = config.indexOf("return 'vendor-misc'")
    const expectedChunks = [
      ['axios', 'vendor-http'],
      ['marked', 'vendor-markdown'],
      ['qrcode', 'vendor-qrcode'],
      ['dijkstrajs', 'vendor-qrcode'],
      ['@tanstack/vue-virtual', 'vendor-table'],
      ['driver.js', 'vendor-onboarding'],
      ['@airwallex/components-sdk', 'vendor-airwallex'],
    ]

    for (const [dependency, chunk] of expectedChunks) {
      const rule = config.indexOf(`id.includes('/${dependency}/')`)
      expect(rule, dependency).toBeGreaterThan(-1)
      expect(rule, dependency).toBeLessThan(fallback)
      expect(config.slice(rule, fallback), chunk).toContain(`return '${chunk}'`)
    }
  })

  it('loads Markdown-backed global dialogs only when their state is visible', () => {
    const app = readFrontendFile('src/App.vue')

    expect(app).toContain("defineAsyncComponent(\n  () => import('@/common/widgets/data/AnnouncementPopup.vue')")
    expect(app).toContain('AnnouncementPopup v-if="hasAnnouncementPopup"')
    expect(app).toContain('AdminComplianceDialog v-if="needsAdminCompliance"')
  })
})
