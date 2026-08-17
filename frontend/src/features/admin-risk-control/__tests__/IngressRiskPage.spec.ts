import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import IngressRiskView from '@/features/admin-risk-control/presentation/pages/IngressRiskPage.vue'

const mocks = vi.hoisted(() => ({
  listIngressRejections: vi.fn(),
  getIngressCollectorHealth: vi.fn(),
  getAuthCacheHealth: vi.fn(),
  getCloudflareIngressSettings: vi.fn(),
  updateCloudflareIngressSettings: vi.fn(),
}))

vi.mock('@/features/admin-risk-control/data/datasources/ingressRiskDatasource', () => ({
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

const ToggleStub = defineComponent({
  inheritAttrs: false,
  props: ['modelValue', 'disabled'],
  emits: ['update:modelValue'],
  template: '<button v-bind="$attrs" type="button" :disabled="disabled" @click="$emit(\'update:modelValue\', !modelValue)">{{ modelValue }}</button>',
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
    cloudflare: {
      enabled: true,
      mode: 'zone_access_rules' as 'zone_access_rules' | 'waf_custom_rules',
      running: true,
      queue_depth: 0,
      queue_capacity: 1024,
      active_rules: 2,
      enqueued: 5,
      applied: 5,
      released: 3,
      failures: 0,
      dropped: 0,
      last_error: undefined as string | undefined,
      last_success_at: '2026-08-17T01:00:00Z',
      waf: undefined as undefined | {
        hostname: string
        hostnames: string[]
        hostname_stats: Array<{ hostname: string; requests_24h: number; blocked_requests_24h: number }>
        rule_count: number
        synced_entries: number
        overflow_entries: number
        hostname_requests_24h: number
        blocked_requests_24h: number
        last_synced_at?: string
        analytics_updated_at?: string
        analytics_error?: string
      },
    },
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

const baseCloudflareSettings = () => ({
  enabled: true,
  mode: 'zone_access_rules' as const,
  zone_id: '0123456789abcdef0123456789abcdef',
  api_token_configured: true,
  waf_hostname: '',
  waf_hostnames: [] as string[],
  waf_rule_ids: [] as string[],
  waf_sync_interval_seconds: 15,
  analytics_interval_seconds: 300,
  request_timeout_seconds: 5,
  queue_capacity: 1024,
  max_active_rules: 1000,
  reconcile_interval_seconds: 300,
})

function mountView() {
  return mount(IngressRiskView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        DataTable: DataTableStub,
        Pagination: { template: '<div data-test="pagination" />' },
        Select: { props: ['modelValue', 'options'], template: '<div data-test="select" />' },
        Toggle: ToggleStub,
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
    mocks.getCloudflareIngressSettings.mockResolvedValue(baseCloudflareSettings())
    mocks.updateCloudflareIngressSettings.mockResolvedValue(baseCloudflareSettings())
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
    expect(mocks.getCloudflareIngressSettings).toHaveBeenCalledOnce()
    expect(wrapper.get('[data-test="metric-recorded"]').text()).toBe('123')
    expect(wrapper.get('[data-test="metric-rejected"]').text()).toBe('456')
    expect(wrapper.get('[data-test="health-band"]').text()).toContain('admin.ingressRisk.health.healthy')
    expect(wrapper.get('[data-test="cloudflare-edge"]').text()).toContain('admin.ingressRisk.cloudflare.status.healthy')
    expect(wrapper.get('[data-test="cloudflare-edge"]').text()).toContain('2')
  })

  it('marks overall health as warning when Cloudflare synchronization fails', async () => {
    const health = baseAuthHealth()
    health.invalid_abuse.cloudflare.failures = 1
    health.invalid_abuse.cloudflare.last_error = 'Cloudflare unavailable'
    mocks.getAuthCacheHealth.mockResolvedValue(health)

    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-test="health-band"]').text()).toContain('admin.ingressRisk.health.warning')
    expect(wrapper.get('[data-test="cloudflare-edge"]').text()).toContain('Cloudflare unavailable')
  })

  it('keeps configured Cloudflare settings and advanced fields collapsed by default', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.find('[data-test="cloudflare-zone-id"]').exists()).toBe(false)
    await wrapper.get('[data-test="toggle-cloudflare-settings"]').trigger('click')
    expect(wrapper.get('[data-test="cloudflare-zone-id"]').exists()).toBe(true)
    expect(wrapper.find('#cloudflare-timeout').exists()).toBe(false)
    await wrapper.get('[data-test="toggle-cloudflare-advanced"]').trigger('click')
    expect(wrapper.get('#cloudflare-timeout').exists()).toBe(true)
  })

  it('configures and hot-enables Cloudflare without exposing the saved token', async () => {
    const health = baseAuthHealth()
    health.invalid_abuse.cloudflare.enabled = false
    health.invalid_abuse.cloudflare.running = false
    health.invalid_abuse.cloudflare.active_rules = 0
    mocks.getAuthCacheHealth.mockResolvedValue(health)
    mocks.getCloudflareIngressSettings.mockResolvedValue({
      ...baseCloudflareSettings(),
      enabled: false,
      zone_id: '',
      api_token_configured: false,
    })
    mocks.updateCloudflareIngressSettings.mockResolvedValue({
      ...baseCloudflareSettings(),
      enabled: true,
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="cloudflare-zone-id"]').setValue('0123456789abcdef0123456789abcdef')
    await wrapper.get('[data-test="cloudflare-api-token"]').setValue('new-secret-token')
    await wrapper.get('[data-test="cloudflare-enabled"]').trigger('click')
    await wrapper.get('[data-test="save-cloudflare-settings"]').trigger('click')
    await flushPromises()

    expect(mocks.updateCloudflareIngressSettings).toHaveBeenCalledWith({
      enabled: true,
      mode: 'zone_access_rules',
      zone_id: '0123456789abcdef0123456789abcdef',
      api_token: 'new-secret-token',
      waf_hostname: '',
      waf_hostnames: [],
      waf_rule_ids: [],
      waf_sync_interval_seconds: 15,
      analytics_interval_seconds: 300,
      request_timeout_seconds: 5,
      queue_capacity: 1024,
      max_active_rules: 1000,
      reconcile_interval_seconds: 300,
    })
    expect((wrapper.get('[data-test="cloudflare-api-token"]').element as HTMLInputElement).value).toBe('')
    expect(wrapper.get('[data-test="cloudflare-settings"]').text()).toContain('admin.ingressRisk.cloudflare.settings.saved')
  })

  it('configures sharded WAF rules and displays cached hostname request counts', async () => {
    const firstRule = 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
    const secondRule = 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
    const health = baseAuthHealth()
    health.invalid_abuse.cloudflare.enabled = false
    health.invalid_abuse.cloudflare.running = false
    health.invalid_abuse.cloudflare.active_rules = 0
    health.invalid_abuse.cloudflare.mode = 'waf_custom_rules'
    health.invalid_abuse.cloudflare.waf = {
      hostname: 'api.example.com',
      hostnames: ['api.example.com', 'edge.example.com'],
      hostname_stats: [
        { hostname: 'api.example.com', requests_24h: 10_000, blocked_requests_24h: 500 },
        { hostname: 'edge.example.com', requests_24h: 2_345, blocked_requests_24h: 178 },
      ],
      rule_count: 2,
      synced_entries: 0,
      overflow_entries: 0,
      hostname_requests_24h: 12_345,
      blocked_requests_24h: 678,
      last_synced_at: '2026-08-17T01:00:00Z',
      analytics_updated_at: '2026-08-17T01:05:00Z',
    }
    mocks.getAuthCacheHealth.mockResolvedValue(health)
    mocks.getCloudflareIngressSettings.mockResolvedValue({
      ...baseCloudflareSettings(),
      enabled: false,
    })
    mocks.updateCloudflareIngressSettings.mockResolvedValue({
      ...baseCloudflareSettings(),
      enabled: true,
      mode: 'waf_custom_rules',
      waf_hostname: 'api.example.com',
      waf_hostnames: ['api.example.com', 'edge.example.com'],
      waf_rule_ids: [firstRule, secondRule],
    })

    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.get('[data-test="cloudflare-waf-analytics"]').text()).toContain('12.3K')
    expect(wrapper.get('[data-test="cloudflare-waf-analytics"]').text()).toContain('678')
    expect(wrapper.get('[data-test="cloudflare-waf-hostname-stats"]').text()).toContain('api.example.com')
    expect(wrapper.get('[data-test="cloudflare-waf-hostname-stats"]').text()).toContain('edge.example.com')

    await wrapper.get('[data-test="cloudflare-mode-waf_custom_rules"]').setValue()
    await wrapper.get('[data-test="cloudflare-waf-hostname"]').setValue('edge.example.com\nAPI.example.com.\nedge.example.com')
    await wrapper.get('[data-test="cloudflare-waf-rule-ids"]').setValue(`${firstRule}\n${secondRule}`)
    await wrapper.get('[data-test="cloudflare-enabled"]').trigger('click')
    await wrapper.get('[data-test="save-cloudflare-settings"]').trigger('click')
    await flushPromises()

    expect(mocks.updateCloudflareIngressSettings).toHaveBeenCalledWith(expect.objectContaining({
      enabled: true,
      mode: 'waf_custom_rules',
      waf_hostname: 'api.example.com',
      waf_hostnames: ['api.example.com', 'edge.example.com'],
      waf_rule_ids: [firstRule, secondRule],
      waf_sync_interval_seconds: 15,
      analytics_interval_seconds: 300,
    }))
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
