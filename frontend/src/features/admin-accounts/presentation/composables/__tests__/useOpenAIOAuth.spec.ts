import { describe, expect, it, vi } from 'vitest'

vi.mock('@/core/stores/appStore', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      const messages: Record<string, string> = {
        'admin.accounts.oauth.openai.failedToExchangeCode': 'OpenAI 授权码兑换失败',
        'admin.accounts.oauth.openai.errors.OPENAI_OAUTH_PROXY_REQUIRED':
          '未设置代理，当前服务器无法直连 OpenAI，导致 OpenAI OAuth 请求失败。请先选择可访问 OpenAI 的代理后重试；如果授权码已失效，请重新生成授权链接。'
      }
      return messages[key] ?? key
    }
  })
}))

vi.mock('@/features/admin-accounts/data/datasources/adminAccountOAuthActions', () => ({
  generateOpenAIAuthUrl: vi.fn(),
  exchangeOpenAICode: vi.fn(),
  refreshOpenAIToken: vi.fn()
}))

import { useOpenAIOAuth } from '@/features/admin-accounts/presentation/composables/useOpenAIOAuth'
import { exchangeOpenAICode, generateOpenAIAuthUrl } from '@/features/admin-accounts/data/datasources/adminAccountOAuthActions'

describe('SIWC authorization', () => {
  it('passes the selected protocol and preserves the dynamic credentials', async () => {
    vi.mocked(generateOpenAIAuthUrl).mockResolvedValueOnce({ auth_url: 'https://auth.openai.com/api/accounts/authorize?state=siwc-state', session_id: 'siwc-session' })
    const oauth = useOpenAIOAuth()
    await expect(oauth.generateAuthUrl(7, undefined, 'siwc')).resolves.toBe(true)
    expect(generateOpenAIAuthUrl).toHaveBeenCalledWith({ proxy_id: 7, flow: 'siwc' })
    const callback = 'http://127.0.0.1:1455/auth/callback?code=code&state=siwc-state&client_id=oaiapp_issued'
    vi.mocked(exchangeOpenAICode).mockResolvedValueOnce({ access_token: 'at', refresh_token: 'rt', auth_mode: 'siwc', client_id: 'oaiapp_issued', scope: 'chatgpt.tokens.use.direct', device_id: 'device' })
    const token = await oauth.exchangeAuthCode(callback, oauth.sessionId.value, oauth.oauthState.value, 7)
    expect(exchangeOpenAICode).toHaveBeenLastCalledWith({ session_id: 'siwc-session', code: callback, state: 'siwc-state', proxy_id: 7 })
    expect(oauth.buildCredentials(token!)).toMatchObject({ auth_mode: 'siwc', client_id: 'oaiapp_issued', device_id: 'device', scope: 'chatgpt.tokens.use.direct' })
  })
})

describe('useOpenAIOAuth.buildCredentials', () => {
  it('should keep client_id when token response contains it', () => {
    const oauth = useOpenAIOAuth()
    const creds = oauth.buildCredentials({
      access_token: 'at',
      refresh_token: 'rt',
      client_id: 'app_test_client',
      expires_at: 1700000000
    })

    expect(creds.client_id).toBe('app_test_client')
    expect(creds.access_token).toBe('at')
    expect(creds.refresh_token).toBe('rt')
  })

  it('should keep legacy behavior when client_id is missing', () => {
    const oauth = useOpenAIOAuth()
    const creds = oauth.buildCredentials({
      access_token: 'at',
      refresh_token: 'rt',
      expires_at: 1700000000
    })

    expect(Object.prototype.hasOwnProperty.call(creds, 'client_id')).toBe(false)
    expect(creds.access_token).toBe('at')
    expect(creds.refresh_token).toBe('rt')
  })

  it('should keep ChatGPT subscription expiration from token response', () => {
    const oauth = useOpenAIOAuth()
    const creds = oauth.buildCredentials({
      access_token: 'at',
      refresh_token: 'rt',
      expires_at: 1700000000,
      plan_type: 'team',
      subscription_expires_at: '2026-07-20T19:22:48+00:00'
    })

    expect(creds.plan_type).toBe('team')
    expect(creds.subscription_expires_at).toBe('2026-07-20T19:22:48+00:00')
  })
})

describe('useOpenAIOAuth.exchangeAuthCode', () => {
  it('shows a clear proxy hint when code exchange fails without a proxy', async () => {
    vi.mocked(exchangeOpenAICode).mockRejectedValueOnce({
      status: 502,
      reason: 'OPENAI_OAUTH_PROXY_REQUIRED',
      message: 'OpenAI OAuth token exchange failed: no proxy is configured.'
    })
    const oauth = useOpenAIOAuth()

    const tokenInfo = await oauth.exchangeAuthCode('code', 'session-id', 'state')

    expect(tokenInfo).toBeNull()
    expect(oauth.error.value).toBe(
      '未设置代理，当前服务器无法直连 OpenAI，导致 OpenAI OAuth 请求失败。请先选择可访问 OpenAI 的代理后重试；如果授权码已失效，请重新生成授权链接。'
    )
  })
})
