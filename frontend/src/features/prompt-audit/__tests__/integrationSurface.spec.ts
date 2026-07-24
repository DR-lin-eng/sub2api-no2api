import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const here = dirname(fileURLToPath(import.meta.url))
const read = (path: string) => readFileSync(resolve(here, path), 'utf8')

describe('Prompt Audit integration surface', () => {
  it('registers an admin and risk-control guarded route', () => {
    const router = read('../../../router/index.ts')
    expect(router).toContain("path: '/admin/prompt-audit'")
    const route = router.slice(router.indexOf("path: '/admin/prompt-audit'"), router.indexOf("path: '/admin/usage'"))
    expect(route).toContain('requiresAuth: true')
    expect(route).toContain('requiresAdmin: true')
    expect(route).toContain('requiresRiskControl: true')
  })

  it('keeps security audit available while guarding only feature-specific children', () => {
    const sidebar = read('../../../components/layout/AppSidebar.vue')
    const group = sidebar.slice(sidebar.indexOf("path: '/admin/security-audit'"), sidebar.indexOf("path: '/admin/redeem'"))
    const groupHeader = group.slice(0, group.indexOf('children:'))
    expect(group).toContain('expandOnly: true')
    expect(groupHeader).not.toContain('featureFlag: flagRiskControl')
    expect(group).toContain("path: '/admin/security-audit/ingress'")
    expect(group).toContain("path: '/admin/risk-control'")
    expect(group).toContain("path: '/admin/prompt-audit'")
    expect(group.match(/featureFlag: flagRiskControl/g)).toHaveLength(2)
    expect(group).toContain('featureFlag: flagOpsMonitoring')
  })

  it('keeps Prompt Audit locale trees symmetric and all operational controls named', () => {
    expect(Object.keys(zh.admin.promptAudit)).toEqual(Object.keys(en.admin.promptAudit))
    expect(Object.keys(zh.admin.ingressRisk)).toEqual(Object.keys(en.admin.ingressRisk))
    expect(zh.nav.securityAudit).toBeTruthy()
    expect(en.nav.securityAudit).toBeTruthy()
    expect(zh.nav.ingressRisk).toBeTruthy()
    expect(en.nav.ingressRisk).toBeTruthy()
    const endpoint = read('../components/EndpointPool.vue')
    const events = read('../components/EventWorkspace.vue')
    expect(endpoint).toContain('aria-label')
    expect(events).toContain('aria-label')
    expect(events).toContain('overflow-x-auto')
    expect(events).toContain('sm:grid-cols-2')
  })
})
