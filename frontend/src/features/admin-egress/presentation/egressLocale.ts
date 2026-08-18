import { enumLocaleLabel, type LocaleTranslate } from '@/core/i18n/enumLocale'

const modeKeys = {
  inherit: 'admin.egress.modes.inherit',
  direct: 'admin.egress.modes.direct',
  external_proxy: 'admin.egress.modes.external_proxy',
  ipv6_pool: 'admin.egress.modes.ipv6_pool',
  proxyOverride: 'admin.egress.modes.proxyOverride',
} as const

const statusKeys = {
  active: 'admin.egress.status.active',
  disabled: 'admin.egress.status.disabled',
} as const

const heStateKeys = {
  unavailable: 'admin.egress.he.states.unavailable',
  offline: 'admin.egress.he.states.offline',
  idle: 'admin.egress.he.states.idle',
  pending: 'admin.egress.he.states.pending',
  applying: 'admin.egress.he.states.applying',
  succeeded: 'admin.egress.he.states.succeeded',
  failed: 'admin.egress.he.states.failed',
} as const

const heActionKeys = {
  apply: 'admin.egress.he.actions.apply',
  check: 'admin.egress.he.actions.check',
  remove: 'admin.egress.he.actions.remove',
} as const

const heSuccessKeys = {
  apply: 'admin.egress.he.success.apply',
  check: 'admin.egress.he.success.check',
  remove: 'admin.egress.he.success.remove',
} as const

const heErrorKeys = {
  apply: 'admin.egress.he.errors.apply',
  check: 'admin.egress.he.errors.check',
  remove: 'admin.egress.he.errors.remove',
} as const

export function egressModeLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, modeKeys, value)
}

export function egressPoolStatusLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, statusKeys, value)
}

export function egressHEStateLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, heStateKeys, value)
}

export function egressHEActionLabel(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, heActionKeys, value)
}

export function egressHESuccessMessage(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, heSuccessKeys, value, 'admin.egress.errors.load')
}

export function egressHEErrorMessage(t: LocaleTranslate, value: unknown): string {
  return enumLocaleLabel(t, heErrorKeys, value, 'admin.egress.errors.load')
}
