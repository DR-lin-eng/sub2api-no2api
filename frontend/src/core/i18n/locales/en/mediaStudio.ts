export default {
  mediaStudio: {
    title: 'Media Studio',
    description: 'Choose a creation type, describe what you need, then adjust aspect ratio, quality, and output count.',
    modeItems: {
      image: {
        title: 'Image Generation',
        short: 'Prompt, reference, aspect, and style'
      },
      video: {
        title: 'Video Generation',
        short: 'First frame, motion, duration, and aspect'
      },
      disabled: 'Unavailable'
    },
    composer: {
      greeting: 'Hi, what do you want to create?',
      placeholder: 'Describe the image you want to generate, press Ctrl/⌘ + Enter to start.',
      videoPlaceholder: 'Describe the scene, camera motion, and mood, then press Ctrl/⌘ + Enter to generate a video.',
      model: 'Model',
      customWidth: 'Custom width',
      customHeight: 'Custom height',
      aspectRatio: 'Ratio',
      customAspectRatio: {
        option: 'Custom ratio',
        title: 'Add custom aspect ratio',
        width: 'Horizontal ratio',
        height: 'Vertical ratio',
        hint: 'Use positive integers. The long side cannot be more than 3 times the short side.',
        invalid: 'Enter a valid aspect ratio.',
        add: 'Add'
      },
      selectGroup: 'Select media group',
      loadingGroups: 'Loading media groups…',
      loadingModels: 'Loading models…',
      noGroups: 'No media groups have been configured by the administrator.',
      manualModelHint: 'No model for this creation type was returned by the current group; you can enter a model name manually.',
      reload: 'Retry',
      shortHint: 'Choose an administrator-configured media group to generate. Results are saved in this browser.',
      unit: 'image',
      countValue: '{count} images',
      durationValue: '{count} seconds',
      send: 'Start generation',
      imageEdit: {
        attachHint: 'Drop or paste reference images, up to 9',
        attachCount: '{count}/{limit} images attached',
        remove: 'Remove image'
      }
    },
    session: {
      localHint: 'Session and image results are saved in this browser',
      clear: 'Clear',
      select: 'Select',
      selectAll: 'Select all',
      deselectAll: 'Deselect all',
      deleteSelected: 'Delete selected',
      cancelSelect: 'Cancel selection',
      selectMessage: 'Select message',
      selectedCount: '{count} selected',
      you: 'You',
      studio: 'Media Studio',
      failed: 'Generation failed',
      retry: 'Retry',
      enlargeImage: 'Enlarge image',
      editImage: 'Edit this image',
      noImageResult: 'Task completed, but no preview image was returned.',
      noVideoResult: 'Task completed, but no preview video was returned.'
    },
  }
}
