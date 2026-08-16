import type { ComputedRef, Ref } from 'vue'
import type { Column } from '@/common/types/uiTypes'
import type { PublicSettings } from '@/types/common'
import type {
  ApiKey,
  ApiKeyGroupBinding,
  GroupPlatform,
  SubscriptionType
} from '@/types/gateway'

export interface GroupOption {
  value: number
  label: string
  description: string | null
  rate: number
  userRate: number | null
  peakRateEnabled: boolean
  peakStart: string
  peakEnd: string
  peakRateMultiplier: number
  subscriptionType: SubscriptionType
  platform: GroupPlatform
  [key: string]: unknown
}

export interface KeySelectOption {
  value: string | number | boolean | null
  label: string
  [key: string]: unknown
}

export interface KeyFormState {
  name: string
  group_id: number | null
  group_bindings: ApiKeyGroupBinding[]
  status: 'active' | 'inactive'
  use_custom_key: boolean
  custom_key: string
  enable_ip_restriction: boolean
  ip_whitelist: string
  ip_blacklist: string
  enable_quota: boolean
  quota: number | null
  concurrency_limit: number
  enable_rate_limit: boolean
  rate_limit_5h: number | null
  rate_limit_1d: number | null
  rate_limit_7d: number | null
  enable_expiration: boolean
  expiration_preset: '7' | '30' | '90' | 'custom'
  expiration_date: string
}

export interface KeysTableContext {
  columns: ComputedRef<Column[]>
  apiKeys: Ref<ApiKey[]>
  loading: Ref<boolean>
  copiedKeyId: Ref<number | null>
  copyToClipboard: (text: string, keyId: number) => void | Promise<void>
  groupOptions: ComputedRef<GroupOption[]>
  manageKeyGroups: (key: ApiKey) => void
  isUsageStatsLoading: (apiKeyId: number) => boolean
  hasUsageStatsError: (apiKeyId: number) => boolean
  pendingUsageAvailable: Ref<boolean>
  usageCost: (
    apiKeyId: number,
    field: 'today_actual_cost' | 'total_actual_cost'
  ) => number
  pendingUsage: (apiKeyId: number) => number
  quotaUsedWithPending: (apiKey: ApiKey) => number
  confirmResetRateLimitFromTable: (key: ApiKey) => void
  formatResetTime: (resetAt: string | null) => string
  publicSettings: Ref<PublicSettings | null>
  openUseKeyModal: (key: ApiKey) => void
  importToCcswitch: (key: ApiKey) => void
  toggleKeyStatus: (key: ApiKey) => void | Promise<void>
  editKey: (key: ApiKey) => void
  confirmDelete: (key: ApiKey) => void
  showCreateModal: Ref<boolean>
  handleSort: (key: string, order: 'asc' | 'desc') => void
}

export interface KeyEditorDialogContext {
  showCreateModal: Ref<boolean>
  showEditModal: Ref<boolean>
  selectedKey: Ref<ApiKey | null>
  formData: Ref<KeyFormState>
  groupOptions: ComputedRef<GroupOption[]>
  customKeyError: ComputedRef<string>
  statusOptions: ComputedRef<KeySelectOption[]>
  submitting: Ref<boolean>
  closeModals: () => void
  handleSubmit: () => void | Promise<void>
  confirmResetQuota: () => void
  confirmResetRateLimit: () => void
  setExpirationDays: (days: number) => void
}
