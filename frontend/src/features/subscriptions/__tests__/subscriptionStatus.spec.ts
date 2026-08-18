import { describe, expect, it } from 'vitest'

import enAdmin from '@/core/i18n/locales/en/admin'
import enCommon from '@/core/i18n/locales/en/common'
import enMisc from '@/core/i18n/locales/en/misc'
import zhAdmin from '@/core/i18n/locales/zh/admin'
import zhCommon from '@/core/i18n/locales/zh/common'
import zhMisc from '@/core/i18n/locales/zh/misc'
import {
  adminSubscriptionStatusLabel,
  adminSubscriptionStatusOptions,
} from '@/features/admin-subscriptions/subscriptionStatus'
import {
  subscriptionStatusValues,
  userSubscriptionStatusLabel,
} from '@/features/subscriptions/subscriptionStatus'

type Messages = Record<string, unknown>

const localeMessages = {
  en: { ...enCommon, ...enMisc, admin: enAdmin },
  zh: { ...zhCommon, ...zhMisc, admin: zhAdmin },
} satisfies Record<'en' | 'zh', Messages>

function resolveMessage(messages: Messages, key: string): unknown {
  return key.split('.').reduce<unknown>((current, segment) => {
    if (!current || typeof current !== 'object') return undefined
    return (current as Messages)[segment]
  }, messages)
}

function translator(locale: 'en' | 'zh') {
  return (key: string): string => {
    const value = resolveMessage(localeMessages[locale], key)
    return typeof value === 'string' ? value : key
  }
}

describe('subscription status locale contract', () => {
  const expected = {
    en: {
      user: ['Active', 'Suspended', 'Expired', 'Revoked'],
      admin: ['Active', 'Suspended', 'Expired', 'Revoked'],
      unknown: 'Unknown',
    },
    zh: {
      user: ['有效', '已暂停', '已过期', '已撤销'],
      admin: ['生效中', '已暂停', '已过期', '已撤销'],
      unknown: '未知',
    },
  } as const

  it.each(['en', 'zh'] as const)(
    'maps every backend status for user and admin views in %s',
    (locale) => {
      const t = translator(locale)
      const userLabels = subscriptionStatusValues.map((status) => (
        userSubscriptionStatusLabel(t, status)
      ))
      const adminOptions = adminSubscriptionStatusOptions(t)

      expect(userLabels).toEqual(expected[locale].user)
      expect(adminOptions.map(({ value }) => value)).toEqual(subscriptionStatusValues)
      expect(adminOptions.map(({ label }) => label)).toEqual(expected[locale].admin)
      expect(userLabels.join(' ')).not.toMatch(/(?:userSubscriptions|admin\.subscriptions)\.status\./)
    },
  )

  it.each(['en', 'zh'] as const)('fails closed to a localized unknown label in %s', (locale) => {
    const t = translator(locale)

    expect(userSubscriptionStatusLabel(t, 'future_status')).toBe(expected[locale].unknown)
    expect(adminSubscriptionStatusLabel(t, 'future_status')).toBe(expected[locale].unknown)
  })
})
