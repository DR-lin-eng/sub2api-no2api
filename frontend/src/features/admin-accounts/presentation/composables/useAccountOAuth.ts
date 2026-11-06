import { ref } from 'vue'
import { useAppStore } from '@/core/stores/appStore'
import {
  authenticateAccountWithCookie,
  exchangeAccountAuthCode,
  generateAccountAuthUrl
} from '@/features/admin-accounts/data/datasources/adminAccountOAuthActions'
import type {
  AccountOAuthMethod,
  AccountOAuthTokenInfo
} from '@/features/admin-accounts/data/datasources/adminAccountOAuthActions'

export type AddMethod = AccountOAuthMethod
export type AuthInputMethod = 'manual' | 'siwc' | 'cookie' | 'refresh_token' | 'mobile_refresh_token' | 'session_token' | 'access_token' | 'codex_session' | 'agent_identity' | 'codex_pat' | 'sso_cookie'

export interface OAuthState {
  authUrl: string
  authCode: string
  sessionId: string
  sessionKey: string
  loading: boolean
  error: string
}

export type TokenInfo = AccountOAuthTokenInfo

export function useAccountOAuth() {
  const appStore = useAppStore()

  // State
  const authUrl = ref('')
  const authCode = ref('')
  const sessionId = ref('')
  const sessionKey = ref('')
  const loading = ref(false)
  const error = ref('')

  // Reset state
  const resetState = () => {
    authUrl.value = ''
    authCode.value = ''
    sessionId.value = ''
    sessionKey.value = ''
    loading.value = false
    error.value = ''
  }

  // Generate auth URL
  const generateAuthUrl = async (
    addMethod: AddMethod,
    proxyId?: number | null
  ): Promise<boolean> => {
    loading.value = true
    authUrl.value = ''
    sessionId.value = ''
    error.value = ''

    try {
      const response = await generateAccountAuthUrl(addMethod, proxyId)
      authUrl.value = response.auth_url
      sessionId.value = response.session_id
      return true
    } catch (err: any) {
      error.value = err.response?.data?.detail || 'Failed to generate auth URL'
      appStore.showError(error.value)
      return false
    } finally {
      loading.value = false
    }
  }

  // Exchange auth code for tokens
  const exchangeAuthCode = async (
    addMethod: AddMethod,
    proxyId?: number | null
  ): Promise<TokenInfo | null> => {
    if (!authCode.value.trim() || !sessionId.value) {
      error.value = 'Missing auth code or session ID'
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const tokenInfo = await exchangeAccountAuthCode(addMethod, {
        session_id: sessionId.value,
        code: authCode.value.trim(),
        ...(proxyId ? { proxy_id: proxyId } : {})
      })

      return tokenInfo as TokenInfo
    } catch (err: any) {
      error.value = err.response?.data?.detail || 'Failed to exchange auth code'
      appStore.showError(error.value)
      return null
    } finally {
      loading.value = false
    }
  }

  // Cookie-based authentication
  const cookieAuth = async (
    addMethod: AddMethod,
    sessionKeyValue: string,
    proxyId?: number | null
  ): Promise<TokenInfo | null> => {
    if (!sessionKeyValue.trim()) {
      error.value = 'Please enter sessionKey'
      return null
    }

    loading.value = true
    error.value = ''

    try {
      const tokenInfo = await authenticateAccountWithCookie(addMethod, sessionKeyValue.trim(), proxyId)

      return tokenInfo as TokenInfo
    } catch (err: any) {
      error.value = err.response?.data?.detail || 'Cookie authorization failed'
      return null
    } finally {
      loading.value = false
    }
  }

  // Parse multiple session keys
  const parseSessionKeys = (input: string): string[] => {
    return input
      .split('\n')
      .map((k) => k.trim())
      .filter((k) => k)
  }

  // Build extra info from token response
  const buildExtraInfo = (tokenInfo: TokenInfo): Record<string, string> | undefined => {
    const extra: Record<string, string> = {}
    if (tokenInfo.org_uuid) {
      extra.org_uuid = tokenInfo.org_uuid
    }
    if (tokenInfo.account_uuid) {
      extra.account_uuid = tokenInfo.account_uuid
    }
    if (tokenInfo.email_address) {
      extra.email_address = tokenInfo.email_address
    }
    return Object.keys(extra).length > 0 ? extra : undefined
  }

  return {
    // State
    authUrl,
    authCode,
    sessionId,
    sessionKey,
    loading,
    error,
    // Methods
    resetState,
    generateAuthUrl,
    exchangeAuthCode,
    cookieAuth,
    parseSessionKeys,
    buildExtraInfo
  }
}
