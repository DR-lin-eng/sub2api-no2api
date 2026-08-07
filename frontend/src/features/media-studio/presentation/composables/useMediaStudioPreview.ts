export type MediaStudioModeId = 'image' | 'video' | 'batch'

export interface MediaStudioMode {
  id: MediaStudioModeId
  icon: string
  accentClass: string
  available: boolean
}

export interface MediaStudioPreviewStage {
  id: 'prompt' | 'configure' | 'generate' | 'deliver'
  step: number
}

const mediaStudioModes: MediaStudioMode[] = [
  {
    id: 'image',
    icon: '✦',
    accentClass: 'from-fuchsia-500 to-rose-500',
    available: false,
  },
  {
    id: 'video',
    icon: '▶',
    accentClass: 'from-sky-500 to-indigo-500',
    available: false,
  },
  {
    id: 'batch',
    icon: '▦',
    accentClass: 'from-emerald-500 to-teal-500',
    available: false,
  },
]

const mediaStudioPreviewStages: MediaStudioPreviewStage[] = [
  { id: 'prompt', step: 1 },
  { id: 'configure', step: 2 },
  { id: 'generate', step: 3 },
  { id: 'deliver', step: 4 },
]

export function useMediaStudioPreview() {
  function getModeById(id: MediaStudioModeId): MediaStudioMode {
    return mediaStudioModes.find((mode) => mode.id === id) ?? mediaStudioModes[0]
  }

  return {
    modes: mediaStudioModes,
    previewStages: mediaStudioPreviewStages,
    getModeById,
  }
}
