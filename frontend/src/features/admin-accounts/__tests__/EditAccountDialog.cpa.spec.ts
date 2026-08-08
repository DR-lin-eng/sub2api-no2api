import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'

const { updateAccountMock, testCPAConnectionMock, checkMixedChannelRiskMock, showErrorMock, showSuccessMock } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  testCPAConnectionMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {},
    settings: {},
    tlsFingerprintProfiles: {},
  },
}))

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: showSuccessMock,
    showInfo: vi.fn()
  })
}))

vi.mock('@/features/auth/presentation/stores/authStore', () => ({
  useAuthStore: () => ({ isSimpleMode: true })
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountActions', () => ({
  checkMixedChannelRisk: checkMixedChannelRiskMock,
  syncUpstreamModels: vi.fn().mockResolvedValue({ models: [] }),
  testCPAConnection: testCPAConnectionMock,
  updateAccount: updateAccountMock
}))

vi.mock('@/features/admin-settings/data/datasources/adminSettingsDatasource', () => ({
  getSettings: vi.fn().mockResolvedValue({}),
  getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] })
}))

vi.mock('@/features/admin-settings/data/datasources/tlsFingerprintProfileDatasource', () => ({
  list: vi.fn().mockResolvedValue([])
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountQueries', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

import EditAccountModal from '@/features/admin-accounts/presentation/widgets/EditAccountDialog.vue'
import zhAccounts from '@/core/i18n/locales/zh/admin/accounts'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

function buildAccount(overrides: Record<string, unknown> = {}) {
  return {
    id: 9,
    name: 'CPA pool',
    notes: '',
    platform: 'openai',
    type: 'apikey',
    credentials: {
      base_url: 'http://cpa:8317/v1',
      cpa_mode: true,
      cpa_management_url: 'http://cpa:8317',
      cpa_concurrency_per_credential: 2,
      cpa_exclude_abnormal_credentials: true
    },
    credentials_status: { has_api_key: true, has_cpa_management_key: true },
    extra: {},
    proxy_id: null,
    concurrency: 100,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false,
    ...overrides
  } as any
}

function mountModal(account = buildAccount()) {
  return mount(EditAccountModal, {
    props: { show: true, account, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: true,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: true
      }
    }
  })
}

describe('EditAccountModal CPA concurrency sync', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    updateAccountMock.mockResolvedValue(buildAccount())
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    testCPAConnectionMock.mockReset()
    testCPAConnectionMock.mockResolvedValue({
      total_credentials: 4,
      enabled_credentials: 3,
      abnormal_credentials: 1,
      available_credentials: 2,
      capacity_credentials: 3,
      effective_concurrency: 30,
      concurrency_per_credential: 10,
      exclude_abnormal_credentials: false,
      state: 'fresh',
      latency_ms: 12
    })
  })

  it('labels the CPA secret as the administrator password', () => {
    expect(zhAccounts.accounts.cpaManagementKey).toBe('CPA 管理员密码')
  })

  it('loads CPA settings and keeps the redacted management key on save', async () => {
    const wrapper = mountModal()
    expect(wrapper.get('[data-testid="cpa-mode-toggle"]').attributes('aria-checked')).toBe('true')
    expect((wrapper.get('[data-testid="cpa-management-url"]').element as HTMLInputElement).value).toBe('http://cpa:8317')
    expect((wrapper.get('[data-testid="cpa-concurrency-per-credential"]').element as HTMLInputElement).value).toBe('2')
    expect(wrapper.get('[data-testid="cpa-exclude-abnormal-toggle"]').attributes('aria-checked')).toBe('true')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))

    const payload = updateAccountMock.mock.calls[0]?.[1]
    expect(payload.credentials).toMatchObject({
      cpa_mode: true,
      cpa_management_url: 'http://cpa:8317',
      cpa_concurrency_per_credential: 2,
      cpa_exclude_abnormal_credentials: true
    })
    expect(payload.credentials).not.toHaveProperty('cpa_management_key')
  })

  it('allows rotating the management key and clears CPA settings when disabled', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="cpa-management-key"]').setValue('rotated-key')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.cpa_management_key).toBe('rotated-key')

    updateAccountMock.mockClear()
    await wrapper.get('[data-testid="cpa-mode-toggle"]').trigger('click')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await vi.waitFor(() => expect(updateAccountMock).toHaveBeenCalledTimes(1))
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials).toMatchObject({
      cpa_mode: false,
      cpa_management_key: ''
    })
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials).not.toHaveProperty('cpa_exclude_abnormal_credentials')
  })

  it('defaults to the account Base URL and 10 concurrency per credential', async () => {
    const account = buildAccount({
      credentials: {
        base_url: 'http://cpa:8317/v1',
        cpa_mode: true
      }
    })
    const wrapper = mountModal(account)

    expect(wrapper.find('[data-testid="cpa-management-url"]').exists()).toBe(false)
    expect((wrapper.get('[data-testid="cpa-concurrency-per-credential"]').element as HTMLInputElement).value).toBe('10')
    expect(wrapper.get('[data-testid="cpa-exclude-abnormal-toggle"]').attributes('aria-checked')).toBe('false')

    await wrapper.get('[data-testid="cpa-test-connection"]').trigger('click')
    await vi.waitFor(() => expect(testCPAConnectionMock).toHaveBeenCalledTimes(1))
    expect(testCPAConnectionMock).toHaveBeenCalledWith(9, {
      use_account_base_url: true,
      base_url: 'http://cpa:8317/v1',
      management_url: undefined,
      management_password: undefined,
      concurrency_per_credential: 10,
      exclude_abnormal_credentials: false
    })
    expect(showSuccessMock).toHaveBeenCalledWith('admin.accounts.cpaTestSuccess')
  })
})
