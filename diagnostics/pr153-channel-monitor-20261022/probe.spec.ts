import { createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({ cachedPublicSettings: { channel_monitor_timeline_points: 35 }, showError: vi.fn(), showSuccess: vi.fn() }),
}))
vi.mock('@/features/auth', () => ({ useAuthStore: () => ({ isAdmin: false }) }))
vi.mock('@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource', () => ({
  list: vi.fn().mockResolvedValue({ items: [
    { id: 1, provider: 'openai', monitor_mode: 'active', name: 'Probe', group_name: 'Probe', primary_model: 'gpt', primary_status: 'operational', timeline: [], extra_models: [] },
    { id: 2, provider: 'openai', monitor_mode: 'passive', name: 'Traffic', group_name: 'Traffic', primary_model: 'gpt', primary_status: 'operational', timeline: [], extra_models: [] },
    { id: 3, provider: 'anthropic', monitor_mode: 'active', name: 'Other', group_name: 'Other', primary_model: 'claude', primary_status: 'failed', timeline: [], extra_models: [] },
  ] }),
  statusBatch: vi.fn().mockResolvedValue([]),
}))
import Page from '@/features/channels-user/presentation/pages/ChannelStatusPage.vue'
import Card from '@/features/channel-monitor-user/presentation/widgets/MonitorProviderCard.vue'
import Timeline from '@/features/channel-monitor-user/presentation/widgets/MonitorTimeline.vue'

describe('PR153 identical three-state probe', () => {
  it('observes grouping, timeline and provider selection through the real page', async () => {
    const wrapper = mount(Page, { global: { plugins: [createPinia()], stubs: {
      AppLayout: { template: '<div><slot /></div>' }, MonitorHero: true, MonitorDetailDialog: true,
    } } })
    await new Promise(resolve => setTimeout(resolve, 0))
    const before = { cards: wrapper.findAllComponents(Card).length, rows: wrapper.findAll('button.group\\/item').length,
      timeline_lengths: wrapper.findAllComponents(Timeline).map(item => item.props('length')), filter: wrapper.find('#provider-filter').exists() }
    if (before.filter) await wrapper.get('#provider-filter').setValue('openai')
    const after = { cards: wrapper.findAllComponents(Card).length, rows: wrapper.findAll('button.group\\/item').length }
    expect(before.rows).toBe(3)
    console.log('PR153_PROBE ' + JSON.stringify({ before, after }))
    wrapper.unmount()
  })
})
