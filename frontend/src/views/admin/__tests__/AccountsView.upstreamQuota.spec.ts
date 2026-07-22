import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountsView from '../AccountsView.vue'
import type { UpstreamQuotaQueryResult } from '@/types'

const {
  authUser,
  listAccounts,
  listWithEtag,
  getUpstreamBillingRatesWithEtag,
  getBatchTodayStats,
  getUpstreamBillingProbeSettings,
  getAllProxies,
  getAllGroups,
  getAccountById,
  queryUpstreamQuota,
  showToast,
  hideToast,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  authUser: { id: 99 },
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getUpstreamBillingRatesWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  getAccountById: vi.fn(),
  queryUpstreamQuota: vi.fn(),
  showToast: vi.fn(),
  hideToast: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getUpstreamBillingRatesWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      getById: getAccountById,
      queryUpstreamQuota,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: { getAll: getAllProxies },
    groups: { getAll: getAllGroups }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showToast, hideToast, showError, showSuccess })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: authUser,
    token: 'test-token',
    isSimpleMode: false
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

const DataTableStub = {
  props: ['data', 'loading'],
  template: `
    <div data-test="accounts-table" :data-loading="String(loading)">
      <div v-for="row in data" :key="row.id" data-test="virtual-row" :data-account-id="row.id">
        <slot name="cell-upstream_billing_rate" :row="row" />
        <slot name="cell-usage" :row="row" />
      </div>
    </div>
  `
}

const UpstreamBillingRateCellStub = {
  props: ['account', 'quotaResult', 'quotaError', 'quotaLoading', 'quotaFeedback'],
  emits: ['query-quota'],
  template: `
    <div data-test="quota-cell" :data-account-id="account.id" :data-loading="String(quotaLoading)">
      <span data-test="quota-result">{{ quotaResult?.quota?.remaining ?? '' }}</span>
      <span data-test="quota-error">{{ quotaError ?? '' }}</span>
      <span data-test="quota-feedback">{{ quotaFeedback ?? '' }}</span>
      <button data-test="query-quota" @click="$emit('query-quota')">query</button>
    </div>
  `
}

const AccountUsageCellStub = {
  props: ['account', 'upstreamQuotaResult'],
  template: `
    <div data-test="usage-cell" :data-account-id="account.id">
      {{ upstreamQuotaResult?.quota?.subscription?.windows?.length ?? 0 }}
    </div>
  `
}

const AccountBulkActionsBarStub = {
  props: ['selectedIds', 'queryingUpstreamQuota'],
  emits: ['query-upstream-quota'],
  template: `
    <button
      v-if="selectedIds.length"
      data-test="bulk-query-quota"
      :data-loading="String(queryingUpstreamQuota)"
      @click="$emit('query-upstream-quota')"
    >query balances</button>
  `
}

const EditAccountModalStub = {
  emits: ['updated'],
  template: `
    <button
      data-test="account-updated"
      @click="$emit('updated', {
        id: 7,
        name: 'updated',
        platform: 'openai',
        type: 'apikey',
        status: 'active',
        schedulable: true
      })"
    >updated</button>
  `
}

const account = (id: number) => ({
  id,
  name: `account-${id}`,
  platform: 'openai',
  type: 'apikey',
  status: 'active',
  schedulable: true,
  credentials: { base_url: 'https://upstream.example' },
  credentials_status: { has_api_key: true },
  created_at: '2026-07-17T00:00:00Z',
  updated_at: '2026-07-17T00:00:00Z'
})

const quotaResultFor = (accountID: number, remaining: number): UpstreamQuotaQueryResult => ({
  account_id: accountID,
  observed_at: '2026-07-17T00:00:00Z',
  quota: { provider: 'sub2api', mode: 'balance', unit: 'USD', remaining }
})

const cacheKey = (accountID: number) => `sub2api:admin:upstream-quota:v2:99:${accountID}`

const mountView = () => mount(AccountsView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: { template: '<div><slot name="table" /></div>' },
      DataTable: DataTableStub,
      UpstreamBillingRateCell: UpstreamBillingRateCellStub,
      AccountUsageCell: AccountUsageCellStub,
      AccountBulkActionsBar: AccountBulkActionsBarStub,
      EditAccountModal: EditAccountModalStub,
      Pagination: true,
      ConfirmDialog: true,
      AccountTableActions: true,
      AccountTableFilters: true,
      AccountActionMenu: true,
      ImportDataModal: true,
      ReAuthAccountModal: true,
      AccountTestModal: true,
      AccountStatsModal: true,
      ScheduledTestsPanel: true,
      SyncFromCrsModal: true,
      TempUnschedStatusModal: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      CreateAccountModal: true,
      BulkEditAccountModal: true,
      PlatformTypeBadge: true,
      AccountCapacityCell: true,
      AccountStatusIndicator: true,
      AccountTodayStatsCell: true,
      AccountHourlyUsageCell: true,
      AccountGroupsCell: true,
      HelpTooltip: true,
      Icon: true
    }
  }
})

