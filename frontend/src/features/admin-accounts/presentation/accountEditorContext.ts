import type { ComputedRef, Ref, WritableComputedRef } from 'vue'
import type { useI18n } from 'vue-i18n'
import type {
  Account,
  AccountPlatform,
  AccountType,
  AdminGroup,
  OpenAICompactMode,
  OpenAIEndpointCapability,
  OpenAIResponsesMode,
  OllamaCloudUsageState,
  Proxy,
} from '@/types'
import type { QuotaThresholdType } from '@/core/constants/account'
import type { OpenAIWSMode } from '@/core/utils/openaiWsMode'
import type { HeaderOverrideRow } from './credentialsBuilder'
import type {
  CodexFingerprintMode,
  ModelMapping,
  TempUnschedRuleForm,
} from './accountFormPolicy'

type Translate = ReturnType<typeof useI18n>['t']
type AccountCategory = 'oauth-based' | 'apikey' | 'bedrock' | 'service_account'
type AddMethod = 'oauth' | 'setup-token'
type ResetMode = 'rolling' | 'fixed' | null
type RPMStrategy = 'tiered' | 'sticky_exempt'
type ModelRestrictionMode = 'whitelist' | 'mapping'
type CodexImageToolMode = 'inherit' | 'enabled' | 'disabled' | 'block'
type SelectOption<Value = string> = { value: Value; label: string }
type ModelPreset = { label: string; from: string; to: string; color: string }
type QuotaNotifyState = Record<
  'daily' | 'weekly' | 'total',
  {
    enabled: boolean | null
    threshold: number | null
    thresholdType: QuotaThresholdType | null
  }
>

export interface CreateAccountFormState {
  name: string
  notes: string
  platform: AccountPlatform
  type: AccountType
  credentials: Record<string, unknown>
  proxy_id: number | null
  concurrency: number
  load_factor: number | null
  priority: number
  rate_multiplier: number
  group_ids: number[]
  expires_at: number | null
}

interface SharedMappingActions {
  addModelMapping: () => void
  addPresetMapping: (from: string, to: string) => void
  removeModelMapping: (index: number) => void
}

interface SharedTempUnschedContext {
  addTempUnschedRule: (preset?: TempUnschedRuleForm) => void
  getTempUnschedRuleKey: (rule: TempUnschedRuleForm) => string
  moveTempUnschedRule: (index: number, direction: number) => void
  removeTempUnschedRule: (index: number) => void
  tempUnschedEnabled: Ref<boolean>
  tempUnschedPresets: ComputedRef<Array<{ label: string; rule: TempUnschedRuleForm }>>
  tempUnschedRules: Ref<TempUnschedRuleForm[]>
}

interface SharedOpenAIOptionsContext {
  codexFingerprintMode: Ref<CodexFingerprintMode>
  codexFingerprintModeOptions: ComputedRef<Array<SelectOption<CodexFingerprintMode>>>
  codexPrewarmContinuationEnabled: Ref<boolean>
  codexThinkingTagNormalizationEnabled: Ref<boolean>
  openAICompactMode: Ref<OpenAICompactMode>
  openAICompactModeOptions: ComputedRef<Array<SelectOption<OpenAICompactMode>>>
  openAICompactModelMappings: Ref<ModelMapping[]>
  addOpenAICompactModelMapping: () => void
  getOpenAICompactModelMappingKey: (mapping: ModelMapping) => string
  removeOpenAICompactModelMapping: (index: number) => void
  openAIEndpointCapabilities: Ref<OpenAIEndpointCapability[]>
  openAIEndpointCapabilityOptions: ComputedRef<Array<SelectOption<OpenAIEndpointCapability>>>
  openAIResponsesMode: Ref<OpenAIResponsesMode>
  openAIResponsesModeOptions: ComputedRef<Array<SelectOption<OpenAIResponsesMode>>>
  openAITextGenerationCapabilityEnabled: ComputedRef<boolean>
  openAIWSModeConcurrencyHintKey: ComputedRef<string>
  openAIWSModeOptions: ComputedRef<Array<SelectOption<OpenAIWSMode>>>
  openaiPassthroughEnabled: Ref<boolean>
  openaiFlattenNamespacesEnabled: Ref<boolean>
  openaiResponsesWebSocketV2Mode: WritableComputedRef<OpenAIWSMode>
  toggleOpenAIEndpointCapability: (
    capability: OpenAIEndpointCapability,
    event?: Event,
  ) => void
}

