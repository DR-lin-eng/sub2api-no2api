import type { ComputedRef, Ref, WritableComputedRef } from 'vue'
import type { Column } from '@/common/types/uiTypes'
import type {
  Proxy,
  ProxyAccountSummary,
  ProxyAutoAssignmentSettings,
  ProxyProtocol,
  ProxyQualityCheckResult
} from '@/types/gateway'

export interface ProxySelectOption {
  value: string | number | boolean | null
  label: string
  [key: string]: unknown
}

export interface ProxyFormState {
  name: string
  protocol: ProxyProtocol
  host: string
  port: number
  username: string
  password: string
  expires_at: string
  fallback_mode: 'none' | 'proxy' | 'direct'
  backup_proxy_id: number | null
  expiry_warn_days: number
}

export interface EditProxyFormState extends ProxyFormState {
  status: 'active' | 'inactive' | 'expired'
}

export interface ParsedProxyBatchState {
  total: number
  valid: number
  invalid: number
  duplicate: number
  proxies: Array<{
    protocol: ProxyProtocol
    host: string
    port: number
    username: string
    password: string
  }>
}

export interface ProxyCopyFormat {
  label: string
  value: string
}

export interface ProxyTableContext {
  columns: ComputedRef<Column[]>
  proxies: Ref<Proxy[]>
  loading: Ref<boolean>
  allVisibleSelected: ComputedRef<boolean>
  selectedProxyIds: Ref<Set<number>>
  visiblePasswordIds: Set<number>
  copyMenuProxyId: Ref<number | null>
  testingProxyIds: Ref<Set<number>>
  qualityCheckingProxyIds: Ref<Set<number>>
  showCreateModal: Ref<boolean>
  handleSort: (key: string, order: 'asc' | 'desc') => void
  toggleSelectAllVisible: (event: Event) => void
  toggleSelectRow: (id: number, event: Event) => void
  copyProxyUrl: (proxy: Proxy) => void
  toggleCopyMenu: (id: number) => void
  getCopyFormats: (proxy: Proxy) => ProxyCopyFormat[]
  copyFormat: (value: string) => void
  formatLocation: (proxy: Proxy) => string
  flagUrl: (countryCode: string) => string
  openAccountsModal: (proxy: Proxy) => void | Promise<void>
  qualityOverallClass: (status?: string) => string
  qualityOverallLabel: (status?: string) => string
  expiryLabel: (proxy: Proxy) => string
  expiryBadgeClass: (proxy: Proxy) => string
  handleTestConnection: (proxy: Proxy) => void | Promise<void>
  handleQualityCheck: (proxy: Proxy) => void | Promise<void>
  handleEdit: (proxy: Proxy) => void
  handleDelete: (proxy: Proxy) => void
}

export interface CreateProxyDialogContext {
  showCreateModal: Ref<boolean>
  createPasswordVisible: Ref<boolean>
  createMode: Ref<'standard' | 'batch'>
  batchInput: Ref<string>
  batchParseResult: ParsedProxyBatchState
  createForm: ProxyFormState
  submitting: Ref<boolean>
  protocolSelectOptions: ComputedRef<ProxySelectOption[]>
  createExpiresDays: WritableComputedRef<number | null>
  EXPIRY_PRESETS: readonly number[]
  closeCreateModal: () => void
  handleCreateProxy: () => void | Promise<void>
  handleBatchCreate: () => void | Promise<void>
  parseBatchInput: () => void
  addDaysToBase: (baseDate: string, days: number | null) => string
  backupProxyOptions: (excludeId?: number) => ProxySelectOption[]
}

export interface EditProxyDialogContext {
  showEditModal: Ref<boolean>
  editingProxy: Ref<Proxy | null>
  editPasswordVisible: Ref<boolean>
  editPasswordDirty: Ref<boolean>
  editForm: EditProxyFormState
  submitting: Ref<boolean>
  protocolSelectOptions: ComputedRef<ProxySelectOption[]>
  editStatusOptions: ComputedRef<ProxySelectOption[]>
  editExpiresDays: WritableComputedRef<number | null>
  editBaseDate: ComputedRef<string>
  EXPIRY_PRESETS: readonly number[]
  closeEditModal: () => void
  handleUpdateProxy: () => void | Promise<void>
  addDaysToBase: (baseDate: string, days: number | null) => string
  backupProxyOptions: (excludeId?: number) => ProxySelectOption[]
}

export interface ProxyPageDialogsContext {
  showDeleteDialog: Ref<boolean>
  deletingProxy: Ref<Proxy | null>
  showBatchDeleteDialog: Ref<boolean>
  showExportDataDialog: Ref<boolean>
  showImportData: Ref<boolean>
  selectedCount: ComputedRef<number>
  confirmDelete: () => void | Promise<void>
  confirmBatchDelete: () => void | Promise<void>
  handleExportData: () => void | Promise<void>
  handleDataImported: () => void
  showQualityReportDialog: Ref<boolean>
  qualityReportProxy: Ref<Proxy | null>
  qualityReport: Ref<ProxyQualityCheckResult | null>
  closeQualityReportDialog: () => void
  qualityStatusClass: (status: string) => string
  qualityStatusLabel: (status: string) => string
  qualityTargetLabel: (target: string) => string
  showAccountsModal: Ref<boolean>
  accountsProxy: Ref<Proxy | null>
  proxyAccounts: Ref<ProxyAccountSummary[]>
  accountsLoading: Ref<boolean>
  closeAccountsModal: () => void
}

export interface ProxyAutoAssignmentDialogContext {
  showAutoAssignmentDialog: Ref<boolean>
  autoAssignmentForm: ProxyAutoAssignmentSettings
  autoAssignmentLoading: Ref<boolean>
  autoAssignmentSaving: Ref<boolean>
  autoAssignmentRebalancing: Ref<boolean>
  closeAutoAssignmentDialog: () => void
  saveAutoAssignmentSettings: () => void | Promise<void>
  rebalanceAutoAssignments: () => void | Promise<void>
}
