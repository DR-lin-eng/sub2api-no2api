import { describe, expect, it } from 'vitest'
import enLocale from '@/core/i18n/locales/en/admin/promptAudit'
import zhLocale from '@/core/i18n/locales/zh/admin/promptAudit'
import {
  promptAuditActionLabel,
  promptAuditCategoryDescription,
  promptAuditCategoryLabel,
  promptAuditDecisionLabel,
  promptAuditDependencyStatusLabel,
  promptAuditModeLabel,
  promptAuditProcessStatusLabel,
  promptAuditRiskLevelLabel,
} from '@/features/prompt-audit/presentation/promptAuditLocale'

function translator(messages: typeof enLocale) {
  return (key: string, params?: Record<string, unknown>): string => {
    const path = key.replace(/^admin\./, '').split('.')
    let value: unknown = messages
    for (const part of path) {
      value = typeof value === 'object' && value !== null
        ? (value as Record<string, unknown>)[part]
        : undefined
    }
    if (typeof value !== 'string') return key
    return value.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`))
  }
}

describe.each([
  {
    name: 'English',
    t: translator(enLocale),
    expected: { running: 'Running', async: 'Async audit only', ok: 'OK', decision: 'Flag', action: 'Block', risk: 'High', category: 'PII', description: 'Personal identifying information', unknown: 'Unknown' },
  },
  {
    name: 'Chinese',
    t: translator(zhLocale as typeof enLocale),
    expected: { running: '运行中', async: '异步只审计', ok: '正常', decision: '标记', action: '阻止', risk: '高', category: '个人身份信息', description: '个人身份信息', unknown: '未知' },
  },
])('Prompt Audit locale mapping - $name', ({ t, expected }) => {
  it('maps known process, mode, and dependency values', () => {
    expect(promptAuditProcessStatusLabel(t, 'running')).toBe(expected.running)
    expect(promptAuditModeLabel(t, 'async_audit')).toBe(expected.async)
    expect(promptAuditDependencyStatusLabel(t, 'ok')).toBe(expected.ok)
    expect(promptAuditDecisionLabel(t, 'flag')).toBe(expected.decision)
    expect(promptAuditActionLabel(t, 'Block')).toBe(expected.action)
    expect(promptAuditRiskLevelLabel(t, 'high')).toBe(expected.risk)
    expect(promptAuditCategoryLabel(t, 'pii')).toBe(expected.category)
    expect(promptAuditCategoryDescription(t, 'pii')).toBe(expected.description)
  })

  it('falls back to a localized unknown label for future values', () => {
    expect(promptAuditProcessStatusLabel(t, 'future_process')).toBe(expected.unknown)
    expect(promptAuditModeLabel(t, 'future_mode')).toBe(expected.unknown)
    expect(promptAuditDependencyStatusLabel(t, 'future_dependency')).toBe(expected.unknown)
    expect(promptAuditDecisionLabel(t, 'future_decision')).toBe(expected.unknown)
    expect(promptAuditActionLabel(t, 'future_action')).toBe(expected.unknown)
    expect(promptAuditRiskLevelLabel(t, 'future_risk')).toBe(expected.unknown)
    expect(promptAuditCategoryLabel(t, 'future_category')).toBe(expected.unknown)
    expect(promptAuditCategoryDescription(t, 'future_category')).toBe(expected.unknown)
    expect(promptAuditProcessStatusLabel(t, null)).toBe(expected.unknown)
  })
})
