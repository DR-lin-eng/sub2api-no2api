import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import EgressPage from '@/features/admin-egress/presentation/pages/EgressPage.vue'

const egressAPI = vi.hoisted(() => ({
  getRuntime: vi.fn(),
  listPools: vi.fn(),
  discoverPrefixes: vi.fn(),
  createPool: vi.fn(),
  updatePool: vi.fn(),
  deletePool: vi.fn(),
  listBindings: vi.fn(),
  setAccountRoute: vi.fn(),
  rotateBinding: vi.fn(),
  probeAccount: vi.fn(),
  reconcileDefault: vi.fn(),
  getHETunnel: vi.fn(),
  saveHETunnel: vi.fn(),
  runHETunnelAction: vi.fn(),
}))

const accountsAPI = vi.hoisted(() => ({ list: vi.fn() }))
const appStore = vi.hoisted(() => ({ showSuccess: vi.fn(), showError: vi.fn() }))

vi.mock('@/features/admin-egress/data/datasources/adminEgressDatasource', () => ({ default: egressAPI }))
vi.mock('@/features/admin-accounts/data/datasources/adminAccountsDatasource', () => ({ default: accountsAPI }))
vi.mock('@/core/stores/appStore', () => ({ useAppStore: () => appStore }))
vi.mock('@/common/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('@/common/composables/useStepUp', () => ({
  useStepUp: () => ({ run: (action: () => unknown) => action() }),
  isStepUpCancelled: () => false,
}))
vi.mock('@/core/utils/apiError', () => ({
  extractApiErrorMessage: (_error: unknown, fallback: string) => fallback,
}))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  }
})

const runtime = {
  enabled: true,
  ready: true,
  supported: true,
  platform: 'linux',
  freebind: true,
  secret_configured: true,
  fail_closed: true,
  reconcile_interval_seconds: 60,
  probe_configured: true,
  control_enabled: true,
}

const heControl = {
  available: true,
  config: {
    enabled: true,
    server_ipv4: '216.66.80.30',
    local_ipv4: '',
    client_ipv6: '2001:470:1::2/64',
    server_ipv6: '2001:470:1::1',
    pool_cidr: '2001:470:2::/64',
    mtu: 1480,
    route_metric: 2048,
    probe_ipv6: '2606:4700:4700::1111',
    probe_timeout_seconds: 5,
    allow_private_ipv4: true,
    update_enabled: true,
    tunnel_id: '12345',
    username: 'operator',
    update_key_configured: true,
  },
  agent: {
    online: true,
    state: 'idle',
  },
}

const pool = {
  id: 2,
  name: 'primary-v6',
  cidr: '2001:db8:10::/64',
  node_id: 'node-a',
  status: 'active',
  is_default: true,
  allocation_version: 1,
  allocated_count: 1,
  capacity: '18446744073709551615',
  route_healthy: true,
  last_probe_at: '2026-08-17T00:00:00Z',
  created_at: '2026-08-17T00:00:00Z',
  updated_at: '2026-08-17T00:00:00Z',
}

const account = {
  id: 17,
  name: 'codex-a',
  platform: 'openai',
  type: 'oauth',
  proxy_id: null,
  egress_mode: 'ipv6_pool',
  egress_binding: {
    id: 4,
    account_id: 17,
    pool_id: 2,
    pool_name: 'primary-v6',
    pool_status: 'active',
    source_ipv6: '2001:db8:10::17',
    status: 'active',
    version: 3,
    created_at: '2026-08-17T00:00:00Z',
    updated_at: '2026-08-17T00:00:00Z',
  },
}

function mountPage() {
  return mount(EgressPage, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        Icon: { template: '<span />' },
        Select: { props: ['modelValue', 'options'], template: '<div class="select-stub" />' },
        Toggle: { props: ['modelValue', 'disabled'], template: '<button type="button" class="toggle-stub" :disabled="disabled" />' },
        Pagination: { template: '<div />' },
        BaseDialog: {
          props: ['show', 'title'],
          template: '<div v-if="show" class="dialog-stub"><slot /><slot name="footer" /></div>',
        },
        ConfirmDialog: { template: '<div />' },
        TotpStepUpDialog: { template: '<div />' },
      },
    },
  })
}