export interface CreateAccountPlatformContext {
  VERTEX_LOCATION_OPTIONS: typeof import('@/core/constants/account').VERTEX_LOCATION_OPTIONS
  accountCategory: Ref<AccountCategory>
  addAntigravityModelMapping: () => void
  addAntigravityPresetMapping: (from: string, to: string) => void
  antigravityAccountType: Ref<'oauth' | 'upstream'>
  antigravityModelMappings: Ref<ModelMapping[]>
  antigravityPresetMappings: ComputedRef<ModelPreset[]>
  antigravityProjectId: Ref<string>
  form: CreateAccountFormState
  geminiAIStudioOAuthEnabled: Ref<boolean>
  geminiHelpLinks: Record<string, string>
  geminiOAuthType: Ref<'code_assist' | 'google_one' | 'ai_studio'>
  geminiTierAIStudio: Ref<'aistudio_free' | 'aistudio_paid'>
  geminiTierGcp: Ref<'gcp_standard' | 'gcp_enterprise'>
  geminiTierGoogleOne: Ref<'google_one_free' | 'google_ai_pro' | 'google_ai_ultra'>
  getAntigravityModelMappingKey: (mapping: ModelMapping) => string
  handleSelectGeminiOAuthType: (
    oauthType: 'code_assist' | 'google_one' | 'ai_studio',
  ) => void
  handleVertexServiceAccountDrop: (event: DragEvent) => Promise<void>
  handleVertexServiceAccountFile: (event: Event) => Promise<void>
  isGrokSSOInputMethod: ComputedRef<boolean>
  isValidWildcardPattern: (pattern: string) => boolean
  removeAntigravityModelMapping: (index: number) => void
  showAdvancedOAuth: Ref<boolean>
  showGeminiHelpDialog: Ref<boolean>
  t: Translate
  upstreamApiKey: Ref<string>
  upstreamBaseUrl: Ref<string>
  vertexClientEmail: Ref<string>
  vertexLocation: Ref<string>
  vertexProjectId: Ref<string>
  vertexServiceAccountDragActive: Ref<boolean>
  vertexServiceAccountFileInput: Ref<HTMLInputElement | null>
}

export interface CreateAccountCredentialContext extends SharedMappingActions {
  DEFAULT_POOL_MODE_RETRY_COUNT: number
  DEFAULT_POOL_MODE_RETRY_STATUS_CODES: readonly number[]
  MAX_POOL_MODE_RETRY_COUNT: number
  accountCategory: Ref<AccountCategory>
  addCustomErrorCode: () => void
  addMethod: Ref<AddMethod>
  allowedModels: Ref<string[]>
  apiKeyBaseUrl: Ref<string>
  apiKeyHint: ComputedRef<string>
  apiKeyValue: Ref<string>
  autoDisableOnUpstreamInsufficientBalance: Ref<boolean>
  baseUrlHint: ComputedRef<string>
  bedrockAccessKeyId: Ref<string>
  bedrockApiKeyValue: Ref<string>
  bedrockAuthMode: Ref<'sigv4' | 'apikey'>
  bedrockForceGlobal: Ref<boolean>
  bedrockPresets: ComputedRef<ModelPreset[]>
  bedrockRegion: Ref<string>
  bedrockSecretAccessKey: Ref<string>
  bedrockSessionToken: Ref<string>
  commonErrorCodes: Array<{ value: number; label: string }>
  customErrorCodeInput: Ref<number | null>
  customErrorCodesEnabled: Ref<boolean>
  editDailyResetHour: Ref<number | null>
  editDailyResetMode: Ref<ResetMode>
  editQuotaDailyLimit: Ref<number | null>
  editQuotaLimit: Ref<number | null>
  editQuotaWeeklyLimit: Ref<number | null>
  editResetTimezone: Ref<string | null>
  editWeeklyResetDay: Ref<number | null>
  editWeeklyResetHour: Ref<number | null>
  editWeeklyResetMode: Ref<ResetMode>
  form: CreateAccountFormState
  geminiTierAIStudio: Ref<'aistudio_free' | 'aistudio_paid'>
  getModelMappingKey: (mapping: ModelMapping) => string
  grokOAuthBaseUrl: Ref<string>
  grokOAuthCustomBaseUrlEnabled: Ref<boolean>
  headerOverrideEnabled: Ref<boolean>
  headerOverrideRows: Ref<HeaderOverrideRow[]>
  isHeaderOverrideCapable: (platform: string, type: string) => boolean
  isOAuthFlow: ComputedRef<boolean>
  isOpenAIModelRestrictionDisabled: ComputedRef<boolean>
  modelMappings: Ref<ModelMapping[]>
  modelRestrictionMode: Ref<ModelRestrictionMode>
  poolModeEnabled: Ref<boolean>
  poolModeRetryCount: Ref<number>
  poolModeRetryStatusCodesInput: Ref<string>
  presetMappings: ComputedRef<ModelPreset[]>
  quotaNotifyGlobalEnabled: Ref<boolean>
  quotaNotifyState: QuotaNotifyState
  removeErrorCode: (code: number) => void
  selectedErrorCodes: Ref<number[]>
  syncPreviewCredentials: ComputedRef<
    | {
        platform: AccountPlatform
        type: AccountType
        base_url: string | undefined
        api_key: string
      }
    | undefined
  >
  t: Translate
  toggleErrorCode: (code: number) => void
  upstreamBillingAutoProbeEnabled: Ref<boolean>
}

