import { watch, type Ref } from 'vue'
import type { useI18n } from 'vue-i18n'
import type { AccountType } from '@/types'
import type { OpenAIWSMode } from '@/core/utils/openaiWsMode'
import { OPENAI_WS_MODE_OFF } from '@/core/utils/openaiWsMode'
import {
  addEmptyModelMapping,
  addPresetModelMapping,
  buildTempUnschedRules,
  createTempUnschedRule,
  moveTempUnschedRule as moveTempUnschedRuleInPlace,
  removeModelMapping as removeModelMappingAt,
  type TempUnschedRuleForm,
} from '../accountFormPolicy'
import {
  fetchAntigravityDefaultMappings,
  getDefaultModelsByPlatform,
} from './useModelWhitelist'
import type { useAccountOAuth } from './useAccountOAuth'
import type { useOpenAIOAuth } from './useOpenAIOAuth'
import type { useGeminiOAuth } from './useGeminiOAuth'
import type { useAntigravityOAuth } from './useAntigravityOAuth'
import type { useGrokOAuth } from './useGrokOAuth'
import type {
  CreateAccountAdvancedContext,
  CreateAccountCredentialContext,
  CreateAccountPlatformContext,
} from '../accountEditorContext'

type EditorFields =
  Pick<
    CreateAccountCredentialContext,
    | 'accountCategory'
    | 'addMethod'
    | 'allowedModels'
    | 'apiKeyBaseUrl'
    | 'bedrockAccessKeyId'
    | 'bedrockApiKeyValue'
    | 'bedrockAuthMode'
    | 'bedrockForceGlobal'
    | 'bedrockRegion'
    | 'bedrockSecretAccessKey'
    | 'bedrockSessionToken'
    | 'customErrorCodeInput'
    | 'form'
    | 'grokOAuthBaseUrl'
    | 'grokOAuthCustomBaseUrlEnabled'
    | 'headerOverrideEnabled'
    | 'headerOverrideRows'
    | 'modelMappings'
    | 'modelRestrictionMode'
    | 'selectedErrorCodes'
  > &
  Pick<
    CreateAccountAdvancedContext,
    | 'allowOverages'
      | 'anthropicAPIKeyAuthScheme'
    | 'anthropicPassthroughEnabled'
    | 'codexPrewarmContinuationEnabled'
    | 'codexThinkingTagNormalizationEnabled'
      | 'codexCLIOnlyAppServerEnabled'
    | 'codexCLIOnlyEnabled'
    | 'interceptWarmupRequests'
    | 'openAICompactModelMappings'
    | 'openAIEndpointCapabilities'
    | 'openAIForceImageAPIEnabled'
    | 'openaiFlattenNamespacesEnabled'
    | 'openaiPassthroughEnabled'
    | 'tempUnschedEnabled'
    | 'tempUnschedRules'
    | 'tlsFingerprintProfiles'
    | 'webSearchEmulationMode'
  > &
  Pick<
    CreateAccountPlatformContext,
    | 'antigravityAccountType'
    | 'antigravityModelMappings'
    | 'antigravityProjectId'
    | 'geminiAIStudioOAuthEnabled'
    | 'geminiOAuthType'
    | 'vertexClientEmail'
    | 'vertexLocation'
    | 'vertexProjectId'
  >

interface CreateAccountEditorPolicyContext extends EditorFields {
  antigravityModelRestrictionMode: Ref<'whitelist' | 'mapping'>
  antigravityOAuth: ReturnType<typeof useAntigravityOAuth>
  antigravityWhitelistModels: Ref<string[]>
  geminiOAuth: ReturnType<typeof useGeminiOAuth>
  grokOAuth: ReturnType<typeof useGrokOAuth>
  loadTLSFingerprintProfiles: () => Promise<Array<{ id: number; name: string }>>
  notifications: {
    showError: (message: string) => void
    showInfo: (message: string) => void
  }
  oauth: ReturnType<typeof useAccountOAuth>
  openaiAPIKeyResponsesWebSocketV2Mode: Ref<OpenAIWSMode>
  openaiOAuth: ReturnType<typeof useOpenAIOAuth>
  openaiOAuthResponsesWebSocketV2Mode: Ref<OpenAIWSMode>
  resetForm: () => void
  show: () => boolean
  t: ReturnType<typeof useI18n>['t']
  vertexServiceAccountJson: Ref<string>
}

