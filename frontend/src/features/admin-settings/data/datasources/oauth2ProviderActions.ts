import { apiClient } from '@/core/networks/client'
import type {
  OAuth2ProviderClient,
  OAuth2ProviderClientCreate,
  OAuth2ProviderClientSecretResult,
  OAuth2ProviderClientUpdate,
  OAuth2ProviderConfig,
  OAuth2ProviderConfigUpdate,
} from '@/features/admin-settings/data/dtos/oauth2ProviderDtos'

export async function updateOAuth2ProviderConfig(input: OAuth2ProviderConfigUpdate): Promise<OAuth2ProviderConfig> {
  const { data } = await apiClient.put<OAuth2ProviderConfig>('/admin/oauth2-provider', input)
  return data
}

export async function createOAuth2ProviderClient(input: OAuth2ProviderClientCreate): Promise<OAuth2ProviderClientSecretResult> {
  const { data } = await apiClient.post<OAuth2ProviderClientSecretResult>('/admin/oauth2-provider/clients', input)
  return data
}

export async function updateOAuth2ProviderClient(clientId: string, input: OAuth2ProviderClientUpdate): Promise<OAuth2ProviderClient> {
  const { data } = await apiClient.put<OAuth2ProviderClient>(`/admin/oauth2-provider/clients/${encodeURIComponent(clientId)}`, input)
  return data
}

export async function rotateOAuth2ProviderClientSecret(clientId: string): Promise<OAuth2ProviderClientSecretResult> {
  const { data } = await apiClient.post<OAuth2ProviderClientSecretResult>(`/admin/oauth2-provider/clients/${encodeURIComponent(clientId)}/rotate-secret`)
  return data
}

export async function deleteOAuth2ProviderClient(clientId: string): Promise<void> {
  await apiClient.delete(`/admin/oauth2-provider/clients/${encodeURIComponent(clientId)}`)
}
