export type OAuth2ClientType = 'confidential' | 'public'

export interface OAuth2ProviderScope {
  name: string
  description: string
}

export interface OAuth2ProviderClient {
  client_id: string
  name: string
  client_type: OAuth2ClientType
  redirect_uris: string[]
  allowed_scopes: string[]
  enabled: boolean
  secret_configured: boolean
  created_at: string
  updated_at: string
}

export interface OAuth2ProviderConfig {
  enabled: boolean
  issuer: string
  access_token_ttl_seconds: number
  clients: OAuth2ProviderClient[]
  scopes: OAuth2ProviderScope[]
}

export interface OAuth2ProviderConfigUpdate {
  enabled: boolean
  issuer: string
  access_token_ttl_seconds: number
}

export interface OAuth2ProviderClientCreate {
  name: string
  client_type: OAuth2ClientType
  redirect_uris: string[]
  allowed_scopes: string[]
  enabled: boolean
}

export interface OAuth2ProviderClientUpdate {
  name?: string
  redirect_uris?: string[]
  allowed_scopes?: string[]
  enabled?: boolean
}

export interface OAuth2ProviderClientSecretResult {
  client: OAuth2ProviderClient
  client_secret?: string
}
