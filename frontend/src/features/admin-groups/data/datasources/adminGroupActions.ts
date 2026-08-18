import { apiClient } from '@/core/networks/client'
import type {
  AdminGroup,
  CompositeModelRoute,
  CompositeModelRouteInput,
  CreateGroupRequest,
  GroupRateMultiplierUpdate,
  GroupRPMOverrideUpdate,
  GroupSortOrderUpdate,
  MessageResponse,
  UpdateGroupRequest,
} from '../dtos/adminGroupDtos'

export async function create(groupData: CreateGroupRequest): Promise<AdminGroup> {
  const { data } = await apiClient.post<AdminGroup>('/admin/groups', groupData)
  return data
}

const duplicateOperationKeys = new Map<string, string>()

interface DuplicateOperationScope {
  adminID: string
  key: string
}

function getCurrentAdminID(): string | null {
  try {
    const rawUser = globalThis.localStorage?.getItem('auth_user')
    if (!rawUser) return null

    const user: unknown = JSON.parse(rawUser)
    if (typeof user !== 'object' || user === null) return null

    const id = (user as { id?: unknown }).id
    if (typeof id !== 'number' || !Number.isSafeInteger(id) || id <= 0) return null
    return String(id)
  } catch {
    return null
  }
}

function duplicateOperationScope(id: number): DuplicateOperationScope | null {
  const adminID = getCurrentAdminID()
  if (!adminID) return null
  return {
    adminID,
    key: `sub2api:admin:group-duplicate:${adminID}:${id}`,
  }
}

function getStoredDuplicateOperationKey(storageKey: string): string | null {
  try {
    return globalThis.sessionStorage?.getItem(storageKey) ?? null
  } catch {
    return null
  }
}

function storeDuplicateOperationKey(storageKey: string, key: string | null): void {
  try {
    if (key) globalThis.sessionStorage?.setItem(storageKey, key)
    else globalThis.sessionStorage?.removeItem(storageKey)
  } catch {
    // The in-memory retry key still protects this page session.
  }
}

export async function duplicate(id: number): Promise<AdminGroup> {
  const scope = duplicateOperationScope(id)
  let idempotencyKey = scope
    ? duplicateOperationKeys.get(scope.key) ?? getStoredDuplicateOperationKey(scope.key)
    : null
  if (!idempotencyKey) {
    const requestID = globalThis.crypto?.randomUUID?.()
      ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
    idempotencyKey = `group-duplicate-${scope?.adminID ?? 'unknown-admin'}-${id}-${requestID}`
  }
  if (scope) {
    duplicateOperationKeys.set(scope.key, idempotencyKey)
    storeDuplicateOperationKey(scope.key, idempotencyKey)
  }

  const { data } = await apiClient.post<AdminGroup>(`/admin/groups/${id}/duplicate`, undefined, {
    headers: { 'Idempotency-Key': idempotencyKey },
  })

  if (scope) {
    duplicateOperationKeys.delete(scope.key)
    storeDuplicateOperationKey(scope.key, null)
  }
  return data
}

export async function update(id: number, updates: UpdateGroupRequest): Promise<AdminGroup> {
  const { data } = await apiClient.put<AdminGroup>(`/admin/groups/${id}`, updates)
  return data
}

export async function deleteGroup(id: number): Promise<MessageResponse> {
  const { data } = await apiClient.delete<MessageResponse>(`/admin/groups/${id}`)
  return data
}

export async function toggleStatus(
  id: number,
  status: 'active' | 'inactive',
): Promise<AdminGroup> {
  return update(id, { status })
}

export async function createCompositeRoute(
  id: number,
  route: CompositeModelRouteInput,
): Promise<CompositeModelRoute> {
  const { data } = await apiClient.post<CompositeModelRoute>(
    `/admin/groups/${id}/composite-routes`,
    route,
  )
  return data
}

export async function updateCompositeRoute(
  id: number,
  routeId: number,
  route: CompositeModelRouteInput,
): Promise<CompositeModelRoute> {
  const { data } = await apiClient.put<CompositeModelRoute>(
    `/admin/groups/${id}/composite-routes/${routeId}`,
    route,
  )
  return data
}

export async function deleteCompositeRoute(id: number, routeId: number): Promise<MessageResponse> {
  const { data } = await apiClient.delete<MessageResponse>(
    `/admin/groups/${id}/composite-routes/${routeId}`,
  )
  return data
}

export async function updateSortOrder(updates: GroupSortOrderUpdate[]): Promise<MessageResponse> {
  const { data } = await apiClient.put<MessageResponse>('/admin/groups/sort-order', { updates })
  return data
}

export async function clearGroupRateMultipliers(id: number): Promise<MessageResponse> {
  const { data } = await apiClient.delete<MessageResponse>(`/admin/groups/${id}/rate-multipliers`)
  return data
}

export async function batchSetGroupRateMultipliers(
  id: number,
  entries: GroupRateMultiplierUpdate[],
): Promise<MessageResponse> {
  const { data } = await apiClient.put<MessageResponse>(`/admin/groups/${id}/rate-multipliers`, {
    entries,
  })
  return data
}

export async function batchSetGroupRPMOverrides(
  id: number,
  entries: GroupRPMOverrideUpdate[],
): Promise<MessageResponse> {
  const { data } = await apiClient.put<MessageResponse>(`/admin/groups/${id}/rpm-overrides`, {
    entries,
  })
  return data
}

export async function clearGroupRPMOverrides(id: number): Promise<MessageResponse> {
  const { data } = await apiClient.delete<MessageResponse>(`/admin/groups/${id}/rpm-overrides`)
  return data
}
