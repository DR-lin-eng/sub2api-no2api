export type MediaStudioModeId = 'image' | 'video' | 'batch'

export interface MediaStudioMode {
  id: MediaStudioModeId
  iconName: 'grid' | 'play' | 'copy'
  available: boolean
}

const mediaStudioModes: MediaStudioMode[] = [
  {
    id: 'image',
    iconName: 'grid',
    available: true,
  },
  {
    id: 'video',
    iconName: 'play',
    available: false,
  },
  {
    id: 'batch',
    iconName: 'copy',
    available: false,
  },
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
