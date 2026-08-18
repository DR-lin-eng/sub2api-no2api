import { describe, expect, it } from 'vitest'
import enLocale from '@/core/i18n/locales/en/admin/ingressRisk'
import zhLocale from '@/core/i18n/locales/zh/admin/ingressRisk'
import {
  cloudflareStatusDescription,
  cloudflareStatusLabel,
  cloudflareTokenStatusLabel,
  ingressRiskProtocolLabel,
  ingressRiskReasonLabel,
  ingressRiskRouteLabel,
} from '@/features/admin-risk-control/presentation/ingressRiskLocale'

function translator(messages: typeof enLocale, unknownLabel: string) {
  return (key: string): string => {
    if (key === 'common.unknown') return unknownLabel
    const path = key.replace(/^admin\./, '').split('.')
    let value: unknown = messages
    for (const part of path) {
      value = typeof value === 'object' && value !== null
        ? (value as Record<string, unknown>)[part]
        : undefined
    }
    return typeof value === 'string' ? value : key
  }
}

describe.each([
  { name: 'English', t: translator(enLocale, 'Unknown'), expected: ['Invalid API Key', 'Chat Completions', 'OpenAI', 'Unknown'] },
  { name: 'Chinese', t: translator(zhLocale as typeof enLocale, '未知'), expected: ['API Key 无效', 'Chat Completions', 'OpenAI', '未知'] },
])('ingress risk locale mapping - $name', ({ t, expected }) => {
  it('maps known values and localizes future values', () => {
    expect(ingressRiskReasonLabel(t, 'invalid_api_key')).toBe(expected[0])
    expect(ingressRiskRouteLabel(t, 'chat_completions')).toBe(expected[1])
    expect(ingressRiskProtocolLabel(t, 'openai')).toBe(expected[2])
    expect(ingressRiskReasonLabel(t, 'future_reason')).toBe(expected[3])
    expect(ingressRiskRouteLabel(t, 'future_route')).toBe(expected[3])
    expect(ingressRiskProtocolLabel(t, 'future_protocol')).toBe(expected[3])
    expect(cloudflareStatusLabel(t, 'future_cloudflare')).toBe(expected[3])
    expect(cloudflareStatusDescription(t, 'future_cloudflare')).toBe(expected[3])
    expect(cloudflareTokenStatusLabel(t, true)).toBe(t('admin.ingressRisk.cloudflare.settings.tokenConfigured'))
  })
})
