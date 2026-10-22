import { computed, getCurrentScope, onScopeDispose, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { batchSyncUpstreamModels } from '@/features/admin-accounts/data/datasources/adminAccountActions'
import type { UpstreamModelSyncResult } from '@/features/admin-accounts/data/dtos/adminAccountDtos'

// Sequential batches bound load even when every filtered account is selected.
// Twenty accounts fit within four workers and a twenty-second probe budget.
const BATCH_SIZE = 20

export function useAccountUpstreamModelSync(onComplete: (results: UpstreamModelSyncResult[]) => void) {
  const { t } = useI18n()
  const syncingUpstreamModels = ref(false)
  const showModelSyncResults = ref(false)
  const modelSyncResults = ref<UpstreamModelSyncResult[]>([])
  const modelSyncTotal = ref(0)
  const modelSyncCompleted = computed(() => modelSyncResults.value.length)
  let active = true
  if (getCurrentScope()) onScopeDispose(() => { active = false })

  async function syncAccountModels(accountIds: number[]) {
    if (!active || syncingUpstreamModels.value || accountIds.length === 0) return
    const ids = [...new Set(accountIds)]
    modelSyncResults.value = []
    modelSyncTotal.value = ids.length
    syncingUpstreamModels.value = true
    showModelSyncResults.value = true
    try {
      for (let offset = 0; offset < ids.length; offset += BATCH_SIZE) {
        if (!active) break
        const batch = ids.slice(offset, offset + BATCH_SIZE)
        try {
          const result = await batchSyncUpstreamModels(batch)
          modelSyncResults.value.push(...result.results)
        } catch {
          // A dropped response cannot reveal which writes succeeded. Keep the
          // accounts selected so an additive retry can confirm their results.
          modelSyncResults.value.push(...batch.map(account_id => ({
            account_id,
            status: 'failed' as const,
            model_count: 0,
            added_count: 0,
            error: t('admin.accounts.bulkActions.modelSyncRequestFailed')
          })))
        }
      }
    } finally {
      syncingUpstreamModels.value = false
      if (active) onComplete(modelSyncResults.value)
    }
  }

  return { syncingUpstreamModels, showModelSyncResults, modelSyncResults, modelSyncTotal, modelSyncCompleted, syncAccountModels }
}
