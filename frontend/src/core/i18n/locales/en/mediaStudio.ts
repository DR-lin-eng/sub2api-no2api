export default {
  mediaStudio: {
    title: 'Media Studio',
    description: 'A unified workspace preview for future image, video, and batch creative generation. The creation entry, capability cards, and task-flow shell are ready for real generation APIs later.',
    hero: {
      eyebrow: 'Media Studio Preview',
      previewLabel: 'Current preview mode',
      tags: {
        images: 'Image generation',
        videos: 'Video generation',
        preview: 'Task preview'
      },
      stats: {
        modes: {
          value: '3',
          label: 'Media entries'
        },
        flow: {
          value: '4',
          label: 'Flow steps'
        },
        api: {
          value: '0',
          label: 'APIs wired'
        }
      }
    },
    status: {
      preview: 'Preview shell',
      comingSoon: 'Coming soon'
    },
    modes: {
      eyebrow: 'Generation types',
      title: 'Reserved entry points for multimedia generation',
      description: 'Each entry keeps its own UI and state owner so image, video, or batch jobs can be wired later without reshaping the page.',
      selectPreview: 'Preview this'
    },
    modeItems: {
      image: {
        title: 'Image Generation',
        description: 'Reserved areas for prompts, size, style, model selection, and result previews.',
        hint: 'Image generation will show result grids, downloads, and retry states here.'
      },
      video: {
        title: 'Video Generation',
        description: 'Reserved areas for scripts, duration, aspect ratio, reference images, and storyboard previews.',
        hint: 'Video generation will show covers, transcoding status, playable previews, and asset references here.'
      },
      batch: {
        title: 'Batch Creation',
        description: 'Reserved areas for prompt batches, queue progress, result archiving, and unified exports.',
        hint: 'Batch creation will show task queues, success rate, cost holds, and batch downloads here.'
      }
    },
    workspace: {
      eyebrow: 'Workflow shell',
      title: 'A unified path from prompt to delivery',
      description: 'The shell fixes the four-stage structure: input, configuration, generation, and delivery. When real APIs are added, only the datasource and task state need to be wired.',
      canvasLabel: 'Preview Canvas',
      generatePlaceholder: 'Generate placeholder',
      previewState: 'Waiting for generation service'
    },
    stages: {
      prompt: {
        title: 'Enter creative prompt',
        description: 'Holds text, reference assets, and batch items.'
      },
      configure: {
        title: 'Choose model and parameters',
        description: 'Holds size, aspect ratio, duration, style, and quality settings.'
      },
      generate: {
        title: 'Submit generation task',
        description: 'Holds queue, progress, retry, and cancellation states.'
      },
      deliver: {
        title: 'Preview and deliver results',
        description: 'Holds previews, downloads, archives, and usage feedback.'
      }
    }
  }
}
