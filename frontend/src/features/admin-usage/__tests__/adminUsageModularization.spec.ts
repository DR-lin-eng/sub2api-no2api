import { readFileSync, readdirSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const featureDir = resolve(currentDir, '..')
const frontendDir = resolve(featureDir, '../../..')
const readFeatureSource = (relativePath: string) =>
  readFileSync(resolve(featureDir, relativePath), 'utf8')

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
      path: relative(featureDir, absolutePath),
      source: readFileSync(absolutePath, 'utf8'),
    })
  }
  return sources
}

const presentationSources = collectRuntimeSources(resolve(featureDir, 'presentation'))
const pageSource = readFeatureSource('presentation/pages/UsagePage.vue')
const filtersSource = readFeatureSource('presentation/widgets/UsageFilters.vue')
const cleanupSource = readFeatureSource('presentation/widgets/UsageCleanupDialog.vue')
const usageTablePublicSource = readFeatureSource('usageTable.ts')
const usageStatsCardsPublicSource = readFeatureSource('usageStatsCards.ts')
const userUsagePageSource = readFileSync(
  resolve(frontendDir, 'src/features/usage/presentation/pages/UsagePage.vue'),
  'utf8',
)
const opsDetailPublicSource = readFileSync(
  resolve(frontendDir, 'src/features/admin-ops/errorDetailDialog.ts'),
  'utf8',
)
const opsTablePublicSource = readFileSync(
  resolve(frontendDir, 'src/features/admin-ops/errorLogTable.ts'),
  'utf8',
)
const usersPublicSource = readFileSync(
  resolve(frontendDir, 'src/features/admin-users/userBalanceHistoryDialog.ts'),
  'utf8',
)
const baselineSource = readFileSync(
  resolve(frontendDir, 'eslint/architecture-debt-baseline.cjs'),
  'utf8',
)

describe('admin usage modularization', () => {
  it('keeps all presentation code off legacy admin API objects and its own facade object', () => {
    const findings = presentationSources.flatMap(({ path, source }) => {
      const reasons: string[] = []
      if (source.includes("from '@/api/admin'")) reasons.push('@/api/admin')
      if (source.includes('adminAPI')) reasons.push('adminAPI')
      if (source.includes('adminUsageAPI')) reasons.push('adminUsageAPI')
      return reasons.map((reason) => `${path}: ${reason}`)
    })

    expect(findings).toEqual([])
  })

  it('uses explicit request owners without changing feature responsibilities', () => {
    expect(pageSource).toContain(
      "from '@/features/admin-usage/data/datasources/adminUsageDatasource'",
    )
    expect(pageSource).toContain(
      "from '@/features/admin-dashboard/data/datasources/adminDashboardDatasource'",
    )
    expect(pageSource).toContain(
      "from '@/features/admin-users/data/datasources/adminUsersDatasource'",
    )
    expect(filtersSource).toContain(
      "from '@/features/admin-accounts/data/datasources/adminAccountQueries'",
    )
    expect(filtersSource).toContain('}, 300)')
    expect(cleanupSource).toContain('listCleanupTasks')
    expect(cleanupSource).toContain('createCleanupTask')
    expect(cleanupSource).toContain('cancelCleanupTask')
  })

  it('consumes cross-feature UI through narrow public component contracts', () => {
    expect(pageSource).toContain("from '@/features/admin-ops/errorDetailDialog'")
    expect(pageSource).toContain("from '@/features/admin-ops/errorLogTable'")
    expect(pageSource).toContain(
      "from '@/features/admin-users/userBalanceHistoryDialog'",
    )
    expect(pageSource).not.toMatch(/@\/features\/(?:admin-ops|admin-users)\/presentation\//)
    expect(opsDetailPublicSource).toContain(
      "export { default } from './presentation/widgets/OpsErrorDetailDialog.vue'",
    )
    expect(opsTablePublicSource).toContain(
      "export { default } from './presentation/widgets/OpsErrorLogTable.vue'",
    )
    expect(usersPublicSource).toContain(
      "export { default } from './presentation/widgets/UserBalanceHistoryDialog.vue'",
    )
    expect(userUsagePageSource).toContain("from '@/features/admin-usage/usageTable'")
    expect(userUsagePageSource).not.toContain(
      "@/features/admin-usage/presentation/widgets/UsageTable.vue",
    )
    expect(usageTablePublicSource).toContain(
      "export { default } from './presentation/widgets/UsageTable.vue'",
    )
    expect(userUsagePageSource).toContain("from '@/features/admin-usage/usageStatsCards'")
    expect(userUsagePageSource).not.toContain(
      "@/features/admin-usage/presentation/widgets/UsageStatsCards.vue",
    )
    expect(usageStatsCardsPublicSource).toContain(
      "export { default } from './presentation/widgets/UsageStatsCards.vue'",
    )
  })

  it('removes the migrated legacy and private-presentation baseline entries', () => {
    expect(baselineSource).not.toContain(
      "'src/features/admin-usage/presentation/pages/UsagePage.vue'",
    )
    expect(baselineSource).not.toContain(
      "'src/features/admin-usage/presentation/widgets/UsageFilters.vue'",
    )
    expect(baselineSource).not.toContain(
      "'@/features/admin-usage/presentation/widgets/UsageTable.vue'",
    )
    expect(baselineSource).not.toContain(
      "'@/features/admin-usage/presentation/widgets/UsageStatsCards.vue'",
    )
  })
})
