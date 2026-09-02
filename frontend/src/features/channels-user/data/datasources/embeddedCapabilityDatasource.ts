import apiClient from '@/core/networks/client'

export interface EmbeddedCapabilityIssue {
  token: string
  token_type: 'embedded_capability'
  expires_at: string
  menu_id: string
  target_origin: string
}

export async function issueEmbeddedCapability(
  menuId: string,
  targetOrigin: string,
): Promise<EmbeddedCapabilityIssue> {
  const { data } = await apiClient.post<EmbeddedCapabilityIssue>('/auth/embedded-capability', {
    menu_id: menuId,
    target_origin: targetOrigin,
  })
  return data
}
