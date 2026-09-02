import { reactive, ref, toRaw, watch, type Ref } from 'vue'
import { useIntervalFn } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import {
  getById,
  getUpstreamBillingRatesWithEtag
} from '@/features/admin-accounts/data/datasources/adminAccountQueries'
import {
  probeUpstreamBilling,
  probeUpstreamBillingBatch,
  refreshOpenAIQuotaBatch,
  queryUpstreamQuota
} from '@/features/admin-accounts/data/datasources/adminAccountActions'
import type { OpenAIQuotaRefreshResult } from '@/features/admin-accounts/data/dtos/openAIQuotaDtos'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import {
  persistUpstreamQuotaCache,
  readUpstreamQuotaCache,
  removeUpstreamQuotaCache,
  upstreamQuotaCacheIdentity
} from '@/core/utils/upstreamQuotaCache'
import type {
  Account,
  UpstreamBillingProbeResult,
  UpstreamBillingProbeSnapshot,
  UpstreamQuotaQueryResult
} from '@/types'

type UpstreamActionFeedback = 'success' | 'error'

interface AccountPaginationState {
  page: number
  page_size: number
  total: number
}

interface AccountSortState {
  sort_by: string
  sort_order: 'asc' | 'desc'
}

interface AccountsUpstreamBillingOptions {
  currentAdminID: () => number | null
  getAccounts: () => Account[]
  setAccounts: (accounts: Account[]) => void
  getSelectedAccountIDs: () => number[]
  getPagination: () => AccountPaginationState
  getParams: () => Record<string, unknown>
  getSortState: () => AccountSortState
  isLoading: () => boolean
  isAnyModalOpen: () => boolean
  isActionMenuOpen: () => boolean
  isToolsDropdownOpen: () => boolean
  isAutoRefreshDropdownOpen: () => boolean
  syncAccountListDerivedParams: () => void
  loadAccounts: (options?: { refreshTodayStats?: boolean }) => Promise<void>
  syncAccountRefs: (account: Account) => void
  patchAccountInList: (account: Account) => void
  showError: (message: string) => void
  showSuccess: (message: string) => void
  showProgress: (message: string) => string
  hideProgress: (toastID: string) => void
}

const UPSTREAM_BILLING_PROBE_BATCH_SIZE = 20
const UPSTREAM_QUOTA_QUERY_BATCH_SIZE = 4
const OPENAI_QUOTA_BATCH_SIZE = 20

