import { ref } from 'vue'
import { ROUTING_SCHEME_PREVIEW_STORAGE_KEY } from '@/core/constants/routingSchemePreview'
import { safeLocalStorage } from '@/core/utils/safeStorage'
import type { GroupPlatform } from '@/types/group'
import type {
  RoutingScheme,
  RoutingSchemeDraft,
  RoutingSchemeGroupOption,
  RoutingSchemeItem,
  RoutingSchemeRule,
  RoutingSchemeSeedLabels,
  RoutingSchemeSortMode,
} from '../../data/dtos/routingSchemeDtos'
import { normalizeRules } from '../../data/routingRuleCatalog'

const STORAGE_KEY = ROUTING_SCHEME_PREVIEW_STORAGE_KEY

const DEFAULT_SEED_LABELS: RoutingSchemeSeedLabels = {
  stable: '更稳',
  value: '更性价比',
  cheap: '更低价',
  stableDescription: '稳定性更高，价格无法保证',
  valueDescription: '稳定性与价格更均衡',
  cheapDescription: '价格更低，稳定性可能波动',
}

const schemes = ref<RoutingScheme[]>([])
let loaded = false
let nextID = 1

function nowISO(): string {
  return new Date().toISOString()
}

function normalizeItems(items: RoutingSchemeItem[]): RoutingSchemeItem[] {
  return items
    .filter(item => Number.isFinite(item.group_id) && item.group_id > 0)
    .map(item => ({
      group_id: item.group_id,
      // Admin UI uses a 1-10 score; normalization happens at render/scheduling time.
      weight: Math.max(1, Math.min(10, Math.trunc(item.weight) || 1)),
      rules: normalizeRules(item.rules),
    }))
}

function normalizeSortMode(value: unknown): RoutingSchemeSortMode {
  return value === 'ttft' || value === 'error_count' ? value : 'score'
}

function hydrate(value: unknown): RoutingScheme[] {
  if (!Array.isArray(value)) return []
  return value
    .filter((item): item is RoutingScheme => !!item && typeof item === 'object')
    .map((item): RoutingScheme => ({
      id: Number(item.id) || 0,
      name: String(item.name ?? ''),
      description: String(item.description ?? ''),
      platform: item.platform as GroupPlatform,
      status: item.status === 'disabled' ? 'disabled' : 'active',
      sort_order: Number(item.sort_order) || 0,
      sort_mode: normalizeSortMode(item.sort_mode),
      items: normalizeItems(Array.isArray(item.items) ? item.items : []),
      created_at: String(item.created_at ?? nowISO()),
      updated_at: String(item.updated_at ?? nowISO()),
    }))
    .filter(item => item.id > 0 && item.name && item.platform)
}

function load(): void {
  if (loaded) return
  loaded = true
  try {
    const raw = safeLocalStorage.getItem(STORAGE_KEY)
    if (!raw) return
    schemes.value = hydrate(JSON.parse(raw))
    nextID = schemes.value.reduce((max, item) => Math.max(max, item.id), 0) + 1
  } catch {
    schemes.value = []
  }
}

function persist(): void {
  try {
    safeLocalStorage.setItem(STORAGE_KEY, JSON.stringify(schemes.value))
  } catch {
    // Preview data is non-critical; keep the in-memory copy.
  }
}

function sortSchemes(list: RoutingScheme[]): RoutingScheme[] {
  return [...list].sort((a, b) => {
    if (a.platform !== b.platform) return a.platform.localeCompare(b.platform)
    if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order
    return a.id - b.id
  })
}

function isEligibleSeedGroup(group: RoutingSchemeGroupOption): boolean {
  if (group.platform === 'composite' || group.platform === 'multimodal') return false
  if ((group.status ?? 'active') !== 'active') return false
  if ((group.subscription_type ?? 'standard') !== 'standard') return false
  return true
}

function seedWeights(count: number, kind: 'stable' | 'value' | 'cheap'): number[] {
  if (kind === 'stable') {
    if (count === 1) return [10]
    if (count === 2) return [8, 2]
    return [10, 5, 2]
  }
  if (kind === 'value') {
    if (count === 1) return [10]
    if (count === 2) return [5, 5]
    return [6, 4, 2]
  }
  if (count === 1) return [10]
  if (count === 2) return [4, 6]
  return [3, 5, 7]
}

