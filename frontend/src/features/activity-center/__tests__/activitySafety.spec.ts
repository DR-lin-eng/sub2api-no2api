import { describe, expect, it } from 'vitest'
import { completeCheckinCalendar } from '../presentation/checkinCalendar'
import { sanitizeActivityBannerHtml } from '../presentation/activityHtml'

describe('activity editor invariants', () => {
  it('builds every day of each cycle without overwriting entered rewards', () => {
    const weekly = completeCheckinCalendar('weekly')
    expect(weekly.map(item => item.day)).toEqual([1, 2, 3, 4, 5, 6, 7])
    weekly[0]!.value = '1.25'
    const monthly = completeCheckinCalendar('monthly', weekly)
    expect(monthly).toHaveLength(30)
    expect(monthly[0]!.value).toBe('1.25')
    expect(completeCheckinCalendar('biweekly', monthly)).toHaveLength(14)
  })
  it('keeps banner markup but excludes executable content and overlay stylesheets', () => {
    const html = sanitizeActivityBannerHtml('<div>Hello<script>alert(1)</script><img src="x" onerror="alert(1)"><style>body{display:none}</style><a href="javascript:alert(1)">go</a></div>')
    expect(html).toContain('Hello')
    expect(html).not.toMatch(/<script|onerror|javascript:|<style/)
  })
})

import enActivity from '@/core/i18n/locales/en/activityCenter'
import zhActivity from '@/core/i18n/locales/zh/activityCenter'
import { activityText } from '../presentation/activityCenterText'
import { getLocaleScopesForRoute } from '@/core/i18n'
it('keeps activity code and locales lazy outside activity routes', () => {
  expect(getLocaleScopesForRoute('/dashboard')).not.toContain('activityCenter')
  expect(getLocaleScopesForRoute('/activity-center')).toContain('activityCenter')
  expect(getLocaleScopesForRoute('/admin/activity-center/campaigns')).toContain('activityCenter')
})
it('covers every activity and reward enum in both languages and preserves administrator copy', () => {
  for (const messages of [enActivity.activityCenter, zhActivity.activityCenter]) {
    for (const value of ['lottery', 'inflate', 'redeem', 'custom', 'checkin']) expect(messages.types).toHaveProperty(value)
    for (const value of ['none', 'balance', 'concurrency', 'subscription', 'card']) expect(messages.prizeTypes).toHaveProperty(value)
  }
  expect(activityText('My custom campaign', key => key)).toBe('My custom campaign')
  expect(activityText('Default', key => key === 'activityCenter.legacy.defaultPool' ? 'Basic pool' : key)).toBe('Basic pool')
})
