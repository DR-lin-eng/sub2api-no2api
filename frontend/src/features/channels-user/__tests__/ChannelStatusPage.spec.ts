import { createPinia, setActivePinia } from 'pinia'
import { mount, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

const cachedPublicSettings = ref<Record<string, unknown> | null>({})
vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    cachedPublicSettings: cachedPublicSettings.value,
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/features/auth', () => ({
  useAuthStore: () => ({ isAdmin: false }),
}))

const listChannelMonitorViews = vi.fn()
vi.mock('@/features/channel-monitor-user/data/datasources/channelMonitorUserDatasource', () => ({
  list: (...args: unknown[]) => listChannelMonitorViews(...args),
  statusBatch: vi.fn().mockResolvedValue([]),
}))

import ChannelStatusPage from '@/features/channels-user/presentation/pages/ChannelStatusPage.vue'
import MonitorCardGrid from '@/features/channel-monitor-user/presentation/widgets/MonitorCardGrid.vue'
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

async function flush() {
  await Promise.resolve()
  await new Promise(resolve => setTimeout(resolve, 0))
}

const mounted: VueWrapper[] = []

afterEach(() => {
  for (const wrapper of mounted.splice(0)) wrapper.unmount()
})

beforeEach(() => {
  setActivePinia(createPinia())
  cachedPublicSettings.value = { channel_monitor_timeline_points: 20 }
  listChannelMonitorViews.mockReset()
})

async function mountPage(items: UserMonitorView[]) {
  listChannelMonitorViews.mockResolvedValue({ items })
  const wrapper = mount(ChannelStatusPage, {
    global: {
      plugins: [createPinia()],
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        MonitorHero: true,
        MonitorDetailDialog: true,
      },
    },
  })
  mounted.push(wrapper)
  await flush()
  return wrapper
}

describe('ChannelStatusPage provider filtering', () => {
  it('shows every provider by default and forwards the configured timeline length', async () => {
    const wrapper = await mountPage([
      monitor({ id: 1, provider: 'openai' }),
      monitor({ id: 2, provider: 'anthropic' }),
    ])

    const grid = wrapper.getComponent(MonitorCardGrid)
    expect(grid.props('items')).toHaveLength(2)
    expect(grid.props('timelinePoints')).toBe(20)
  })

  it('narrows the grid and the overall status to the selected provider', async () => {
    const wrapper = await mountPage([
      monitor({ id: 1, provider: 'openai', primary_status: 'operational' }),
      monitor({ id: 2, provider: 'anthropic', primary_status: 'failed' }),
    ])

    const select = wrapper.get('#provider-filter')
    await select.setValue('openai')

    const grid = wrapper.getComponent(MonitorCardGrid)
    expect(grid.props('items')).toHaveLength(1)
    expect(grid.props('items')[0].provider).toBe('openai')
    expect(wrapper.text()).toContain('channelStatus.allOperational')
  })

  it('falls back to 20 timeline points when the public setting is absent', async () => {
    cachedPublicSettings.value = {}
    const wrapper = await mountPage([monitor({ id: 1 })])
    expect(wrapper.getComponent(MonitorCardGrid).props('timelinePoints')).toBe(20)
  })
})
