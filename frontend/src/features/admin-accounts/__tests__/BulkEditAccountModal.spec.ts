import { describe, expect, it, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import BulkEditAccountModal from '@/features/admin-accounts/presentation/widgets/BulkEditAccountDialog.vue'
import ModelWhitelistSelector from '@/features/admin-accounts/presentation/widgets/ModelWhitelistSelector.vue'
import {
  bulkUpdate,
  checkMixedChannelRisk
} from '@/features/admin-accounts/data/datasources/adminAccountActions'

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountActions', () => ({
  bulkUpdate: vi.fn(),
  checkMixedChannelRisk: vi.fn(),
  syncUpstreamModels: vi.fn(),
  syncUpstreamModelsPreview: vi.fn()
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountQueries', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountsDatasource', () => ({
  default: {}
}))

vi.mock('@/features/admin-settings/data/datasources/tlsFingerprintProfileDatasource', () => ({
  list: vi.fn().mockResolvedValue([
    { id: 7, name: 'Codex desktop' }
  ]),
  default: {
    list: vi.fn().mockResolvedValue([])
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function mountModal(extraProps: Record<string, unknown> = {}) {
  return mount(BulkEditAccountModal, {
    props: {
      show: true,
      accountIds: [1, 2],
      selectedPlatforms: ['antigravity'],
      selectedTypes: ['apikey'],
      proxies: [],
      groups: [],
      ...extraProps
    } as any,
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
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
          `
        },
        ProxySelector: true,
        GroupSelector: true,
        Icon: true
      }
    }
  })
}

describe('BulkEditAccountModal', () => {
  beforeEach(() => {
    vi.mocked(bulkUpdate).mockReset()
    vi.mocked(checkMixedChannelRisk).mockReset()

    vi.mocked(bulkUpdate).mockResolvedValue({
      success: 2,
      failed: 0,
      results: []
    } as any)
    vi.mocked(checkMixedChannelRisk).mockResolvedValue({
      has_risk: false
    } as any)
  })

  it('antigravity 白名单包含 Gemini 图片模型且过滤掉普通 GPT 模型', async () => {
    const wrapper = mountModal()
    const selector = wrapper.findComponent(ModelWhitelistSelector)
    expect(selector.exists()).toBe(true)

    await selector.find('div.cursor-pointer').trigger('click')

    expect(wrapper.text()).toContain('gemini-3.1-flash-image')
    expect(wrapper.text()).toContain('gemini-2.5-flash-image')
    expect(wrapper.text()).not.toContain('gpt-5.3-codex')
  })

  it('antigravity 映射预设包含图片映射并过滤 OpenAI 预设', async () => {
    const wrapper = mountModal()

    const mappingTab = wrapper.findAll('button').find((btn) => btn.text().includes('admin.accounts.modelMapping'))
    expect(mappingTab).toBeTruthy()
    await mappingTab!.trigger('click')

    expect(wrapper.text()).toContain('3.1-Flash-Image透传')
    expect(wrapper.text()).toContain('3-Pro-Image→3.1')
    expect(wrapper.text()).not.toContain('GPT-5.3 Codex Spark')
  })

  it('仅勾选模型限制且白名单留空时，应提交空 model_mapping 以支持所有模型', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['anthropic'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-model-restriction-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        model_mapping: {}
      }
    })
  })

  it('全部目标为 Grok OAuth 时，官方主机 base_url 作为手动端点切换正常提交', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-base-url-enabled').setValue(true)
    await wrapper.get('#bulk-edit-base-url').setValue('https://api.x.ai/v1')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        base_url: 'https://api.x.ai/v1'
      }
    })
  })

  it('所选全为 grok 时展示快捷端点，点击后填入并自动勾选 base_url', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok'],
      selectedTypes: ['oauth']
    })

    const presets = wrapper.findAll('[data-testid="grok-base-url-preset"]')
    expect(presets.length).toBe(5)

    // 第三个预设为区域 API (us-east-1.api.x.ai/v1)
    await presets[2].trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        base_url: 'https://us-east-1.api.x.ai/v1'
      }
    })
  })

  it('所选含非 grok 平台时不展示快捷端点', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok', 'anthropic'],
      selectedTypes: ['apikey']
    })

    expect(wrapper.findAll('[data-testid="grok-base-url-preset"]').length).toBe(0)
  })

  it('全部目标为 Grok OAuth 时，第三方 base_url 正常提交', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-base-url-enabled').setValue(true)
    await wrapper.get('#bulk-edit-base-url').setValue('https://relay.example.com/v1')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        base_url: 'https://relay.example.com/v1'
      }
    })
  })

  it('混合类型选择（含 apikey）时官方主机 base_url 不拦截', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['grok'],
      selectedTypes: ['apikey', 'oauth']
    })

    await wrapper.get('#bulk-edit-base-url-enabled').setValue(true)
    await wrapper.get('#bulk-edit-base-url').setValue('https://api.x.ai/v1')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        base_url: 'https://api.x.ai/v1'
      }
    })
  })

  it('OpenAI 账号批量编辑可开启自动透传', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-passthrough-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-passthrough-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_passthrough: true
      }
    })
  })

  it('OpenAI OAuth 批量编辑可开启 TLS 指纹模拟并按账号稳定分配 profile', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-tls-fingerprint-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-tls-fingerprint-toggle"]').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        enable_tls_fingerprint: true,
        tls_fingerprint_profile_id: -1
      }
    })
  })

  it('OpenAI OAuth 批量编辑可统一选择 TLS 指纹 profile', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-tls-fingerprint-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-tls-fingerprint-toggle"]').trigger('click')
    const profile = wrapper.get('[data-testid="bulk-edit-openai-tls-fingerprint-profile"]')
    await profile.trigger('focus')
    await flushPromises()
    await profile.setValue('7')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        enable_tls_fingerprint: true,
        tls_fingerprint_profile_id: 7
      }
    })
  })

  it('TLS 指纹模拟批量入口仅对 OpenAI OAuth 展示', () => {
    const apiKeyWrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })
    expect(apiKeyWrapper.find('#bulk-edit-openai-tls-fingerprint-enabled').exists()).toBe(false)

    const setupTokenWrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['setup-token']
    })
    expect(setupTokenWrapper.find('#bulk-edit-openai-tls-fingerprint-enabled').exists()).toBe(false)
  })

  it('OpenAI OAuth 批量编辑可开启 namespace 摊平兼容开关', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-flatten-namespaces-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-flatten-namespaces-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: { openai_responses_flatten_namespaces: true }
    })
  })

  it('namespace 摊平兼容开关不对混合 OAuth 类型展示', () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth', 'setup-token']
    })

    expect(wrapper.find('#bulk-edit-openai-flatten-namespaces-enabled').exists()).toBe(false)
  })

    it('OpenAI OAuth 批量编辑应提交 OAuth 专属 WS mode 字段（含 http_bridge）', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-ws-mode-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-ws-mode-select"]').setValue('http_bridge')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_oauth_responses_websockets_v2_mode: 'http_bridge',
        openai_oauth_responses_websockets_v2_enabled: true
      }
      })
    })

    it('OpenAI OAuth 批量编辑应提交 Codex 空预热续发开关', async () => {
      const wrapper = mountModal({
        selectedPlatforms: ['openai'],
        selectedTypes: ['oauth']
      })

      await wrapper.get('#bulk-edit-codex-prewarm-continuation-enabled').setValue(true)
      await wrapper.get('[data-testid="bulk-edit-codex-prewarm-continuation-toggle"]').trigger('click')
      await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
      await flushPromises()

      expect(bulkUpdate).toHaveBeenCalledTimes(1)
      expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
        extra: {
          codex_prewarm_continuation_enabled: true
        }
      })
    })

    it('OpenAI OAuth 批量编辑应显式提交 off 以关闭已有指纹收敛', async () => {
      const wrapper = mountModal({
        selectedPlatforms: ['openai'],
        selectedTypes: ['oauth']
      })

      await wrapper.get('#bulk-edit-codex-fingerprint-mode-enabled').setValue(true)
      await wrapper.get('[data-testid="bulk-codex-fingerprint-mode-select"]').setValue('off')
      await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
      await flushPromises()

      expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
        extra: {
          codex_fingerprint_mode: 'off'
        }
      })
    })

    it('Codex 指纹收敛不对 setup-token 混合类型或 API Key 展示', () => {
      const mixedWrapper = mountModal({
        selectedPlatforms: ['openai'],
        selectedTypes: ['oauth', 'setup-token']
      })
      expect(mixedWrapper.find('#bulk-edit-codex-fingerprint-mode-enabled').exists()).toBe(false)

      const apiKeyWrapper = mountModal({
        selectedPlatforms: ['openai'],
        selectedTypes: ['apikey']
      })
      expect(apiKeyWrapper.find('#bulk-edit-codex-fingerprint-mode-enabled').exists()).toBe(false)
    })

  it('OpenAI API Key 批量编辑不显示 WS mode 入口', () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    expect(wrapper.find('#bulk-edit-openai-ws-mode-enabled').exists()).toBe(false)
  })

  it('API Key 批量编辑默认启用 CPA 并跟随各账号 Base URL', async () => {
    const wrapper = mountModal({ selectedPlatforms: ['openai'], selectedTypes: ['apikey'] })

    await wrapper.get('#bulk-edit-cpa-enabled').setValue(true)
    expect((wrapper.get('[data-testid="bulk-edit-cpa-concurrency-per-credential"]').element as HTMLInputElement).value).toBe('10')
    expect(wrapper.get('[data-testid="bulk-edit-cpa-exclude-abnormal-toggle"]').attributes('aria-checked')).toBe('false')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        cpa_mode: true,
        cpa_management_url: null,
        cpa_concurrency_per_credential: 10,
        cpa_exclude_abnormal_credentials: false
      }
    })
  })

  it('API Key 批量编辑可统一设置 CPA 管理地址和管理员密码', async () => {
    const wrapper = mountModal({ selectedPlatforms: ['anthropic'], selectedTypes: ['apikey'] })

    await wrapper.get('#bulk-edit-cpa-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-cpa-use-custom-url"]').trigger('click')
    await wrapper.get('[data-testid="bulk-edit-cpa-management-url"]').setValue('https://cpa.example.com/v0/management/')
    await wrapper.get('[data-testid="bulk-edit-cpa-management-password"]').setValue('rotated-password')
    await wrapper.get('[data-testid="bulk-edit-cpa-concurrency-per-credential"]').setValue('4')
    await wrapper.get('[data-testid="bulk-edit-cpa-exclude-abnormal-toggle"]').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: {
        cpa_mode: true,
        cpa_management_url: 'https://cpa.example.com/v0/management',
        cpa_management_key: 'rotated-password',
        cpa_concurrency_per_credential: 4,
        cpa_exclude_abnormal_credentials: true
      }
    })
  })

  it('API Key 批量编辑可关闭 CPA，非 API Key 目标不展示 CPA 区域', async () => {
    const wrapper = mountModal({ selectedPlatforms: ['openai'], selectedTypes: ['apikey'] })
    await wrapper.get('#bulk-edit-cpa-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-cpa-mode-toggle"]').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      credentials: { cpa_mode: false }
    })

    const oauthWrapper = mountModal({ selectedPlatforms: ['openai'], selectedTypes: ['oauth'] })
    expect(oauthWrapper.find('#bulk-edit-cpa-enabled').exists()).toBe(false)
  })

  it('OpenAI OAuth 批量编辑应提交 codex_cli_only 字段', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-codex-cli-only-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-codex-cli-only-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        codex_cli_only: true
      }
    })
  })

  it('OpenAI OAuth 批量编辑应提交 codex_cli_only_allow_app_server 字段（需同时开启父开关）', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    // 子开关从属于 codex_cli_only：必须同时批量开启父开关才写入
    await wrapper.get('#bulk-edit-openai-codex-cli-only-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-codex-cli-only-toggle').trigger('click')
    await wrapper.get('#bulk-edit-openai-codex-app-server-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-codex-app-server-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        codex_cli_only: true,
        codex_cli_only_allow_app_server: true
      }
    })
  })

  it('未同时开启父开关时不应写入 codex_cli_only_allow_app_server', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    // 仅开启子开关、不批量设置父开关 codex_cli_only：不应写入孤立字段，也不应调用接口
    await wrapper.get('#bulk-edit-openai-codex-app-server-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-codex-app-server-toggle').trigger('click')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).not.toHaveBeenCalled()
  })

  it('OpenAI API Key 批量编辑应提交 API Key 专属 WS mode 字段', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-openai-apikey-ws-mode-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-apikey-ws-mode-select"]').setValue('ctx_pool')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_apikey_responses_websockets_v2_mode: 'ctx_pool',
        openai_apikey_responses_websockets_v2_enabled: true
      }
    })
  })

  it('OpenAI API Key 批量编辑可统一开启上游倍率自动探测', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-upstream-billing-auto-probe-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      upstream_billing_probe_enabled: true
    })
  })

  it('多平台 API Key 目标可统一设置上游倍率自动探测', () => {
    const wrapper = mountModal({
      selectedPlatforms: ['anthropic', 'gemini', 'grok'],
      selectedTypes: ['apikey']
    })

    expect(wrapper.find('#bulk-edit-upstream-billing-auto-probe-enabled').exists()).toBe(true)
  })

  it('批量手工倍率修改显示自动同步冲突警告', async () => {
    const wrapper = mountModal()

    await wrapper.get('#bulk-edit-rate-multiplier-enabled').setValue(true)

    expect(wrapper.get('[data-testid="bulk-rate-sync-warning"]').text()).toBe(
      'admin.accounts.bulkEdit.rateSyncWarning'
    )
  })

  it('OpenAI API Key 批量编辑可统一关闭上游倍率自动探测', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-upstream-billing-auto-probe-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-upstream-billing-auto-probe-select"]').setValue('disabled')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      upstream_billing_probe_enabled: false
    })
  })

  it('非 OpenAI API Key 目标不显示上游倍率自动探测批量开关', () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    expect(wrapper.find('#bulk-edit-upstream-billing-auto-probe-enabled').exists()).toBe(false)
  })

  it('筛选结果批量编辑可统一开启上游倍率自动探测', async () => {
    const wrapper = mountModal({
      accountIds: [],
      selectedPlatforms: [],
      selectedTypes: [],
      target: {
        mode: 'filtered',
        filters: { platform: 'openai', type: 'apikey', status: 'active' },
        previewCount: 20,
        selectedPlatforms: ['openai'],
        selectedTypes: ['apikey']
      }
    })

    await wrapper.get('#bulk-edit-upstream-billing-auto-probe-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith({
      filters: { platform: 'openai', type: 'apikey', status: 'active' },
      upstream_billing_probe_enabled: true
    })
  })

  it('筛选 OpenAI 账号批量编辑应提交 Compact 模式和专属模型映射', async () => {
    const wrapper = mountModal({
      accountIds: [],
      selectedPlatforms: [],
      selectedTypes: [],
      target: {
        mode: 'filtered',
        filters: { platform: 'openai' },
        previewCount: 12,
        selectedPlatforms: ['openai'],
        selectedTypes: ['oauth', 'apikey']
      }
    })

    await wrapper.get('#bulk-edit-openai-compact-mode-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-compact-mode-select"]').setValue('force_on')
    await wrapper.get('#bulk-edit-openai-compact-model-mapping-enabled').setValue(true)
    await wrapper.get('[data-testid="bulk-edit-openai-compact-model-mapping-add"]').trigger('click')
    const inputs = wrapper.findAll('[data-testid="bulk-edit-openai-compact-model-mapping-input"]')
    await inputs[0].setValue('gpt-5.4')
    await inputs[1].setValue('gpt-5.4-openai-compact')
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith({
      filters: { platform: 'openai' },
      extra: {
        openai_compact_mode: 'force_on'
      },
      credentials: {
        compact_model_mapping: {
          'gpt-5.4': 'gpt-5.4-openai-compact'
        }
      }
    })
  })

  it('OpenAI 账号批量编辑可关闭自动透传', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['apikey']
    })

    await wrapper.get('#bulk-edit-openai-passthrough-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_passthrough: false,
        openai_oauth_passthrough: false
      }
    })
  })

  it('开启 OpenAI 自动透传时不再同时提交模型限制', async () => {
    const wrapper = mountModal({
      selectedPlatforms: ['openai'],
      selectedTypes: ['oauth']
    })

    await wrapper.get('#bulk-edit-openai-passthrough-enabled').setValue(true)
    await wrapper.get('#bulk-edit-openai-passthrough-toggle').trigger('click')
    await wrapper.get('#bulk-edit-model-restriction-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith([1, 2], {
      extra: {
        openai_passthrough: true
      }
    })
    expect(wrapper.text()).toContain('admin.accounts.openai.modelRestrictionDisabledByPassthrough')
  })

  it('filtered-results 模式下应提交 filters 而不是 account_ids', async () => {
    const wrapper = mountModal({
      accountIds: [],
      target: {
        mode: 'filtered',
        filters: {
          platform: 'openai',
          type: 'oauth',
          status: 'active',
          group: '12',
          search: 'bulk-target',
          privacy_mode: 'training_set_cf_blocked'
        },
        previewCount: 5,
        selectedPlatforms: ['openai'],
        selectedTypes: ['oauth']
      }
    })

    await wrapper.get('#bulk-edit-status-enabled').setValue(true)
    await wrapper.get('#bulk-edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(bulkUpdate).toHaveBeenCalledTimes(1)
    expect(bulkUpdate).toHaveBeenCalledWith({
      filters: {
        platform: 'openai',
        type: 'oauth',
        status: 'active',
        group: '12',
        search: 'bulk-target',
        privacy_mode: 'training_set_cf_blocked'
      },
      status: 'active'
    })
  })
})
