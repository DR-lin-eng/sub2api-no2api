export default {
  title: 'Custom Model Config',
  description: 'Configure multimodal capabilities for non-mainstream models',

  table: {
    modelName: 'Model Name',
    prefixMatch: 'Prefix Match',
    capabilities: 'Capabilities',
    actions: 'Actions',
    empty: 'No configurations',
    emptyHint: 'Click "Add Config" button to add custom model configuration'
  },

  capabilities: {
    image: 'Image Generation',
    video: 'Video Generation',
    audio: 'Audio Generation'
  },

  actions: {
    backToSettings: 'Back to feature settings',
    create: 'Add Config',
    edit: 'Edit',
    delete: 'Delete',
    deleteConfirm: 'Are you sure you want to delete this configuration?',
    deleteFailed: 'Failed to delete model configuration'
  },

  modal: {
    createTitle: 'Add Model Configuration',
    editTitle: 'Edit Model Configuration',
    modelName: 'Model Name',
    modelNamePlaceholder: 'e.g., flux-pro, midjourney-v6',
    modelNameHint: 'Enter the full model name, must match exactly with the API-returned model name',
    modelNamePrefixHint: 'Enter a model prefix, such as agnes-, to match every model that starts with it',
    prefixMatch: 'Match by model prefix',
    prefixMatchHint: 'When enabled, the value matches multiple models by prefix; otherwise it matches one exact model name',
    capabilities: 'Model Capabilities',
    capabilitiesHint: 'Select all capabilities this model supports',
    saveFailed: 'Failed to save, please try again'
  },

  tips: {
    title: 'Usage',
    content: 'This feature allows you to manually configure multimodal capabilities for non-mainstream models. After configuration, the system will determine whether the model supports image, video, or audio generation based on these settings.'
  }
}
