export interface PermissionDefinition {
  key: string
  name: string
  description: string
}

export interface PermissionGroup {
  id: string
  name: string
  permissions: string[]
  built_in: boolean
}

export interface PermissionGroupsResponse {
  groups: PermissionGroup[]
  permissions: PermissionDefinition[]
}