describe('admin AccountsView upstream quota state', () => {
  beforeEach(() => {
    localStorage.clear()
    authUser.id = 99
    for (const mock of [
      listAccounts,
      listWithEtag,
      getUpstreamBillingRatesWithEtag,
      getBatchTodayStats,
      getUpstreamBillingProbeSettings,
      getAllProxies,
      getAllGroups,
      getAccountById,
      queryUpstreamQuota,
      showToast,
      hideToast,
      showError,
      showSuccess
    ]) mock.mockReset()

    listAccounts.mockResolvedValue({
      items: [account(7), account(11)],
      total: 2,
      page: 1,
      page_size: 20,
      pages: 1
    })
    listWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getUpstreamBillingRatesWithEtag.mockResolvedValue({ notModified: true, etag: null, data: null })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getUpstreamBillingProbeSettings.mockResolvedValue({ enabled: true, interval_minutes: 30 })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    getAccountById.mockImplementation((id: number) => Promise.resolve(account(id)))
  })

  afterEach(() => vi.useRealTimers())

  it('runs selected quota queries in sequential groups of four with progress', async () => {
    const selected = Array.from({ length: 5 }, (_, index) => account(index + 1))
    listAccounts.mockResolvedValueOnce({
      items: selected.slice(0, 4),
      total: selected.length,
      page: 1,
      page_size: 20,
      pages: 1
    })
    const resolvers = new Map<number, (result: UpstreamQuotaQueryResult) => void>()
    queryUpstreamQuota.mockImplementation((id: number) => id === 5
      ? Promise.resolve(quotaResultFor(id, 50))
      : new Promise(resolve => { resolvers.set(id, resolve) }))
    showToast.mockReturnValueOnce('batch-start').mockReturnValueOnce('batch-progress')

    const wrapper = mountView()
    await flushPromises()
    const setupState = wrapper.vm.$.setupState as unknown as { setSelectedIds: (ids: number[]) => void }
    setupState.setSelectedIds(selected.map(({ id }) => id))
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-test="bulk-query-quota"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(queryUpstreamQuota.mock.calls.map(([id]) => id)).toEqual([1, 2, 3, 4])
    expect(showToast).toHaveBeenCalledWith('info', 'admin.accounts.upstreamBilling.quotaBatchStarted')

    for (const [id, resolve] of resolvers) resolve(quotaResultFor(id, 100 - id))
    await flushPromises()

    expect(queryUpstreamQuota).toHaveBeenLastCalledWith(5)
    expect(getAccountById).toHaveBeenCalledWith(5)
    expect(showToast).toHaveBeenLastCalledWith('info', 'admin.accounts.upstreamBilling.quotaBatchProgress')
    expect(hideToast).toHaveBeenLastCalledWith('batch-progress')
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.upstreamBilling.quotaBatchCompleted')
    wrapper.unmount()
  })

  it('hydrates valid cache and rejects a result invalidated by an account edit', async () => {
    queryUpstreamQuota.mockResolvedValueOnce(quotaResultFor(7, 80))
    const first = mountView()
    await flushPromises()
    await first.get('[data-test="quota-cell"][data-account-id="7"] [data-test="query-quota"]').trigger('click')
    await flushPromises()

    expect(first.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('80')
    expect(localStorage.getItem(cacheKey(7))).not.toBeNull()
    first.unmount()

    const restored = mountView()
    await flushPromises()
    expect(restored.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('80')
    expect(queryUpstreamQuota).toHaveBeenCalledOnce()

    localStorage.removeItem(cacheKey(7))
    let resolveStale!: (result: UpstreamQuotaQueryResult) => void
    queryUpstreamQuota.mockReturnValueOnce(new Promise(resolve => { resolveStale = resolve }))
    await restored.get('[data-test="quota-cell"][data-account-id="7"] [data-test="query-quota"]').trigger('click')
    await restored.get('[data-test="account-updated"]').trigger('click')
    resolveStale(quotaResultFor(7, 75))
    await flushPromises()

    expect(restored.get('[data-test="quota-cell"][data-account-id="7"] [data-test="quota-result"]').text()).toBe('')
    expect(localStorage.getItem(cacheKey(7))).toBeNull()
    restored.unmount()
  })
})
