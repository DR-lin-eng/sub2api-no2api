import { createPinia } from 'pinia'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import MonitorCardGrid from '@/features/channel-monitor-user/presentation/widgets/MonitorCardGrid.vue'
import MonitorProviderCard from '@/features/channel-monitor-user/presentation/widgets/MonitorProviderCard.vue'
import EmptyState from '@/common/widgets/feedback/EmptyState.vue'
import type {
  Provider,
  UserMonitorView,
} from '@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource'

function monitor(overrides: Partial<UserMonitorView> & { id: number }): UserMonitorView {
  return {
    name: `Monitor ${overrides.id}`,
    provider: 'openai' as Provider,
    group_name: `Group ${overrides.id}`,
    primary_model: 'gpt-5.2',
    primary_status: 'operational',
    primary_latency_ms: 100,
    primary_ping_latency_ms: 120,
    availability_7d: 99.9,
    extra_models: [],
    timeline: [],
    ...overrides,
  }
}

function mountGrid(items: UserMonitorView[], loading = false) {
  return mount(MonitorCardGrid, {
    props: {
      items,
      window: '7d' as const,
      countdownSeconds: 10,
      loading,
      detailCache: {},
    },
    global: {
      plugins: [createPinia()],
      stubs: { MonitorTimeline: true },
    },
  })
}

describe('MonitorCardGrid provider and monitor-mode aggregation', () => {
  it('collapses every group of one provider into a single card', () => {
    const wrapper = mountGrid([
      monitor({ id: 1, provider: 'openai', group_name: 'Group A' }),
      monitor({ id: 2, provider: 'openai', group_name: 'Group B' }),
      monitor({ id: 3, provider: 'openai', group_name: 'Group C' }),
    ])

    expect(wrapper.findAllComponents(MonitorProviderCard)).toHaveLength(1)
    expect(wrapper.findAll('li')).toHaveLength(3)
    expect(wrapper.text()).toContain('Group A')
    expect(wrapper.text()).toContain('Group C')
  })

  it('keeps different providers in separate cards', () => {
    const wrapper = mountGrid([
      monitor({ id: 1, provider: 'openai' }),
      monitor({ id: 2, provider: 'anthropic' }),
      monitor({ id: 3, provider: 'openai' }),
    ])

    const cards = wrapper.findAllComponents(MonitorProviderCard)
    expect(cards).toHaveLength(2)
    expect(cards[0].props('items')).toHaveLength(2)
    expect(cards[1].props('items')).toHaveLength(1)
  })

  it('splits active probes and passive monitors of one provider into separate cards', () => {
    const wrapper = mountGrid([
      monitor({ id: 1, provider: 'openai', monitor_mode: 'active', group_name: 'Probe A' }),
      monitor({ id: 2, provider: 'openai', monitor_mode: 'passive', group_name: 'Traffic A' }),
      monitor({ id: 3, provider: 'openai', monitor_mode: 'active', group_name: 'Probe B' }),
    ])

    const cards = wrapper.findAllComponents(MonitorProviderCard)
    expect(cards).toHaveLength(2)
    expect(cards[0].props('mode')).toBe('active')
    expect(cards[0].props('items')).toHaveLength(2)
    expect(cards[1].props('mode')).toBe('passive')
    expect(cards[1].props('items')).toHaveLength(1)
  })

  it('keeps legacy monitors without a mode in their own card', () => {
    const wrapper = mountGrid([
      monitor({ id: 1, provider: 'openai' }),
      monitor({ id: 2, provider: 'openai', monitor_mode: 'active' }),
    ])

    const cards = wrapper.findAllComponents(MonitorProviderCard)
    expect(cards).toHaveLength(2)
    expect(cards[0].props('mode')).toBeUndefined()
    expect(cards[1].props('mode')).toBe('active')
  })

  it('ranks abnormal groups above healthy ones without dropping them', () => {
    const wrapper = mountGrid([
      monitor({ id: 1, group_name: 'Healthy', primary_status: 'operational' }),
      monitor({ id: 2, group_name: 'Broken', primary_status: 'failed' }),
      monitor({ id: 3, group_name: 'Slow', primary_status: 'degraded' }),
    ])

    const labels = wrapper.findAll('li').map(li => li.text())
    expect(labels[0]).toContain('Broken')
    expect(labels[1]).toContain('Slow')
    expect(labels[2]).toContain('Healthy')
  })

  it('emits the clicked row so the detail dialog opens the right monitor', async () => {
    const wrapper = mountGrid([
      monitor({ id: 1, group_name: 'Healthy' }),
      monitor({ id: 2, group_name: 'Broken', primary_status: 'failed' }),
    ])

    await wrapper.findAll('li > button')[0].trigger('click')

    expect(wrapper.emitted('cardClick')?.[0][0]).toMatchObject({ id: 2 })
  })

  it('renders every row without collapsing groups', () => {
    const items = Array.from({ length: 7 }, (_, index) =>
      monitor({ id: index + 1, group_name: `Group ${index + 1}` }),
    )
    items.push(monitor({ id: 99, group_name: 'Failing', primary_status: 'failed' }))

    const wrapper = mountGrid(items)
    expect(wrapper.findAll('li')).toHaveLength(8)
    expect(wrapper.text()).toContain('Failing')
    expect(wrapper.find('section > button').exists()).toBe(false)
  })

  it('renders skeletons while loading and an empty state once settled', () => {
    const loading = mountGrid([], true)
    expect(loading.findComponent(EmptyState).exists()).toBe(false)
    expect(loading.findAll('.animate-pulse')).toHaveLength(4)

    const empty = mountGrid([], false)
    expect(empty.findComponent(EmptyState).exists()).toBe(true)
  })
})
