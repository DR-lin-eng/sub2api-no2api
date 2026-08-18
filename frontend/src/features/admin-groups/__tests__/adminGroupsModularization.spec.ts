import { readFileSync, readdirSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const featureDir = resolve(currentDir, '..')
const srcDir = resolve(featureDir, '../..')
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

const dtoSource = readFeatureSource('data/dtos/adminGroupDtos.ts')
const querySource = readFeatureSource('data/datasources/adminGroupQueries.ts')
const actionSource = readFeatureSource('data/datasources/adminGroupActions.ts')
const facadeSource = readFeatureSource('data/datasources/adminGroupsDatasource.ts')
const sharedGroupSource = readFileSync(resolve(srcDir, 'types/group.ts'), 'utf8')
const gatewaySource = readFileSync(resolve(srcDir, 'types/gateway.ts'), 'utf8')
const presentationSources = collectRuntimeSources(resolve(featureDir, 'presentation'))

describe('admin groups modularization', () => {
  it('owns shared and admin-only group contracts outside the legacy gateway declarations', () => {
    expect(sharedGroupSource).toContain('export interface Group')
    expect(sharedGroupSource).toContain('export interface ChannelModelPricing')
    expect(dtoSource).toContain('export interface AdminGroup extends Group')
    expect(dtoSource).toContain('export interface CompositeModelRoute')
    expect(dtoSource).toContain('export interface GroupRateMultiplierEntry')
    expect(dtoSource).toContain('export interface GroupRPMOverrideEntry')
    expect(dtoSource).not.toContain('apiClient')
    expect(gatewaySource).toContain("export * from './group'")
    expect(gatewaySource).toContain("from '@/features/admin-groups/data/dtos/adminGroupDtos'")
    expect(gatewaySource).not.toContain('export interface AdminGroup')
    expect(gatewaySource).not.toContain('export interface CreateGroupRequest')
  })

  it('keeps requests in explicit Query and Action owners with a pure compatibility facade', () => {
    expect(querySource).toContain("from '@/core/networks/client'")
    expect(querySource).toContain('export async function listCompositeRoutes')
    expect(querySource).toContain('export async function getGroupRateMultipliers')
    expect(actionSource).toContain("from '@/core/networks/client'")
    expect(actionSource).toContain('export async function createCompositeRoute')
    expect(actionSource).toContain('export async function batchSetGroupRateMultipliers')
    expect(actionSource).toContain('export async function batchSetGroupRPMOverrides')
    expect(facadeSource).toContain("export * from './adminGroupActions'")
    expect(facadeSource).toContain("export * from './adminGroupQueries'")
    expect(facadeSource).toContain("export * from '../dtos/adminGroupDtos'")
    expect(facadeSource).not.toContain('apiClient')
    expect(facadeSource.split('\n').length).toBeLessThan(100)
  })

  it('keeps all admin-groups presentation code off compatibility API owners', () => {
    const findings = presentationSources.flatMap(({ path, source }) => {
      const reasons: string[] = []
      if (source.includes("from '@/api'")) reasons.push('@/api')
      if (source.includes("from '@/api/admin'")) reasons.push('@/api/admin')
      if (source.includes('adminAPI')) reasons.push('adminAPI')
      if (source.includes('adminGroupsDatasource')) reasons.push('adminGroupsDatasource')
      if (source.includes('groupsAPI.')) reasons.push('groupsAPI')
      return reasons.map((reason) => `${path}: ${reason}`)
    })

    expect(findings).toEqual([])
  })

  it('uses the user owner for rate-dialog search and preserves the 300ms debounce', () => {
    for (const path of [
      'presentation/widgets/GroupRateMultipliersDialog.vue',
      'presentation/widgets/GroupRPMOverridesDialog.vue',
    ]) {
      const source = readFeatureSource(path)
      expect(source).toContain(
        "from '@/features/admin-users/data/datasources/adminUsersDatasource'",
      )
      expect(source).toContain('}, 300)')
    }
  })
})
