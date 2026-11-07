import { onMounted, onUnmounted, ref, watch } from 'vue'
import { queryOpenAIUsage, type OpenAIUsageDetails, type OpenAIUsageQuery } from '../../data/datasources/openAIUsageDatasource'

export function useOpenAIUsageDetails(accountID: () => number, query: () => OpenAIUsageQuery | null, identity?: () => unknown) {
  const result = ref<OpenAIUsageDetails | null>(null)
  const loading = ref(false)
  const failed = ref(false)
  let controller: AbortController | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let sequence = 0
  let disposed = false
  const load = async (cursor?: string) => {
    const request = query()
    if (!request || disposed) return
    const current = ++sequence
    controller?.abort()
    controller = new AbortController()
    clearTimeout(timer)
    loading.value = true
    failed.value = false
    try {
      const response = await queryOpenAIUsage(accountID(), { ...request, ...(cursor ? { cursor } : {}) }, controller.signal)
      if (disposed || current !== sequence) return
      if (cursor && result.value?.data && response.data) {
        const previousPoints = Array.isArray(result.value.data.points) ? result.value.data.points : []
        const nextPoints = Array.isArray(response.data.points) ? response.data.points : []
        response.data = { ...response.data, points: [...previousPoints, ...nextPoints] }
      }
      result.value = response
    } catch (error) {
      if (!disposed && current === sequence && (error as { code?: string }).code !== 'ERR_CANCELED') failed.value = true
    } finally {
      if (!disposed && current === sequence) {
        loading.value = false
        timer = setTimeout(() => { if (!document.hidden) void load() }, (result.value?.refresh_after_seconds ?? 60) * 1000)
      }
    }
  }
  const visibility = () => {
    if (document.hidden) { clearTimeout(timer); controller?.abort(); return }
    const kind = query()?.kind
    if (kind === 'monthly' || kind === 'remaining_balance' || !result.value || Date.now() - result.value.fetched_at * 1000 >= result.value.refresh_after_seconds * 1000) void load()
  }
  watch(() => JSON.stringify([accountID(), identity?.(), query()]), () => {
    sequence += 1
    controller?.abort()
    clearTimeout(timer)
    result.value = null
    loading.value = false
    failed.value = false
    void load()
  }, { immediate: true })
  onMounted(() => document.addEventListener('visibilitychange', visibility))
  onUnmounted(() => {
    disposed = true
    sequence += 1
    controller?.abort()
    clearTimeout(timer)
    document.removeEventListener('visibilitychange', visibility)
  })
  return { result, loading, failed, load }
}