export interface CreateAccountAdvancedContext
  extends SharedTempUnschedContext,
    SharedOpenAIOptionsContext {
  accountCategory: Ref<AccountCategory>
  allowOverages: Ref<boolean>
  anthropicAPIKeyAuthScheme: Ref<'x_api_key' | 'authorization_bearer'>
  anthropicPassthroughEnabled: Ref<boolean>
  isSimpleMode: ComputedRef<boolean>
  autoPauseOnExpired: Ref<boolean>
  baseRpm: Ref<number | null>
  cacheTTLOverrideEnabled: Ref<boolean>
  cacheTTLOverrideTarget: Ref<string>
  codexCLIOnlyAppServerEnabled: Ref<boolean>
  codexCLIOnlyEnabled: Ref<boolean>
  customBaseUrl: Ref<string>
  customBaseUrlEnabled: Ref<boolean>
  expiresAtInput: WritableComputedRef<string>
  form: CreateAccountFormState
  groups: ComputedRef<AdminGroup[]>
  interceptWarmupRequests: Ref<boolean>
  maxSessions: Ref<number | null>
  mixedScheduling: Ref<boolean>
  openAIForceImageAPIEnabled: Ref<boolean>
  openAILongContextBillingEnabled: Ref<boolean>
  proxies: ComputedRef<Proxy[]>
  rpmLimitEnabled: Ref<boolean>
  rpmStickyBuffer: Ref<number | null>
  rpmStrategy: Ref<RPMStrategy>
  sessionIdMaskingEnabled: Ref<boolean>
  sessionIdleTimeout: Ref<number | null>
  sessionLimitEnabled: Ref<boolean>
  t: Translate
  tlsFingerprintEnabled: Ref<boolean>
  tlsFingerprintProfileId: Ref<number | null>
  tlsFingerprintProfiles: Ref<Array<{ id: number; name: string }>>
  toggleOpenAILongContextBilling: () => void
  umqModeOptions: ComputedRef<Array<SelectOption<string>>>
  userMsgQueueMode: Ref<string>
  webSearchEmulationMode: Ref<string>
  webSearchGlobalEnabled: Ref<boolean>
  windowCostEnabled: Ref<boolean>
  windowCostLimit: Ref<number | null>
  windowCostStickyReserve: Ref<number | null>
}

export interface CreateAccountFooterContext {
  canExchangeCode: ComputedRef<boolean | ''>
  currentOAuthLoading: ComputedRef<boolean>
  goBackToBasicInfo: () => void
  handleClose: () => void
  handleExchangeCode: () => Promise<void>
  isManualInputMethod: ComputedRef<boolean>
  isOAuthFlow: ComputedRef<boolean>
  step: Ref<number>
  submitting: Ref<boolean>
  t: Translate
}

export interface CreateAccountStepIndicatorContext {
  isOAuthFlow: ComputedRef<boolean>
  oauthStepTitle: ComputedRef<string>
  step: Ref<number>
  t: Translate
}

export interface GeminiAccountHelpContext {
  geminiHelpLinks: Record<string, string>
  geminiQuotaDocs: Record<string, string>
  showGeminiHelpDialog: Ref<boolean>
  t: Translate
}

export interface EditAccountFormState {
  name: string
  notes: string
  status: Account['status']
  proxy_id: number | null
  concurrency: number
  load_factor: number | null
  priority: number
  rate_multiplier: number
  group_ids: number[]
  expires_at: number | null
}

