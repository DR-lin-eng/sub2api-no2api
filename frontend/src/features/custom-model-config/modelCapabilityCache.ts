import { customModelConfigDatasource } from './data/datasources/customModelConfigDatasource'
import {
  clearModelCapabilities,
  initializeModelCapabilities,
} from './domain/services/modelCapabilityService'

const MODEL_CAPABILITY_CACHE_TTL_MS = 60_000

let expiresAt = 0
let refreshRequest: Promise<void> | null = null

export async function loadRuntimeModelCapabilities(force = false): Promise<void> {
  if (!force && Date.now() < expiresAt) return
  if (refreshRequest) return refreshRequest

  refreshRequest = customModelConfigDatasource.getRuntimeCapabilities()
    .then((configs) => {
      initializeModelCapabilities(configs)
      expiresAt = Date.now() + MODEL_CAPABILITY_CACHE_TTL_MS
    })
    .finally(() => {
      refreshRequest = null
    })
  return refreshRequest
}

export function invalidateRuntimeModelCapabilities(): void {
  expiresAt = 0
  clearModelCapabilities()
}
