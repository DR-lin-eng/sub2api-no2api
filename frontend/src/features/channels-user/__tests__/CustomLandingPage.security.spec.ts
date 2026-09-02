import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(currentDir, '../../../..')
const pageSource = readFileSync(
  resolve(currentDir, '../presentation/pages/CustomLandingPage.vue'),
  'utf8',
)
const settingsSource = readFileSync(
  resolve(frontendDir, 'src/features/admin-settings/presentation/widgets/settings-tabs/SettingsGeneralTab.vue'),
  'utf8',
)
const baselineSource = readFileSync(
  resolve(frontendDir, 'eslint/architecture-debt-baseline.cjs'),
  'utf8',
)

describe('CustomLandingPage security boundary', () => {
  it('keeps access tokens out of iframe and new-window URLs', () => {
    const builderStart = pageSource.indexOf('const embeddedUrl = computed')
    const builderEnd = pageSource.indexOf('const isValidUrl = computed')
    const builderSource = pageSource.slice(builderStart, builderEnd)

    expect(builderSource).not.toContain('authStore.token')
    expect(pageSource).not.toContain('authToken: authStore.token')
    expect(pageSource).toContain('postEmbeddedAuthContext')
    expect(pageSource).toContain('issueEmbeddedCapability')
    expect(pageSource).toContain('capabilityToken: activeEmbeddedCapability.token')
    expect(pageSource).toContain('forward_access_token !== true')
    expect(pageSource).toContain('referrerpolicy="no-referrer"')
    expect(pageSource).toContain('EMBEDDED_AUTH_RETRY_DELAYS_MS')
    expect(pageSource).toContain('if (embeddedAuthIssuing) return')
    expect(pageSource).toContain('sub2api:embedded-auth-ready')
    expect(pageSource).toContain('event.source !== frame.contentWindow')
    expect(pageSource).toContain('event.origin !== targetOrigin')
  })

  it('re-renders Markdown for locale changes and rejects stale responses', () => {
    expect(pageSource).toContain('watch([markdownSlug, locale]')
    expect(pageSource).toContain('markdownRenderRequestSeq')
    expect(pageSource).toContain('requestSeq !== markdownRenderRequestSeq')
  })

  it('uses the public admin-settings entry and removes the private-import debt entry', () => {
    expect(pageSource).toContain("from '@/features/admin-settings/adminSettingsStore'")
    expect(pageSource).not.toContain(
      "@/features/admin-settings/presentation/stores/adminSettingsStore",
    )
    expect(baselineSource).not.toContain(
      "'src/features/channels-user/presentation/pages/CustomLandingPage.vue'",
    )
  })

  it('requires an explicit per-menu administrator opt-in with a visible warning', () => {
    expect(settingsSource).toContain(':model-value="item.forward_access_token === true"')
    expect(settingsSource).toContain(
      ":aria-label=\"t('admin.settings.customMenu.forwardAccessToken')\"",
    )
    expect(settingsSource).toContain('item.forward_access_token = value')
    expect(settingsSource).toContain('admin.settings.customMenu.forwardAccessToken')
    expect(settingsSource).toContain('admin.settings.customMenu.forwardAccessTokenHint')
  })
})
