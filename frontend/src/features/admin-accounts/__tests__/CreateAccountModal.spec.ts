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

import CreateAccountModal from '@/features/admin-accounts/presentation/widgets/CreateAccountWizard.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show" role="dialog"><slot name="after-title" /><slot /><slot name="footer" /></div>',
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
  if (text === 'OAuth' || text === 'API Key') {
    await wrapper.get(`[data-tour="account-form-type"] input[value="${text === 'OAuth' ? 'oauth-based' : 'apikey'}"]`).setValue(true)
    return
  }
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
  await wrapper.get(`[data-testid="account-platform-${platform}"]`).trigger('click')
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
  await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
  await selectButtonByText(wrapper, 'OAuth')
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

  it('selects Gemini first inside the same open dialog and exposes its own URL and key', async () => {
    const wrapper = mountModal()
    const dialog = wrapper.get('[role="dialog"]').element
    await wrapper.get('[data-testid="account-platform-gemini"]').trigger('click')
    expect(wrapper.get('[role="dialog"]').element).toBe(dialog)
    expect(wrapper.get<HTMLInputElement>('#create-account-base-url').element.value).toBe('https://generativelanguage.googleapis.com')
    expect(wrapper.get('#create-account-api-key').isVisible()).toBe(true)
    await wrapper.get('input[name="create-account-auth-type"][value="service_account"]').setValue(true)
    expect(wrapper.text()).toContain('Service Account JSON')
    expect(wrapper.find('#create-account-api-key').exists()).toBe(false)
  })

  it('keeps the chosen OAuth TLS policy in the session import request', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
    await selectButtonByText(wrapper, 'OAuth')
    await wrapper.get('[data-tour="account-tab-openai"]').trigger('click')
    const title = wrapper.get('[data-account-tab="openai"]').findAll('label')
      .find(label => label.text() === 'admin.accounts.quotaControl.tlsFingerprint.label')
    const toggle = title?.element.parentElement?.parentElement?.querySelector('button')
    expect(toggle).toBeInstanceOf(HTMLButtonElement)
    toggle!.click()
    await wrapper.get('#create-account-name').setValue('TLS import')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
    await flushPromises()
    expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra).toMatchObject({ enable_tls_fingerprint: true })
  })

  it('submits OpenAI account policies selected in the provider tab', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
    await wrapper.get('#create-account-name').setValue('policy account')
    await wrapper.get('#create-account-api-key').setValue('test-api-key')
    await wrapper.get('[data-tour="account-tab-openai"]').trigger('click')
    await wrapper.get('[data-testid="codex-image-tool-block"]').trigger('click')
    await wrapper.get('[data-testid="auto-pause-5h-threshold"]').setValue('75')
    await wrapper.get('[data-testid="auto-pause-7d-disabled"]').trigger('click')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    expect(createAccountMock.mock.calls[0]?.[0]).toMatchObject({
      platform: 'openai', type: 'apikey', extra: {
        codex_image_generation_explicit_tool_policy: 'strip', auto_pause_5h_threshold: 0.75, auto_pause_7d_disabled: true,
      },
    })
  })

  it('exposes one dialog at a time and never carries an API key into another provider', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
    await wrapper.get('#create-account-api-key').setValue('only-for-openai')
    await selectButtonByText(wrapper, 'admin.accounts.providerSetup.change')
    expect(wrapper.findAll('[role="dialog"]')).toHaveLength(1)
    expect(wrapper.get('#create-account-form').isVisible()).toBe(false)
    await selectButtonByText(wrapper, 'admin.accounts.providerSetup.back')
    expect(wrapper.get<HTMLInputElement>('#create-account-api-key').element.value).toBe('only-for-openai')
    await selectButtonByText(wrapper, 'admin.accounts.providerSetup.change')
    await wrapper.get('[data-testid="account-platform-anthropic"]').trigger('click')
    expect(wrapper.get<HTMLInputElement>('#create-account-api-key').element.value).toBe('')
    expect(wrapper.get<HTMLInputElement>('#create-account-base-url').element.value).toBe('https://api.anthropic.com')
  })

  it('sends false explicitly for normal OpenAI account creation by default', async () => {
    await submitApiKeyAccount('openai')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_long_context_billing_enabled).toBe(false)
  })

    it('shows namespace flattening only for OpenAI OAuth accounts', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
    await selectButtonByText(wrapper, 'OAuth')
    expect(wrapper.find('[data-testid="create-openai-flatten-namespaces-toggle"]').exists()).toBe(true)

    await selectButtonByText(wrapper, 'API Key')
      expect(wrapper.find('[data-testid="create-openai-flatten-namespaces-toggle"]').exists()).toBe(false)
    })

    it('stores Codex prewarm continuation for OpenAI OAuth imports only', async () => {
      const wrapper = mountModal()
      await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
      await selectButtonByText(wrapper, 'OAuth')
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
      await apiKeyWrapper.get('[data-testid="account-platform-openai"]').trigger('click')
      await selectButtonByText(apiKeyWrapper, 'API Key')
      expect(
        apiKeyWrapper.find('[data-testid="create-codex-prewarm-continuation"]').exists()
      ).toBe(false)
    })

    it('keeps Codex fingerprint convergence off by default and stores explicit session mode', async () => {
      const wrapper = mountModal()
      await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
      await selectButtonByText(wrapper, 'OAuth')
      const select = wrapper.get('[data-testid="create-codex-fingerprint-mode-select"]')
      expect((select.element as HTMLSelectElement).value).toBe('off')

      await select.setValue('session')
      await wrapper.get('form#create-account-form input[type="text"]').setValue('Codex fingerprint')
      await wrapper.get('form#create-account-form').trigger('submit.prevent')
      await wrapper.get('[data-testid="import-codex-session"]').trigger('click')
      await flushPromises()

      expect(importCodexSessionMock.mock.calls[0]?.[0]?.extra?.codex_fingerprint_mode).toBe('session')

      const apiKeyWrapper = mountModal()
      await apiKeyWrapper.get('[data-testid="account-platform-openai"]').trigger('click')
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
    await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
    await selectButtonByText(wrapper, 'OAuth')
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
    await wrapper.get('[data-testid="account-platform-openai"]').trigger('click')
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

  it.each([
    ['kimi', 'https://api.moonshot.cn/v1'],
    ['zhipu', 'https://open.bigmodel.cn/api/paas/v4'],
    ['deepseek', 'https://api.deepseek.com'],
    ['minimax', 'https://api.minimaxi.com/v1'],
  ] as const)('creates first-class %s API-key accounts with the provider default', async (platform, baseURL) => {
    const wrapper = mountModal()
    await wrapper.get(`[data-testid="account-platform-${platform}"]`).trigger('click')
    await flushPromises()
    const baseURLInput = wrapper
      .findAll('form#create-account-form input[type="text"]')
      .find((input) => (input.element as HTMLInputElement).value === baseURL)
    expect(baseURLInput?.attributes('placeholder')).toBe(baseURL)
    expect(wrapper.get('form#create-account-form input[type="password"]').attributes('placeholder')).toBe(
      platform === 'kimi' || platform === 'deepseek' ? 'sk-...' : 'API Key'
    )
    expect(wrapper.text()).toContain('admin.accounts.cnProvider.baseUrlHint')
    expect(wrapper.text()).toContain('admin.accounts.cnProvider.apiKeyHint')
    await wrapper.get('form#create-account-form input[type="text"]').setValue(`${platform} account`)
    await wrapper.get('form#create-account-form input[type="password"]').setValue('provider-key')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]).toMatchObject({
      platform,
      type: 'apikey',
      credentials: {
        api_key: 'provider-key',
        base_url: baseURL,
        account_mode: 'payg',
        api_protocol: 'chat_completions',
      },
    })
  })

  it('stores CN coding-plan and adaptive protocol selections', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="account-platform-kimi"]').trigger('click')
    await wrapper.get('[data-testid="cn-account-mode"]').setValue('coding')
    await wrapper.get('[data-testid="cn-api-protocol"]').setValue('adaptive')
    expect(wrapper.findAll('input[type="text"]').some(input =>
      (input.element as HTMLInputElement).value === 'https://api.kimi.com/coding/v1'
    )).toBe(true)

    await wrapper.get('form#create-account-form input[type="text"]').setValue('Kimi coding')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('provider-key')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
      account_mode: 'coding',
      api_protocol: 'adaptive',
      base_url: 'https://api.kimi.com/coding/v1',
      api_base_urls: {
        chat_completions: 'https://api.kimi.com/coding/v1',
        anthropic: 'https://api.kimi.com/coding',
        responses: 'https://api.kimi.com/coding/v1',
      },
    })
  })

  it('preserves custom adaptive endpoints while changing CN account mode', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="account-platform-kimi"]').trigger('click')
    await wrapper.get('[data-testid="cn-api-protocol"]').setValue('adaptive')
    await wrapper.get('[data-testid="cn-adaptive-base-url-anthropic"]').setValue('https://relay.example/anthropic')
    await wrapper.get('[data-testid="cn-account-mode"]').setValue('coding')

    expect((wrapper.get('[data-testid="cn-adaptive-base-url-chat-completions"]').element as HTMLInputElement).value)
      .toBe('https://api.kimi.com/coding/v1')
    expect((wrapper.get('[data-testid="cn-adaptive-base-url-anthropic"]').element as HTMLInputElement).value)
      .toBe('https://relay.example/anthropic')
  })

  it('creates OpenCode Go and Zen accounts with adaptive protocol defaults', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="account-platform-opencode_go"]').trigger('click')
    await flushPromises()
    expect((wrapper.get('[data-testid="cn-adaptive-base-url-chat-completions"]').element as HTMLInputElement).value)
      .toBe('https://opencode.ai/zen/go/v1')

    await wrapper.get('[data-testid="opencode-account-mode"]').setValue('zen')
    expect((wrapper.get('[data-testid="cn-adaptive-base-url-chat-completions"]').element as HTMLInputElement).value)
      .toBe('https://opencode.ai/zen/v1')
    expect((wrapper.get('[data-testid="opencode-protocol-pattern-3"]').element as HTMLInputElement).value)
      .toBe('claude-*')

    await wrapper.get('form#create-account-form input[type="text"]').setValue('OpenCode Zen')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('provider-key')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock.mock.calls[0]?.[0]).toMatchObject({
      platform: 'opencode_go',
      type: 'apikey',
      credentials: {
        account_mode: 'zen',
        api_protocol: 'adaptive',
        base_url: 'https://opencode.ai/zen/v1',
        api_base_urls: {
          chat_completions: 'https://opencode.ai/zen/v1',
          anthropic: 'https://opencode.ai/zen',
          responses: 'https://opencode.ai/zen/v1',
        },
        protocol_rules: expect.arrayContaining([
          { pattern: 'claude-*', protocol: 'anthropic' },
          { pattern: 'gpt-*', protocol: 'responses' },
        ]),
      },
    })
  })

  it('stores Zhipu team Coding Plan identifiers', async () => {
    const wrapper = mountModal()
    await wrapper.get('[data-testid="account-platform-zhipu"]').trigger('click')
    await wrapper.get('[data-testid="cn-account-mode"]').setValue('coding')
    await wrapper.get('[data-testid="zhipu-organization"]').setValue('org-123')
    await wrapper.get('[data-testid="zhipu-project"]').setValue('project-456')
    await wrapper.get('form#create-account-form input[type="text"]').setValue('Zhipu team')
    await wrapper.get('form#create-account-form input[type="password"]').setValue('provider-key')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock.mock.calls[0]?.[0]?.credentials).toMatchObject({
      account_mode: 'coding',
      zhipu_organization: 'org-123',
      zhipu_project: 'project-456',
    })
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
