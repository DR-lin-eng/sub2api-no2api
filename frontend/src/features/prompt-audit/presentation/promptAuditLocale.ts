import type { PromptAuditMode } from '../domain/models/promptAuditTypes'
import { enumLocaleLabel } from '@/core/i18n/enumLocale'

type Translate = (key: string, params?: Record<string, unknown>) => string

const PROCESS_STATUS_KEYS = {
  disabled: 'admin.promptAudit.status.disabled',
  running: 'admin.promptAudit.status.running',
  degraded: 'admin.promptAudit.status.degraded',
  error: 'admin.promptAudit.status.error',
} as const

const MODE_KEYS: Record<PromptAuditMode, string> = {
  off: 'admin.promptAudit.mode.off',
  async_audit: 'admin.promptAudit.mode.async_audit',
  blocking: 'admin.promptAudit.mode.blocking',
}

const DEPENDENCY_STATUS_KEYS = {
  ok: 'admin.promptAudit.runtime.dependencyStatus.ok',
  error: 'admin.promptAudit.runtime.dependencyStatus.error',
  unavailable: 'admin.promptAudit.runtime.dependencyStatus.unavailable',
} as const

const DECISION_KEYS = {
  pass: 'admin.promptAudit.decisions.pass',
  flag: 'admin.promptAudit.decisions.flag',
  critical: 'admin.promptAudit.decisions.critical',
} as const

const ACTION_KEYS = {
  Allow: 'admin.promptAudit.actions.Allow',
  Warn: 'admin.promptAudit.actions.Warn',
  Block: 'admin.promptAudit.actions.Block',
} as const

const RISK_LEVEL_KEYS = {
  low: 'admin.promptAudit.riskLevels.low',
  medium: 'admin.promptAudit.riskLevels.medium',
  high: 'admin.promptAudit.riskLevels.high',
  critical: 'admin.promptAudit.riskLevels.critical',
} as const

const CATEGORY_KEYS = {
  violent: 'admin.promptAudit.scanners.violent',
  non_violent_illegal_acts: 'admin.promptAudit.scanners.non_violent_illegal_acts',
  sexual_content_or_sexual_acts: 'admin.promptAudit.scanners.sexual_content_or_sexual_acts',
  pii: 'admin.promptAudit.scanners.pii',
  suicide_and_self_harm: 'admin.promptAudit.scanners.suicide_and_self_harm',
  unethical_acts: 'admin.promptAudit.scanners.unethical_acts',
  politically_sensitive_topics: 'admin.promptAudit.scanners.politically_sensitive_topics',
  copyright_violation: 'admin.promptAudit.scanners.copyright_violation',
  jailbreak: 'admin.promptAudit.scanners.jailbreak',
} as const

const CATEGORY_DESCRIPTION_KEYS = {
  violent: 'admin.promptAudit.scannerDescriptions.violent',
  non_violent_illegal_acts: 'admin.promptAudit.scannerDescriptions.non_violent_illegal_acts',
  sexual_content_or_sexual_acts: 'admin.promptAudit.scannerDescriptions.sexual_content_or_sexual_acts',
  pii: 'admin.promptAudit.scannerDescriptions.pii',
  suicide_and_self_harm: 'admin.promptAudit.scannerDescriptions.suicide_and_self_harm',
  unethical_acts: 'admin.promptAudit.scannerDescriptions.unethical_acts',
  politically_sensitive_topics: 'admin.promptAudit.scannerDescriptions.politically_sensitive_topics',
  copyright_violation: 'admin.promptAudit.scannerDescriptions.copyright_violation',
  jailbreak: 'admin.promptAudit.scannerDescriptions.jailbreak',
} as const

export function promptAuditProcessStatusLabel(t: Translate, value: unknown): string {
  return enumLocaleLabel(t, PROCESS_STATUS_KEYS, value, 'admin.promptAudit.status.unknown')
}

export function promptAuditModeLabel(t: Translate, value: unknown): string {
  return enumLocaleLabel(t, MODE_KEYS, value, 'admin.promptAudit.mode.unknown')
}

export function promptAuditDependencyStatusLabel(t: Translate, value: unknown): string {
  return enumLocaleLabel(t, DEPENDENCY_STATUS_KEYS, value, 'admin.promptAudit.runtime.dependencyStatus.unknown')
}

export function promptAuditDecisionLabel(t: Translate, value: unknown): string {
  return enumLocaleLabel(t, DECISION_KEYS, value, 'admin.promptAudit.common.unknown')
}

export function promptAuditActionLabel(t: Translate, value: unknown): string {
  return enumLocaleLabel(t, ACTION_KEYS, value, 'admin.promptAudit.common.unknown')
}

export function promptAuditRiskLevelLabel(t: Translate, value: unknown): string {
  return enumLocaleLabel(t, RISK_LEVEL_KEYS, value, 'admin.promptAudit.common.unknown')
}

export function promptAuditCategoryLabel(t: Translate, value: unknown): string {
  return enumLocaleLabel(t, CATEGORY_KEYS, value, 'admin.promptAudit.common.unknown')
}

export function promptAuditCategoryDescription(t: Translate, value: unknown): string {
  return enumLocaleLabel(t, CATEGORY_DESCRIPTION_KEYS, value, 'admin.promptAudit.common.unknown')
}
