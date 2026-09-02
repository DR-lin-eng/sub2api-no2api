import type { ComputedRef, Ref } from 'vue'
import type { useI18n } from 'vue-i18n'
import type { AccountPlatform } from '@/types'
import type { HeaderOverrideRow } from './credentialsBuilder'
import type { ModelMapping } from './accountFormPolicy'

type Translate = ReturnType<typeof useI18n>['t']
type ModelRestrictionMode = 'whitelist' | 'mapping'
type ModelPreset = {
  color: string
  from: string
  label: string
  to: string
}

export interface BulkEditRoutingPolicyContext {
  addCustomErrorCode: () => void
  addModelMapping: () => void
  addPresetMapping: (from: string, to: string) => void
  allHeaderOverrideCapable: ComputedRef<boolean>
  allowedModels: Ref<string[]>
  allOpenAIPassthroughCapable: ComputedRef<boolean>
  allOpenAIOAuthOnly: ComputedRef<boolean>
  allTargetsGrok: ComputedRef<boolean>
  baseUrl: Ref<string>
  commonErrorCodes: Array<{ value: number; label: string }>
  customErrorCodeInput: Ref<number | null>
  enableBaseUrl: Ref<boolean>
  enableCustomErrorCodes: Ref<boolean>
  enableHeaderOverride: Ref<boolean>
  enableInterceptWarmup: Ref<boolean>
  enableModelRestriction: Ref<boolean>
  enableOpenAIPassthrough: Ref<boolean>
  enableOpenAIFlattenNamespaces: Ref<boolean>
  filteredPresets: ComputedRef<ModelPreset[]>
  headerOverrideEnabled: Ref<boolean>
  headerOverrideRows: Ref<HeaderOverrideRow[]>
  interceptWarmupRequests: Ref<boolean>
  isOpenAIModelRestrictionDisabled: ComputedRef<boolean>
  modelMappings: Ref<ModelMapping[]>
  modelRestrictionMode: Ref<ModelRestrictionMode>
  openaiPassthroughEnabled: Ref<boolean>
  openaiFlattenNamespacesEnabled: Ref<boolean>
  removeErrorCode: (code: number) => void
  removeModelMapping: (index: number) => void
  selectedErrorCodes: Ref<number[]>
  t: Translate
  targetSelectedPlatforms: ComputedRef<AccountPlatform[]>
  toggleErrorCode: (code: number) => void
}

export interface BulkEditCPAContext {
  MAX_CPA_CONCURRENCY_PER_CREDENTIAL: number
  cpaConcurrencyPerCredential: Ref<number>
  cpaExcludeAbnormalCredentials: Ref<boolean>
  cpaManagementPassword: Ref<string>
  cpaManagementUrl: Ref<string>
  cpaModeEnabled: Ref<boolean>
  cpaUseBaseUrl: Ref<boolean>
  enableCPA: Ref<boolean>
  t: Translate
}

export interface BulkEditCapacityContext {
  concurrency: Ref<number>
  enableConcurrency: Ref<boolean>
  enableLoadFactor: Ref<boolean>
  enablePriority: Ref<boolean>
  enableProxy: Ref<boolean>
  enableRateMultiplier: Ref<boolean>
  loadFactor: Ref<number | null>
  priority: Ref<number>
  proxyId: Ref<number | null>
  rateMultiplier: Ref<number>
  t: Translate
}
