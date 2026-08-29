export type MediaStudioModeId = 'image' | 'video'

export interface MediaStudioMode {
  id: MediaStudioModeId
  iconName: 'grid' | 'play'
  available: boolean
}

const mediaStudioModes: MediaStudioMode[] = [
  { id: 'image', iconName: 'grid', available: true },
  { id: 'video', iconName: 'play', available: true },
]

export function useMediaStudioPreview() {
  function getModeById(id: MediaStudioModeId): MediaStudioMode {
    return mediaStudioModes.find((mode) => mode.id === id) ?? mediaStudioModes[0]
  }

  return {
    modes: mediaStudioModes,
    getModeById,
  }
}
