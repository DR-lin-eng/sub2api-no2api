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
      disabled: 'Unavailable'
    },
    composer: {
      greeting: 'Hi, what do you want to create?',
      placeholder: 'Describe the image you want to generate, press Ctrl/⌘ + Enter to start.',
      videoPlaceholder: 'Describe the scene, camera motion, and mood, then press Ctrl/⌘ + Enter to generate a video.',
      model: 'Model',
      selectKey: 'Select Key',
      loadingKeys: 'Loading keys…',
      loadingModels: 'Loading models…',
      noKeys: 'No enabled API keys available.',
      manualModelHint: 'No model for this creation type was returned by the current key; you can enter a model name manually.',
      reload: 'Retry',
      shortHint: 'Images and videos use the selected API key. Generated results remain in this page\'s memory only.',
      batchHint: 'Batch creation reuses the existing job queue, cost estimate, preview, and download workflow.',
      unit: 'image',
      countValue: '{count} images',
      durationValue: '{count} seconds',
      send: 'Start generation'
    },
    session: {
      localHint: 'Session stays only in this page memory',
      clear: 'Clear',
      you: 'You',
      studio: 'Media Studio',
      failed: 'Generation failed',
      retry: 'Retry',
      noImageResult: 'Task completed, but no preview image was returned.',
      noVideoResult: 'Task completed, but no preview video was returned.'
    },
    batch: {
      title: 'Open the batch image workspace',
      description: 'Use the existing verified Gemini batch flow to submit prompts, track jobs, preview results, and download archives. API-key and group capability checks run again in that workspace.',
      open: 'Open batch workspace'
    }
  }
}