function seedItemRules(
  schemeID: number,
  groupIndex: number,
  kind: 'stable' | 'value' | 'cheap',
): RoutingSchemeRule[] {
  const base = `seed-${schemeID}-${groupIndex}`
  if (groupIndex === 0) {
    return [
      {
        id: `${base}-ttft`,
        enabled: true,
        trigger: { kind: 'ttft', threshold_seconds: 3, window_seconds: 120, consecutive: 3 },
        action: { kind: 'weight_zero', cooldown_seconds: 180 },
      },
      {
        id: `${base}-errors`,
        enabled: true,
        trigger: { kind: 'error_class', classes: ['timeout', 'http_5xx', 'empty_stream'] },
        action: { kind: 'failover', cooldown_seconds: 60 },
      },
    ]
  }
  if (groupIndex === 1) {
    return [
      {
        id: `${base}-errors`,
        enabled: true,
        trigger: { kind: 'error_class', classes: ['http_429', 'auth'] },
        action: { kind: 'cooldown', cooldown_seconds: kind === 'cheap' ? 60 : 120 },
      },
    ]
  }
  return []
}

function buildSeedScheme(
  labels: RoutingSchemeSeedLabels,
  platform: GroupPlatform,
  groups: RoutingSchemeGroupOption[],
  kind: 'stable' | 'value' | 'cheap',
  sortOrder: number,
): RoutingScheme {
  const picked = groups.slice(0, 3)
  const weights = seedWeights(picked.length, kind)
  const description = kind === 'stable'
    ? labels.stableDescription
    : kind === 'value'
      ? labels.valueDescription
      : labels.cheapDescription
  const id = nextID++
  return {
    id,
    name: labels[kind],
    description,
    platform,
    status: 'active',
    sort_order: sortOrder,
    sort_mode: 'score',
    items: picked.map((group, index) => ({
      group_id: group.id,
      weight: weights[index] ?? 1,
      rules: seedItemRules(id, index, kind),
    })),
    created_at: nowISO(),
    updated_at: nowISO(),
  }
}

/**
 * UI preview only: the backend routing-scheme API is not wired yet.
 * Seeds three demo schemes for the first platform that has at least two
 * standard groups, so the admin page shows realistic content.
 */
export function ensurePreviewSchemes(
  groups: RoutingSchemeGroupOption[],
  labels: Partial<RoutingSchemeSeedLabels> = {},
): void {
  load()
  if (schemes.value.length > 0) return

  const eligible = groups.filter(isEligibleSeedGroup)
  const grouped = new Map<GroupPlatform, RoutingSchemeGroupOption[]>()
  for (const group of eligible) {
    const list = grouped.get(group.platform) ?? []
    list.push(group)
    grouped.set(group.platform, list)
  }

  const platform = Array.from(grouped.entries())
    .filter(([, list]) => list.length >= 2)
    .sort((a, b) => b[1].length - a[1].length)[0]?.[0]
  if (!platform) return

  const seedLabels = { ...DEFAULT_SEED_LABELS, ...labels }
  const list = grouped.get(platform) ?? []
  schemes.value = [
    buildSeedScheme(seedLabels, platform, list, 'stable', 10),
    buildSeedScheme(seedLabels, platform, list, 'value', 20),
    buildSeedScheme(seedLabels, platform, list, 'cheap', 30),
  ]
  persist()
}

export function useRoutingSchemePreviewStore() {
  load()

  function createScheme(draft: RoutingSchemeDraft): RoutingScheme {
    const timestamp = nowISO()
    const scheme: RoutingScheme = {
      ...draft,
      id: nextID++,
      items: normalizeItems(draft.items),
      created_at: timestamp,
      updated_at: timestamp,
    }
    schemes.value = sortSchemes([...schemes.value, scheme])
    persist()
    return scheme
  }

  function updateScheme(id: number, draft: RoutingSchemeDraft): void {
    schemes.value = sortSchemes(schemes.value.map(item => item.id === id
      ? {
          ...item,
          ...draft,
          items: normalizeItems(draft.items),
          updated_at: nowISO(),
        }
      : item))
    persist()
  }

  function deleteScheme(id: number): void {
    schemes.value = schemes.value.filter(item => item.id !== id)
    persist()
  }

  function toggleScheme(id: number): void {
    schemes.value = schemes.value.map(item => item.id === id
      ? { ...item, status: item.status === 'active' ? 'disabled' : 'active', updated_at: nowISO() }
      : item)
    persist()
  }

  function resetPreviewSchemes(groups: RoutingSchemeGroupOption[], labels: Partial<RoutingSchemeSeedLabels> = {}): void {
    schemes.value = []
    nextID = 1
    loaded = true
    try {
      safeLocalStorage.removeItem(STORAGE_KEY)
    } catch {
      // Ignore storage failures.
    }
    ensurePreviewSchemes(groups, labels)
  }

  return {
    schemes,
    createScheme,
    updateScheme,
    deleteScheme,
    toggleScheme,
    resetPreviewSchemes,
  }
}
