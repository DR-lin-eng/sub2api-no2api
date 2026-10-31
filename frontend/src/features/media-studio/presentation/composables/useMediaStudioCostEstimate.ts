import { computed, getCurrentInstance, onBeforeUnmount, ref, watch } from 'vue'
import { mediaStudioErrorMessage, type MediaStudioCostEstimate, type MediaStudioCostEstimateRequest } from '@/features/media-studio/data/datasources/mediaStudioDatasource'

// Own the estimate request lifecycle, independently of browser media jobs and history.
export function useMediaStudioCostEstimate(
  request: () => MediaStudioCostEstimateRequest | null,
  estimate: (request: MediaStudioCostEstimateRequest) => Promise<MediaStudioCostEstimate>,
) {
  const estimatedCost = ref<number | null>(null)
  const estimatedCurrency = ref('')
  const estimateLoading = ref(false)
  const estimateError = ref('')
  const signature = computed(() => JSON.stringify(request()))
  let sequence = 0
  let disposed = false
  let timer: ReturnType<typeof setTimeout> | null = null

  async function refreshCostEstimate() {
    const payload = request()
    const current = ++sequence
    if (!payload) {
      estimatedCost.value = null
      estimatedCurrency.value = ''
      estimateError.value = ''
      estimateLoading.value = false
      return
    }
    estimateLoading.value = true
    estimateError.value = ''
    try {
      const result = await estimate(payload)
      if (disposed || current !== sequence) return
      estimatedCost.value = result.amount
      estimatedCurrency.value = result.currency || ''
    } catch (error) {
      if (disposed || current !== sequence) return
      estimatedCost.value = null
      estimatedCurrency.value = ''
      estimateError.value = mediaStudioErrorMessage(error, 'cost estimate failed')
    } finally {
      if (!disposed && current === sequence) estimateLoading.value = false
    }
  }

  watch(signature, () => {
    if (disposed) return
    sequence += 1
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => { timer = null; void refreshCostEstimate() }, 300)
  })
  if (getCurrentInstance()) onBeforeUnmount(() => {
    disposed = true
    sequence += 1
    if (timer) clearTimeout(timer)
  })
  return { estimatedCost, estimatedCurrency, estimateLoading, estimateError, refreshCostEstimate }
}
