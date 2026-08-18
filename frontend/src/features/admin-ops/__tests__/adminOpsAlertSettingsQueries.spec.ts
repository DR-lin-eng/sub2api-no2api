import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put, remove } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  remove: vi.fn()
}))

vi.mock('@/core/networks/client', () => ({
  apiClient: { get, post, put, delete: remove }
}))

import opsAPI from '@/features/admin-ops/data/datasources/adminOpsDatasource'
import {
  createAlertRule,
  createAlertSilence,
  deleteAlertRule,
  updateAlertEventStatus,
  updateAlertRule
} from '@/features/admin-ops/data/datasources/opsAlertActions'
import {
  getAlertEvent,
  listAlertEvents,
  listAlertRules
} from '@/features/admin-ops/data/datasources/opsAlertQueries'
import {
  updateAdvancedSettings,
  updateAlertRuntimeSettings,
  updateEmailNotificationConfig,
  updateMetricThresholds
} from '@/features/admin-ops/data/datasources/opsSettingsActions'
import {
  getAdvancedSettings,
  getAlertRuntimeSettings,
  getEmailNotificationConfig,
  getMetricThresholds,
  getSettingsSnapshot
} from '@/features/admin-ops/data/datasources/opsSettingsQueries'
import type {
  AlertEventsQuery,
  AlertRule,
  AlertSilenceRequest
} from '@/features/admin-ops/data/dtos/opsAlertDtos'
import type {
  EmailNotificationConfig,
  OpsAdvancedSettings,
  OpsAlertRuntimeSettings,
  OpsMetricThresholds
} from '@/features/admin-ops/data/dtos/opsSettingsDtos'

