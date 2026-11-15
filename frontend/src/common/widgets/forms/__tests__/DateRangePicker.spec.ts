import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

const messages: Record<string, string> = {
  'dates.today': 'Today',
  'dates.yesterday': 'Yesterday',
  'dates.last24Hours': 'Last 24 Hours',
  'dates.last7Days': 'Last 7 Days',
  'dates.last14Days': 'Last 14 Days',
  'dates.last30Days': 'Last 30 Days',
  'dates.thisMonth': 'This Month',
  'dates.lastMonth': 'Last Month',
  'dates.startDate': 'Start Date',
  'dates.endDate': 'End Date',
  'dates.apply': 'Apply',
  'dates.selectDateRange': 'Select date range'
}

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
    locale: ref('en')
  })
}))

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

describe('DateRangePicker', () => {
  afterEach(() => vi.useRealTimers())

  it('discards edits when dismissed and preserves a later applied range', async () => {
    const wrapper = mount(DateRangePicker, { props: { startDate: '2026-11-01', endDate: '2026-11-05' }, global: { stubs: { Icon: true } } })
    await wrapper.get('.date-picker-trigger').trigger('click')
    await wrapper.findAll('input[type="date"]')[0].setValue('2026-11-02')
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await wrapper.vm.$nextTick()
    await wrapper.get('.date-picker-trigger').trigger('click')
    expect((wrapper.findAll('input[type="date"]')[0].element as HTMLInputElement).value).toBe('2026-11-01')
    await wrapper.setProps({ startDate: '2026-11-03', endDate: '2026-11-06' })
    await wrapper.get('.date-picker-apply').trigger('click')
    await wrapper.get('.date-picker-trigger').trigger('click')
    expect((wrapper.findAll('input[type="date"]')[0].element as HTMLInputElement).value).toBe('2026-11-03')
    wrapper.unmount()
  })

  it('computes relative presets again after midnight without a timer', async () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 10, 15, 23, 59))
    const wrapper = mount(DateRangePicker, { props: { startDate: '2026-11-01', endDate: '2026-11-05' }, global: { stubs: { Icon: true } } })
    await wrapper.get('.date-picker-trigger').trigger('click')
    await wrapper.findAll('.date-picker-preset').find(node => node.text() === 'Today')!.trigger('click')
    expect((wrapper.findAll('input[type="date"]')[0].element as HTMLInputElement).value).toBe('2026-11-15')
    vi.setSystemTime(new Date(2026, 10, 16, 0, 1))
    await wrapper.findAll('.date-picker-preset').find(node => node.text() === 'Today')!.trigger('click')
    expect((wrapper.findAll('input[type="date"]')[0].element as HTMLInputElement).value).toBe('2026-11-16')
    wrapper.unmount()
  })
  it('uses last 24 hours as the default recognized preset', () => {
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: formatLocalDate(yesterday),
        endDate: formatLocalDate(now)
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Last 24 Hours')
  })

  it('emits range updates with last24Hours preset when applied', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.date-picker-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last 24 Hours')
    )
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')
    await wrapper.find('.date-picker-apply').trigger('click')

    const RFC3339_RE = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[+-]\d{2}:\d{2}$/
    const emittedStart = wrapper.emitted('update:startDate')?.[0]?.[0] as string
    const emittedEnd = wrapper.emitted('update:endDate')?.[0]?.[0] as string
    expect(emittedStart).toMatch(RFC3339_RE)
    expect(emittedEnd).toMatch(RFC3339_RE)
    expect(new Date(emittedEnd).getTime() - new Date(emittedStart).getTime()).toBe(
      24 * 60 * 60 * 1000
    )
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: emittedStart,
        endDate: emittedEnd,
        preset: 'last24Hours'
      }
    ])
  })
})
