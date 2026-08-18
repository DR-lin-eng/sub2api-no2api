import { apiClient } from '@/core/networks/client'
import type { CurrentUserResponse, PublicSettings } from '@/types'
import type { LocalCaptchaChallenge } from '../dtos/authDtos'

export function getCurrentUser() {
  return apiClient.get<CurrentUserResponse>('/auth/me')
}

export async function getPublicSettings(): Promise<PublicSettings> {
  const { data } = await apiClient.get<PublicSettings>('/settings/public')
  return data
}

export async function getLocalCaptcha(): Promise<LocalCaptchaChallenge> {
  const { data } = await apiClient.get<LocalCaptchaChallenge>('/auth/captcha')
  return data
}
