import { KEY_ROUTING_SCHEME_PREVIEW_STORAGE_KEY } from '@/core/constants/routingSchemePreview'
import { safeLocalStorage } from '@/core/utils/safeStorage'
import type { ApiKeyRoutingSchemeSummary } from '@/types'

type PreviewMap = Record<string, ApiKeyRoutingSchemeSummary>

function readMap(): PreviewMap {
  try {
    const raw = safeLocalStorage.getItem(KEY_ROUTING_SCHEME_PREVIEW_STORAGE_KEY)
    if (!raw) return {}
    const parsed = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return {}
    return parsed as PreviewMap
  } catch {
    return {}
  }
}

function writeMap(map: PreviewMap): void {
  try {
    safeLocalStorage.setItem(KEY_ROUTING_SCHEME_PREVIEW_STORAGE_KEY, JSON.stringify(map))
  } catch {
    // Preview only; the key list falls back to legacy group rendering.
  }
}

/**
 * Preview bridge for rows created with a quick routing scheme.
 * Replace with the key list response (`apiKey.routing_scheme`) once the backend
 * serves scheme metadata; the rendering contract is the same.
 */
export function rememberKeyRoutingScheme(
  apiKeyId: number,
  scheme: ApiKeyRoutingSchemeSummary
): void {
  if (!apiKeyId) return
  const map = readMap()
  map[String(apiKeyId)] = scheme
  writeMap(map)
}

export function forgetKeyRoutingScheme(apiKeyId: number): void {
  const map = readMap()
  if (!(String(apiKeyId) in map)) return
  delete map[String(apiKeyId)]
  writeMap(map)
}

export function getKeyRoutingScheme(apiKeyId: number): ApiKeyRoutingSchemeSummary | null {
  return readMap()[String(apiKeyId)] ?? null
}

export function listKeyRoutingSchemes(): PreviewMap {
  return readMap()
}