export interface EditAccountCredentialContext extends SharedMappingActions {
  CPA_SNAPSHOT_INTERVAL_SECONDS: number
  DEFAULT_POOL_MODE_RETRY_COUNT: number
  DEFAULT_POOL_MODE_RETRY_STATUS_CODES: readonly number[]
  MAX_CPA_CONCURRENCY_PER_CREDENTIAL: number
  MAX_POOL_MODE_RETRY_COUNT: number
  VERTEX_LOCATION_OPTIONS: typeof import('@/core/constants/account').VERTEX_LOCATION_OPTIONS
  account: ComputedRef<Account>
  addAntigravityModelMapping: () => void
  addAntigravityPresetMapping: (from: string, to: string) => void
  addCustomErrorCode: () => void
  allowedModels: Ref<string[]>
  antigravityModelMappings: Ref<ModelMapping[]>
  antigravityPresetMappings: ComputedRef<ModelPreset[]>
  antigravityProjectId: Ref<string>
  autoDisableOnUpstreamInsufficientBalance: Ref<boolean>
  baseUrlHint: ComputedRef<string>
  bedrockPresets: ComputedRef<ModelPreset[]>
  commonErrorCodes: Array<{ value: number; label: string }>
  cpaConcurrencyPerCredential: Ref<number>
  cpaExcludeAbnormalCredentials: Ref<boolean>
  cpaManagementKey: Ref<string>
  cpaManagementUrl: Ref<string>
  cpaModeEnabled: Ref<boolean>
  cpaUseBaseUrl: Ref<boolean>
  customErrorCodeInput: Ref<number | null>
  customErrorCodesEnabled: Ref<boolean>
  editApiKey: Ref<string>
  editBaseUrl: Ref<string>
  editBedrockAccessKeyId: Ref<string>
  editBedrockApiKeyValue: Ref<string>
  editBedrockForceGlobal: Ref<boolean>
  editBedrockRegion: Ref<string>
  editBedrockSecretAccessKey: Ref<string>
  editBedrockSessionToken: Ref<string>
  editVertexLocation: Ref<string>
  editVertexProjectId: Ref<string>
  form: EditAccountFormState
  getAntigravityModelMappingKey: (mapping: ModelMapping) => string
  getModelMappingKey: (mapping: ModelMapping) => string
  grokClientToolCacheEnabled: Ref<boolean>
  grokOAuthBaseUrl: Ref<string>
  grokOAuthCustomBaseUrlEnabled: Ref<boolean>
  headerOverrideCapable: ComputedRef<boolean>
  headerOverrideEnabled: Ref<boolean>
  headerOverrideRows: Ref<HeaderOverrideRow[]>
  isBedrockAPIKeyMode: ComputedRef<boolean>
  isOpenAIModelRestrictionDisabled: ComputedRef<boolean>
  isSyncingAntigravityUpstream: Ref<boolean>
  isTestingCPA: Ref<boolean>
  isValidWildcardPattern: (pattern: string) => boolean
  modelMappings: Ref<ModelMapping[]>
  modelRestrictionMode: Ref<ModelRestrictionMode>
  poolModeEnabled: Ref<boolean>
  poolModeRetryCount: Ref<number>
  poolModeRetryStatusCodesInput: Ref<string>
  presetMappings: ComputedRef<ModelPreset[]>
  removeAntigravityModelMapping: (index: number) => void
  removeErrorCode: (code: number) => void
  selectedErrorCodes: Ref<number[]>
  syncAntigravityUpstreamModels: () => Promise<void>
  testCPAConnection: () => Promise<void>
  t: Translate
  toggleErrorCode: (code: number) => void
}

