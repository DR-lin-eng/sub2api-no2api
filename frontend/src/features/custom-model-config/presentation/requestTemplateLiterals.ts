const IMPORT_TEMPLATE_SAMPLE = {
  name: 'Image edit to JSON',
  request_adapter: {
    version: 1,
    match: { endpoint: '/v1/images/edits' },
    upstream: {
      path: '/v1/images/generations',
      content_type: 'application/json',
    },
    body: {
      mode: 'merge',
      value: { image: '{{request.input_images}}' },
    },
  },
}

export const REQUEST_TEMPLATE_IMPORT_PLACEHOLDER = JSON.stringify(
  IMPORT_TEMPLATE_SAMPLE,
  null,
  2,
)

export const REQUEST_TEMPLATE_VARIABLES = [
  '{{request.model}}',
  '{{request.upstream_model}}',
  '{{request.prompt}}',
  '{{request.size}}',
  '{{request.quality}}',
  '{{request.n}}',
  '{{request.stream}}',
  '{{request.input_images}}',
  '{{request.mask_image}}',
  '{{request.body.some_field}}',
].join('  ')
