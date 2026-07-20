import axios from 'axios'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const refreshPayload = {
  access_token: 'access-2',
  refresh_token: 'refresh-2',
  expires_in: 3600,
  token_type: 'Bearer',
}

describe('browser session refresh coordination', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    vi.resetModules()
    localStorage.clear()
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: undefined,
    })
  })

  it('shares one refresh request between concurrent callers in a tab', async () => {
    let resolveRequest!: (value: unknown) => void
    const post = vi.spyOn(axios, 'post').mockReturnValue(new Promise((resolve) => {
      resolveRequest = resolve
    }))
    const { refreshBrowserSession } = await import('@/api/sessionRefresh')

    const first = refreshBrowserSession()
    const second = refreshBrowserSession()
    expect(post).toHaveBeenCalledTimes(1)

    resolveRequest({ data: { code: 0, message: 'success', data: refreshPayload } })
    await expect(Promise.all([first, second])).resolves.toEqual([refreshPayload, refreshPayload])
  })

  it('uses a same-origin Web Lock before rotating the refresh cookie', async () => {
    const request = vi.fn(async (_name, _options, callback: () => Promise<unknown>) => callback())
    Object.defineProperty(navigator, 'locks', {
      configurable: true,
      value: { request },
    })
    vi.spyOn(axios, 'post').mockResolvedValue({
      data: { code: 0, message: 'success', data: refreshPayload },
    })
    const { refreshBrowserSession } = await import('@/api/sessionRefresh')

    await expect(refreshBrowserSession()).resolves.toEqual(refreshPayload)
    expect(request).toHaveBeenCalledWith(
      'sub2api-auth-refresh',
      { mode: 'exclusive' },
      expect.any(Function),
    )
  })
})
