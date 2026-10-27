const diagnostics = {
  state: 'fresh', fetched_at: '2026-10-27T14:00:00Z', models: ['grok-4.6', 'grok-composer-2.5-fast', 'private-relay-custom-model'],
  mappings: [
    { requested: 'xai/grok-4.6-latest', upstream: 'grok-4.6', status: 'listed' },
    { requested: 'my-chat-model', upstream: 'grok-4.5', status: 'not_listed' },
    { requested: 'grok-imagine-image', upstream: 'grok-imagine-image', status: 'out_of_scope' },
    { requested: 'my-private-model-with-a-long-name', upstream: 'private-relay-custom-model', status: 'unverifiable' },
  ],
}
export async function getById() { return { grok_model_diagnostics: diagnostics } }
export const syncUpstreamModels = getById
