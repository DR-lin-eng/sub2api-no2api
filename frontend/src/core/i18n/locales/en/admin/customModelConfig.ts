export default {
  title: 'Custom Model Config',
  description: 'Configure multimodal capabilities for non-mainstream models',

  table: {
    modelName: 'Model Name',
    prefixMatch: 'Prefix Match',
    capabilities: 'Capabilities',
    requestTemplate: 'Request Template',
    noTemplate: 'No template',
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

  template: {
    manage: 'Templates',
    import: 'Import Template',
    export: 'Copy Template JSON',
    copied: 'Template JSON copied',
    copyFailed: 'Failed to copy template JSON',
    managerTitle: 'Request Templates',
    managerHint: 'Store request headers, body and endpoint adaptation rules once and reuse them across models.',
    create: 'Add Template',
    createTitle: 'Add Request Template',
    editTitle: 'Edit Request Template',
    importTitle: 'Import Request Template',
    importInput: 'Paste Template JSON',
    importPreview: 'Formatted Preview',
    importAction: 'Import and Save',
    importHint: 'Paste a complete template JSON or only request_adapter. Formatting fills missing structures automatically.',
    importPlaceholder: '{\n  "name": "Image edit to JSON",\n  "request_adapter": {\n    "version": 1,\n    "match": { "endpoint": "/v1/images/edits" },\n    "upstream": { "path": "/v1/images/generations", "content_type": "application/json" },\n    "body": { "mode": "merge", "value": { "image": "{{request.input_images}}" } }\n  }\n}',
    importedName: 'Imported Request Template',
    invalidTemplate: 'Template must be a valid JSON object',
    importFailed: 'Failed to import template',
    name: 'Template Name',
    namePlaceholder: 'e.g. Image edit to JSON',
    description: 'Description',
    descriptionPlaceholder: 'e.g. Convert multipart image edits into JSON',
    sourceEndpoint: 'Match Client Endpoint',
    targetEndpoint: 'Upstream Endpoint',
    contentType: 'Upstream Content Type',
    preserve: 'Preserve request format',
    headers: 'Custom Headers',
    headersHint: 'These values override matching upstream headers; authentication and Content-Type remain system-managed.',
    addHeader: 'Add Header',
    headerName: 'Header name',
    headerValue: 'Header value',
    noHeaders: 'No custom headers',
    bodyMode: 'Request Body Mode',
    bodyModes: {
      off: 'Preserve request body',
      merge: 'Merge partial body',
      replace: 'Replace full body'
    },
    requestBody: 'Request Body JSON',
    format: 'Format',
    loadSample: 'Load Sample',
    sampleName: 'Image Edit to JSON Sample',
    sampleDescription: 'Convert image edit requests to JSON and place reference images in the image array.',
    variablesTitle: 'Available variables',
    variables: '{{request.model}}  {{request.upstream_model}}  {{request.prompt}}  {{request.size}}  {{request.quality}}  {{request.n}}  {{request.stream}}  {{request.input_images}}  {{request.mask_image}}  {{request.body.some_field}}',
    mergeHint: 'The configured object is recursively merged into the original JSON request body.',
    replaceHint: 'The upstream request uses this JSON object as its complete body.',
    noDescription: 'No description',
    empty: 'No request templates',
    invalidJson: 'Request body must be a valid JSON object',
    saveFailed: 'Failed to save request template',
    deleteConfirm: 'Delete this request template? Models using it will be unlinked.',
    deleteFailed: 'Failed to delete request template'
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
    requestTemplate: 'Request Template',
    noTemplate: 'Do not use a request template',
    requestTemplateHint: 'Multiple models can share one template, and template changes apply centrally.',
    saveFailed: 'Failed to save, please try again'
  },

  tips: {
    title: 'Usage',
    content: 'This feature allows you to manually configure multimodal capabilities for non-mainstream models. After configuration, the system will determine whether the model supports image, video, or audio generation based on these settings.'
  }
}
