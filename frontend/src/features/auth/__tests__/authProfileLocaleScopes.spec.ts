import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { describe, expect, it, vi } from 'vitest'

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
import TotpStepUpDialog from '@/features/auth/totpStepUpDialog'
import ProfilePasswordForm from '@/features/profile/presentation/widgets/ProfilePasswordForm.vue'
import { useStepUp } from '@/common/composables/useStepUp'

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}))

vi.mock('@/features/profile/data/datasources/totpDatasource', () => ({
  stepUp: vi.fn(),
}))

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

function i18nForRoute(locale: LocaleCode, routePath: string) {
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
  return { i18n, scopes }
}

describe('auth and profile locale scope contracts', () => {
  const profileCases = [
    { locale: 'en', expected: 'Change Password' },
    { locale: 'zh', expected: '修改密码' },
  ] as const

  it.each(profileCases)('renders profile/$locale without admin vocabulary', ({ expected, locale }) => {
    const { i18n, scopes } = i18nForRoute(locale, '/profile')
    const wrapper = mount(ProfilePasswordForm, { global: { plugins: [i18n] } })

    expect(wrapper.text()).toContain(expected)
    expect(wrapper.text()).not.toContain('profile.changePassword')
    expect(scopes).toEqual(['base', 'user'])
    expect(i18n.global.te('profile.changePassword')).toBe(true)
    expect(i18n.global.te('admin.settings.title')).toBe(false)
  })

  const adminCases = [
    { locale: 'en', expected: 'Two-Factor Verification Required' },
    { locale: 'zh', expected: '需要二次验证' },
  ] as const

  it.each(adminCases)('renders admin step-up/$locale with shared base copy', ({ expected, locale }) => {
    const { i18n, scopes } = i18nForRoute(locale, '/admin/accounts')
    const controller = useStepUp()
    controller.visible.value = true
    const wrapper = mount(TotpStepUpDialog, {
      props: { controller },
      global: { plugins: [i18n] },
    })

    expect(wrapper.text()).toContain(expected)
    expect(wrapper.text()).not.toContain('stepUp.title')
    expect(scopes).toEqual(['base', 'user', 'admin'])
    expect(i18n.global.te('stepUp.title')).toBe(true)
    expect(i18n.global.te('admin.settings.title')).toBe(true)
  })
})
