import { readFileSync, readdirSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const currentDir = dirname(fileURLToPath(import.meta.url))
const featureDir = resolve(currentDir, '..')
const frontendDir = resolve(featureDir, '../../..')
const srcDir = resolve(frontendDir, 'src')

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

const adminOrderSources = collectRuntimeSources(featureDir)
const featureSources = collectRuntimeSources(resolve(srcDir, 'features'))
const billingSources = collectRuntimeSources(resolve(srcDir, 'features/billing'))
const contractsSource = readFileSync(resolve(srcDir, 'features/billing/paymentContracts.ts'), 'utf8')
const legacyContractsSource = readFileSync(resolve(srcDir, 'types/payment.ts'), 'utf8')
const displaySource = readFileSync(resolve(srcDir, 'features/billing/paymentDisplay.ts'), 'utf8')
const facadeSource = readFileSync(
  resolve(featureDir, 'data/datasources/adminPaymentDatasource.ts'),
  'utf8',
)

describe('admin orders and billing ownership boundaries', () => {
  it('keeps admin-orders presentation off the compatibility facade and private UI', () => {
    const findings = adminOrderSources.flatMap(({ path, source }) => {
      const reasons: string[] = []
      if (path.startsWith('features/admin-orders/presentation/')) {
        if (source.includes('adminPaymentAPI')) reasons.push('adminPaymentAPI')
        if (source.includes('adminPaymentDatasource')) reasons.push('adminPaymentDatasource')
        if (/from ['"]@\/features\/(?!admin-orders\/)[^/]+\/presentation\//.test(source)) {
          reasons.push('cross-feature private presentation')
        }
      }
      return reasons.map((reason) => `${path}: ${reason}`)
    })

    expect(findings).toEqual([])
  })

  it('exposes payment contracts and display rules from framework-free owners', () => {
    expect(contractsSource).toContain('export interface PaymentOrder')
    expect(contractsSource).toContain('export interface SubscriptionPlan')
    expect(contractsSource).not.toContain('apiClient')
    expect(legacyContractsSource.trim()).toBe(
      "/** @deprecated Import payment contracts from '@/features/billing/paymentContracts'. */\n" +
      "export type * from '@/features/billing/paymentContracts'",
    )
    expect(displaySource).toContain('export function formatPaymentAmount')
    expect(displaySource).toContain('export function planValiditySuffix')
    expect(displaySource).not.toMatch(/from ['"](?:vue|@\/core\/networks)/)
  })

  it('keeps all feature consumers off billing private presentation exports', () => {
    const findings = featureSources
      .filter(({ path }) => !path.startsWith('features/billing/'))
      .flatMap(({ path, source }) =>
        source.includes("@/features/billing/presentation/") ? [path] : [],
      )

    expect(findings).toEqual([])
  })

  it('keeps billing off auth and subscriptions private presentation exports', () => {
    const findings = billingSources.flatMap(({ path, source }) =>
      /@\/features\/(?:auth|subscriptions)\/presentation\//.test(source) ? [path] : [],
    )

    expect(findings).toEqual([])
  })

  it('keeps the compatibility datasource network-free', () => {
    expect(facadeSource).not.toContain('apiClient')
    expect(facadeSource).toContain("from './adminPaymentQueries'")
    expect(facadeSource).toContain("from './adminPaymentActions'")
  })
})