describe('admin ops alert and settings owners', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
    remove.mockReset()
  })

  it('keeps compatibility facade function identities', () => {
    expect(opsAPI.listAlertRules).toBe(listAlertRules)
    expect(opsAPI.createAlertRule).toBe(createAlertRule)
    expect(opsAPI.updateAlertRule).toBe(updateAlertRule)
    expect(opsAPI.deleteAlertRule).toBe(deleteAlertRule)
    expect(opsAPI.listAlertEvents).toBe(listAlertEvents)
    expect(opsAPI.getAlertEvent).toBe(getAlertEvent)
    expect(opsAPI.updateAlertEventStatus).toBe(updateAlertEventStatus)
    expect(opsAPI.createAlertSilence).toBe(createAlertSilence)
    expect(opsAPI.getSettingsSnapshot).toBe(getSettingsSnapshot)
    expect(opsAPI.getEmailNotificationConfig).toBe(getEmailNotificationConfig)
    expect(opsAPI.updateEmailNotificationConfig).toBe(updateEmailNotificationConfig)
    expect(opsAPI.getAlertRuntimeSettings).toBe(getAlertRuntimeSettings)
    expect(opsAPI.updateAlertRuntimeSettings).toBe(updateAlertRuntimeSettings)
    expect(opsAPI.getAdvancedSettings).toBe(getAdvancedSettings)
    expect(opsAPI.updateAdvancedSettings).toBe(updateAdvancedSettings)
    expect(opsAPI.getMetricThresholds).toBe(getMetricThresholds)
    expect(opsAPI.updateMetricThresholds).toBe(updateMetricThresholds)
  })

  it('preserves alert and settings query paths and parameters', async () => {
    const response = { marker: 'query' }
    const params: AlertEventsQuery = {
      limit: 20,
      status: 'firing',
      severity: 'P1',
      email_sent: false,
      time_range: '24h',
      start_time: '2026-08-16T00:00:00Z',
      end_time: '2026-08-17T00:00:00Z',
      before_fired_at: '2026-08-16T12:00:00Z',
      before_id: 31,
      platform: 'openai',
      group_id: 7
    }
    get.mockResolvedValue({ data: response })

    await expect(listAlertRules()).resolves.toBe(response)
    await expect(listAlertEvents(params)).resolves.toBe(response)
    await expect(getAlertEvent(37)).resolves.toBe(response)
    await expect(getSettingsSnapshot()).resolves.toBe(response)
    await expect(getEmailNotificationConfig()).resolves.toBe(response)
    await expect(getAlertRuntimeSettings()).resolves.toBe(response)
    await expect(getAdvancedSettings()).resolves.toBe(response)
    await expect(getMetricThresholds()).resolves.toBe(response)

    expect(get).toHaveBeenNthCalledWith(1, '/admin/ops/alert-rules')
    expect(get).toHaveBeenNthCalledWith(2, '/admin/ops/alert-events', { params })
    expect(get).toHaveBeenNthCalledWith(3, '/admin/ops/alert-events/37')
    expect(get).toHaveBeenNthCalledWith(4, '/admin/ops/settings/snapshot')
    expect(get).toHaveBeenNthCalledWith(5, '/admin/ops/email-notification/config')
    expect(get).toHaveBeenNthCalledWith(6, '/admin/ops/runtime/alert')
    expect(get).toHaveBeenNthCalledWith(7, '/admin/ops/advanced-settings')
    expect(get).toHaveBeenNthCalledWith(8, '/admin/ops/settings/metric-thresholds')
  })

  it('preserves alert and settings action paths and payloads', async () => {
    const response = { marker: 'action' }
    const rule = {
      name: 'High error rate',
      enabled: true,
      metric_type: 'error_rate',
      operator: '>',
      threshold: 5,
      window_minutes: 5,
      sustained_minutes: 2,
      severity: 'P1',
      cooldown_minutes: 10,
      notify_email: true
    } satisfies AlertRule
    const silence: AlertSilenceRequest = {
      rule_id: 41,
      platform: 'openai',
      group_id: 7,
      region: 'us-east',
      until: '2026-08-18T00:00:00Z',
      reason: 'maintenance'
    }
    const email = { marker: 'email' } as unknown as EmailNotificationConfig
    const runtime = { marker: 'runtime' } as unknown as OpsAlertRuntimeSettings
    const advanced = { marker: 'advanced' } as unknown as OpsAdvancedSettings
    const thresholds = { sla_percent_min: 99.5 } satisfies OpsMetricThresholds
    post.mockResolvedValue({ data: response })
    put.mockResolvedValue({ data: response })
    remove.mockResolvedValue({ data: undefined })

    await expect(createAlertRule(rule)).resolves.toBe(response)
    await expect(updateAlertRule(43, { enabled: false })).resolves.toBe(response)
    await expect(deleteAlertRule(47)).resolves.toBeUndefined()
    await expect(updateAlertEventStatus(53, 'manual_resolved')).resolves.toBeUndefined()
    await expect(createAlertSilence(silence)).resolves.toBeUndefined()
    await expect(updateEmailNotificationConfig(email)).resolves.toBe(response)
    await expect(updateAlertRuntimeSettings(runtime)).resolves.toBe(response)
    await expect(updateAdvancedSettings(advanced)).resolves.toBe(response)
    await expect(updateMetricThresholds(thresholds)).resolves.toBeUndefined()

    expect(post).toHaveBeenNthCalledWith(1, '/admin/ops/alert-rules', rule)
    expect(put).toHaveBeenNthCalledWith(1, '/admin/ops/alert-rules/43', { enabled: false })
    expect(remove).toHaveBeenCalledWith('/admin/ops/alert-rules/47')
    expect(put).toHaveBeenNthCalledWith(2, '/admin/ops/alert-events/53/status', {
      status: 'manual_resolved'
    })
    expect(post).toHaveBeenNthCalledWith(2, '/admin/ops/alert-silences', silence)
    expect(put).toHaveBeenNthCalledWith(3, '/admin/ops/email-notification/config', email)
    expect(put).toHaveBeenNthCalledWith(4, '/admin/ops/runtime/alert', runtime)
    expect(put).toHaveBeenNthCalledWith(5, '/admin/ops/advanced-settings', advanced)
    expect(put).toHaveBeenNthCalledWith(6, '/admin/ops/settings/metric-thresholds', thresholds)
  })
})
