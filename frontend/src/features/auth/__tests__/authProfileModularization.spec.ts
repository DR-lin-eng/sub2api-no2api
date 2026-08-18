import { readFileSync, readdirSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const authDir = resolve(currentDir, '..')
const srcDir = resolve(authDir, '../..')
const profileDir = resolve(srcDir, 'features/profile')

function collectRuntimeSources(directory: string): Array<{ path: string; source: string }> {
  const sources: Array<{ path: string; source: string }> = []
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (entry.name === '__tests__') continue
    const absolutePath = join(directory, entry.name)
    if (entry.isDirectory()) {
      sources.push(...collectRuntimeSources(absolutePath))
      continue
    }
    if (!new Set(['.ts', '.vue']).has(extname(entry.name))) continue
    sources.push({
      path: relative(srcDir, absolutePath),
      source: readFileSync(absolutePath, 'utf8'),
    })
  }
  return sources
}

const ownedSources = [
  ...collectRuntimeSources(authDir),
  ...collectRuntimeSources(profileDir),
]
const allRuntimeSources = collectRuntimeSources(srcDir)
const allFeatureSources = collectRuntimeSources(resolve(srcDir, 'features'))
const facadeSource = readFileSync(
  resolve(authDir, 'data/datasources/authDatasource.ts'),
  'utf8',
)
const sessionSource = readFileSync(
  resolve(authDir, 'data/datasources/authSessionActions.ts'),
  'utf8',
)
const authPublicSource = readFileSync(resolve(authDir, 'index.ts'), 'utf8')
const appHeaderSource = readFileSync(
  resolve(srcDir, 'common/widgets/layout/AppHeader.vue'),
  'utf8',
)
const stepUpPublicSource = readFileSync(resolve(authDir, 'totpStepUpDialog.ts'), 'utf8')
const passkeyPublicSource = readFileSync(
  resolve(srcDir, 'features/passkeys/profilePasskeyCard.ts'),
  'utf8',
)

describe('auth and profile modularization', () => {
  it('keeps runtime presentation off legacy barrels and compatibility facades', () => {
    const findings = ownedSources
      .filter(({ path }) => path.includes('/presentation/'))
      .flatMap(({ path, source }) => {
        const reasons: string[] = []
        if (/from ['"]@\/(?:api|stores)(?:\/|['"])/.test(source)) reasons.push('legacy barrel')
        if (source.includes('authDatasource')) reasons.push('authDatasource')
        if (source.includes('authAPI')) reasons.push('authAPI')
        if (source.includes('userAPI')) reasons.push('userAPI')
        if (source.includes('totpAPI')) reasons.push('totpAPI')
        return reasons.map((reason) => `${path}: ${reason}`)
      })

    expect(findings).toEqual([])
  })

  it('keeps feature consumers off auth and passkey private presentation files', () => {
    const findings = allFeatureSources
      .filter(({ path }) => !path.startsWith('features/auth/'))
      .flatMap(({ path, source }) => {
        const reasons: string[] = []
        if (source.includes('@/features/auth/presentation/')) reasons.push('auth presentation')
        if (source.includes('@/features/passkeys/presentation/widgets/ProfilePasskeyCard.vue')) {
          reasons.push('passkey presentation')
        }
        return reasons.map((reason) => `${path}: ${reason}`)
      })

    expect(findings).toEqual([])
  })

  it('keeps token persistence and browser refresh coordination in explicit owners', () => {
    expect(sessionSource).toContain("from '@/core/networks/tokenStore'")
    expect(sessionSource).toContain("from '@/core/networks/sessionRefresh'")
    expect(sessionSource).not.toMatch(/localStorage\.setItem\(['"](?:access|refresh)_token/)
    expect(sessionSource).not.toContain('document.cookie')
  })

  it('keeps current-user admin decisions on the verified auth-store signal', () => {
    const directRolePatterns = [
      /(?:authStore|userStore)\.user\??\.role/,
      /\buser\.value\??\.role\s*===\s*['"]admin['"]/,
    ]
    const findings = allRuntimeSources
      .filter(({ path, source }) => (
        path !== 'features/auth/presentation/stores/authStore.ts'
        && source.includes('useAuthStore')
      ))
      .filter(({ source }) => directRolePatterns.some((pattern) => pattern.test(source)))
      .map(({ path }) => path)

    expect(findings).toEqual([])
    expect(appHeaderSource).not.toContain('{{ user.role }}')
    expect(appHeaderSource).toContain("authStore.isAdmin ? t('profile.administrator') : t('profile.user')")
  })

  it('keeps the compatibility facade network-free and delegates by concern', () => {
    expect(facadeSource).not.toContain('apiClient')
    expect(facadeSource).toContain("from './authSessionActions'")
    expect(facadeSource).toContain("from './authQueries'")
    expect(facadeSource).toContain("from './authVerificationActions'")
    expect(facadeSource).toContain("from './authOAuthActions'")
  })

  it('uses narrow public UI and Store entrypoints', () => {
    expect(authPublicSource.trim()).toBe(
      "export { useAuthStore } from './presentation/stores/authStore'\nexport { default as TotpStepUpDialog } from './presentation/widgets/TotpStepUpDialog.vue'",
    )
    expect(stepUpPublicSource.trim()).toBe(
      "export { default } from './presentation/widgets/TotpStepUpDialog.vue'",
    )
    expect(passkeyPublicSource.trim()).toBe(
      "export { default } from './presentation/widgets/ProfilePasskeyCard.vue'",
    )
  })
})
