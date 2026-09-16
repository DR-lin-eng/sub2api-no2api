import { apiClient } from '@/core/networks/client'
import type { OAuth2ProviderConfig } from '@/features/admin-settings/data/dtos/oauth2ProviderDtos'

export async function getOAuth2ProviderConfig(): Promise<OAuth2ProviderConfig> {
  const { data } = await apiClient.get<OAuth2ProviderConfig>('/admin/oauth2-provider')
  return data
}
