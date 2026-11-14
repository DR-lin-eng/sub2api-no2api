import { ref } from 'vue'
import type {
  RoutingScheme,
  RoutingSchemeDraft,
} from '../../data/dtos/routingSchemeDtos'
import {
  createRoutingScheme,
  deleteRoutingScheme,
  listRoutingSchemes,
  updateRoutingScheme,
} from '../../data/datasources/routingSchemeQueries'

const schemes = ref<RoutingScheme[]>([])
const loading = ref(false)
let loaded = false

function sortSchemes(list: RoutingScheme[]): RoutingScheme[] {
  return [...list].sort((a, b) => {
    if (a.platform !== b.platform) return a.platform.localeCompare(b.platform)
    if (a.sort_order !== b.sort_order) return a.sort_order - b.sort_order
    return a.id - b.id
  })
}

/**
 * Admin routing scheme store. All scores and metrics in `items` are read-only
 * projections computed by the backend; the editor only writes groups, weights
 * and rules.
 */
export function useRoutingSchemeStore() {
  async function load(force = false): Promise<void> {
    if (loading.value || (loaded && !force)) return
    loading.value = true
    try {
      schemes.value = sortSchemes(await listRoutingSchemes({ includeDisabled: true }))
      loaded = true
    } finally {
      loading.value = false
    }
  }

  async function createScheme(draft: RoutingSchemeDraft): Promise<RoutingScheme> {
    const created = await createRoutingScheme(draft)
    schemes.value = sortSchemes([...schemes.value, created])
    return created
  }

  async function updateScheme(id: number, draft: RoutingSchemeDraft): Promise<RoutingScheme> {
    const updated = await updateRoutingScheme(id, draft)
    schemes.value = sortSchemes(schemes.value.map(item => item.id === id ? updated : item))
    return updated
  }

  async function deleteScheme(id: number): Promise<void> {
    await deleteRoutingScheme(id)
    schemes.value = schemes.value.filter(item => item.id !== id)
  }

  async function toggleScheme(scheme: RoutingScheme): Promise<RoutingScheme> {
    return updateScheme(scheme.id, {
      name: scheme.name,
      description: scheme.description,
      platform: scheme.platform,
      status: scheme.status === 'active' ? 'disabled' : 'active',
      sort_order: scheme.sort_order,
      sort_mode: scheme.sort_mode,
      items: scheme.items,
    })
  }

  return {
    schemes,
    loading,
    load,
    createScheme,
    updateScheme,
    deleteScheme,
    toggleScheme,
  }
}
