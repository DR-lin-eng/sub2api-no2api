import { apiClient } from '@/core/networks/client'
import type { SendVerifyCodeRequest, SendVerifyCodeResponse } from '@/types'
import type {
  ForgotPasswordRequest,
  ForgotPasswordResponse,
  ResetPasswordRequest,
  ResetPasswordResponse,
  ValidateInvitationCodeResponse,
  ValidatePromoCodeResponse,
} from '../dtos/authDtos'

export async function sendVerifyCode(
  request: SendVerifyCodeRequest,
): Promise<SendVerifyCodeResponse> {
  const { data } = await apiClient.post<SendVerifyCodeResponse>('/auth/send-verify-code', request)
  return data
}

export async function validatePromoCode(code: string): Promise<ValidatePromoCodeResponse> {
  const { data } = await apiClient.post<ValidatePromoCodeResponse>(
    '/auth/validate-promo-code',
    { code },
  )
  return data
}

export async function validateInvitationCode(
  code: string,
): Promise<ValidateInvitationCodeResponse> {
  const { data } = await apiClient.post<ValidateInvitationCodeResponse>(
    '/auth/validate-invitation-code',
    { code },
  )
  return data
}

export async function forgotPassword(
  request: ForgotPasswordRequest,
): Promise<ForgotPasswordResponse> {
  const { data } = await apiClient.post<ForgotPasswordResponse>('/auth/forgot-password', request)
  return data
}

export async function resetPassword(
  request: ResetPasswordRequest,
): Promise<ResetPasswordResponse> {
  const { data } = await apiClient.post<ResetPasswordResponse>('/auth/reset-password', request)
  return data
}
