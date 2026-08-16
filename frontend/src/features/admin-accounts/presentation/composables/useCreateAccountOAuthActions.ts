import type { ComputedRef, Ref } from 'vue'
import type { useI18n } from 'vue-i18n'
import type {
  AccountPlatform,
  AccountType,
} from '@/types'
import type {
  CodexSessionImportMessage,
  CreateAccountRequest,
} from '../../data/dtos/adminAccountDtos'
import type { useQuotaNotifyState } from './useQuotaNotifyState'
import type { useAccountOAuth, AuthInputMethod } from './useAccountOAuth'
import type { useOpenAIOAuth } from './useOpenAIOAuth'
import type { useGeminiOAuth } from './useGeminiOAuth'
import type { useAntigravityOAuth } from './useAntigravityOAuth'
import type { useGrokOAuth } from './useGrokOAuth'
import type {
  CreateAccountAdvancedContext,
  CreateAccountCredentialContext,
  CreateAccountPlatformContext,
} from '../accountEditorContext'
import {
  authenticateAccountWithCookie,
  exchangeAccountAuthCode
} from '../../data/datasources/adminAccountOAuthActions'
import {
  createAccount,
  createOpenAICodexPAT,
  importCodexSession
} from '../../data/datasources/adminAccountActions'
import { createFromSSO } from '../../data/datasources/grokDatasource'
import {
  applyAntigravityProjectID,
  applyInterceptWarmup,
} from '../credentialsBuilder'
import { buildModelMappingObject } from './useModelWhitelist'
import { buildTempUnschedRules } from '../accountFormPolicy'

export interface OAuthFlowExposed {
  authCode: string
  oauthState: string
  projectId: string
  sessionKey: string
  refreshToken: string
  sessionToken: string
  codexSession: string
  codexPAT: string
  ssoCookie: string
  inputMethod: AuthInputMethod
  reset: () => void
}

type EditorFields =
  Pick<
    CreateAccountCredentialContext,
    | 'addMethod'
    | 'allowedModels'
    | 'apiKeyBaseUrl'
    | 'editDailyResetHour'
    | 'editDailyResetMode'
    | 'editQuotaDailyLimit'
    | 'editQuotaLimit'
    | 'editQuotaWeeklyLimit'
    | 'editResetTimezone'
    | 'editWeeklyResetDay'
    | 'editWeeklyResetHour'
    | 'editWeeklyResetMode'
    | 'form'
    | 'modelMappings'
    | 'modelRestrictionMode'
  > &
  Pick<
    CreateAccountAdvancedContext,
    | 'autoPauseOnExpired'
    | 'baseRpm'
    | 'cacheTTLOverrideEnabled'
    | 'cacheTTLOverrideTarget'
    | 'customBaseUrl'
    | 'customBaseUrlEnabled'
    | 'interceptWarmupRequests'
    | 'maxSessions'
    | 'rpmLimitEnabled'
    | 'rpmStickyBuffer'
    | 'rpmStrategy'
    | 'sessionIdMaskingEnabled'
    | 'sessionIdleTimeout'
    | 'sessionLimitEnabled'
    | 'tempUnschedEnabled'
    | 'tempUnschedRules'
    | 'tlsFingerprintEnabled'
    | 'tlsFingerprintProfileId'
    | 'userMsgQueueMode'
    | 'windowCostEnabled'
    | 'windowCostLimit'
    | 'windowCostStickyReserve'
  > &
  Pick<
    CreateAccountPlatformContext,
    'antigravityModelMappings' | 'antigravityProjectId' | 'geminiOAuthType'
  >

interface CreateAccountOAuthActionsContext extends EditorFields {
  antigravityOAuth: ReturnType<typeof useAntigravityOAuth>
  applyGrokOAuthUpstreamConfig: (credentials: Record<string, unknown>) => void
  applyOpenAIEndpointCapabilities: (credentials: Record<string, unknown>) => void
  applyTempUnschedConfig: (credentials: Record<string, unknown>) => boolean
  buildAntigravityExtra: () => Record<string, unknown> | undefined
  buildOpenAICodexImportExtra: () => Record<string, unknown> | undefined
  buildOpenAICompactModelMapping: () => Record<string, string> | null
  buildOpenAIExtra: (base?: Record<string, unknown>) => Record<string, unknown> | undefined
  doCreateAccount: (payload: CreateAccountRequest) => Promise<void>
  geminiOAuth: ReturnType<typeof useGeminiOAuth>
  geminiSelectedTier: ComputedRef<string>
  grokOAuth: ReturnType<typeof useGrokOAuth>
  handleClose: () => void
  isOpenAIModelRestrictionDisabled: ComputedRef<boolean>
  notifications: {
    showError: (message: string) => void
    showSuccess: (message: string) => void
    showWarning: (message: string) => void
  }
  oauth: ReturnType<typeof useAccountOAuth>
  oauthFlowRef: Ref<OAuthFlowExposed | null>
  onCreated: () => void
  openaiOAuth: ReturnType<typeof useOpenAIOAuth>
  step: Ref<number>
  t: ReturnType<typeof useI18n>['t']
  validateGrokOAuthUpstreamConfig: () => boolean
  withAntigravityConfirmFlag: (payload: CreateAccountRequest) => CreateAccountRequest
  writeQuotaNotifyToExtra: ReturnType<typeof useQuotaNotifyState>['writeToExtra']
}

