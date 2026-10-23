import { apiClient } from '@/core/networks/client'

export type MediaStudioGroupMediaType = 'image' | 'video' | 'audio'

export interface MediaStudioGroupRoute {
 group_id: number
 media_type: MediaStudioGroupMediaType
 priority: number
 enabled: boolean
 models?: string[]
}

export type MediaStudioGroupRoutes = MediaStudioGroupRoute[]

export async function getMediaStudioGroupRoutes(): Promise<MediaStudioGroupRoutes> {
  const { data } = await apiClient.get<MediaStudioGroupRoutes>('/admin/media-studio/group-routes')
  return data
}

export async function saveMediaStudioGroupRoutes(
  routes: MediaStudioGroupRoutes,
): Promise<MediaStudioGroupRoutes> {
  const { data } = await apiClient.put<MediaStudioGroupRoutes>(
    '/admin/media-studio/group-routes',
    routes,
  )
  return data
}
