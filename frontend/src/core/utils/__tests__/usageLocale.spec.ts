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
import { formatImageSizeSource } from '@/core/utils/imageUsage'
import {
  usageBillingCostLineLabel,
  usageBillingCostLineValues,
  usageErrorCategoryLabel,
} from '@/core/utils/usageLocale'
import type { UsageLog } from '@/types'

type Messages = Record<string, unknown>
type LocaleCode = 'en' | 'zh'

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
        merged[key] && typeof merged[key] === 'object' && !Array.isArray(merged[key])
        && value && typeof value === 'object' && !Array.isArray(value)
      ) {
        merged[key] = mergeMessages([merged[key] as Messages, value as Messages])
      } else {
        merged[key] = value
      }
    }
  }
  return merged
}

function translator(locale: LocaleCode, routePath: string) {
  const messages = mergeMessages(
    getLocaleScopesForRoute(routePath).map((scope) => localeScopes[locale][scope]),
  )
  return (key: string): string => {
    const value = key.split('.').reduce<unknown>((current, segment) => {
      if (!current || typeof current !== 'object') return undefined
      return (current as Messages)[segment]
    }, messages)
    return typeof value === 'string' ? value : key
  }
}

describe('shared usage locale contract', () => {
  const imageSources = ['output', 'input', 'default', 'legacy'] as const
  const expected = {
    en: {
      sources: ['Upstream output', 'Request input', 'Default billing tier', 'Legacy record'],
      costLines: ['Text input', 'Image input', 'Text output', 'Image output', 'Cache creation', 'Cache read', 'Per request', 'Image generation', 'Video generation'],
      missing: 'Not recorded',
      legacy: 'Legacy record',
      unknown: 'Unknown',
      errorCategory: 'Invalid request',
    },
    zh: {
      sources: ['上游输出', '请求输入', '默认计费档位', '历史记录'],
      costLines: ['文本输入', '图片输入', '文本输出', '图片输出', '缓存创建', '缓存读取', '按次请求', '图片生成', '视频生成'],
      missing: '未记录',
      legacy: '历史记录',
      unknown: '未知',
      errorCategory: '参数错误',
    },
  } as const

  it.each(['/usage', '/admin/usage'])('maps every image source and cost line on %s', (routePath) => {
    for (const locale of ['en', 'zh'] as const) {
      const t = translator(locale, routePath)
      const sourceLabels = imageSources.map((image_size_source) => formatImageSizeSource({
        image_size: '1K',
        image_size_source,
      } as UsageLog, t))
      const costLabels = usageBillingCostLineValues.map((key) => (
        usageBillingCostLineLabel(t, key)
      ))

      expect(sourceLabels).toEqual(expected[locale].sources)
      expect(costLabels).toEqual(expected[locale].costLines)
      expect([...sourceLabels, ...costLabels].join(' ')).not.toMatch(/usage\.[A-Za-z0-9_.-]+/)
      expect(usageErrorCategoryLabel(t, 'invalid_request')).toBe(expected[locale].errorCategory)
    }
  })

  it.each(['en', 'zh'] as const)('uses localized fallbacks for unknown values in %s', (locale) => {
    const t = translator(locale, '/usage')

    expect(formatImageSizeSource({ image_size: '1K', image_size_source: 'future' } as UsageLog, t))
      .toBe(expected[locale].legacy)
    expect(formatImageSizeSource({ image_size: '', image_size_source: 'future' } as UsageLog, t))
      .toBe(expected[locale].missing)
    expect(usageBillingCostLineLabel(t, 'future')).toBe(expected[locale].unknown)
    expect(usageErrorCategoryLabel(t, 'future')).toBe(expected[locale].unknown)
  })
})
