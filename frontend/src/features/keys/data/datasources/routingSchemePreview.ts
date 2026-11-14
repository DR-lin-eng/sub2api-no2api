import { ROUTING_SCHEME_PREVIEW_STORAGE_KEY } from '@/core/constants/routingSchemePreview'
import { safeLocalStorage } from '@/core/utils/safeStorage'
import type { GroupPlatform } from '@/types/group'

export interface QuickSchemePreviewItem {
  group_id: number
  weight: number
}

export interface QuickSchemePreview {
  id: number
  name: string
  description: string
  platform: GroupPlatform
  items: QuickSchemePreviewItem[]
}

/**
 * Read-only preview bridge for the key editor quick-select button.
 * The admin preview store writes the same local storage key; once the backend
 * API exists this function should be replaced by a user-facing datasource.
 */
export function listQuickSchemes(): QuickSchemePreview[] {
  try {
    const raw = safeLocalStorage.getItem(ROUTING_SCHEME_PREVIEW_STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter(item => item && typeof item === 'object' && item.status !== 'disabled')
      .map((item): QuickSchemePreview => ({
        id: Number(item.id) || 0,
        name: String(item.name ?? ''),
        description: String(item.description ?? ''),
        platform: item.platform as GroupPlatform,
        items: Array.isArray(item.items)
          ? item.items
              .filter((entry: unknown): entry is QuickSchemePreviewItem =>
                !!entry
                && typeof entry === 'object'
                && Number.isFinite(Number((entry as QuickSchemePreviewItem).group_id))
                && Number.isFinite(Number((entry as QuickSchemePreviewItem).weight)))
              .map((entry: QuickSchemePreviewItem) => ({
                group_id: Number(entry.group_id),
                weight: Math.max(1, Math.trunc(Number(entry.weight)) || 1),
              }))
          : [],
      }))
      .filter(item => item.id > 0 && item.name && item.platform && item.items.length > 0)
  } catch {
    return []
  }
}