describe('EgressPage', () => {
  beforeEach(() => {
    Object.values(egressAPI).forEach((mock) => mock.mockReset())
    accountsAPI.list.mockReset()
    appStore.showSuccess.mockReset()
    appStore.showError.mockReset()
    egressAPI.getRuntime.mockResolvedValue(runtime)
    egressAPI.listPools.mockResolvedValue([pool])
    egressAPI.discoverPrefixes.mockResolvedValue({
      items: [{ prefix: '2001:db8:20::/64', interface: 'eth0', address: '2001:db8:20::2', global: true, tunnel: false, usable: true }],
      suggested_pool_cidr: '2001:db8:20::/64',
    })
    egressAPI.probeAccount.mockResolvedValue({
      source_ipv6: '2001:db8:10::17',
      observed_ip: '2001:db8:10::17',
      latency_ms: 12,
      probe_target: 'echo.test',
    })
    egressAPI.setAccountRoute.mockResolvedValue({ mode: 'inherit', binding: null })
    egressAPI.getHETunnel.mockResolvedValue(heControl)
    egressAPI.saveHETunnel.mockResolvedValue(heControl)
    egressAPI.runHETunnelAction.mockResolvedValue({
      ...heControl,
      agent: { online: true, state: 'pending', action: 'apply', request_id: '1'.repeat(32) },
    })
    accountsAPI.list.mockResolvedValue({ items: [account], total: 1, page: 1, page_size: 25, pages: 1 })
  })

  it('renders runtime readiness and the configured pool', async () => {
    const wrapper = mountPage()
    await flushPromises()

    expect(egressAPI.getRuntime).toHaveBeenCalledTimes(1)
    expect(egressAPI.listPools).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('admin.egress.runtime.ready')
    expect(wrapper.text()).toContain('primary-v6')
    expect(wrapper.text()).toContain('2001:db8:10::/64')
    expect(wrapper.text()).toContain('18446744073709551615')
    expect(wrapper.text()).toContain('admin.egress.health.healthy')
  })

  it('loads account bindings on demand and probes through the selected account route', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const accountsTab = wrapper.findAll('button').find((button) => button.text() === 'admin.egress.tabs.accounts')
    expect(accountsTab).toBeTruthy()
    await accountsTab!.trigger('click')
    await flushPromises()

    expect(accountsAPI.list).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('codex-a')
    expect(wrapper.text()).toContain('2001:db8:10::17')

    const poolLoadsBeforeProbe = egressAPI.listPools.mock.calls.length
    const probeButton = wrapper.find('button[title="admin.egress.actions.probe"]')
    expect(probeButton.exists()).toBe(true)
    await probeButton.trigger('click')
    await flushPromises()

    expect(egressAPI.probeAccount).toHaveBeenCalledWith(17)
    expect(egressAPI.listPools).toHaveBeenCalledTimes(poolLoadsBeforeProbe + 1)
    expect(appStore.showSuccess).toHaveBeenCalledWith(expect.stringContaining('2001:db8:10::17'))
  })

  it('keeps an unprobed pool from being selected as the system default', async () => {
    egressAPI.listPools.mockResolvedValue([{ ...pool, is_default: false, route_healthy: undefined }])
    const wrapper = mountPage()
    await flushPromises()

    const editButton = wrapper.find('button[title="common.edit"]')
    expect(editButton.exists()).toBe(true)
    await editButton.trigger('click')

    const toggle = wrapper.find('.toggle-stub')
    expect(toggle.exists()).toBe(true)
    expect(toggle.attributes('disabled')).toBeDefined()
  })

  it('prefills a pool from the locally discovered routed prefix', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const detectButton = wrapper.findAll('button').find((button) => button.text() === 'admin.egress.actions.detectPrefix')
    expect(detectButton).toBeTruthy()
    await detectButton!.trigger('click')
    await flushPromises()

    expect(egressAPI.discoverPrefixes).toHaveBeenCalledTimes(1)
    expect((wrapper.get('#egress-pool-cidr').element as HTMLInputElement).value).toBe('2001:db8:20::/64')
    expect((wrapper.get('#egress-pool-name').element as HTMLInputElement).value).toContain('2001:db8:20::/64')
  })

  it('loads, saves, and queues the HE tunnel from its dedicated tab', async () => {
    const wrapper = mountPage()
    await flushPromises()

    const heTab = wrapper.findAll('button').find((button) => button.text() === 'admin.egress.tabs.he')
    expect(heTab).toBeTruthy()
    await heTab!.trigger('click')
    await flushPromises()

    expect(egressAPI.getHETunnel).toHaveBeenCalledTimes(1)
    expect((wrapper.get('#he-server-ipv4').element as HTMLInputElement).value).toBe('216.66.80.30')
    expect((wrapper.get('#he-pool-cidr').element as HTMLInputElement).value).toBe('2001:470:2::/64')
    expect(wrapper.text()).toContain('admin.egress.he.states.idle')

    const applyButton = wrapper.findAll('button').find((button) => button.text().includes('admin.egress.he.actions.apply'))
    expect(applyButton).toBeTruthy()
    await applyButton!.trigger('click')
    await flushPromises()

    expect(egressAPI.saveHETunnel).toHaveBeenCalledWith(expect.objectContaining({
      enabled: true,
      server_ipv4: '216.66.80.30',
      pool_cidr: '2001:470:2::/64',
    }))
    expect(egressAPI.runHETunnelAction).toHaveBeenCalledWith('apply')
  })
})