export interface EditAccountAdvancedContext
  extends SharedTempUnschedContext,
    Omit<
      SharedOpenAIOptionsContext,
      | 'addOpenAICompactModelMapping'
      | 'getOpenAICompactModelMappingKey'
      | 'openAICompactMode'
      | 'openAICompactModeOptions'
      | 'openAICompactModelMappings'
      | 'removeOpenAICompactModelMapping'
    > {
  account: ComputedRef<Account>
  anthropicAPIKeyAuthScheme: Ref<'x_api_key' | 'authorization_bearer'>
  anthropicPassthroughEnabled: Ref<boolean>
  codexCLIOnlyAppServerEnabled: Ref<boolean>
  codexCLIOnlyEnabled: Ref<boolean>
  codexThinkingTagNormalizationEnabled: Ref<boolean>
  codexImageToolBadgeClass: ComputedRef<string>
  codexImageToolBadgeLabel: ComputedRef<string>
  codexImageToolMode: Ref<CodexImageToolMode>
  codexImageToolOptions: ComputedRef<
    Array<
      SelectOption<CodexImageToolMode> & {
        description: string
        selectedCardClass: string
        selectedDotClass: string
      }
    >
  >
  codexWebSearchEnabled: Ref<boolean>
  editDailyResetHour: Ref<number | null>
  editDailyResetMode: Ref<ResetMode>
  editQuotaDailyLimit: Ref<number | null>
  editQuotaLimit: Ref<number | null>
  editQuotaWeeklyLimit: Ref<number | null>
  editResetTimezone: Ref<string | null>
  editWeeklyResetDay: Ref<number | null>
  editWeeklyResetHour: Ref<number | null>
  editWeeklyResetMode: Ref<ResetMode>
  expiresAtInput: WritableComputedRef<string>
  form: EditAccountFormState
  handleOllamaCloudUsageUpdated: (state: OllamaCloudUsageState) => void
  interceptWarmupRequests: Ref<boolean>
  isOpenAIPersonalAccessTokenAccount: ComputedRef<boolean>
  isSparkShadow: ComputedRef<boolean>
  openAIForceImageAPIEnabled: Ref<boolean>
  openAILongContextBillingEnabled: Ref<boolean>
  openAIResponsesStatusKey: ComputedRef<string>
  proxies: ComputedRef<Proxy[]>
  quotaNotifyGlobalEnabled: Ref<boolean>
  quotaNotifyState: QuotaNotifyState
  t: Translate
  upstreamBillingAutoProbeEnabled: Ref<boolean>
  upstreamBillingRateSyncEnabled: Ref<boolean>
  webSearchEmulationMode: Ref<string>
  webSearchGlobalEnabled: Ref<boolean>
}

export interface EditAccountPolicyContext {
  account: ComputedRef<Account>
  addOpenAICompactModelMapping: () => void
  allowOverages: Ref<boolean>
  isSimpleMode: ComputedRef<boolean>
  autoPause5hDisabled: Ref<boolean>
  autoPause5hThreshold: Ref<number | null>
  autoPause7dDisabled: Ref<boolean>
  autoPause7dThreshold: Ref<number | null>
  autoPauseOnExpired: Ref<boolean>
  baseRpm: Ref<number | null>
  cacheTTLOverrideEnabled: Ref<boolean>
  cacheTTLOverrideTarget: Ref<string>
  customBaseUrl: Ref<string>
  customBaseUrlEnabled: Ref<boolean>
  editPlanType: Ref<string>
  form: EditAccountFormState
  formatDateTime: (value: string | Date | null | undefined) => string
  getOpenAICompactModelMappingKey: (mapping: ModelMapping) => string
  groups: ComputedRef<AdminGroup[]>
  isSparkShadow: ComputedRef<boolean>
  maxSessions: Ref<number | null>
  mixedScheduling: Ref<boolean>
  openAICompactMode: Ref<OpenAICompactMode>
  openAICompactModeOptions: ComputedRef<Array<SelectOption<OpenAICompactMode>>>
  openAICompactModelMappings: Ref<ModelMapping[]>
  openAICompactStatusKey: ComputedRef<string>
  planTypeOptions: ComputedRef<Array<SelectOption<string>>>
  removeOpenAICompactModelMapping: (index: number) => void
  rpmLimitEnabled: Ref<boolean>
  rpmStickyBuffer: Ref<number | null>
  rpmStrategy: Ref<RPMStrategy>
  sessionIdMaskingEnabled: Ref<boolean>
  sessionIdleTimeout: Ref<number | null>
  sessionLimitEnabled: Ref<boolean>
  statusOptions: ComputedRef<Array<SelectOption<Account['status']>>>
  t: Translate
  tlsFingerprintEnabled: Ref<boolean>
  tlsFingerprintProfileId: Ref<number | null>
  tlsFingerprintProfiles: Ref<Array<{ id: number; name: string }>>
  umqModeOptions: ComputedRef<Array<SelectOption<string>>>
  userMsgQueueMode: Ref<string>
  windowCostEnabled: Ref<boolean>
  windowCostLimit: Ref<number | null>
  windowCostStickyReserve: Ref<number | null>
}
