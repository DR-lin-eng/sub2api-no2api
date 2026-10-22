import { beforeEach, describe, expect, it, vi } from 'vitest'
import { effectScope } from 'vue'
import { useAccountUpstreamModelSync } from '@/features/admin-accounts/presentation/composables/useAccountUpstreamModelSync'

const { batchSync } = vi.hoisted(() => ({ batchSync: vi.fn() }))
vi.mock('@/features/admin-accounts/data/datasources/adminAccountActions', () => ({ batchSyncUpstreamModels: batchSync }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('upstream model batch sync', () => {
  beforeEach(() => { batchSync.mockReset() })

  it('processes a large selection in bounded sequential batches with per-account results', async () => {
    batchSync.mockImplementation(async (ids: number[]) => ({ results: ids.map(account_id => ({ account_id, status: 'success', model_count: 2, added_count: 1 })) }))
    const completed = vi.fn()
    const state = useAccountUpstreamModelSync(completed)
    await state.syncAccountModels([...Array.from({ length: 45 }, (_, index) => index + 1), 1])
    expect(batchSync.mock.calls.map(call => call[0].length)).toEqual([20, 20, 5])
    expect(state.modelSyncTotal.value).toBe(45)
    expect(state.modelSyncCompleted.value).toBe(45)
    expect(state.syncingUpstreamModels.value).toBe(false)
    expect(completed).toHaveBeenCalledWith(state.modelSyncResults.value)
  })

  it('keeps failed batches retryable while processing subsequent accounts', async () => {
    batchSync.mockRejectedValueOnce(new Error('network failed'))
    batchSync.mockResolvedValueOnce({ results: [{ account_id: 21, status: 'skipped', model_count: 0, added_count: 0, error: 'unsupported' }] })
    const state = useAccountUpstreamModelSync(vi.fn())
    await state.syncAccountModels(Array.from({ length: 21 }, (_, index) => index + 1))
    expect(state.modelSyncCompleted.value).toBe(21)
    expect(state.modelSyncResults.value.filter(result => result.status === 'failed')).toHaveLength(20)
    expect(state.modelSyncResults.value[20].status).toBe('skipped')
  })

  it('ignores duplicate clicks until the current sync finishes', async () => {
    let resolve!: (result: unknown) => void
    batchSync.mockReturnValue(new Promise(done => { resolve = done }))
    const state = useAccountUpstreamModelSync(vi.fn())
    const running = state.syncAccountModels([1])
    await state.syncAccountModels([2])
    expect(batchSync).toHaveBeenCalledTimes(1)
    expect(state.syncingUpstreamModels.value).toBe(true)
    resolve({ results: [{ account_id: 1, status: 'success', model_count: 1, added_count: 1 }] })
    await running
    expect(state.syncingUpstreamModels.value).toBe(false)
  })

  it('stops queued batches and page callbacks after leaving the page', async () => {
    let resolve!: (result: unknown) => void
    batchSync.mockReturnValue(new Promise(done => { resolve = done }))
    const completed = vi.fn()
    const scope = effectScope()
    const state = scope.run(() => useAccountUpstreamModelSync(completed))!
    const running = state.syncAccountModels(Array.from({ length: 21 }, (_, index) => index + 1))
    scope.stop()
    resolve({ results: [] })
    await running
    expect(batchSync).toHaveBeenCalledTimes(1)
    expect(completed).not.toHaveBeenCalled()
  })
})
