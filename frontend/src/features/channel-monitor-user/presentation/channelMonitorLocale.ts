type Translate = (key: string) => string

const statusKeys = {
  operational: 'monitorCommon.status.operational',
  degraded: 'monitorCommon.status.degraded',
  failed: 'monitorCommon.status.failed',
  error: 'monitorCommon.status.error',
  unknown: 'monitorCommon.status.unknown',
} as const

const modeKeys = {
  active: 'monitorCommon.modes.active',
  passive: 'monitorCommon.modes.passive',
} as const

const overallKeys = {
  operational: 'channelStatus.overall.operational',
  degraded: 'channelStatus.overall.degraded',
  unavailable: 'channelStatus.overall.unavailable',
} as const

const providerKeys = {
  openai: 'monitorCommon.providers.openai',
  anthropic: 'monitorCommon.providers.anthropic',
  gemini: 'monitorCommon.providers.gemini',
  grok: 'monitorCommon.providers.grok',
} as const

const bodyModeKeys = {
  off: 'admin.channelMonitor.advanced.bodyModeOff',
  merge: 'admin.channelMonitor.advanced.bodyModeMerge',
  replace: 'admin.channelMonitor.advanced.bodyModeReplace',
} as const

export function channelMonitorStatusLabel(t: Translate, value: unknown): string {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : ''
  return normalized in statusKeys
    ? t(statusKeys[normalized as keyof typeof statusKeys])
    : t(statusKeys.unknown)
}

export function channelMonitorModeLabel(t: Translate, value: unknown): string {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : ''
  return normalized in modeKeys
    ? t(modeKeys[normalized as keyof typeof modeKeys])
    : t('common.unknown')
}

export function channelMonitorOverallLabel(t: Translate, value: unknown): string {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : ''
  return normalized in overallKeys
    ? t(overallKeys[normalized as keyof typeof overallKeys])
    : t(overallKeys.unavailable)
}

export function channelMonitorProviderLabel(t: Translate, value: unknown): string {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : ''
  return normalized in providerKeys
    ? t(providerKeys[normalized as keyof typeof providerKeys])
    : t('common.unknown')
}

export function channelMonitorBodyModeLabel(t: Translate, value: unknown): string {
  const normalized = typeof value === 'string' ? value.trim().toLowerCase() : ''
  return normalized in bodyModeKeys
    ? t(bodyModeKeys[normalized as keyof typeof bodyModeKeys])
    : t('common.unknown')
}
