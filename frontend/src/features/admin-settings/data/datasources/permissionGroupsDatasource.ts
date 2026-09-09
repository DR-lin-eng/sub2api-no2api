import { apiClient } from '@/core/networks/client'
import type {
  PermissionGroup,
  PermissionGroupsResponse,
} from '@/features/admin-settings/data/dtos/permissionGroupDtos'

export async function getPermissionGroups(): Promise<PermissionGroupsResponse> {
  const { data } = await apiClient.get<PermissionGroupsResponse>('/admin/settings/permission-groups')
  return data
}

export async function updatePermissionGroups(groups: PermissionGroup[]): Promise<PermissionGroupsResponse> {
  const { data } = await apiClient.put<PermissionGroupsResponse>('/admin/settings/permission-groups', { groups })
  return data
}
