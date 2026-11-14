import { apiClient } from '@/core/networks/client'
import type { GroupPlatform } from '@/types/group'

/**
 * User-facing routing scheme summary. The backend deliberately omits the
 * scheme's internal groups, weights and rules.
 */
export interface UserRoutingScheme {
  id: number
  name: string
  description: string
  platform: GroupPlatform
  min_rate_multiplier: number | null
  max_rate_multiplier: number | null
}

export async function listAvailableRoutingSchemes(
  platform?: string,
  options?: { signal?: AbortSignal },
): Promise<UserRoutingScheme[]> {
  const { data } = await apiClient.get<UserRoutingScheme[]>('/routing-schemes/available', {
    params: platform ? { platform } : undefined,
    signal: options?.signal,
  })
  return data ?? []
}
