import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProxyTable from '@/features/admin-proxies/presentation/widgets/ProxyTable.vue'

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params?.time ? `${key}:${params.time}` : key,
    }),
  }
})

const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <div v-for="row in data" :key="row.id">
        <slot name="cell-location" :row="row" />
        <slot name="cell-latency" :row="row" />
      </div>
    </div>
  `,
}

describe('ProxyTable exit IP and check timestamp', () => {
  it('renders exit IP under location and latency_checked_at under latency', () => {
    const row = {
      id: 7,
      name: 'jp-edge',
      protocol: 'socks5',
      host: '127.0.0.1',
      port: 1080,
      status: 'active',
      ip_address: '198.51.100.24',
      country: 'Japan',
      region: 'Tokyo',
      city: 'Chiyoda',
      latency_status: 'success',
      latency_ms: 68,
      latency_checked_at: '2026-11-27T08:00:00Z',
      health_status: 'healthy',
      health_consecutive_failures: 0,
    } as any

    const wrapper = mount(ProxyTable, {
      props: {
        context: {
          allVisibleSelected: false,
          columns: [],
          copyFormat: vi.fn(),
          copyMenuProxyId: null,
          copyProxyUrl: vi.fn(),
          expiryBadgeClass: () => '',
          expiryLabel: () => '',
          flagUrl: (c: string) => c,
          formatLocation: (p: any) => [p.country, p.region, p.city].filter(Boolean).join(' · '),
          getCopyFormats: () => [],
          handleDelete: vi.fn(),
          handleEdit: vi.fn(),
          handleQualityCheck: vi.fn(),
          handleSort: vi.fn(),
          handleTestConnection: vi.fn(),
          loading: false,
          openAccountsModal: vi.fn(),
          proxies: [row],
          qualityCheckingProxyIds: new Set(),
          qualityOverallClass: () => '',
          qualityOverallLabel: () => '',
          selectedProxyIds: new Set(),
          showCreateModal: false,
          testingProxyIds: new Set(),
          toggleCopyMenu: vi.fn(),
          toggleSelectAllVisible: vi.fn(),
          toggleSelectRow: vi.fn(),
          visiblePasswordIds: new Set(),
        } as any,
      },
      global: {
        stubs: {
          DataTable: DataTableStub,
          EmptyState: true,
          Icon: true,
        },
      },
    })

    expect(wrapper.text()).toContain('Japan · Tokyo · Chiyoda')
    expect(wrapper.text()).toContain('198.51.100.24')
    expect(wrapper.text()).toContain('68ms')
    expect(wrapper.text()).toContain('admin.proxies.latencyCheckedAt:')
  })
})
