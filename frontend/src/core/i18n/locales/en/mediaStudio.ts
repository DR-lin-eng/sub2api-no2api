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
      group: 'Group',
      model: 'Model',
      resolution: 'Resolution',
      quality: 'Quality',
      count: 'Count',
      duration: 'Duration',
      durationIncrease: 'Increase duration',
      durationDecrease: 'Decrease duration',
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
      manualModelHint: 'The current group did not return any models for this creation type.',
      reload: 'Retry',
      shortHint: 'Choose an administrator-configured media group to generate. Results are saved in this browser.',
      advancedSettings: 'More',
      resourceSettings: 'Basic settings',
      imageSettings: 'Image options',
      videoSettings: 'Video options',
      unit: 'image',
      countValue: '{count} images',
      durationValue: '{count} seconds',
      send: 'Start generation',
      imageEdit: {
        attachHint: 'Drop or paste reference images, up to 9',
        attachCount: '{count}/{limit} images attached',
        remove: 'Remove image'
      },
      attachmentType: {
        image: 'Image',
        video: 'Video',
        audio: 'Audio'
      },
      upload: {
        addHint: 'Add reference files (image / video / audio)',
        dropHint: 'Drop files here to upload',
        disabled: 'The current model does not allow file uploads',
        noTypes: 'The current model has no enabled file types',
        storageUnavailable: 'Object storage is not configured yet, uploads are unavailable',
        retry: 'Retry upload',
        rejections: {
          type: '"{name}" is not a supported file type for this model',
          size: '"{name}" exceeds the per-file size limit ({limit} MB)',
          count: '"{name}" exceeds the file count limit ({limit} files)',
          total: '"{name}" would exceed the total size limit ({limit} MB)'
        }
      },
      estimate: {
        loading: 'Estimating cost…',
        value: 'Estimated cost {amount}',
        failed: 'Cost estimate unavailable: {message}'
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
      elapsed: 'Took {time}',
      cost: 'Charged {amount}',
      costHeld: 'Reserved {amount}',
      editImage: 'Edit this image',
      noImageResult: 'Task completed, but no preview image was returned.',
      noVideoResult: 'Task completed, but no preview video was returned.'
    },
  }
}