export function useCreateAccountOAuthActions(context: CreateAccountOAuthActionsContext) {
  const {
    addMethod, allowedModels, antigravityModelMappings, antigravityOAuth,
    antigravityProjectId, apiKeyBaseUrl, applyGrokOAuthUpstreamConfig,
    applyOpenAIEndpointCapabilities, applyTempUnschedConfig, autoPauseOnExpired, baseRpm,
    buildAntigravityExtra, buildOpenAICodexImportExtra, buildOpenAICompactModelMapping,
    buildOpenAIExtra, cacheTTLOverrideEnabled, cacheTTLOverrideTarget, customBaseUrl,
    customBaseUrlEnabled, doCreateAccount, editDailyResetHour, editDailyResetMode,
    editQuotaDailyLimit, editQuotaLimit, editQuotaWeeklyLimit, editResetTimezone,
    editWeeklyResetDay, editWeeklyResetHour, editWeeklyResetMode, form, geminiOAuth,
    geminiOAuthType, geminiSelectedTier, grokOAuth, handleClose, interceptWarmupRequests,
    isOpenAIModelRestrictionDisabled, maxSessions, modelMappings, modelRestrictionMode,
    notifications, oauth, oauthFlowRef, onCreated, openaiOAuth, rpmLimitEnabled,
    rpmStickyBuffer, rpmStrategy, sessionIdMaskingEnabled, sessionIdleTimeout,
    sessionLimitEnabled, step, t, tempUnschedEnabled, tempUnschedRules,
    tlsFingerprintEnabled, tlsFingerprintProfileId, userMsgQueueMode,
    validateGrokOAuthUpstreamConfig, windowCostEnabled, windowCostLimit,
    windowCostStickyReserve, withAntigravityConfirmFlag, writeQuotaNotifyToExtra,
  } = context

  const goBackToBasicInfo = () => {
    step.value = 1
    oauth.resetState()
    openaiOAuth.resetState()
    geminiOAuth.resetState()
    antigravityOAuth.resetState()
    grokOAuth.resetState()
    oauthFlowRef.value?.reset()
  }

  const handleGenerateUrl = async () => {
    if (form.platform === 'openai') {
      await openaiOAuth.generateAuthUrl(form.proxy_id)
    } else if (form.platform === 'gemini') {
      await geminiOAuth.generateAuthUrl(
        form.proxy_id,
        oauthFlowRef.value?.projectId,
        geminiOAuthType.value,
        geminiSelectedTier.value
      )
    } else if (form.platform === 'antigravity') {
      await antigravityOAuth.generateAuthUrl(form.proxy_id)
    } else if (form.platform === 'grok') {
      await grokOAuth.generateAuthUrl(form.proxy_id)
    } else {
      await oauth.generateAuthUrl(addMethod.value, form.proxy_id)
    }
  }

  const handleValidateRefreshToken = (rt: string) => {
    if (form.platform === 'openai') {
      handleOpenAIValidateRT(rt)
    } else if (form.platform === 'antigravity') {
      handleAntigravityValidateRT(rt)
    } else if (form.platform === 'grok') {
      handleGrokValidateRT(rt)
    }
  }

  const handleValidateSessionToken = (_sessionToken: string) => {
    // Session token validation removed
  }

  // Create account and handle success/failure
  const createAccountAndFinish = async (
    platform: AccountPlatform,
    type: AccountType,
    credentials: Record<string, unknown>,
    extra?: Record<string, unknown>
  ) => {
    if (!applyTempUnschedConfig(credentials)) {
      return
    }
    // Inject quota limits for apikey/bedrock accounts
    let finalExtra = extra
    if (type === 'apikey' || type === 'bedrock') {
      const quotaExtra: Record<string, unknown> = { ...(extra || {}) }
      if (editQuotaLimit.value != null && editQuotaLimit.value > 0) {
        quotaExtra.quota_limit = editQuotaLimit.value
      }
      if (editQuotaDailyLimit.value != null && editQuotaDailyLimit.value > 0) {
        quotaExtra.quota_daily_limit = editQuotaDailyLimit.value
      }
      if (editQuotaWeeklyLimit.value != null && editQuotaWeeklyLimit.value > 0) {
        quotaExtra.quota_weekly_limit = editQuotaWeeklyLimit.value
      }
      // Quota reset mode config
      if (editDailyResetMode.value === 'fixed') {
        quotaExtra.quota_daily_reset_mode = 'fixed'
        quotaExtra.quota_daily_reset_hour = editDailyResetHour.value ?? 0
      }
      if (editWeeklyResetMode.value === 'fixed') {
        quotaExtra.quota_weekly_reset_mode = 'fixed'
        quotaExtra.quota_weekly_reset_day = editWeeklyResetDay.value ?? 1
        quotaExtra.quota_weekly_reset_hour = editWeeklyResetHour.value ?? 0
      }
      if (editDailyResetMode.value === 'fixed' || editWeeklyResetMode.value === 'fixed') {
        quotaExtra.quota_reset_timezone = editResetTimezone.value || 'UTC'
      }
      // Quota notify config
      writeQuotaNotifyToExtra(quotaExtra, 'create')
      if (Object.keys(quotaExtra).length > 0) {
        finalExtra = quotaExtra
      }
    }
    if (platform === 'openai') {
      if (type === 'apikey') {
        applyOpenAIEndpointCapabilities(credentials)
      }
      const compactModelMapping = buildOpenAICompactModelMapping()
      if (compactModelMapping) {
        credentials.compact_model_mapping = compactModelMapping
      } else {
        delete credentials.compact_model_mapping
      }
    }
    if (platform === 'grok') {
      if (!credentials.base_url) {
        credentials.base_url = apiKeyBaseUrl.value.trim() || 'https://api.x.ai/v1'
      }
      const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
      if (modelMapping) {
        credentials.model_mapping = modelMapping
      } else {
        delete credentials.model_mapping
      }
    }
    await doCreateAccount({
      name: form.name,
      notes: form.notes,
      platform,
      type,
      credentials,
      extra: finalExtra,
      proxy_id: form.proxy_id,
      concurrency: form.concurrency,
      load_factor: form.load_factor ?? undefined,
      priority: form.priority,
      rate_multiplier: form.rate_multiplier,
      group_ids: form.group_ids,
      expires_at: form.expires_at,
      auto_pause_on_expired: autoPauseOnExpired.value
    })
  }

  // Grok 手动 RT 批量验证和创建
  const handleGrokValidateRT = async (refreshTokenInput: string) => {
    if (!refreshTokenInput.trim()) return

    const refreshTokens = refreshTokenInput
      .split('\n')
      .map((rt) => rt.trim())
      .filter((rt) => rt)

    if (refreshTokens.length === 0) {
      grokOAuth.error.value = t('admin.accounts.oauth.grok.pleaseEnterRefreshToken')
      return
    }
    if (!validateGrokOAuthUpstreamConfig()) return

    grokOAuth.loading.value = true
    grokOAuth.error.value = ''

    let successCount = 0
    let failedCount = 0
    const errors: string[] = []

    try {
      for (let i = 0; i < refreshTokens.length; i++) {
        try {
          const tokenInfo = await grokOAuth.validateRefreshToken(refreshTokens[i], form.proxy_id)
          if (!tokenInfo) {
            failedCount++
            errors.push(`#${i + 1}: ${grokOAuth.error.value || 'Validation failed'}`)
            grokOAuth.error.value = ''
            continue
          }

          const credentials = grokOAuth.buildCredentials(tokenInfo)
          applyGrokOAuthUpstreamConfig(credentials)
          const extra = grokOAuth.buildExtraInfo(tokenInfo)
          const accountName = refreshTokens.length > 1 ? `${form.name || tokenInfo.email || 'Grok OAuth Account'} #${i + 1}` : (form.name || tokenInfo.email || 'Grok OAuth Account')

          const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
          if (modelMapping) {
            credentials.model_mapping = modelMapping
          }
          if (!applyTempUnschedConfig(credentials)) {
            return
          }

          await createAccount({
            name: accountName,
            notes: form.notes,
            platform: 'grok',
            type: 'oauth',
            credentials,
            extra,
            proxy_id: form.proxy_id,
            concurrency: form.concurrency,
            load_factor: form.load_factor ?? undefined,
            priority: form.priority,
            rate_multiplier: form.rate_multiplier,
            group_ids: form.group_ids,
            expires_at: form.expires_at,
            auto_pause_on_expired: autoPauseOnExpired.value
          })
          successCount++
        } catch (error: any) {
          failedCount++
          const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
          errors.push(`#${i + 1}: ${errMsg}`)
        }
      }

      if (successCount > 0 && failedCount === 0) {
        notifications.showSuccess(
          refreshTokens.length > 1
            ? t('admin.accounts.oauth.batchSuccess', { count: successCount })
            : t('admin.accounts.accountCreated')
        )
        onCreated()
        handleClose()
      } else if (successCount > 0) {
        notifications.showWarning(t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount }))
        grokOAuth.error.value = errors.join('\n')
        onCreated()
      } else {
        grokOAuth.error.value = errors.join('\n')
        notifications.showError(t('admin.accounts.oauth.batchFailed'))
      }
    } finally {
      grokOAuth.loading.value = false
    }
  }

  const handleGrokImportSSO = async (ssoInput: string) => {
    // Align with OpenAI/Grok RT batch import: one token per line, no client-side dedupe.
    const ssoTokens = ssoInput
      .split('\n')
      .map((token) => token.trim())
      .filter((token) => token)
    if (ssoTokens.length === 0) return
    if (!validateGrokOAuthUpstreamConfig()) return

    grokOAuth.loading.value = true
    grokOAuth.error.value = ''

    const credentials: Record<string, unknown> = {}
    applyGrokOAuthUpstreamConfig(credentials)
    const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
    if (modelMapping) {
      credentials.model_mapping = modelMapping
    }
    if (!applyTempUnschedConfig(credentials)) {
      grokOAuth.loading.value = false
      return
    }

    try {
      const result = await createFromSSO({
        sso_tokens: ssoTokens,
        name: form.name || undefined,
        notes: form.notes || undefined,
        proxy_id: form.proxy_id,
        group_ids: form.group_ids,
        credentials,
        concurrency: form.concurrency,
        load_factor: form.load_factor ?? undefined,
        priority: form.priority,
        rate_multiplier: form.rate_multiplier,
        expires_at: form.expires_at,
        auto_pause_on_expired: autoPauseOnExpired.value
      })

      const successCount = result.created?.length || 0
      const failedCount = result.failed?.length || 0
      if (successCount > 0 && failedCount === 0) {
        notifications.showSuccess(
          ssoTokens.length > 1
            ? t('admin.accounts.oauth.batchSuccess', { count: successCount })
            : t('admin.accounts.accountCreated')
        )
        onCreated()
        handleClose()
      } else if (successCount > 0 && failedCount > 0) {
        // Same as OpenAI/Grok RT: keep input, show failures, refresh list.
        notifications.showWarning(
          t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount })
        )
        grokOAuth.error.value = (result.failed || [])
          .map((item) => `#${item.index}: ${item.error || 'Unknown error'}`)
          .join('\n')
        onCreated()
      } else {
        grokOAuth.error.value = (result.failed || [])
          .map((item) => `#${item.index}: ${item.error || 'Unknown error'}`)
          .join('\n') || t('admin.accounts.oauth.grok.failedToConvertSSO')
        notifications.showError(t('admin.accounts.oauth.batchFailed'))
      }
    } catch (error: any) {
      grokOAuth.error.value = error.response?.data?.detail || error.message || t('admin.accounts.oauth.grok.failedToConvertSSO')
      notifications.showError(grokOAuth.error.value)
    } finally {
      grokOAuth.loading.value = false
    }
  }

  // OpenAI OAuth 授权码兑换
  const handleOpenAIExchange = async (authCode: string) => {
    const oauthClient = openaiOAuth
    if (!authCode.trim() || !oauthClient.sessionId.value) return

    oauthClient.loading.value = true
    oauthClient.error.value = ''

    try {
      const stateToUse = (oauthFlowRef.value?.oauthState || oauthClient.oauthState.value || '').trim()
      if (!stateToUse) {
        oauthClient.error.value = t('admin.accounts.oauth.authFailed')
        notifications.showError(oauthClient.error.value)
        return
      }

      const tokenInfo = await oauthClient.exchangeAuthCode(
        authCode.trim(),
        oauthClient.sessionId.value,
        stateToUse,
        form.proxy_id
      )
      if (!tokenInfo) return

      const credentials = oauthClient.buildCredentials(tokenInfo)
      const oauthExtra = oauthClient.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
      const extra = buildOpenAIExtra(oauthExtra)
      const shouldCreateOpenAI = form.platform === 'openai'

      // Add model mapping for OpenAI OAuth accounts（透传模式下不应用）
      if (shouldCreateOpenAI && !isOpenAIModelRestrictionDisabled.value) {
        const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
        if (modelMapping) {
          credentials.model_mapping = modelMapping
        }
      }
      if (shouldCreateOpenAI) {
        const compactModelMapping = buildOpenAICompactModelMapping()
        if (compactModelMapping) {
          credentials.compact_model_mapping = compactModelMapping
        }
      }

      // 应用临时不可调度配置
      if (!applyTempUnschedConfig(credentials)) {
        return
      }

      if (shouldCreateOpenAI) {
        await createAccount({
          name: form.name,
          notes: form.notes,
          platform: 'openai',
          type: 'oauth',
          credentials,
          extra,
          proxy_id: form.proxy_id,
          concurrency: form.concurrency,
          load_factor: form.load_factor ?? undefined,
          priority: form.priority,
          rate_multiplier: form.rate_multiplier,
          group_ids: form.group_ids,
          expires_at: form.expires_at,
          auto_pause_on_expired: autoPauseOnExpired.value
        })
        notifications.showSuccess(t('admin.accounts.accountCreated'))
      }

      onCreated()
      handleClose()
    } catch (error: any) {
      oauthClient.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      notifications.showError(oauthClient.error.value)
    } finally {
      oauthClient.loading.value = false
    }
  }

  // OpenAI 手动 RT 批量验证和创建
  // OpenAI Mobile RT client_id
  const OPENAI_MOBILE_RT_CLIENT_ID = 'app_LlGpXReQgckcGGUo2JrYvtJK'

  const buildOpenAICodexImportCredentialExtras = (): Record<string, unknown> | null => {
    const credentials: Record<string, unknown> = {}
    if (!isOpenAIModelRestrictionDisabled.value) {
      const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
      if (modelMapping) {
        credentials.model_mapping = modelMapping
      }
    }

    const compactModelMapping = buildOpenAICompactModelMapping()
    if (compactModelMapping) {
      credentials.compact_model_mapping = compactModelMapping
    }

    if (!applyTempUnschedConfig(credentials)) {
      return null
    }
    return credentials
  }

  const formatCodexImportMessages = (messages?: CodexSessionImportMessage[]) => {
    return (messages || [])
      .map((item) => {
        const name = item.name ? ` ${item.name}` : ''
        return `#${item.index}${name}: ${item.message}`
      })
      .join('\n')
  }

  const isAgentIdentityImportContent = (content: string) => {
    const isAgentIdentityValue = (value: unknown): boolean => {
      if (Array.isArray(value)) return value.length > 0 && value.every(isAgentIdentityValue)
      if (!value || typeof value !== 'object') return false
      const record = value as Record<string, unknown>
      const authMode = record.auth_mode ?? record.authMode
      const agentIdentity = record.agent_identity ?? record.agentIdentity
      return (typeof authMode === 'string' && authMode.toLowerCase() === 'agentidentity')
        || (!!agentIdentity && typeof agentIdentity === 'object')
    }

    try {
      return isAgentIdentityValue(JSON.parse(content))
    } catch {
      const lines = content.split('\n').map((line) => line.trim()).filter(Boolean)
      if (lines.length === 0) return false
      try {
        return lines.every((line) => isAgentIdentityValue(JSON.parse(line)))
      } catch {
        return false
      }
    }
  }

  const handleOpenAIImportCodexSession = async (content: string) => {
    const oauthClient = openaiOAuth
    const trimmed = content.trim()
    if (!trimmed) {
      oauthClient.error.value = t('admin.accounts.oauth.openai.codexSessionEmpty')
      return
    }
    if (oauthFlowRef.value?.inputMethod === 'agent_identity' && !isAgentIdentityImportContent(trimmed)) {
      oauthClient.error.value = t('admin.accounts.oauth.openai.agentIdentityInvalid')
      return
    }

    const credentialExtras = buildOpenAICodexImportCredentialExtras()
    if (credentialExtras === null) {
      return
    }

    oauthClient.loading.value = true
    oauthClient.error.value = ''

    try {
      const extra = buildOpenAICodexImportExtra()
      const result = await importCodexSession({
        content: trimmed,
        name: form.name,
        notes: form.notes || null,
        proxy_id: form.proxy_id,
        concurrency: form.concurrency,
        load_factor: form.load_factor ?? undefined,
        priority: form.priority,
        rate_multiplier: form.rate_multiplier,
        group_ids: form.group_ids,
        expires_at: form.expires_at,
        auto_pause_on_expired: autoPauseOnExpired.value,
        credential_extras: Object.keys(credentialExtras).length > 0 ? credentialExtras : undefined,
        extra,
        update_existing: true
      })

      const successCount = result.created + result.updated
      const params = {
        created: result.created,
        updated: result.updated,
        skipped: result.skipped,
        failed: result.failed
      }

      if (successCount > 0 && result.failed === 0) {
        notifications.showSuccess(t('admin.accounts.oauth.openai.codexSessionImportSuccess', params))
        onCreated()
        handleClose()
        return
      }

      const errorText = formatCodexImportMessages(result.errors)
      const warningText = formatCodexImportMessages(result.warnings)
      oauthClient.error.value = [errorText, warningText].filter(Boolean).join('\n')

      if (result.failed === 0) {
        notifications.showWarning(t('admin.accounts.oauth.openai.codexSessionImportSuccess', params))
        return
      }

      if (successCount > 0) {
        notifications.showWarning(t('admin.accounts.oauth.openai.codexSessionImportPartial', params))
        onCreated()
        return
      }

      notifications.showError(t('admin.accounts.oauth.openai.codexSessionImportFailed'))
    } catch (error: any) {
      oauthClient.error.value =
        error.response?.data?.detail ||
        error.response?.data?.message ||
        error.message ||
        t('admin.accounts.oauth.openai.codexSessionImportFailed')
      notifications.showError(oauthClient.error.value)
    } finally {
      oauthClient.loading.value = false
    }
  }

  const handleOpenAIImportCodexPAT = async (accessToken: string) => {
    const oauthClient = openaiOAuth
    const trimmed = accessToken.trim()
    if (!trimmed) {
      oauthClient.error.value = t('admin.accounts.oauth.openai.codexPatEmpty')
      return
    }

    const credentialExtras = buildOpenAICodexImportCredentialExtras()
    if (credentialExtras === null) {
      return
    }

    oauthClient.loading.value = true
    oauthClient.error.value = ''

    try {
      const extra = buildOpenAICodexImportExtra()
      await createOpenAICodexPAT({
        access_token: trimmed,
        name: form.name,
        notes: form.notes || null,
        proxy_id: form.proxy_id,
        concurrency: form.concurrency,
        load_factor: form.load_factor ?? undefined,
        priority: form.priority,
        rate_multiplier: form.rate_multiplier,
        group_ids: form.group_ids,
        expires_at: form.expires_at,
        auto_pause_on_expired: autoPauseOnExpired.value,
        credential_extras: Object.keys(credentialExtras).length > 0 ? credentialExtras : undefined,
        extra
      })

      notifications.showSuccess(t('admin.accounts.accountCreated'))
      onCreated()
      handleClose()
    } catch (error: any) {
      oauthClient.error.value =
        error.response?.data?.detail ||
        error.response?.data?.message ||
        error.message ||
        t('admin.accounts.oauth.openai.codexPatImportFailed')
      notifications.showError(oauthClient.error.value)
    } finally {
      oauthClient.loading.value = false
    }
  }

  // OpenAI RT 批量验证和创建（共享逻辑）
  const handleOpenAIBatchRT = async (refreshTokenInput: string, clientId?: string) => {
    const oauthClient = openaiOAuth
    if (!refreshTokenInput.trim()) return

    const refreshTokens = refreshTokenInput
      .split('\n')
      .map((rt) => rt.trim())
      .filter((rt) => rt)

    if (refreshTokens.length === 0) {
      oauthClient.error.value = t('admin.accounts.oauth.openai.pleaseEnterRefreshToken')
      return
    }

    oauthClient.loading.value = true
    oauthClient.error.value = ''

    let successCount = 0
    let failedCount = 0
    const errors: string[] = []
    const shouldCreateOpenAI = form.platform === 'openai'

    try {
      for (let i = 0; i < refreshTokens.length; i++) {
        try {
          const tokenInfo = await oauthClient.validateRefreshToken(
            refreshTokens[i],
            form.proxy_id,
            clientId
          )
          if (!tokenInfo) {
            failedCount++
            errors.push(`#${i + 1}: ${oauthClient.error.value || 'Validation failed'}`)
            oauthClient.error.value = ''
            continue
          }

          const credentials = oauthClient.buildCredentials(tokenInfo)
          if (clientId) {
            credentials.client_id = clientId
          }
          const oauthExtra = oauthClient.buildExtraInfo(tokenInfo) as Record<string, unknown> | undefined
          const extra = buildOpenAIExtra(oauthExtra)

          // Add model mapping for OpenAI OAuth accounts（透传模式下不应用）
          if (shouldCreateOpenAI && !isOpenAIModelRestrictionDisabled.value) {
            const modelMapping = buildModelMappingObject(modelRestrictionMode.value, allowedModels.value, modelMappings.value)
            if (modelMapping) {
              credentials.model_mapping = modelMapping
            }
          }
          if (shouldCreateOpenAI) {
            const compactModelMapping = buildOpenAICompactModelMapping()
            if (compactModelMapping) {
              credentials.compact_model_mapping = compactModelMapping
            }
          }

          // Generate account name; fallback to email if name is empty (ent schema requires NotEmpty)
          const baseName = form.name || tokenInfo.email || 'OpenAI OAuth Account'
          const accountName = refreshTokens.length > 1 ? `${baseName} #${i + 1}` : baseName

          if (shouldCreateOpenAI) {
            await createAccount({
              name: accountName,
              notes: form.notes,
              platform: 'openai',
              type: 'oauth',
              credentials,
              extra,
              proxy_id: form.proxy_id,
              concurrency: form.concurrency,
              load_factor: form.load_factor ?? undefined,
              priority: form.priority,
              rate_multiplier: form.rate_multiplier,
              group_ids: form.group_ids,
              expires_at: form.expires_at,
              auto_pause_on_expired: autoPauseOnExpired.value
            })
          }

          successCount++
        } catch (error: any) {
          failedCount++
          const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
          errors.push(`#${i + 1}: ${errMsg}`)
        }
      }

      // Show results
      if (successCount > 0 && failedCount === 0) {
        notifications.showSuccess(
          refreshTokens.length > 1
            ? t('admin.accounts.oauth.batchSuccess', { count: successCount })
            : t('admin.accounts.accountCreated')
        )
        onCreated()
        handleClose()
      } else if (successCount > 0 && failedCount > 0) {
        notifications.showWarning(
          t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount })
        )
        oauthClient.error.value = errors.join('\n')
        onCreated()
      } else {
        oauthClient.error.value = errors.join('\n')
        notifications.showError(t('admin.accounts.oauth.batchFailed'))
      }
    } finally {
      oauthClient.loading.value = false
    }
  }

  // 手动输入 RT（Codex CLI client_id，默认）
  const handleOpenAIValidateRT = (rt: string) => handleOpenAIBatchRT(rt)

  // 手动输入 Mobile RT
  const handleOpenAIValidateMobileRT = (rt: string) => handleOpenAIBatchRT(rt, OPENAI_MOBILE_RT_CLIENT_ID)

  // Antigravity 手动 RT 批量验证和创建
  const handleAntigravityValidateRT = async (refreshTokenInput: string) => {
    if (!refreshTokenInput.trim()) return

    // Parse multiple refresh tokens (one per line)
    const refreshTokens = refreshTokenInput
      .split('\n')
      .map((rt) => rt.trim())
      .filter((rt) => rt)

    if (refreshTokens.length === 0) {
      antigravityOAuth.error.value = t('admin.accounts.oauth.antigravity.pleaseEnterRefreshToken')
      return
    }

    antigravityOAuth.loading.value = true
    antigravityOAuth.error.value = ''

    let successCount = 0
    let failedCount = 0
    const errors: string[] = []

    try {
      for (let i = 0; i < refreshTokens.length; i++) {
        try {
          const tokenInfo = await antigravityOAuth.validateRefreshToken(
            refreshTokens[i],
            form.proxy_id
          )
          if (!tokenInfo) {
            failedCount++
            errors.push(`#${i + 1}: ${antigravityOAuth.error.value || 'Validation failed'}`)
            antigravityOAuth.error.value = ''
            continue
          }

          const credentials = antigravityOAuth.buildCredentials(tokenInfo, refreshTokens[i])
          applyAntigravityProjectID(credentials, antigravityProjectId.value, 'create')

          // Generate account name with index for batch
          const accountName = refreshTokens.length > 1 ? `${form.name} #${i + 1}` : form.name

          // Note: Antigravity doesn't have buildExtraInfo, so we pass empty extra or rely on credentials
          const createPayload = withAntigravityConfirmFlag({
            name: accountName,
            notes: form.notes,
            platform: 'antigravity',
            type: 'oauth',
            credentials,
            extra: {},
            proxy_id: form.proxy_id,
            concurrency: form.concurrency,
            load_factor: form.load_factor ?? undefined,
            priority: form.priority,
            rate_multiplier: form.rate_multiplier,
            group_ids: form.group_ids,
            expires_at: form.expires_at,
            auto_pause_on_expired: autoPauseOnExpired.value
          })
          await createAccount(createPayload)
          successCount++
        } catch (error: any) {
          failedCount++
          const errMsg = error.response?.data?.detail || error.message || 'Unknown error'
          errors.push(`#${i + 1}: ${errMsg}`)
        }
      }

      // Show results
      if (successCount > 0 && failedCount === 0) {
        notifications.showSuccess(
          refreshTokens.length > 1
            ? t('admin.accounts.oauth.batchSuccess', { count: successCount })
            : t('admin.accounts.accountCreated')
        )
        onCreated()
        handleClose()
      } else if (successCount > 0 && failedCount > 0) {
        notifications.showWarning(
          t('admin.accounts.oauth.batchPartialSuccess', { success: successCount, failed: failedCount })
        )
        antigravityOAuth.error.value = errors.join('\n')
        onCreated()
      } else {
        antigravityOAuth.error.value = errors.join('\n')
        notifications.showError(t('admin.accounts.oauth.batchFailed'))
      }
    } finally {
      antigravityOAuth.loading.value = false
    }
  }

  // Gemini OAuth 授权码兑换
  const handleGeminiExchange = async (authCode: string) => {
    if (!authCode.trim() || !geminiOAuth.sessionId.value) return

    geminiOAuth.loading.value = true
    geminiOAuth.error.value = ''

    try {
      const stateFromInput = oauthFlowRef.value?.oauthState || ''
      const stateToUse = stateFromInput || geminiOAuth.state.value
      if (!stateToUse) {
        geminiOAuth.error.value = t('admin.accounts.oauth.authFailed')
        notifications.showError(geminiOAuth.error.value)
        return
      }

      const tokenInfo = await geminiOAuth.exchangeAuthCode({
        code: authCode.trim(),
        sessionId: geminiOAuth.sessionId.value,
        state: stateToUse,
        proxyId: form.proxy_id,
        oauthType: geminiOAuthType.value,
        tierId: geminiSelectedTier.value
      })
      if (!tokenInfo) return

      const credentials = geminiOAuth.buildCredentials(tokenInfo)
      const extra = geminiOAuth.buildExtraInfo(tokenInfo)
      await createAccountAndFinish('gemini', 'oauth', credentials, extra)
    } catch (error: any) {
      geminiOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      notifications.showError(geminiOAuth.error.value)
    } finally {
      geminiOAuth.loading.value = false
    }
  }

  // Antigravity OAuth 授权码兑换
  const handleAntigravityExchange = async (authCode: string) => {
    if (!authCode.trim() || !antigravityOAuth.sessionId.value) return

    antigravityOAuth.loading.value = true
    antigravityOAuth.error.value = ''

    try {
      const stateFromInput = oauthFlowRef.value?.oauthState || ''
      const stateToUse = stateFromInput || antigravityOAuth.state.value
      if (!stateToUse) {
        antigravityOAuth.error.value = t('admin.accounts.oauth.authFailed')
        notifications.showError(antigravityOAuth.error.value)
        return
      }

      const tokenInfo = await antigravityOAuth.exchangeAuthCode({
        code: authCode.trim(),
        sessionId: antigravityOAuth.sessionId.value,
        state: stateToUse,
        proxyId: form.proxy_id
      })
      if (!tokenInfo) return

      const credentials = antigravityOAuth.buildCredentials(tokenInfo)
      applyAntigravityProjectID(credentials, antigravityProjectId.value, 'create')
      applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
      // Antigravity 只使用映射模式
      const antigravityModelMapping = buildModelMappingObject(
        'mapping',
        [],
        antigravityModelMappings.value
      )
      if (antigravityModelMapping) {
        credentials.model_mapping = antigravityModelMapping
      }
      const extra = buildAntigravityExtra()
      await createAccountAndFinish('antigravity', 'oauth', credentials, extra)
    } catch (error: any) {
      antigravityOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      notifications.showError(antigravityOAuth.error.value)
    } finally {
      antigravityOAuth.loading.value = false
    }
  }

  // Grok OAuth 授权码兑换
  const handleGrokExchange = async (authCode: string) => {
    if (!authCode.trim() || !grokOAuth.sessionId.value) return
    if (!validateGrokOAuthUpstreamConfig()) return

    grokOAuth.loading.value = true
    grokOAuth.error.value = ''

    try {
      const stateFromInput = oauthFlowRef.value?.oauthState || ''
      const stateToUse = stateFromInput || grokOAuth.state.value
      if (!stateToUse) {
        grokOAuth.error.value = t('admin.accounts.oauth.authFailed')
        notifications.showError(grokOAuth.error.value)
        return
      }

      const tokenInfo = await grokOAuth.exchangeAuthCode({
        code: authCode.trim(),
        sessionId: grokOAuth.sessionId.value,
        state: stateToUse,
        proxyId: form.proxy_id
      })
      if (!tokenInfo) return

      const credentials = grokOAuth.buildCredentials(tokenInfo)
      applyGrokOAuthUpstreamConfig(credentials)
      const extra = grokOAuth.buildExtraInfo(tokenInfo)
      await createAccountAndFinish('grok', 'oauth', credentials, extra)
    } catch (error: any) {
      grokOAuth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      notifications.showError(grokOAuth.error.value)
    } finally {
      grokOAuth.loading.value = false
    }
  }

  // Anthropic OAuth 授权码兑换
  const handleAnthropicExchange = async (authCode: string) => {
    if (!authCode.trim() || !oauth.sessionId.value) return

    oauth.loading.value = true
    oauth.error.value = ''

    try {
      const tokenInfo = await exchangeAccountAuthCode(addMethod.value, {
        session_id: oauth.sessionId.value,
        code: authCode.trim(),
        ...(form.proxy_id ? { proxy_id: form.proxy_id } : {})
      })

      // Build extra with quota control settings
      const baseExtra = oauth.buildExtraInfo(tokenInfo) || {}
      const extra: Record<string, unknown> = { ...baseExtra }

      // Add window cost limit settings
      if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
        extra.window_cost_limit = windowCostLimit.value
        extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
      }

      // Add session limit settings
      if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
        extra.max_sessions = maxSessions.value
        extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
      }

      // Add RPM limit settings
      if (rpmLimitEnabled.value) {
        const DEFAULT_BASE_RPM = 15
        extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0)
          ? baseRpm.value
          : DEFAULT_BASE_RPM
        extra.rpm_strategy = rpmStrategy.value
        if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) {
          extra.rpm_sticky_buffer = rpmStickyBuffer.value
        }
      }

      // UMQ mode（独立于 RPM）
      if (userMsgQueueMode.value) {
        extra.user_msg_queue_mode = userMsgQueueMode.value
      }

      // Add TLS fingerprint settings
      if (tlsFingerprintEnabled.value) {
        extra.enable_tls_fingerprint = true
        if (tlsFingerprintProfileId.value) {
          extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
        }
      }

      // Add session ID masking settings
      if (sessionIdMaskingEnabled.value) {
        extra.session_id_masking_enabled = true
      }

      // Add cache TTL override settings
      if (cacheTTLOverrideEnabled.value) {
        extra.cache_ttl_override_enabled = true
        extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
      }

      // Add custom base URL settings
      if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
        extra.custom_base_url_enabled = true
        extra.custom_base_url = customBaseUrl.value.trim()
      }

      const credentials: Record<string, unknown> = { ...tokenInfo }
      applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
      await createAccountAndFinish(form.platform, addMethod.value as AccountType, credentials, extra)
    } catch (error: any) {
      oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
      notifications.showError(oauth.error.value)
    } finally {
      oauth.loading.value = false
    }
  }

  // 主入口：根据平台路由到对应处理函数
  const handleExchangeCode = async () => {
    const authCode = oauthFlowRef.value?.authCode || ''

    switch (form.platform) {
      case 'openai':
        return handleOpenAIExchange(authCode)
      case 'gemini':
        return handleGeminiExchange(authCode)
      case 'antigravity':
        return handleAntigravityExchange(authCode)
      case 'grok':
        return handleGrokExchange(authCode)
      default:
        return handleAnthropicExchange(authCode)
    }
  }

  const handleCookieAuth = async (sessionKey: string) => {
    oauth.loading.value = true
    oauth.error.value = ''

    try {
      const keys = oauth.parseSessionKeys(sessionKey)

      if (keys.length === 0) {
        oauth.error.value = t('admin.accounts.oauth.pleaseEnterSessionKey')
        return
      }

      const tempUnschedPayload = tempUnschedEnabled.value
        ? buildTempUnschedRules(tempUnschedRules.value)
        : []
      if (tempUnschedEnabled.value && tempUnschedPayload.length === 0) {
        notifications.showError(t('admin.accounts.tempUnschedulable.rulesInvalid'))
        return
      }

      let successCount = 0
      let failedCount = 0
      const errors: string[] = []

      for (let i = 0; i < keys.length; i++) {
        try {
          const tokenInfo = await authenticateAccountWithCookie(
            addMethod.value,
            keys[i],
            form.proxy_id
          )

          // Build extra with quota control settings
          const baseExtra = oauth.buildExtraInfo(tokenInfo) || {}
          const extra: Record<string, unknown> = { ...baseExtra }

          // Add window cost limit settings
          if (windowCostEnabled.value && windowCostLimit.value != null && windowCostLimit.value > 0) {
            extra.window_cost_limit = windowCostLimit.value
            extra.window_cost_sticky_reserve = windowCostStickyReserve.value ?? 10
          }

          // Add session limit settings
          if (sessionLimitEnabled.value && maxSessions.value != null && maxSessions.value > 0) {
            extra.max_sessions = maxSessions.value
            extra.session_idle_timeout_minutes = sessionIdleTimeout.value ?? 5
          }

          // Add RPM limit settings
          if (rpmLimitEnabled.value) {
            const DEFAULT_BASE_RPM = 15
            extra.base_rpm = (baseRpm.value != null && baseRpm.value > 0)
              ? baseRpm.value
              : DEFAULT_BASE_RPM
            extra.rpm_strategy = rpmStrategy.value
            if (rpmStickyBuffer.value != null && rpmStickyBuffer.value > 0) {
              extra.rpm_sticky_buffer = rpmStickyBuffer.value
            }
          }

          // UMQ mode（独立于 RPM）
          if (userMsgQueueMode.value) {
            extra.user_msg_queue_mode = userMsgQueueMode.value
          }

          // Add TLS fingerprint settings
          if (tlsFingerprintEnabled.value) {
            extra.enable_tls_fingerprint = true
            if (tlsFingerprintProfileId.value) {
              extra.tls_fingerprint_profile_id = tlsFingerprintProfileId.value
            }
          }

          // Add session ID masking settings
          if (sessionIdMaskingEnabled.value) {
            extra.session_id_masking_enabled = true
          }

          // Add cache TTL override settings
          if (cacheTTLOverrideEnabled.value) {
            extra.cache_ttl_override_enabled = true
            extra.cache_ttl_override_target = cacheTTLOverrideTarget.value
          }

          // Add custom base URL settings
          if (customBaseUrlEnabled.value && customBaseUrl.value.trim()) {
            extra.custom_base_url_enabled = true
            extra.custom_base_url = customBaseUrl.value.trim()
          }

          const accountName = keys.length > 1 ? `${form.name} #${i + 1}` : form.name

          const credentials: Record<string, unknown> = { ...tokenInfo }
          applyInterceptWarmup(credentials, interceptWarmupRequests.value, 'create')
          if (tempUnschedEnabled.value) {
            credentials.temp_unschedulable_enabled = true
            credentials.temp_unschedulable_rules = tempUnschedPayload
          }

          await createAccount({
            name: accountName,
            notes: form.notes,
            platform: form.platform,
            type: addMethod.value, // Use addMethod as type: 'oauth' or 'setup-token'
            credentials,
            extra,
            proxy_id: form.proxy_id,
            concurrency: form.concurrency,
            load_factor: form.load_factor ?? undefined,
            priority: form.priority,
            rate_multiplier: form.rate_multiplier,
            group_ids: form.group_ids,
            expires_at: form.expires_at,
            auto_pause_on_expired: autoPauseOnExpired.value
          })

          successCount++
        } catch (error: any) {
          failedCount++
          errors.push(
            t('admin.accounts.oauth.keyAuthFailed', {
              index: i + 1,
              error: error.response?.data?.detail || t('admin.accounts.oauth.authFailed')
            })
          )
        }
      }

      if (successCount > 0) {
        notifications.showSuccess(t('admin.accounts.oauth.successCreated', { count: successCount }))
        if (failedCount === 0) {
          onCreated()
          handleClose()
        } else {
          onCreated()
        }
      }

      if (failedCount > 0) {
        oauth.error.value = errors.join('\n')
      }
    } catch (error: any) {
      oauth.error.value = error.response?.data?.detail || t('admin.accounts.oauth.cookieAuthFailed')
    } finally {
      oauth.loading.value = false
    }
  }

  return {
    createAccountAndFinish,
    goBackToBasicInfo,
    handleCookieAuth,
    handleExchangeCode,
    handleGenerateUrl,
    handleGrokImportSSO,
    handleOpenAIImportCodexPAT,
    handleOpenAIImportCodexSession,
    handleOpenAIValidateMobileRT,
    handleValidateRefreshToken,
    handleValidateSessionToken,
  }
}
