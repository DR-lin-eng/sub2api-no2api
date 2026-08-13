import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const {
  createAccountMock,
  probeUpstreamBillingMock,
  importCodexSessionMock,
  createOpenAICodexPATMock,
} = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  probeUpstreamBillingMock: vi.fn(),
  importCodexSessionMock: vi.fn(),
  createOpenAICodexPATMock: vi.fn(),
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
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/features/auth/presentation/stores/authStore', () => ({
  useAuthStore: () => ({ isSimpleMode: true }),
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountActions', () => ({
  checkMixedChannelRisk: vi.fn().mockResolvedValue({ has_risk: false }),
  createAccount: createAccountMock,
  createOpenAICodexPAT: createOpenAICodexPATMock,
  importCodexSession: importCodexSessionMock,
  probeUpstreamBilling: probeUpstreamBillingMock,
}))

vi.mock('@/features/admin-settings/data/datasources/adminSettingsQueries', () => ({
  getSettings: vi.fn().mockResolvedValue({}),
  getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
}))

vi.mock('@/features/admin-settings/data/datasources/tlsFingerprintProfileDatasource', () => ({
  list: vi.fn().mockResolvedValue([]),
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountQueries', () => ({
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([]),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

import CreateAccountModal from '@/features/admin-accounts/presentation/widgets/CreateAccountDialog.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  props: {
    showManualOption: Boolean,
    showCodexSessionImportOption: Boolean,
    showAgentIdentityOption: Boolean,
    showCodexPatOption: Boolean,
    initialInputMethod: String,
  },
  data: () => ({ inputMethod: 'manual' }),
  emits: ['import-codex-session', 'import-codex-pat'],
  template: `
    <div>
      <button data-testid="import-codex-session" @click="$emit('import-codex-session', 'session-json')">session</button>
      <button data-testid="import-codex-pat" @click="$emit('import-codex-pat', 'pat-token')">pat</button>
    </div>
  `,
})

function mountModal() {
  return mount(CreateAccountModal, {
    props: { show: true, proxies: [], groups: [] },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub,
        ConfirmDialog: true,
        Select: {
          props: ['modelValue', 'options'],
          emits: ['update:modelValue'],
          template: `
            <select
              v-bind="$attrs"
              :value="modelValue"
              @change="$emit('update:modelValue', $event.target.value)"
            >
              <option v-for="option in options" :key="option.value" :value="option.value">
                {{ option.label }}
              </option>
            </select>
          `,
        },
        Icon: true,
        PlatformIcon: true,
        ProxySelector: true,
        ProxyAdBanner: true,
        GroupSelector: true,
        ModelWhitelistSelector: true,
        QuotaLimitCard: true,
      },
    },
  })
}

async function selectButtonByText(wrapper: ReturnType<typeof mountModal>, text: string) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text().includes(text))
  expect(button).toBeDefined()
  await button?.trigger('click')
}

async function submitApiKeyAccount(
  platform: 'openai' | 'anthropic',
  enableLongContextBilling = false,
  disableUpstreamBillingProbe = false,
  autoDisableOnInsufficientBalance = false
) {
  const wrapper = mountModal()
  await selectButtonByText(wrapper, platform === 'openai' ? 'OpenAI' : 'admin.accounts.claudeConsole')
  if (platform === 'openai') {
    await selectButtonByText(wrapper, 'API Key')
  }
  await wrapper.get('form#create-account-form input[type="text"]').setValue(`${platform} account`)
  await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')
  if (enableLongContextBilling) {
    await wrapper.get('[data-testid="openai-long-context-billing-toggle"]').trigger('click')
  }
  if (disableUpstreamBillingProbe) {
    await wrapper.get('[data-testid="upstream-billing-auto-probe"]').trigger('click')
  }
  if (autoDisableOnInsufficientBalance) {
    await wrapper.get('[data-testid="auto-disable-on-insufficient-balance"]').trigger('click')
  }
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  await flushPromises()
  return wrapper
}

