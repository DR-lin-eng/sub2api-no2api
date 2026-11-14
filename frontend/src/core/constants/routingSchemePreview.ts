/**
 * Shared preview storage key while the backend routing-scheme API is not wired.
 * The admin preview store writes this key; the key editor reads it for quick select.
 */
export const ROUTING_SCHEME_PREVIEW_STORAGE_KEY = 'sub2api:preview:group-routing-schemes:v3'

/**
 * Preview mapping from API key id to the scheme metadata shown in the key list.
 * Backend support is not wired yet, so the key list decorates rows from here.
 */
export const KEY_ROUTING_SCHEME_PREVIEW_STORAGE_KEY = 'sub2api:preview:key-routing-schemes:v1'