export function useAccountsUpstreamBilling(options: AccountsUpstreamBillingOptions) {
  const { t } = useI18n()
  const probingUpstreamBilling = reactive(new Set<number>())
  const queryingUpstreamQuota = reactive(new Map<number, symbol>())
  const upstreamQuotaResults = reactive(new Map<number, UpstreamQuotaQueryResult>())
  const upstreamQuotaErrors = reactive(new Map<number, string>())
  const upstreamQuotaStateIdentities = new Map<number, string>()
  let upstreamQuotaGlobalGeneration = 0
  const upstreamQuotaAccountGenerations = new Map<number, number>()
  const bulkQueryingUpstreamQuota = ref(false)
  const bulkQueryingOpenAIQuota = ref(false)
  const bulkOpenAIQuotaResults = reactive(new Map<number, OpenAIQuotaRefreshResult>())
  const upstreamBillingFeedback = reactive(new Map<number, UpstreamActionFeedback>())
  const upstreamQuotaFeedback = reactive(new Map<number, UpstreamActionFeedback>())
  const upstreamBillingFeedbackTimers = new Map<number, ReturnType<typeof setTimeout>>()
  const upstreamQuotaFeedbackTimers = new Map<number, ReturnType<typeof setTimeout>>()
  let upstreamFeedbackDisposed = false

  const clearUpstreamActionFeedback = (
    feedback: Map<number, UpstreamActionFeedback>,
    timers: Map<number, ReturnType<typeof setTimeout>>,
    accountID: number
  ) => {
    const timer = timers.get(accountID)
    if (timer !== undefined) clearTimeout(timer)
    timers.delete(accountID)
    feedback.delete(accountID)
  }

  const showUpstreamActionFeedback = (
    feedback: Map<number, UpstreamActionFeedback>,
    timers: Map<number, ReturnType<typeof setTimeout>>,
    accountID: number,
    value: UpstreamActionFeedback
  ) => {
    clearUpstreamActionFeedback(feedback, timers, accountID)
    if (upstreamFeedbackDisposed) return
    feedback.set(accountID, value)
    timers.set(accountID, setTimeout(() => {
      feedback.delete(accountID)
      timers.delete(accountID)
    }, 1000))
  }

  const removePersistedUpstreamQuota = (accountID?: number) => {
    const adminID = options.currentAdminID()
    if (typeof adminID === 'number') removeUpstreamQuotaCache(localStorage, adminID, accountID)
  }

  const invalidateUpstreamQuotaState = (accountID?: number) => {
    if (accountID == null) {
      upstreamQuotaGlobalGeneration += 1
      upstreamQuotaAccountGenerations.clear()
      queryingUpstreamQuota.clear()
      upstreamQuotaResults.clear()
      bulkOpenAIQuotaResults.clear()
      upstreamQuotaStateIdentities.clear()
      upstreamQuotaErrors.clear()
      upstreamQuotaFeedbackTimers.forEach(timer => clearTimeout(timer))
      upstreamQuotaFeedbackTimers.clear()
      upstreamQuotaFeedback.clear()
      removePersistedUpstreamQuota()
      return
    }
    upstreamQuotaAccountGenerations.set(accountID, (upstreamQuotaAccountGenerations.get(accountID) ?? 0) + 1)
    queryingUpstreamQuota.delete(accountID)
    upstreamQuotaResults.delete(accountID)
    bulkOpenAIQuotaResults.delete(accountID)
    upstreamQuotaStateIdentities.delete(accountID)
    upstreamQuotaErrors.delete(accountID)
    clearUpstreamActionFeedback(upstreamQuotaFeedback, upstreamQuotaFeedbackTimers, accountID)
    removePersistedUpstreamQuota(accountID)
  }

  const upstreamBillingProbeGloballyEnabled = ref<boolean | undefined>(undefined)
  const upstreamBillingNow = ref(Date.now())
  const upstreamBillingRateETag = ref<string | null>(null)
  const upstreamBillingRateRefreshing = ref(false)
  let upstreamBillingRateAbortController: AbortController | null = null
  useIntervalFn(() => { upstreamBillingNow.value = Date.now() }, 60_000)

  let hydratedUpstreamQuotaAdminID: number | null | undefined
  const registerQuotaHydrationWatch = (accounts: Ref<Account[]>) => watch(
    () => ({ accounts: accounts.value, adminID: options.currentAdminID() }),
    ({ accounts: visibleAccounts, adminID }) => {
      if (hydratedUpstreamQuotaAdminID !== adminID) {
        upstreamQuotaGlobalGeneration += 1
        queryingUpstreamQuota.clear()
        upstreamQuotaResults.clear()
        bulkOpenAIQuotaResults.clear()
        upstreamQuotaStateIdentities.clear()
        upstreamQuotaErrors.clear()
        upstreamBillingFeedbackTimers.forEach(timer => clearTimeout(timer))
        upstreamQuotaFeedbackTimers.forEach(timer => clearTimeout(timer))
        upstreamBillingFeedbackTimers.clear()
        upstreamQuotaFeedbackTimers.clear()
        upstreamBillingFeedback.clear()
        upstreamQuotaFeedback.clear()
        hydratedUpstreamQuotaAdminID = adminID
      }
      if (typeof adminID !== 'number') return

      for (const account of visibleAccounts) {
        if (account.platform !== 'openai' || account.type !== 'apikey') {
          upstreamQuotaResults.delete(account.id)
          upstreamQuotaStateIdentities.delete(account.id)
          upstreamQuotaErrors.delete(account.id)
          removeUpstreamQuotaCache(localStorage, adminID, account.id)
          continue
        }

        const identity = upstreamQuotaCacheIdentity(account)
        const previousIdentity = upstreamQuotaStateIdentities.get(account.id)
        if (previousIdentity && previousIdentity !== identity) invalidateUpstreamQuotaState(account.id)
        if (upstreamQuotaResults.has(account.id)) continue

        const cached = readUpstreamQuotaCache(localStorage, adminID, account)
        if (!cached) continue
        upstreamQuotaResults.set(account.id, cached)
        upstreamQuotaStateIdentities.set(account.id, identity)
      }
    },
    { immediate: true }
  )

  const buildUpstreamBillingRateFilters = () => {
    const rawParams = toRaw(options.getParams())
    const sortState = options.getSortState()
    return {
      platform: typeof rawParams.platform === 'string' ? rawParams.platform : '',
      type: typeof rawParams.type === 'string' ? rawParams.type : '',
      status: typeof rawParams.status === 'string' ? rawParams.status : '',
      oauth_quota: typeof rawParams.oauth_quota === 'string' ? rawParams.oauth_quota : '',
      group: typeof rawParams.group === 'string' ? rawParams.group : '',
      search: typeof rawParams.search === 'string' ? rawParams.search : '',
      privacy_mode: typeof rawParams.privacy_mode === 'string' ? rawParams.privacy_mode : '',
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order
    }
  }

  const upstreamBillingRateContextKey = (page?: number, pageSize?: number) => {
    const pagination = options.getPagination()
    return JSON.stringify({
      page: page ?? pagination.page,
      pageSize: pageSize ?? pagination.page_size,
      filters: buildUpstreamBillingRateFilters()
    })
  }

  const sameAccountIDOrder = (left: number[], right: number[]) =>
    left.length === right.length && left.every((id, index) => id === right[index])

  const applyUpstreamBillingRateSnapshots = async (
    result: NonNullable<Awaited<ReturnType<typeof getUpstreamBillingRatesWithEtag>>['data']>
  ) => {
    const pagination = options.getPagination()
    const currentAccounts = options.getAccounts()
    const nextIDs = result.items.map(item => item.account_id)
    const currentIDs = currentAccounts.map(account => account.id)
    if (result.total !== pagination.total || !sameAccountIDOrder(nextIDs, currentIDs)) {
      await options.loadAccounts({ refreshTodayStats: false })
      return
    }

    const itemsByID = new Map(result.items.map(item => [item.account_id, item]))
    let changed = false
    const nextAccounts = currentAccounts.map(account => {
      const item = itemsByID.get(account.id)
      if (!item) return account
      const nextSnapshot = item.snapshot ?? null
      const previousSnapshot = account.extra?.upstream_billing_probe ?? null
      if (JSON.stringify(previousSnapshot) === JSON.stringify(nextSnapshot)) return account

      const nextExtra = { ...(account.extra ?? {}) }
      if (nextSnapshot) nextExtra.upstream_billing_probe = nextSnapshot
      else delete nextExtra.upstream_billing_probe
      const nextAccount = { ...account, extra: nextExtra }
      options.syncAccountRefs(nextAccount)
      changed = true
      return nextAccount
    })
    if (changed) {
      options.setAccounts(nextAccounts)
      upstreamBillingNow.value = Date.now()
    }
  }

  const refreshUpstreamBillingRates = async (force = false) => {
    if (upstreamBillingRateRefreshing.value || options.isLoading()) return
    if (!force && (
      options.getAccounts().length === 0 ||
      probingUpstreamBilling.size > 0 ||
      options.isAnyModalOpen() ||
      options.isActionMenuOpen() ||
      options.isToolsDropdownOpen() ||
      options.isAutoRefreshDropdownOpen() ||
      (typeof document !== 'undefined' && document.hidden)
    )) return

    const controller = new AbortController()
    upstreamBillingRateAbortController = controller
    upstreamBillingRateRefreshing.value = true
    try {
      options.syncAccountListDerivedParams()
      const pagination = options.getPagination()
      const requestPage = pagination.page
      const requestPageSize = pagination.page_size
      const requestContext = upstreamBillingRateContextKey(requestPage, requestPageSize)
      const result = await getUpstreamBillingRatesWithEtag(
        requestPage,
        requestPageSize,
        buildUpstreamBillingRateFilters(),
        { etag: force ? null : upstreamBillingRateETag.value, signal: controller.signal }
      )
      if (options.isLoading() || requestContext !== upstreamBillingRateContextKey()) return
      if (result.etag) upstreamBillingRateETag.value = result.etag
      if (!result.notModified && result.data) await applyUpstreamBillingRateSnapshots(result.data)
    } catch (error) {
      const refreshError = error as { name?: string; code?: string }
      if (refreshError.name !== 'AbortError' &&
        refreshError.name !== 'CanceledError' &&
        refreshError.code !== 'ERR_CANCELED') {
        console.error('Failed to refresh upstream billing rates:', error)
      }
    } finally {
      if (upstreamBillingRateAbortController === controller) upstreamBillingRateAbortController = null
      upstreamBillingRateRefreshing.value = false
    }
  }

  const refreshUpstreamBillingSortedList = async (force = false) => {
    if (!force && options.getSortState().sort_by !== 'upstream_billing_rate') return
    await refreshUpstreamBillingRates(force)
  }

  const refreshAccountsAfterUpstreamBillingProbe = async () => {
    try {
      await options.loadAccounts({ refreshTodayStats: false })
    } catch (error) {
      console.error('Failed to refresh accounts after upstream billing probe:', error)
    }
  }

  const patchUpstreamBillingSnapshot = (accountID: number, snapshot: UpstreamBillingProbeSnapshot) => {
    const account = options.getAccounts().find(item => item.id === accountID)
    if (!account) return
    upstreamBillingNow.value = Date.now()
    options.patchAccountInList({
      ...account,
      extra: { ...account.extra, upstream_billing_probe: snapshot }
    })
  }

  const handleProbeUpstreamBilling = async (account: Account) => {
    if (probingUpstreamBilling.has(account.id)) return
    clearUpstreamActionFeedback(upstreamBillingFeedback, upstreamBillingFeedbackTimers, account.id)
    probingUpstreamBilling.add(account.id)
    let feedback: UpstreamActionFeedback = 'error'
    try {
      const result = await probeUpstreamBilling(account.id)
      if (result.snapshot) {
        patchUpstreamBillingSnapshot(account.id, result.snapshot)
        await refreshAccountsAfterUpstreamBillingProbe()
      }
      feedback = result.snapshot?.status === 'ok' ? 'success' : 'error'
    } catch (error) {
      console.error('Failed to probe upstream billing:', error)
      options.showError(extractApiErrorMessage(error, t('admin.accounts.upstreamBilling.probeFailed')))
    } finally {
      probingUpstreamBilling.delete(account.id)
      showUpstreamActionFeedback(upstreamBillingFeedback, upstreamBillingFeedbackTimers, account.id, feedback)
    }
  }

  const handleQueryUpstreamQuota = async (account: Account): Promise<boolean> => {
    if (queryingUpstreamQuota.has(account.id)) return false
    clearUpstreamActionFeedback(upstreamQuotaFeedback, upstreamQuotaFeedbackTimers, account.id)
    const requestToken = Symbol()
    const adminID = options.currentAdminID()
    const globalGeneration = upstreamQuotaGlobalGeneration
    const accountGeneration = upstreamQuotaAccountGenerations.get(account.id) ?? 0
    const identity = upstreamQuotaCacheIdentity(account)
    queryingUpstreamQuota.set(account.id, requestToken)
    upstreamQuotaStateIdentities.set(account.id, identity)
    upstreamQuotaErrors.delete(account.id)
    const isCurrent = () => (
      adminID === options.currentAdminID() &&
      globalGeneration === upstreamQuotaGlobalGeneration &&
      queryingUpstreamQuota.get(account.id) === requestToken &&
      (upstreamQuotaAccountGenerations.get(account.id) ?? 0) === accountGeneration &&
      upstreamQuotaCacheIdentity(options.getAccounts().find(item => item.id === account.id) ?? account) === identity
    )
    let feedback: UpstreamActionFeedback | null = null
    try {
      const result = await queryUpstreamQuota(account.id)
      if (!isCurrent()) return false
      if (!result.quota) {
        upstreamQuotaErrors.set(account.id, t('admin.accounts.upstreamBilling.noQuotaData'))
        feedback = 'error'
        return false
      }
      upstreamQuotaResults.set(account.id, result)
      upstreamQuotaStateIdentities.set(account.id, identity)
      if (typeof adminID === 'number') persistUpstreamQuotaCache(localStorage, adminID, account, result)
      feedback = 'success'
      return true
    } catch (error) {
      if (isCurrent()) {
        upstreamQuotaErrors.set(
          account.id,
          extractApiErrorMessage(error, t('admin.accounts.upstreamBilling.quotaQueryFailed'))
        )
        feedback = 'error'
      }
      return false
    } finally {
      if (queryingUpstreamQuota.get(account.id) === requestToken) queryingUpstreamQuota.delete(account.id)
      if (feedback) {
        showUpstreamActionFeedback(upstreamQuotaFeedback, upstreamQuotaFeedbackTimers, account.id, feedback)
      }
    }
  }

  const replaceProgressToast = (toastID: string | null, message: string) => {
    if (toastID) options.hideProgress(toastID)
    return options.showProgress(message)
  }

  const handleBulkQueryOpenAIQuota = async () => {
    if (bulkQueryingOpenAIQuota.value) return
    const accountIDs = [...options.getSelectedAccountIDs()]
    if (accountIDs.length === 0) {
      options.showError(t('admin.accounts.bulkActions.noOpenAIOAuthAccounts'))
      return
    }

    const batches: number[][] = []
    for (let start = 0; start < accountIDs.length; start += OPENAI_QUOTA_BATCH_SIZE) {
      batches.push(accountIDs.slice(start, start + OPENAI_QUOTA_BATCH_SIZE))
    }
    let progressToastID: string | null = batches.length > 1
      ? replaceProgressToast(null, t('admin.accounts.bulkActions.openAIQuotaBatchStarted', {
          count: accountIDs.length,
          total: batches.length
        }))
      : null
    let succeeded = 0
    let failed = 0
    let skipped = 0
    bulkQueryingOpenAIQuota.value = true
    try {
      for (let batchIndex = 0; batchIndex < batches.length; batchIndex += 1) {
        const batch = batches[batchIndex]
        try {
          const result = await refreshOpenAIQuotaBatch(batch)
          for (const [accountID, quota] of Object.entries(result.results)) {
            const id = Number(accountID)
            if (Number.isSafeInteger(id) && quota) bulkOpenAIQuotaResults.set(id, quota)
          }
          succeeded += Object.keys(result.results).length
          failed += Object.keys(result.errors).length
          skipped += result.skipped_account_ids.length
        } catch (error) {
          failed += batch.length
          console.error('Failed to query OpenAI quota batch:', error)
        }

        if (progressToastID && batchIndex < batches.length - 1) {
          progressToastID = replaceProgressToast(progressToastID, t('admin.accounts.bulkActions.openAIQuotaBatchProgress', {
            completed: batchIndex + 1,
            next: batchIndex + 2,
            total: batches.length
          }))
        }
      }

      if (succeeded === 0 && failed === 0) {
        options.showError(t('admin.accounts.bulkActions.noOpenAIOAuthAccounts'))
      } else if (failed > 0 || skipped > 0) {
        options.showError(t('admin.accounts.bulkActions.openAIQuotaBatchPartial', { succeeded, skipped, failed }))
      } else {
        options.showSuccess(t('admin.accounts.bulkActions.openAIQuotaBatchCompleted', { count: succeeded }))
      }
    } finally {
      if (progressToastID) options.hideProgress(progressToastID)
      bulkQueryingOpenAIQuota.value = false
    }
  }

  const handleBulkProbeUpstreamBilling = async () => {
    const accountIDs = [...options.getSelectedAccountIDs()]
    if (accountIDs.length === 0) {
      options.showError(t('admin.accounts.upstreamBilling.noEligibleAccounts'))
      return
    }
    const batches: number[][] = []
    for (let start = 0; start < accountIDs.length; start += UPSTREAM_BILLING_PROBE_BATCH_SIZE) {
      batches.push(accountIDs.slice(start, start + UPSTREAM_BILLING_PROBE_BATCH_SIZE))
    }
    let progressToastID: string | null = batches.length > 1
      ? replaceProgressToast(null, t('admin.accounts.upstreamBilling.batchStarted', {
          count: accountIDs.length,
          total: batches.length
        }))
      : null
    accountIDs.forEach(id => probingUpstreamBilling.add(id))
    try {
      let patched = false
      let resultCount = 0
      let failed = 0
      for (let batchIndex = 0; batchIndex < batches.length; batchIndex += 1) {
        const results: UpstreamBillingProbeResult[] = await probeUpstreamBillingBatch(batches[batchIndex])
        resultCount += results.length
        failed += results.filter(result => result.error).length
        results.forEach(result => {
          if (result.snapshot) {
            patchUpstreamBillingSnapshot(result.account_id, result.snapshot)
            patched = true
          }
        })
        if (progressToastID && batchIndex < batches.length - 1) {
          progressToastID = replaceProgressToast(progressToastID, t('admin.accounts.upstreamBilling.batchProgress', {
            completed: batchIndex + 1,
            next: batchIndex + 2,
            total: batches.length
          }))
        }
      }
      if (patched) await refreshAccountsAfterUpstreamBillingProbe()
      if (failed > 0) {
        options.showError(t('admin.accounts.upstreamBilling.batchPartial', { success: resultCount - failed, failed }))
      } else {
        options.showSuccess(t('admin.accounts.upstreamBilling.batchCompleted', { count: resultCount }))
      }
    } catch (error) {
      console.error('Failed to probe upstream billing in batch:', error)
      options.showError(extractApiErrorMessage(error, t('admin.accounts.upstreamBilling.probeFailed')))
    } finally {
      if (progressToastID) options.hideProgress(progressToastID)
      accountIDs.forEach(id => probingUpstreamBilling.delete(id))
    }
  }

  const handleBulkQueryUpstreamQuota = async () => {
    if (bulkQueryingUpstreamQuota.value) return
    const accountIDs = [...options.getSelectedAccountIDs()]
    if (accountIDs.length === 0) {
      options.showError(t('admin.accounts.upstreamBilling.noEligibleAccounts'))
      return
    }

    const batches: number[][] = []
    for (let start = 0; start < accountIDs.length; start += UPSTREAM_QUOTA_QUERY_BATCH_SIZE) {
      batches.push(accountIDs.slice(start, start + UPSTREAM_QUOTA_QUERY_BATCH_SIZE))
    }
    let progressToastID: string | null = batches.length > 1
      ? replaceProgressToast(null, t('admin.accounts.upstreamBilling.quotaBatchStarted', {
          count: accountIDs.length,
          total: batches.length
        }))
      : null
    bulkQueryingUpstreamQuota.value = true
    let succeeded = 0
    let failed = 0
    try {
      for (let batchIndex = 0; batchIndex < batches.length; batchIndex += 1) {
        const results = await Promise.allSettled(batches[batchIndex].map(async accountID => {
          const account = options.getAccounts().find(item => item.id === accountID) ?? await getById(accountID)
          if (account.platform !== 'openai' || account.type !== 'apikey') return false
          return handleQueryUpstreamQuota(account)
        }))
        const batchSucceeded = results.filter(result => result.status === 'fulfilled' && result.value).length
        succeeded += batchSucceeded
        failed += results.length - batchSucceeded
        if (progressToastID && batchIndex < batches.length - 1) {
          progressToastID = replaceProgressToast(progressToastID, t('admin.accounts.upstreamBilling.quotaBatchProgress', {
            completed: batchIndex + 1,
            next: batchIndex + 2,
            total: batches.length
          }))
        }
      }
      if (failed > 0) {
        options.showError(t('admin.accounts.upstreamBilling.quotaBatchPartial', { success: succeeded, failed }))
      } else {
        options.showSuccess(t('admin.accounts.upstreamBilling.quotaBatchCompleted', { count: succeeded }))
      }
    } finally {
      if (progressToastID) options.hideProgress(progressToastID)
      bulkQueryingUpstreamQuota.value = false
    }
  }

  const disposeUpstreamBilling = () => {
    upstreamBillingRateAbortController?.abort()
    upstreamBillingRateAbortController = null
    upstreamFeedbackDisposed = true
    upstreamBillingFeedbackTimers.forEach(timer => clearTimeout(timer))
    upstreamQuotaFeedbackTimers.forEach(timer => clearTimeout(timer))
    upstreamBillingFeedbackTimers.clear()
    upstreamQuotaFeedbackTimers.clear()
  }

  return {
    probingUpstreamBilling,
    queryingUpstreamQuota,
    upstreamQuotaResults,
    upstreamQuotaErrors,
    bulkQueryingUpstreamQuota,
    bulkQueryingOpenAIQuota,
    bulkOpenAIQuotaResults,
    upstreamBillingFeedback,
    upstreamQuotaFeedback,
    upstreamBillingProbeGloballyEnabled,
    upstreamBillingNow,
    upstreamBillingRateETag,
    registerQuotaHydrationWatch,
    invalidateUpstreamQuotaState,
    refreshUpstreamBillingRates,
    refreshUpstreamBillingSortedList,
    handleProbeUpstreamBilling,
    handleQueryUpstreamQuota,
    handleBulkProbeUpstreamBilling,
    handleBulkQueryUpstreamQuota,
    handleBulkQueryOpenAIQuota,
    disposeUpstreamBilling
  }
}
