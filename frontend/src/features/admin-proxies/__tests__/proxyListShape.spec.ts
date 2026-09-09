import { beforeEach, describe, expect, it, vi } from 'vitest'
const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/core/networks/client', () => ({ apiClient: { get } }))
import { list, getAll, getAllWithCount } from '../data/datasources/adminProxiesDatasource'

describe('proxy response shape', () => {
  beforeEach(() => get.mockReset())
  it.each([getAll, getAllWithCount])('rejects a malformed all-proxies response', async (read) => {
    get.mockResolvedValue({ data: '<html>Error</html>' })
    await expect(read()).rejects.toThrow('Invalid proxy list response')
    get.mockResolvedValue({ data: [] })
    await expect(read()).resolves.toEqual([])
  })
  it('checks paginated items before updating page state', async () => {
    get.mockResolvedValue({ data: { items: null } })
    await expect(list()).rejects.toThrow('Invalid proxy list response')
    const page = { items: [], total: 0 }
    get.mockResolvedValue({ data: page })
    await expect(list()).resolves.toEqual(page)
  })
})
