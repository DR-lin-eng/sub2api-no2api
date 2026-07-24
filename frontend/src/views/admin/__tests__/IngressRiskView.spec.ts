import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import IngressRiskView from '../IngressRiskView.vue'

const mocks = vi.hoisted(() => ({
  listIngressRejections: vi.fn(),
  getIngressCollectorHealth: vi.fn(),
  getAuthCacheHealth: vi.fn(),
}))

vi.mock('@/api/admin/ingressRisk', () => ({
  ingressRiskAPI: mocks,
  default: mocks,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`)),
    }),
  }
})

const DataTableStub = defineComponent({
  props: ['data', 'columns', 'loading'],
  template: `
    <div data-test="table">
      <div v-for="row in data" :key="row.id">
        <slot name="cell-client_ip" :value="row.client_ip" :row="row" />
      </div>
      <slot v-if="!loading && data.length === 0" name="empty" />
    </div>
  `,
})

const baseCollector = () => ({
  cardinality: 3,
  capacity: 8192,
  pending_batches: 0,
  pending_rows: 0,
  overflowed_count: 0,
  dropped_count: 0,
  flushed_request_count: 42,
  flush_failure_count: 0,
  accepting: true,
})

const baseAuthHealth = () => ({
  outbox: {
    running: true,
    processed: 76,
    failures: 0,
    pending: 0,
    oldest_lag: 0,
    healthy_sla: 35_000_000_000,
    recovery_sla: 360_000_000_000,
    max_attempts: 0,
  },
  subscriber: { connected: true, failures: 0 },
  lookup: { total: 100, rejected: 0, in_flight: 0, capacity: 64 },
  invalid_abuse: {
    enabled: true,
    tracked: 7,
    capacity: 16_384,
    recorded: 123,
    blocks: 4,
    rejected: 456,
    expired: 2,
    overflowed: 0,
    global_blocked: 0,
  },
})

const baseList = () => ({
  items: [{
    id: 1,
    bucket_start: '2026-07-25T01:00:00Z',
    reject_reason: 'api_key_required',
    route_family: 'models',
    protocol: 'openai',
    client_ip: '192.0.2.10',
    request_count: 8,
    first_seen: '2026-07-25T01:00:02Z',
    last_seen: '2026-07-25T01:00:45Z',
  }],
  total: 1,
  page: 1,
  page_size: 25,
})

function mountView() {
  return mount(IngressRiskView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        DataTable: DataTableStub,
        Pagination: { template: '<div data-test="pagination" />' },
        Select: { props: ['modelValue', 'options'], template: '<div data-test="select" />' },
        Icon: { props: ['name'], template: '<i :data-icon="name" />' },
      },
    },
  })
}

describe('IngressRiskView', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
    mocks.listIngressRejections.mockResolvedValue(baseList())
    mocks.getIngressCollectorHealth.mockResolvedValue(baseCollector())
    mocks.getAuthCacheHealth.mockResolvedValue(baseAuthHealth())
  })

  it('loads the one-hour rejection window and both health surfaces', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(mocks.listIngressRejections).toHaveBeenCalledWith(expect.objectContaining({
      time_range: '1h',
      page: 1,
      page_size: 25,
    }))
    expect(mocks.getIngressCollectorHealth).toHaveBeenCalledOnce()
    expect(mocks.getAuthCacheHealth).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-test="metric-recorded"]').text()).toBe('123')
    expect(wrapper.get('[data-test="metric-rejected"]').text()).toBe('456')
    expect(wrapper.get('[data-test="health-band"]').text()).toContain('admin.ingressRisk.health.healthy')
  })

  it('turns a table IP into an exact server-side filter', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('button[aria-label="admin.ingressRisk.actions.filterIp"]').trigger('click')
    await flushPromises()

    expect(mocks.listIngressRejections).toHaveBeenLastCalledWith(expect.objectContaining({
      client_ip: '192.0.2.10',
      page: 1,
    }))
  })

  it('keeps rejection records visible when one health endpoint fails', async () => {
    mocks.getAuthCacheHealth.mockRejectedValue(new Error('health unavailable'))
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="table"]').text()).toContain('192.0.2.10')
    expect(wrapper.text()).toContain('health unavailable')
  })
})
