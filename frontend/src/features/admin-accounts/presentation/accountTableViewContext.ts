import type { CSSProperties, ComputedRef, Ref } from 'vue'
import type DataTable from '@/common/widgets/data/DataTable.vue'
import type { Column } from '@/common/types/uiTypes'
import type {
  Account,
  AccountSchedulerGroupScore,
  AdminGroup,
  Proxy as AccountProxy,
  UpstreamQuotaQueryResult,
  WindowStats
} from '@/types'
import type { OpenAIQuotaRefreshResult } from '@/features/admin-accounts/data/dtos/openAIQuotaDtos'

type UpstreamFeedback = 'success' | 'error'

interface AccountTableParams extends Record<string, unknown> {
  search: string
}

interface AccountTablePagination {
  page: number
  page_size: number
  total: number
}

interface AccountToolsDropdownPosition {
  top: number | null
  bottom: number | null
  left: number
  width: number
  maxHeight: number
}

export interface AccountTableViewContext {
  params: AccountTableParams
  groups: Ref<AdminGroup[]>
  loading: Ref<boolean>
  debouncedReload: () => void
  handleManualRefresh: () => Promise<void>
  showCreate: Ref<boolean>
  autoRefreshDropdownRef: Ref<HTMLElement | null>
  showAutoRefreshDropdown: Ref<boolean>
  showAccountToolsDropdown: Ref<boolean>
  autoRefreshEnabled: Ref<boolean>
  autoRefreshCountdown: Ref<number>
  autoRefreshIntervals: readonly [5, 10, 15, 30]
  autoRefreshIntervalSeconds: Ref<5 | 10 | 15 | 30>
  autoRefreshIntervalLabel: (seconds: number) => string
  setAutoRefreshEnabled: (enabled: boolean) => void
  setAutoRefreshInterval: (seconds: 5 | 10 | 15 | 30) => void
  accountToolsDropdownRef: Ref<HTMLElement | null>
  accountToolsTriggerRef: Ref<HTMLElement | null>
  accountToolsDropdownStyle: ComputedRef<CSSProperties>
  accountToolsDropdownPosition: AccountToolsDropdownPosition
  toggleAccountToolsDropdown: () => void
  openSyncFromCrs: () => void
  openImportData: () => void
  openExportDataDialogFromMenu: () => void
  openErrorPassthrough: () => void
  openTLSFingerprintProfiles: () => void
  toggleableColumns: ComputedRef<Column[]>
  toggleColumn: (key: string) => void
  isColumnVisible: (key: string) => boolean
  hasPendingListSync: Ref<boolean>
  syncPendingListChanges: () => Promise<void>
  selIds: ComputedRef<number[]>
  selectingAllResults: Ref<boolean>
  allResultsSelected: ComputedRef<boolean>
  bulkQueryingUpstreamQuota: Ref<boolean>
  bulkQueryingOpenAIQuota: Ref<boolean>
  handleBulkDelete: () => Promise<void>
  handleBulkResetStatus: () => Promise<void>
  handleBulkRefreshToken: () => Promise<void>
  handleBulkQueryUpstreamQuota: () => Promise<void>
  handleBulkQueryOpenAIQuota: () => Promise<void>
  handleBulkProbeUpstreamBilling: () => Promise<void>
  openBulkEditSelected: () => void
  openBulkEditFiltered: () => Promise<void>
  clearSelection: () => void
  selectPage: () => void
  handleSelectAllResults: () => Promise<void>
  handleBulkToggleSchedulable: (schedulable: boolean) => Promise<void>
  accountTableRef: Ref<HTMLElement | null>
  dataTableRef: Ref<InstanceType<typeof DataTable> | null>
  cols: ComputedRef<Column[]>
  accounts: Ref<Account[]>
  accountSortStorageKey: string
  handleSort: (key: string, order: 'asc' | 'desc') => void
  pagination: AccountTablePagination
  handlePageChange: (page: number) => void
  handlePageSizeChange: (pageSize: number) => void
  allVisibleSelected: ComputedRef<boolean>
  toggleSelectAllVisible: (event: Event) => void
  isSelected: (accountID: number) => boolean
  toggleSel: (accountID: number) => void
  accountHomepageUrl: (account: Account) => string
  accountDisplayEmail: (account: Account) => string
  getOpenAIAuthMode: (account: Account) => string | undefined
  getAccountPlanType: (account: Account) => string | undefined
  getAntigravityTierLabel: (account: Account) => string | null
  getAntigravityTierClass: (account: Account) => string
  getOpenAICompactMeta: (account: Account) => { label: string; className: string; dotClass: string } | null
  getOpenAICompactTitle: (account: Account) => string
  handleShowTempUnsched: (account: Account) => void
  togglingSchedulable: Ref<number | null>
  handleToggleSchedulable: (account: Account) => Promise<void>
  handleAccountUpdated: (account: Account) => void
  todayStatsByAccountId: Ref<Record<string, WindowStats>>
  todayStatsLoading: Ref<boolean>
  todayStatsError: Ref<string | null>
  usageManualRefreshToken: Ref<number>
  bulkOpenAIQuotaResults: Map<number, OpenAIQuotaRefreshResult>
  upstreamQuotaResults: Map<number, UpstreamQuotaQueryResult>
  upstreamBillingNow: Ref<number>
  upstreamBillingProbeGloballyEnabled: Ref<boolean | undefined>
  probingUpstreamBilling: Set<number>
  upstreamQuotaErrors: Map<number, string>
  queryingUpstreamQuota: Map<number, symbol>
  upstreamBillingFeedback: Map<number, UpstreamFeedback>
  upstreamQuotaFeedback: Map<number, UpstreamFeedback>
  handleProbeUpstreamBilling: (account: Account) => Promise<void>
  handleQueryUpstreamQuota: (account: Account) => Promise<boolean>
  proxyExpiryBadge: (proxy: AccountProxy) => string
  proxyExpiryText: (proxy: AccountProxy) => string
  onRevertFallback: (account: Account) => Promise<void>
  getSchedulerScoreRows: (account: Account) => AccountSchedulerGroupScore[]
  formatSchedulerScoreGroup: (score: AccountSchedulerGroupScore) => string
  formatSchedulerScore: (value: unknown) => string
  formatStickySchedulerScore: (score: AccountSchedulerGroupScore) => string
  formatExpiresAt: (value: number | null) => string
  isExpired: (value: number | null) => boolean
  handleEdit: (account: Account) => void
  handleDelete: (account: Account) => void
  openMenu: (account: Account, event: MouseEvent) => void
}
