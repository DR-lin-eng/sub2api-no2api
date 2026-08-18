import { getCurrentUser, getLocalCaptcha, getPublicSettings } from './authQueries'
import {
  clearAuthToken,
  getAuthToken,
  getRefreshToken,
  getTokenExpiresAt,
  isAuthenticated,
  isTotp2FARequired,
  login,
  login2FA,
  logout,
  refreshToken,
  register,
  revokeAllSessions,
  setAuthToken,
  setRefreshToken,
  setTokenExpiresAt,
} from './authSessionActions'
import {
  completeLinuxDoOAuthRegistration,
  completeOIDCOAuthRegistration,
  completePendingOAuthBindLogin,
  completeWeChatOAuthRegistration,
  createPendingDingTalkOAuthAccount,
  createPendingLinuxDoOAuthAccount,
  createPendingOIDCOAuthAccount,
  createPendingWeChatOAuthAccount,
  exchangePendingOAuthCompletion,
  getPendingOAuthBindLoginKind,
  hasPendingOAuthSuggestedProfile,
  isPendingOAuthCreateAccountRequired,
  sendPendingOAuthVerifyCode,
} from './authOAuthActions'
import {
  forgotPassword,
  resetPassword,
  sendVerifyCode,
  validateInvitationCode,
  validatePromoCode,
} from './authVerificationActions'

export * from '../dtos/authDtos'
export * from './authOAuthActions'
export * from './authQueries'
export * from './authSessionActions'
export * from './authVerificationActions'

export const authAPI = {
  login,
  login2FA,
  isTotp2FARequired,
  register,
  getCurrentUser,
  logout,
  isAuthenticated,
  setAuthToken,
  setRefreshToken,
  setTokenExpiresAt,
  getAuthToken,
  getRefreshToken,
  getTokenExpiresAt,
  clearAuthToken,
  getPublicSettings,
  getLocalCaptcha,
  sendVerifyCode,
  sendPendingOAuthVerifyCode,
  validatePromoCode,
  validateInvitationCode,
  forgotPassword,
  resetPassword,
  refreshToken,
  revokeAllSessions,
  getPendingOAuthBindLoginKind,
  isPendingOAuthCreateAccountRequired,
  hasPendingOAuthSuggestedProfile,
  completePendingOAuthBindLogin,
  createPendingLinuxDoOAuthAccount,
  createPendingOIDCOAuthAccount,
  createPendingWeChatOAuthAccount,
  exchangePendingOAuthCompletion,
  completeLinuxDoOAuthRegistration,
  completeOIDCOAuthRegistration,
  completeWeChatOAuthRegistration,
  createPendingDingTalkOAuthAccount,
}

export default authAPI
