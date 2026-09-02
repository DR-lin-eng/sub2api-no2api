import { describe, expect, it } from 'vitest'

import { getLocaleScopesForRoute, i18n, loadLocaleMessages } from '@/core/i18n'

const USAGE_ROUTE_KEYS = [
  'dashboard.timeRange',
  'dashboard.granularity',
  'dashboard.day',
  'dashboard.hour',
  'dashboard.modelDistribution',
  'dashboard.groupDistribution',
  'dashboard.metricTokens',
  'dashboard.metricActualCost',
  'dashboard.viewModelDistribution',
  'dashboard.viewSpendingRanking',
  'dashboard.spendingRankingTitle',
  'dashboard.spendingRankingUser',
  'dashboard.spendingRankingRequests',
  'dashboard.spendingRankingTokens',
  'dashboard.spendingRankingSpend',
  'dashboard.spendingRankingOther',
  'dashboard.userPrefix',
  'dashboard.tokenUsageTrend',
  'dashboard.model',
  'dashboard.group',
  'dashboard.requests',
  'dashboard.tokens',
  'dashboard.actual',
  'dashboard.accountCost',
  'dashboard.standard',
  'dashboard.noDataAvailable',
  'dashboard.failedToLoad',
  'keys.columnSettings',
  'usage.group',
  'usage.allGroups',
  'usage.allModels',
  'usage.allTypes',
  'usage.billingType',
  'usage.allBillingTypes',
  'usage.billingTypeBalance',
  'usage.billingTypeSubscription',
  'usage.billingMode',
  'usage.allBillingModes',
  'usage.billingModeToken',
  'usage.billingModePerRequest',
  'usage.billingModeImage',
  'usage.billingModeVideo',
] as const

function resolveMessage(messages: Record<string, unknown>, key: string): unknown {
  return key.split('.').reduce<unknown>((current, segment) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as Record<string, unknown>)[segment]
  }, messages)
}

describe('route locale scopes', () => {
  it.each(['/home', '/login', '/register', '/key-usage', '/setup'])(
    'keeps public route %s on the base messages',
    (path) => {
      expect(getLocaleScopesForRoute(path)).toEqual(['base'])
    },
  )

  it('loads user messages for authenticated and data-rich public pages', () => {
    expect(getLocaleScopesForRoute('/dashboard')).toEqual(['base', 'user'])
    expect(getLocaleScopesForRoute('/model-plaza')).toEqual(['base', 'user'])
    expect(getLocaleScopesForRoute('/monitor/public')).toEqual(['base', 'user'])
  })

  it('adds feature messages only for their routes', () => {
    expect(getLocaleScopesForRoute('/batch-image')).toEqual(['base', 'user', 'batchImage'])
    expect(getLocaleScopesForRoute('/media-studio')).toEqual(['base', 'user', 'mediaStudio'])
    expect(getLocaleScopesForRoute('/support')).toEqual(['base', 'user', 'supportChat'])
  })

  it('loads admin messages only for admin routes', () => {
    expect(getLocaleScopesForRoute('/admin/dashboard')).toEqual(['base', 'user', 'admin'])
    expect(getLocaleScopesForRoute('/admin/support')).toEqual([
      'base',
      'user',
      'admin',
      'supportChat',
    ])
  })

  it.each([
    ['/ADMIN/dashboard', ['base', 'user', 'admin']],
    ['/Admin/support?tab=overview#details', ['base', 'user', 'admin', 'supportChat']],
  ] as const)('resolves admin messages for uppercase route URLs (%s)', (path, expectedScopes) => {
    expect(getLocaleScopesForRoute(path)).toEqual(expectedScopes)
  })

  it.each(['en', 'zh'] as const)(
    'resolves every shared usage-page key from the %s user scope',
    async (locale) => {
      const scopes = getLocaleScopesForRoute('/usage')
      expect(scopes).toEqual(['base', 'user'])

      await loadLocaleMessages(locale, scopes)
      const messages = i18n.global.getLocaleMessage(locale) as Record<string, unknown>

      for (const key of USAGE_ROUTE_KEYS) {
        expect(resolveMessage(messages, key), `${locale}:${key}`).toEqual(expect.any(String))
      }
    },
  )
})
