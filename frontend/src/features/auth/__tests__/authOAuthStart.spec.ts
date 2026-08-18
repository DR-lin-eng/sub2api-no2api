import { beforeEach, describe, expect, it, vi } from 'vitest'

const post = vi.hoisted(() => vi.fn())

vi.mock('@/core/networks/client', () => ({
  apiClient: { post }
}))

import {
  buildOAuthLoginStartURL,
  startOAuthLogin
} from '@/features/auth/data/datasources/authOAuthActions'

describe('oauth login start datasource', () => {
  beforeEach(() => {
    post.mockReset()
  })

  it('keeps the legacy GET URL contract when no action proof is needed', () => {
    expect(buildOAuthLoginStartURL({
      provider: 'github',
      params: { redirect: '/billing?plan=pro', aff_code: 'AFF123' }
    })).toBe(
      '/api/v1/auth/oauth/github/start?redirect=%2Fbilling%3Fplan%3Dpro&aff_code=AFF123'
    )
  })

  it('posts Tencent proof while preserving provider query parameters', async () => {
    post.mockResolvedValue({
      data: { authorize_url: 'https://provider.example/authorize' }
    })

    const result = await startOAuthLogin(
      {
        provider: 'wechat',
        params: { mode: 'open', redirect: '/dashboard' }
      },
      {
        tencent_captcha_ticket: 'ticket-value',
        tencent_captcha_randstr: 'rand-value'
      }
    )

    expect(post).toHaveBeenCalledWith(
      '/auth/oauth/wechat/start',
      {
        tencent_captcha_ticket: 'ticket-value',
        tencent_captcha_randstr: 'rand-value'
      },
      { params: { mode: 'open', redirect: '/dashboard' } }
    )
    expect(result).toEqual({ authorize_url: 'https://provider.example/authorize' })
  })

  it('posts Alibaba Cloud proof in the captcha_token field', async () => {
    post.mockResolvedValue({
      data: { authorize_url: 'https://provider.example/authorize' }
    })

    await startOAuthLogin(
      {
        provider: 'oidc',
        params: { redirect: '/dashboard' }
      },
      { captcha_token: 'aliyun-captcha-param' }
    )

    expect(post).toHaveBeenCalledWith(
      '/auth/oauth/oidc/start',
      { captcha_token: 'aliyun-captcha-param' },
      { params: { redirect: '/dashboard' } }
    )
  })
})
