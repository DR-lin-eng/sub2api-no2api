import { apiClient } from '@/core/networks/client'

export interface OAuth2AuthorizationParameters {
  client_id: string
  redirect_uri: string
  response_type: string
  scope: string
  state: string
  code_challenge: string
  code_challenge_method: string
}

export interface OAuth2AuthorizationScope {
  name: string
  description: string
}

export interface OAuth2AuthorizationPreview {
  client_id: string
  client_name: string
  redirect_uri: string
  scopes: OAuth2AuthorizationScope[]
}

export async function getOAuth2AuthorizationPreview(params: OAuth2AuthorizationParameters): Promise<OAuth2AuthorizationPreview> {
  const { data } = await apiClient.get<OAuth2AuthorizationPreview>('/oauth2/authorize', { params })
  return data
}

export async function submitOAuth2Authorization(params: OAuth2AuthorizationParameters, approved: boolean): Promise<{ redirect_url: string }> {
  const { data } = await apiClient.post<{ redirect_url: string }>('/oauth2/authorize', {
    ...params,
    approved,
  })
  return data
}
