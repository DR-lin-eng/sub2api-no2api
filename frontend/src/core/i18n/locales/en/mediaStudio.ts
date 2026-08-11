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
      batch: {
        title: 'Batch Creation',
        short: 'Prompt batches, variables, and queue'
      },
      disabled: 'Later'
    },
    composer: {
      greeting: 'Hi, what do you want to create?',
      placeholder: 'Describe the image you want to generate, press Ctrl/⌘ + Enter to start.',
      model: 'Model',
      selectKey: 'Select Key',
      loadingKeys: 'Loading keys…',
      loadingModels: 'Loading models…',
      noKeys: 'No enabled API keys available.',
      manualModelHint: 'No image model found from the current key; you can enter a model name manually.',
      reload: 'Retry',
      shortHint: 'Current version only submits image tasks; video and batch entries are reserved for future integration.',
      unit: 'image',
      countValue: '{count} images',
      send: 'Start generation'
    },
    session: {
      localHint: 'Session saved in local browser',
      clear: 'Clear',
      you: 'You',
      studio: 'Media Studio',
      failed: 'Generation failed',
      retry: 'Retry',
      noImageResult: 'Task completed, but no preview image was returned.'
    }
  }
}
