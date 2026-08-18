import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'
import {
  channelMonitorModeLabel,
  channelMonitorBodyModeLabel,
  channelMonitorOverallLabel,
  channelMonitorProviderLabel,
  channelMonitorStatusLabel,
} from '@/features/channel-monitor-user/channelMonitorLocale'

const messages: Record<string, string> = {
  'monitorCommon.status.operational': 'Operational',
  'monitorCommon.status.degraded': 'Degraded',
  'monitorCommon.status.failed': 'Failed',
  'monitorCommon.status.error': 'Error',
  'monitorCommon.status.unknown': 'No data',
  'monitorCommon.modes.active': 'Active',
  'monitorCommon.modes.passive': 'Passive',
  'monitorCommon.providers.openai': 'OpenAI',
  'monitorCommon.providers.anthropic': 'Anthropic',
  'monitorCommon.providers.gemini': 'Gemini',
  'monitorCommon.providers.grok': 'Grok',
  'admin.channelMonitor.advanced.bodyModeMerge': 'Merge',
  'channelStatus.overall.operational': 'OPERATIONAL',
  'channelStatus.overall.degraded': 'DEGRADED',
  'channelStatus.overall.unavailable': 'UNAVAILABLE',
  'common.unknown': 'Unknown',
}
const t = (key: string) => messages[key] ?? key

const currentDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(currentDir, '../../../..')
const runtimeSources = [
  resolve(currentDir, '../presentation/composables/useChannelMonitorFormat.ts'),
  resolve(currentDir, '../presentation/widgets/MonitorCard.vue'),
  resolve(currentDir, '../presentation/widgets/MonitorHero.vue'),
  resolve(frontendDir, 'src/features/admin-channel-monitor/presentation/pages/ChannelMonitorPage.vue'),
].map((path) => readFileSync(path, 'utf8')).join('\n')

describe('channel monitor locale mappings', () => {
  it('maps every known status and falls back for future values', () => {
    expect(channelMonitorStatusLabel(t, 'operational')).toBe('Operational')
    expect(channelMonitorStatusLabel(t, 'degraded')).toBe('Degraded')
    expect(channelMonitorStatusLabel(t, 'failed')).toBe('Failed')
    expect(channelMonitorStatusLabel(t, 'error')).toBe('Error')
    expect(channelMonitorStatusLabel(t, 'unknown')).toBe('No data')
    expect(channelMonitorStatusLabel(t, 'maintenance')).toBe('No data')
    expect(channelMonitorStatusLabel(t, null)).toBe('No data')
  })

  it('maps monitor modes and overall health without emitting dynamic keys', () => {
    expect(channelMonitorModeLabel(t, 'active')).toBe('Active')
    expect(channelMonitorModeLabel(t, 'passive')).toBe('Passive')
    expect(channelMonitorModeLabel(t, 'future')).toBe('Unknown')
    expect(channelMonitorOverallLabel(t, 'operational')).toBe('OPERATIONAL')
    expect(channelMonitorOverallLabel(t, 'degraded')).toBe('DEGRADED')
    expect(channelMonitorOverallLabel(t, 'future')).toBe('UNAVAILABLE')
  })

  it('maps providers and localizes future provider values', () => {
    expect(channelMonitorProviderLabel(t, 'openai')).toBe('OpenAI')
    expect(channelMonitorProviderLabel(t, 'future_provider')).toBe('Unknown')
    expect(channelMonitorProviderLabel(t, null)).toBe('Unknown')
  })

  it.each([
    { merge: 'Merge', unknown: 'Unknown' },
    { merge: '合并', unknown: '未知' },
  ])('maps advanced body modes and localizes future values', ({ merge, unknown }) => {
    const translate = (key: string) => ({
      'admin.channelMonitor.advanced.bodyModeMerge': merge,
      'common.unknown': unknown,
    })[key] ?? key
    expect(channelMonitorBodyModeLabel(translate, 'merge')).toBe(merge)
    expect(channelMonitorBodyModeLabel(translate, 'future_mode')).toBe(unknown)
  })

  it('keeps shared user/admin views off direct enum-to-key interpolation', () => {
    expect(runtimeSources).not.toMatch(/t\(`monitorCommon\.(?:status|modes)\.\$\{/)
    expect(runtimeSources).not.toMatch(/t\(`monitorCommon\.providers\.\$\{/)
    expect(runtimeSources).not.toMatch(/t\(`channelStatus\.overall\.\$\{/)
  })
})
