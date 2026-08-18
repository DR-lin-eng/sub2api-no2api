import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it, vi } from 'vitest'

import GroupOptionItem from '../GroupOptionItem.vue'
import { getLocaleScopesForRoute, type LocaleScope } from '@/core/i18n'
import enAdmin from '@/core/i18n/locales/en/admin'
import enCommon from '@/core/i18n/locales/en/common'
import enDashboard from '@/core/i18n/locales/en/dashboard'
import enLanding from '@/core/i18n/locales/en/landing'
import enMisc from '@/core/i18n/locales/en/misc'
import zhAdmin from '@/core/i18n/locales/zh/admin'
import zhCommon from '@/core/i18n/locales/zh/common'
import zhDashboard from '@/core/i18n/locales/zh/dashboard'
import zhLanding from '@/core/i18n/locales/zh/landing'
import zhMisc from '@/core/i18n/locales/zh/misc'

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({ cachedPublicSettings: null }),
}))

type LocaleCode = 'en' | 'zh'
type Messages = Record<string, unknown>

const localeScopes = {
  en: {
    base: { ...enLanding, ...enCommon },
    user: { ...enDashboard, ...enMisc },
    admin: { admin: enAdmin },
  },
  zh: {
    base: { ...zhLanding, ...zhCommon },
    user: { ...zhDashboard, ...zhMisc },
    admin: { admin: zhAdmin },
  },
} satisfies Record<LocaleCode, Pick<Record<LocaleScope, Messages>, 'base' | 'user' | 'admin'>>

function messagesForRoute(locale: LocaleCode, routePath: string): Messages {
  return getLocaleScopesForRoute(routePath).reduce<Messages>((messages, scope) => {
    const scopeMessages = localeScopes[locale][scope as keyof typeof localeScopes.en]
    return scopeMessages ? { ...messages, ...scopeMessages } : messages
  }, {})
}

function toRuntimeMessages(value: unknown): unknown {
  if (typeof value === 'string') return () => value
  if (Array.isArray(value)) return value.map(toRuntimeMessages)
  if (!value || typeof value !== 'object') return value

  return Object.fromEntries(
    Object.entries(value).map(([key, child]) => [key, toRuntimeMessages(child)]),
  )
}

function i18nForRoute(locale: LocaleCode, routePath: string) {
  const messages = messagesForRoute(locale, routePath)
  return createI18n({
    legacy: false,
    locale,
    fallbackLocale: false,
    missingWarn: false,
    fallbackWarn: false,
    messages: { [locale]: toRuntimeMessages(messages) as Messages },
  })
}

function mountWithRouteLocale(locale: LocaleCode, routePath: string) {
  const i18n = i18nForRoute(locale, routePath)
  const wrapper = mount(GroupOptionItem, {
    props: {
      name: 'Example group',
      platform: 'openai',
      rateMultiplier: 1,
    },
    global: {
      plugins: [i18n],
      stubs: { GroupBadge: true },
    },
  })
  return { wrapper, i18n }
}

describe('GroupOptionItem description layout', () => {
  it('applies multiline and overflow-safe text styles', () => {
    const description = 'First section\nvery-long-unbroken-description-value-that-must-not-overflow'
    const i18n = i18nForRoute('en', '/keys')
    const wrapper = mount(GroupOptionItem, {
      props: {
        name: 'Example group',
        platform: 'openai',
        description,
      },
      global: {
        plugins: [i18n],
        stubs: {
          GroupBadge: true,
        },
      },
    })

    const descriptionElement = wrapper
      .findAll('span')
      .find((element) => element.text() === description)

    expect(descriptionElement).toBeDefined()
    expect(descriptionElement?.classes()).toContain('whitespace-pre-line')
    expect(descriptionElement?.classes()).toContain('[overflow-wrap:anywhere]')
    expect(descriptionElement?.classes()).toContain('line-clamp-3')
    expect(wrapper.find('[title]').attributes('title')).toBe(description)
  })
})

describe('GroupOptionItem locale scope contract', () => {
  const cases = [
    { consumer: 'keys', permission: 'user', routePath: '/keys', locale: 'en', expected: '1x Rate' },
    { consumer: 'keys', permission: 'user', routePath: '/keys', locale: 'zh', expected: '1x 倍率' },
    { consumer: 'users', permission: 'admin', routePath: '/admin/users', locale: 'en', expected: '1x Rate' },
    { consumer: 'users', permission: 'admin', routePath: '/admin/users', locale: 'zh', expected: '1x 倍率' },
    { consumer: 'subscriptions', permission: 'admin', routePath: '/admin/subscriptions', locale: 'en', expected: '1x Rate' },
    { consumer: 'subscriptions', permission: 'admin', routePath: '/admin/subscriptions', locale: 'zh', expected: '1x 倍率' },
    { consumer: 'redeem', permission: 'admin', routePath: '/admin/redeem', locale: 'en', expected: '1x Rate' },
    { consumer: 'redeem', permission: 'admin', routePath: '/admin/redeem', locale: 'zh', expected: '1x 倍率' },
    { consumer: 'settings', permission: 'admin', routePath: '/admin/settings', locale: 'en', expected: '1x Rate' },
    { consumer: 'settings', permission: 'admin', routePath: '/admin/settings', locale: 'zh', expected: '1x 倍率' },
  ] as const

  it.each(cases)(
    'renders the shared rate label for $consumer/$permission/$locale without exposing a locale key',
    ({ permission, routePath, locale, expected }) => {
      const { wrapper, i18n } = mountWithRouteLocale(locale, routePath)

      expect(wrapper.text()).toContain(expected)
      expect(wrapper.text()).not.toMatch(/(?:admin\.)?groups\.rateLabel/)
      expect(i18n.global.te('groups.rateLabel')).toBe(true)
      expect(i18n.global.te('admin.groups.rateLabel')).toBe(permission === 'admin')
    },
  )
})