export function useCreateAccountEditorPolicy(context: CreateAccountEditorPolicyContext) {
  const {
    accountCategory, addMethod, allowOverages, allowedModels, anthropicAPIKeyAuthScheme,
    anthropicPassthroughEnabled, antigravityAccountType, antigravityModelMappings,
    antigravityModelRestrictionMode, antigravityOAuth, antigravityProjectId,
    antigravityWhitelistModels, apiKeyBaseUrl, bedrockAccessKeyId, bedrockApiKeyValue,
    bedrockAuthMode, bedrockForceGlobal, bedrockRegion, bedrockSecretAccessKey,
      bedrockSessionToken, codexPrewarmContinuationEnabled, codexCLIOnlyAppServerEnabled, codexCLIOnlyEnabled,
    customErrorCodeInput, form, geminiAIStudioOAuthEnabled, geminiOAuth, geminiOAuthType,
    grokOAuth, grokOAuthBaseUrl, grokOAuthCustomBaseUrlEnabled, headerOverrideEnabled,
    headerOverrideRows, interceptWarmupRequests, loadTLSFingerprintProfiles, modelMappings,
    modelRestrictionMode, notifications, oauth, openAICompactModelMappings,
    openAIEndpointCapabilities, openAIForceImageAPIEnabled,
    openaiAPIKeyResponsesWebSocketV2Mode, openaiOAuth,
    openaiOAuthResponsesWebSocketV2Mode, openaiFlattenNamespacesEnabled,
    openaiPassthroughEnabled, resetForm,
    selectedErrorCodes, show, t, tempUnschedEnabled, tempUnschedRules,
    tlsFingerprintProfiles, vertexClientEmail, vertexLocation, vertexProjectId,
    vertexServiceAccountJson, webSearchEmulationMode,
  } = context

  // Watchers
  watch(
    show,
    (newVal) => {
      if (newVal) {
        // Load TLS fingerprint profiles
        loadTLSFingerprintProfiles()
          .then(profiles => { tlsFingerprintProfiles.value = profiles.map(p => ({ id: p.id, name: p.name })) })
          .catch(() => { tlsFingerprintProfiles.value = [] })
        // Modal opened - fill related models
        allowedModels.value = [...getDefaultModelsByPlatform(form.platform)]
        // Antigravity: 默认使用映射模式并填充默认映射
        if (form.platform === 'antigravity') {
          antigravityModelRestrictionMode.value = 'mapping'
          fetchAntigravityDefaultMappings().then(mappings => {
            antigravityModelMappings.value = [...mappings]
          })
          antigravityWhitelistModels.value = []
        } else {
          antigravityWhitelistModels.value = []
          antigravityModelMappings.value = []
          antigravityModelRestrictionMode.value = 'mapping'
        }
      } else {
        resetForm()
      }
    }
  )

  // Sync form.type based on accountCategory, addMethod, and platform-specific type
  watch(
    [accountCategory, addMethod, antigravityAccountType, () => form.platform],
    ([category, method, agType]) => {
      // Antigravity upstream 类型（实际创建为 apikey）
      if (form.platform === 'antigravity' && agType === 'upstream') {
        form.type = 'apikey'
        return
      }
      // Bedrock 类型
      if (form.platform === 'anthropic' && category === 'bedrock') {
        form.type = 'bedrock' as AccountType
        return
      }
      if ((form.platform === 'gemini' || form.platform === 'anthropic') && category === 'service_account') {
        form.type = 'service_account' as AccountType
      } else if (category === 'oauth-based') {
        form.type = form.platform === 'anthropic' ? method as AccountType : 'oauth'
      } else {
        form.type = 'apikey'
      }
    },
    { immediate: true }
  )

  // Reset platform-specific settings when platform changes
  watch(
    () => form.platform,
    (newPlatform) => {
      // Reset base URL based on platform
      apiKeyBaseUrl.value =
        (newPlatform === 'openai')
          ? 'https://api.openai.com'
          : newPlatform === 'gemini'
            ? 'https://generativelanguage.googleapis.com'
            : newPlatform === 'grok'
              ? 'https://api.x.ai/v1'
              : 'https://api.anthropic.com'
      // Clear model-related settings
      allowedModels.value = []
      modelMappings.value = []
      // Antigravity: 默认使用映射模式并填充默认映射
      if (newPlatform === 'antigravity') {
        antigravityModelRestrictionMode.value = 'mapping'
        fetchAntigravityDefaultMappings().then(mappings => {
          antigravityModelMappings.value = [...mappings]
        })
        antigravityWhitelistModels.value = []
        accountCategory.value = 'oauth-based'
        antigravityAccountType.value = 'oauth'
      } else {
        allowOverages.value = false
        antigravityProjectId.value = ''
        antigravityWhitelistModels.value = []
        antigravityModelMappings.value = []
        antigravityModelRestrictionMode.value = 'mapping'
      }
      if (newPlatform === 'grok') {
        accountCategory.value = 'oauth-based'
        addMethod.value = 'oauth'
        modelRestrictionMode.value = 'mapping'
        form.concurrency = 1
        form.load_factor = null
      }
      if (newPlatform !== 'gemini' && newPlatform !== 'anthropic' && accountCategory.value === 'service_account') {
        accountCategory.value = 'oauth-based'
      }
      if (newPlatform !== 'anthropic' && accountCategory.value === 'bedrock') {
        accountCategory.value = 'oauth-based'
      }
      // Reset Bedrock fields when switching platforms
      bedrockAccessKeyId.value = ''
      bedrockSecretAccessKey.value = ''
      bedrockSessionToken.value = ''
      bedrockRegion.value = 'us-east-1'
      bedrockForceGlobal.value = false
      bedrockAuthMode.value = 'sigv4'
      bedrockApiKeyValue.value = ''
      vertexServiceAccountJson.value = ''
      vertexProjectId.value = ''
      vertexClientEmail.value = ''
      vertexLocation.value = 'global'
      // Reset Anthropic/Antigravity-specific settings when switching to other platforms
      if (newPlatform !== 'anthropic' && newPlatform !== 'antigravity') {
        interceptWarmupRequests.value = false
      }
      if (newPlatform !== 'openai') {
        openaiFlattenNamespacesEnabled.value = false
        openaiPassthroughEnabled.value = false
        openAIEndpointCapabilities.value = ['chat_completions', 'embeddings']
        openAIForceImageAPIEnabled.value = false
        openaiOAuthResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
          openaiAPIKeyResponsesWebSocketV2Mode.value = OPENAI_WS_MODE_OFF
          codexPrewarmContinuationEnabled.value = false
          codexCLIOnlyEnabled.value = false
        codexCLIOnlyAppServerEnabled.value = false
      }
      if (newPlatform !== 'anthropic') {
        anthropicPassthroughEnabled.value = false
        anthropicAPIKeyAuthScheme.value = 'x_api_key'
        webSearchEmulationMode.value = 'default'
      }
      // 请求头覆写为平台相关配置（常用头集合不同），切换平台时清空，
      // 避免上一平台的配置行被提交到新平台账号
      headerOverrideEnabled.value = false
      headerOverrideRows.value = []
      grokOAuthCustomBaseUrlEnabled.value = false
      grokOAuthBaseUrl.value = ''
      // Reset OAuth states
      oauth.resetState()
      openaiOAuth.resetState()

      geminiOAuth.resetState()
      antigravityOAuth.resetState()
      grokOAuth.resetState()
    }
  )

  // Gemini AI Studio OAuth availability (requires operator-configured OAuth client)
  watch(
    [accountCategory, () => form.platform],
    ([category, platform]) => {
        if (platform === 'openai' && category !== 'oauth-based') {
          codexPrewarmContinuationEnabled.value = false
          codexCLIOnlyEnabled.value = false
        codexCLIOnlyAppServerEnabled.value = false
      }
      if (platform !== 'anthropic' || category !== 'apikey') {
        anthropicPassthroughEnabled.value = false
        anthropicAPIKeyAuthScheme.value = 'x_api_key'
        webSearchEmulationMode.value = 'default'
      }
    }
  )

  watch(
    [show, () => form.platform, accountCategory],
    async ([show, platform, category]) => {
      if (!show || platform !== 'gemini' || category !== 'oauth-based') {
        geminiAIStudioOAuthEnabled.value = false
        return
      }
      const caps = await geminiOAuth.getCapabilities()
      geminiAIStudioOAuthEnabled.value = !!caps?.ai_studio_oauth_enabled
      if (!geminiAIStudioOAuthEnabled.value && geminiOAuthType.value === 'ai_studio') {
        geminiOAuthType.value = 'code_assist'
      }
    },
    { immediate: true }
  )

  const handleSelectGeminiOAuthType = (oauthType: 'code_assist' | 'google_one' | 'ai_studio') => {
    if (oauthType === 'ai_studio' && !geminiAIStudioOAuthEnabled.value) {
      notifications.showError(t('admin.accounts.oauth.gemini.aiStudioNotConfigured'))
      return
    }
    geminiOAuthType.value = oauthType
  }

  // Auto-fill related models when switching to whitelist mode or changing platform
  watch(
    [modelRestrictionMode, () => form.platform],
    ([newMode]) => {
      if (newMode === 'whitelist') {
        allowedModels.value = [...getDefaultModelsByPlatform(form.platform)]
      }
    }
  )

  watch(
    [antigravityModelRestrictionMode, () => form.platform],
    ([, platform]) => {
      if (platform !== 'antigravity') return
      // Antigravity 默认不做限制：白名单留空表示允许所有（包含未来新增模型）。
      // 如果需要快速填充常用模型，可在组件内点“填充相关模型”。
    }
  )

  // Model mapping helpers
  const addModelMapping = () => {
    addEmptyModelMapping(modelMappings.value)
  }

  const addOpenAICompactModelMapping = () => {
    addEmptyModelMapping(openAICompactModelMappings.value)
  }

  const removeOpenAICompactModelMapping = (index: number) => {
    removeModelMappingAt(openAICompactModelMappings.value, index)
  }

  const removeModelMapping = (index: number) => {
    removeModelMappingAt(modelMappings.value, index)
  }

  const addPresetMapping = (from: string, to: string) => {
    if (!addPresetModelMapping(modelMappings.value, from, to)) {
      notifications.showInfo(t('admin.accounts.mappingExists', { model: from }))
    }
  }

  const addAntigravityModelMapping = () => {
    addEmptyModelMapping(antigravityModelMappings.value)
  }

  const removeAntigravityModelMapping = (index: number) => {
    removeModelMappingAt(antigravityModelMappings.value, index)
  }

  const addAntigravityPresetMapping = (from: string, to: string) => {
    if (!addPresetModelMapping(antigravityModelMappings.value, from, to)) {
      notifications.showInfo(t('admin.accounts.mappingExists', { model: from }))
    }
  }

  // Error code toggle helper
  const toggleErrorCode = (code: number) => {
    const index = selectedErrorCodes.value.indexOf(code)
    if (index === -1) {
      // Adding code - check for 429/529 warning
      if (code === 429) {
        if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
          return
        }
      } else if (code === 529) {
        if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
          return
        }
      }
      selectedErrorCodes.value.push(code)
    } else {
      selectedErrorCodes.value.splice(index, 1)
    }
  }

  // Add custom error code from input
  const addCustomErrorCode = () => {
    const code = customErrorCodeInput.value
    if (code === null || code < 100 || code > 599) {
      notifications.showError(t('admin.accounts.invalidErrorCode'))
      return
    }
    if (selectedErrorCodes.value.includes(code)) {
      notifications.showInfo(t('admin.accounts.errorCodeExists'))
      return
    }
    // Check for 429/529 warning
    if (code === 429) {
      if (!confirm(t('admin.accounts.customErrorCodes429Warning'))) {
        return
      }
    } else if (code === 529) {
      if (!confirm(t('admin.accounts.customErrorCodes529Warning'))) {
        return
      }
    }
    selectedErrorCodes.value.push(code)
    customErrorCodeInput.value = null
  }

  // Remove error code
  const removeErrorCode = (code: number) => {
    const index = selectedErrorCodes.value.indexOf(code)
    if (index !== -1) {
      selectedErrorCodes.value.splice(index, 1)
    }
  }

  const addTempUnschedRule = (preset?: TempUnschedRuleForm) => {
    tempUnschedRules.value.push(createTempUnschedRule(preset))
  }

  const removeTempUnschedRule = (index: number) => {
    removeModelMappingAt(tempUnschedRules.value, index)
  }

  const moveTempUnschedRule = (index: number, direction: number) => {
    moveTempUnschedRuleInPlace(tempUnschedRules.value, index, direction)
  }

  const applyTempUnschedConfig = (credentials: Record<string, unknown>) => {
    if (!tempUnschedEnabled.value) {
      delete credentials.temp_unschedulable_enabled
      delete credentials.temp_unschedulable_rules
      return true
    }

    const rules = buildTempUnschedRules(tempUnschedRules.value)
    if (rules.length === 0) {
      notifications.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
      return false
    }

    credentials.temp_unschedulable_enabled = true
    credentials.temp_unschedulable_rules = rules
    return true
  }


  return {
    addAntigravityModelMapping,
    addAntigravityPresetMapping,
    addCustomErrorCode,
    addModelMapping,
    addOpenAICompactModelMapping,
    addPresetMapping,
    addTempUnschedRule,
    applyTempUnschedConfig,
    handleSelectGeminiOAuthType,
    moveTempUnschedRule,
    removeAntigravityModelMapping,
    removeErrorCode,
    removeModelMapping,
    removeOpenAICompactModelMapping,
    removeTempUnschedRule,
    toggleErrorCode,
  }
}
