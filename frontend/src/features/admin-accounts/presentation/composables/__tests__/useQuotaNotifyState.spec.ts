import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises } from '@vue/test-utils'
import { QUOTA_THRESHOLD_TYPE_PERCENTAGE } from '@/core/constants/account'

const { getSettings } = vi.hoisted(() => ({
  getSettings: vi.fn()
}))

vi.mock('@/features/admin-settings/data/datasources/adminSettingsQueries', () => ({
  getSettings
}))

import { useQuotaNotifyState } from '@/features/admin-accounts/presentation/composables/useQuotaNotifyState'

describe('useQuotaNotifyState', () => {
  beforeEach(() => {
    getSettings.mockReset()
  })

  it('loads the global switch and falls back to disabled when settings fail', async () => {
    const quotaNotify = useQuotaNotifyState()
    getSettings.mockResolvedValueOnce({ account_quota_notify_enabled: true })

    quotaNotify.loadGlobalState()
    await flushPromises()
    expect(quotaNotify.globalEnabled.value).toBe(true)

    getSettings.mockRejectedValueOnce(new Error('settings unavailable'))
    quotaNotify.loadGlobalState()
    await flushPromises()
    expect(quotaNotify.globalEnabled.value).toBe(false)
  })

  it('preserves quota threshold fields and removes disabled dimensions on update', () => {
    const quotaNotify = useQuotaNotifyState()
    quotaNotify.loadFromExtra({
      quota_notify_daily_enabled: true,
      quota_notify_daily_threshold: 25,
      quota_notify_daily_threshold_type: QUOTA_THRESHOLD_TYPE_PERCENTAGE
    })
    const extra: Record<string, unknown> = {
      keep_me: true,
      quota_notify_weekly_enabled: true,
      quota_notify_weekly_threshold: 50,
      quota_notify_weekly_threshold_type: QUOTA_THRESHOLD_TYPE_PERCENTAGE
    }

    quotaNotify.writeToExtra(extra, 'update')

    expect(extra).toEqual({
      keep_me: true,
      quota_notify_daily_enabled: true,
      quota_notify_daily_threshold: 25,
      quota_notify_daily_threshold_type: QUOTA_THRESHOLD_TYPE_PERCENTAGE
    })
  })
})