async function openCodexImportStep(toggleClicks = 0) {
  const wrapper = mountModal()
  await selectButtonByText(wrapper, 'OpenAI')
  for (let click = 0; click < toggleClicks; click += 1) {
    await wrapper.get('[data-testid="openai-long-context-billing-toggle"]').trigger('click')
  }
  await wrapper.get('form#create-account-form input[type="text"]').setValue('Codex import')
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  return wrapper
}

describe('CreateAccountModal OpenAI long-context billing', () => {
  beforeEach(() => {
    createAccountMock.mockReset().mockResolvedValue({ id: 42, platform: 'openai', type: 'apikey' })
    probeUpstreamBillingMock.mockReset().mockResolvedValue({})
    importCodexSessionMock.mockReset().mockResolvedValue({
      created: 1,
      updated: 0,
      skipped: 0,
      failed: 0,
      errors: [],
      warnings: [],
    })
    createOpenAICodexPATMock.mockReset().mockResolvedValue({})
  })

  it('sends false explicitly for normal OpenAI account creation by default', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })

    it('shows namespace flattening only for OpenAI OAuth accounts', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    expect(wrapper.find('[data-testid="create-openai-flatten-namespaces-toggle"]').exists()).toBe(true)

    await selectButtonByText(wrapper, 'API Key')
      expect(wrapper.find('[data-testid="create-openai-flatten-namespaces-toggle"]').exists()).toBe(false)
    })

    it('stores Codex prewarm continuation for OpenAI OAuth imports only', async () => {
      const wrapper = mountModal()
      await selectButtonByText(wrapper, 'OpenAI')
      const toggle = wrapper.get('[data-testid="create-codex-prewarm-continuation"]')
      expect(toggle.attributes('aria-checked')).toBe('false')
      await toggle.trigger('click')
      await wrapper.get('form#create-account-form input[type="text"]').setValue('Codex prewarm')
      await wrapper.get('form#create-account-form').trigger('submit.prevent')
      await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
      await flushPromises()

      expect(importCodexSessionMock).toHaveBeenCalledTimes(1)
      expect(
        importCodexSessionMock.mock.calls[0]?.[0]?.extra?.codex_prewarm_continuation_enabled
      ).toBe(true)

      const apiKeyWrapper = mountModal()
      await selectButtonByText(apiKeyWrapper, 'OpenAI')
      await selectButtonByText(apiKeyWrapper, 'API Key')
      expect(
        apiKeyWrapper.find('[data-testid="create-codex-prewarm-continuation"]').exists()
      ).toBe(false)
    })

    it('keeps Codex fingerprint convergence off by default and stores explicit session mode', async () => {
      const wrapper = mountModal()
      await selectButtonByText(wrapper, 'OpenAI')
      const select = wrapper.get('[data-testid="create-codex-fingerprint-mode-select"]')
      expect((select.element as HTMLSelectElement).value).toBe('off')

      await select.setValue('session')
      await wrapper.get('form#create-account-form input[type="text"]').setValue('Codex fingerprint')
      await wrapper.get('form#create-account-form').trigger('submit.prevent')
      await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
      await flushPromises()

      expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.codex_fingerprint_mode).toBe('session')

      const apiKeyWrapper = mountModal()
      await selectButtonByText(apiKeyWrapper, 'OpenAI')
      await selectButtonByText(apiKeyWrapper, 'API Key')
      expect(apiKeyWrapper.find('[data-testid="create-codex-fingerprint-mode-select"]').exists()).toBe(false)
    })

  it('enables upstream billing probes by default for new OpenAI API key accounts', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock.mock.calls[0]?.[0]?.upstream_billing_probe_enabled).toBe(true)
  })

  it('waits for the initial upstream billing probe before refreshing the account list', async () => {
    let resolveProbe: (() => void) | undefined
    probeUpstreamBillingMock.mockImplementationOnce(
      () => new Promise<void>((resolve) => {
        resolveProbe = resolve
      })
    )

    const wrapper = await submitApiKeyAccount('openai')

    expect(probeUpstreamBillingMock).toHaveBeenCalledWith(42)
    expect(wrapper.emitted('created')).toBeUndefined()

    resolveProbe?.()
    await flushPromises()

    expect(wrapper.emitted('created')).toHaveLength(1)
  })

  it('sends an explicit disabled state when the create toggle is turned off', async () => {
    await submitApiKeyAccount('openai', false, true)

    expect(createAccountMock.mock.calls[0]?.[0]?.upstream_billing_probe_enabled).toBe(false)
    expect(probeUpstreamBillingMock).not.toHaveBeenCalled()
  })

  it('keeps insufficient-balance auto-disable off by default', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.auto_disable_on_upstream_insufficient_balance).toBeUndefined()
  })

  it('stores the account-level insufficient-balance auto-disable switch', async () => {
    await submitApiKeyAccount('openai', false, false, true)

    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.auto_disable_on_upstream_insufficient_balance).toBe(true)
  })

  it('exposes Agent Identity in the OpenAI authorization methods', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('OpenAI account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    const flow = wrapper.getComponent(OAuthAuthorizationFlowStub)
    expect(flow.props('showManualOption')).toBe(true)
    expect(flow.props('showCodexSessionImportOption')).toBe(true)
    expect(flow.props('showAgentIdentityOption')).toBe(true)
    expect(flow.props('showCodexPatOption')).toBe(true)
    expect(flow.props('initialInputMethod')).toBe('manual')
  })

  it.each([
    ['camelCase', { authMode: 'agentIdentity', agentIdentity: { agentRuntimeId: 'runtime' } }],
    ['nested identity without auth_mode', { agent_identity: { agent_runtime_id: 'runtime' } }],
  ])('accepts backend-compatible %s Agent Identity imports', async (_name, content) => {
    const wrapper = await openCodexImportStep()
    const flow = wrapper.getComponent(OAuthAuthorizationFlowStub)
    flow.vm.inputMethod = 'agent_identity'

    flow.vm.$emit('import-codex-session', JSON.stringify(content))
    await flushPromises()

    expect(importCodexSessionMock).toHaveBeenCalledTimes(1)
  })

  it('sends true explicitly when OpenAI long-context billing is enabled', async () => {
    await submitApiKeyAccount('openai', true)

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('stores the forced Images API switch for OpenAI API key accounts', async () => {
    const wrapper = mountModal()
    await selectButtonByText(wrapper, 'OpenAI')
    await selectButtonByText(wrapper, 'API Key')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('OpenAI image account')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('test-api-key')

    await wrapper.get('[data-testid="openai-force-image-api"]').trigger('click')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_force_image_api).toBe(true)
  })

  it('keeps OpenAI-only settings out of Anthropic accounts while enabling supported probes', async () => {
    await submitApiKeyAccount('anthropic')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
    expect(createAccountMock.mock.calls[0]?.[0]?.upstream_billing_probe_enabled).toBe(true)
  })

  it('leaves Codex session import billing ownership to the backend', async () => {
    const wrapper = await openCodexImportStep()
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock).toHaveBeenCalledTimes(1)
    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
  })

  it('leaves Codex PAT import billing ownership to the backend', async () => {
    const wrapper = await openCodexImportStep()
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock).toHaveBeenCalledTimes(1)
    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBeUndefined()
  })

  it('sends explicit true for Codex session import after the toggle is enabled', async () => {
    const wrapper = await openCodexImportStep(1)
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('sends explicit false for Codex session import after the toggle is changed back', async () => {
    const wrapper = await openCodexImportStep(2)
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()

    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })

  it('sends explicit true for Codex PAT import after the toggle is enabled', async () => {
    const wrapper = await openCodexImportStep(1)
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(true)
  })

  it('sends explicit false for Codex PAT import after the toggle is changed back', async () => {
    const wrapper = await openCodexImportStep(2)
    await wrapper.get('[data-testid="import-codex-pat"]').trigger('click')
    await flushPromises()

    expect(createOpenAICodexPATMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })
})
