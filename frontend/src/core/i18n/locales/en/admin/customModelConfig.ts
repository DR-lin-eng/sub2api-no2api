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
    importedName: 'Imported Request Template',
    invalidTemplate: 'Template must be a valid JSON object',
    importFailed: 'Failed to import template',
    name: 'Template Name',
    namePlaceholder: 'e.g. Image edit to JSON',
    description: 'Description',
    descriptionPlaceholder: 'e.g. Convert multipart image edits into JSON',
    sourceEndpoint: 'Match Client Endpoint',
    targetEndpoint: 'Upstream Endpoint',
    endpointRewrite: 'Endpoint Rewrite',
    endpointRewriteHint:
      'Replace the client request path with an upstream path; leave empty to pass through. The upstream endpoint supports {{request.xxx}} template variables.',
    endpointRewriteModes: {
      exact: 'Exact',
      prefix: 'Prefix'
    },
    endpointRewriteExactHint: 'Rewrites only when the request path equals the client endpoint exactly.',
    endpointRewritePrefixHint:
      'Rewrites whenever the request path starts with the client endpoint; the remainder is appended to the upstream endpoint.',
    endpointRewritePreview: 'Preview',
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
    mergeHint: 'The configured object is recursively merged into the original JSON request body.',
    replaceHint: 'The upstream request uses this JSON object as its complete body.',
    noDescription: 'No description',
    empty: 'No request templates',
    invalidJson: 'Request body must be a valid JSON object',
    attributes: 'Template Attributes',
    attributesHint:
      'Declare the input fields models using this template accept, per modality; leave empty for no restriction. Keys may only contain lowercase letters, digits and underscores.',
    attributesVisual: 'Visual',
    attributesJson: 'JSON',
    attributesAddField: 'Add Field',
    attributesNoFields: 'No fields; leave empty to leave this modality unrestricted',
    attributesFieldKey: 'Key',
    attributesFieldKeyPlaceholder: 'e.g. size',
    attributesFieldType: 'Type',
    attributesFieldLabel: 'Label',
    attributesFieldLabelPlaceholder: 'Defaults to the key',
    attributesFieldUI: 'UI',
    attributesUI: {
      auto: 'Auto',
      hidden: 'Hidden'
    },
    attributesFieldRequired: 'Required',
    attributesFieldDefault: 'Default',
    attributesFieldDefaultUnset: 'Not set',
    attributesFieldMin: 'Min',
    attributesFieldMax: 'Max',
    attributesFieldValues: 'Allowed Values',
    attributesFieldValuesPlaceholder: 'e.g. 1024x1024, 1536x1024',
    attributesFieldValuesHint: 'Separate multiple values with commas.',
    attributesModalities: {
      image: 'Image',
      video: 'Video',
      audio: 'Audio'
    },
    attributesRenderMode: {
      label: 'Render Mode',
      overlay: 'Overlay',
      replace: 'Replace',
      hint: {
        overlay: 'Keep default parameters; same-name fields replace their controls',
        replace: 'Only render fields declared by this template'
      }
    },
    attributesHide: {
      label: 'Hide built-in parameters',
      hint: 'Checked parameters are neither rendered nor submitted',
      options: {
        size: 'Resolution / aspect ratio',
        quality: 'Quality',
        n: 'Count',
        resolution: 'Resolution',
        aspect_ratio: 'Aspect ratio',
        duration: 'Duration'
      }
    },
    attributesJsonInvalid: 'Template attribute JSON is invalid',
    attributesInvalid: 'Template attribute JSON is invalid',
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
    videoApiType: {
      label: 'Video API Type',
      autoDetect: 'Auto Detect',
      grok: 'Grok Video',
      agnes: 'Agnes Video'
    },
    videoApiTypeHint: 'Select the API interface type for this video model',
    saveFailed: 'Failed to save, please try again'
  },

  tips: {
    title: 'Usage',
    content: 'This feature allows you to manually configure multimodal capabilities for non-mainstream models. After configuration, the system will determine whether the model supports image, video, or audio generation based on these settings.'
  }
}
