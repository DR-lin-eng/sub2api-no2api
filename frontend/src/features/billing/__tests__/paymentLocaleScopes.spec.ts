import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it } from 'vitest'

import { getLocaleScopesForRoute, type LocaleScope } from '@/core/i18n'
import enAdmin from '@/core/i18n/locales/en/admin'
import enBatchImage from '@/core/i18n/locales/en/batchImage'
import enCommon from '@/core/i18n/locales/en/common'
import enDashboard from '@/core/i18n/locales/en/dashboard'
import enLanding from '@/core/i18n/locales/en/landing'
import enMediaStudio from '@/core/i18n/locales/en/mediaStudio'
import enMisc from '@/core/i18n/locales/en/misc'
import enSupportChat from '@/core/i18n/locales/en/supportChat'
import zhAdmin from '@/core/i18n/locales/zh/admin'
import zhBatchImage from '@/core/i18n/locales/zh/batchImage'
import zhCommon from '@/core/i18n/locales/zh/common'
import zhDashboard from '@/core/i18n/locales/zh/dashboard'
import zhLanding from '@/core/i18n/locales/zh/landing'
import zhMediaStudio from '@/core/i18n/locales/zh/mediaStudio'
import zhMisc from '@/core/i18n/locales/zh/misc'
import zhSupportChat from '@/core/i18n/locales/zh/supportChat'
import OrderStatusBadge from '@/features/billing/orderStatusBadge'

type LocaleCode = 'en' | 'zh'
type Messages = Record<string, unknown>

const localeScopes = {
  en: {
    base: { ...enLanding, ...enCommon },
    user: { ...enDashboard, ...enMisc },
    batchImage: enBatchImage,
    mediaStudio: enMediaStudio,
    supportChat: enSupportChat,
    admin: { admin: enAdmin },
  },
  zh: {
    base: { ...zhLanding, ...zhCommon },
    user: { ...zhDashboard, ...zhMisc },
    batchImage: zhBatchImage,
    mediaStudio: zhMediaStudio,
    supportChat: zhSupportChat,
    admin: { admin: zhAdmin },
  },
} satisfies Record<LocaleCode, Record<LocaleScope, Messages>>

function mergeMessages(parts: readonly Messages[]): Messages {
  const merged: Messages = {}
  for (const part of parts) {
    for (const [key, value] of Object.entries(part)) {
      if (
        merged[key]
        && typeof merged[key] === 'object'
        && !Array.isArray(merged[key])
        && value
        && typeof value === 'object'
        && !Array.isArray(value)
      ) {
        merged[key] = mergeMessages([merged[key] as Messages, value as Messages])
      } else {
        merged[key] = value
      }
    }
  }
  return merged
}

function toRuntimeMessages(value: unknown): unknown {
  if (typeof value === 'string') return () => value
  if (Array.isArray(value)) return value.map(toRuntimeMessages)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.entries(value).map(([key, child]) => [key, toRuntimeMessages(child)]),
  )
}

function mountForRoute(locale: LocaleCode, routePath: string) {
  const scopes = getLocaleScopesForRoute(routePath)
  const messages = mergeMessages(scopes.map((scope) => localeScopes[locale][scope]))
  const i18n = createI18n({
    legacy: false,
    locale,
    fallbackLocale: false,
    missingWarn: false,
    fallbackWarn: false,
    messages: { [locale]: toRuntimeMessages(messages) as Messages },
  })
  const wrapper = mount(OrderStatusBadge, {
    props: { status: 'COMPLETED' },
    global: { plugins: [i18n] },
  })
  return { i18n, scopes, wrapper }
}

describe('shared payment component locale scopes', () => {
  const cases = [
    { locale: 'en', routePath: '/orders', permission: 'user', expected: 'Completed' },
    { locale: 'zh', routePath: '/orders', permission: 'user', expected: '已完成' },
    { locale: 'en', routePath: '/admin/orders', permission: 'admin', expected: 'Completed' },
    { locale: 'zh', routePath: '/admin/orders', permission: 'admin', expected: '已完成' },
  ] as const

  it.each(cases)(
    'renders $permission/$locale with only the route-owned vocabularies',
    ({ expected, locale, permission, routePath }) => {
      const { i18n, scopes, wrapper } = mountForRoute(locale, routePath)

      expect(wrapper.text()).toBe(expected)
      expect(wrapper.text()).not.toContain('payment.status.completed')
      expect(i18n.global.te('payment.status.completed')).toBe(true)
      expect(scopes.includes('admin')).toBe(permission === 'admin')
      expect(i18n.global.te('admin.settings.title')).toBe(permission === 'admin')
    },
  )
})
