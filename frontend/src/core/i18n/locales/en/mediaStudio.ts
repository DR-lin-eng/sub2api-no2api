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
      }
    },
    composer: {
      assets: 'Assets',
      greeting: 'Hi, what do you want to create?',
      creationType: 'Creation type',
      placeholderPrefix: 'Enter text or',
      placeholderSuffix: 'subject, and describe what you want to generate.',
      model: 'Image 5.0 Lite',
      textTool: 'Text tool',
      mention: 'Mention subject',
      unit: 'image',
      send: 'Start generation'
    }
  }
}
