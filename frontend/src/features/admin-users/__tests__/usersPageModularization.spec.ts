import { readFileSync, readdirSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const featureDir = resolve(currentDir, '..')
const readFeatureSource = (relativePath: string) => readFileSync(resolve(featureDir, relativePath), 'utf8')

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

const pageSource = readFeatureSource('presentation/pages/UsersPage.vue')
const toolbarSource = readFeatureSource('presentation/widgets/UsersTableToolbar.vue')
const facadeSource = readFeatureSource('data/datasources/adminUsersDatasource.ts')
const dtoSource = readFeatureSource('data/dtos/adminUserDtos.ts')
const presentationSources = collectRuntimeSources(resolve(featureDir, 'presentation'))

describe('admin users page modularization', () => {
  it('owns user-management transport contracts in the DTO module', () => {
    expect(dtoSource).toContain('export interface AdminBindAuthIdentityRequest')
    expect(dtoSource).toContain('export interface BatchUpdateUserLimitsRequest')
    expect(dtoSource).toContain('export interface BalanceHistoryItem')
    expect(dtoSource).toContain('export interface PlatformQuotaItem')
    expect(dtoSource).toContain('export interface BatchPlatformQuotasResponse')
    expect(facadeSource).toContain("from '../dtos/adminUserDtos'")
    expect(facadeSource).not.toContain('export interface BalanceHistoryItem')
    expect(facadeSource).not.toContain('export interface PlatformQuotaItem')
  })

  it('keeps all admin-users presentation code off legacy API barrels', () => {
    const findings = presentationSources.flatMap(({ path, source }) => {
      const reasons = []
      if (source.includes("from '@/api'")) reasons.push('@/api')
      if (source.includes("from '@/api/admin'")) reasons.push('@/api/admin')
      if (source.includes('adminAPI')) reasons.push('adminAPI')
      return reasons.map((reason) => `${path}: ${reason}`)
    })

    expect(findings).toEqual([])
    expect(pageSource).toContain("from '@/features/admin-dashboard/data/datasources/adminDashboardDatasource'")
    expect(pageSource).toContain("from '@/features/admin-groups/data/datasources/adminGroupQueries'")
    expect(pageSource).toContain("from '@/features/admin-users/data/datasources/adminUsersDatasource'")
    expect(pageSource).toContain("from '@/features/admin-users/data/datasources/userAttributesDatasource'")
  })

  it('keeps the table toolbar statically owned by the users route chunk', () => {
    expect(pageSource).toContain(
      "import UsersTableToolbar from '@/features/admin-users/presentation/widgets/UsersTableToolbar.vue'"
    )
    expect(pageSource).toContain('<UsersTableToolbar')
    expect(toolbarSource).not.toContain('import(')
  })

  it('keeps request timing and dialog orchestration in the page owner', () => {
    expect(pageSource).toContain('new AbortController()')
    expect(pageSource).toContain('}, 50)')
    expect(pageSource).toContain('}, 300)')
    expect(pageSource).toContain('<UserCreateModal :show="showCreateModal"')
    expect(pageSource).toContain('<UserEditModal :show="showEditModal"')
    expect(pageSource).toContain('<BulkEditUserModal')
    expect(toolbarSource).not.toContain('localStorage')
    expect(toolbarSource).not.toContain('setTimeout')
  })
})
