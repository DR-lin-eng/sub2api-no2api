import { ref } from 'vue'

// Every example in the docs uses this origin. Readers (or an operator linking
// here with ?base_url=) can swap it for their own gateway; the API key
// placeholder is never collected by this site.
export const PLACEHOLDER_BASE_URL = 'https://api.example.com'
const STORAGE_KEY = 'sub2api-docs:base-url'
const QUERY_KEY = 'base_url'

export const gatewayBaseUrl = ref('')

const SAFE_BASE_URL = /^https?:\/\/(?:[a-z0-9.-]+|\[[0-9a-f:.]+\])(?::\d+)?(?:\/[A-Za-z0-9._~-]+)*$/

export function normalizeBaseUrl(input: string): string {
  const value = input.trim()
  if (!value) return ''
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return ''
  }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return ''
  if (url.username || url.password) return ''
  // Keep a sub-path deployment, drop a trailing /v1 so examples do not repeat it.
  const path = url.pathname.replace(/\/+$/, '').replace(/\/v1$/, '')
  const normalized = `${url.origin}${path}`
  // The value is pasted into shell examples and can arrive from a shared link,
  // so anything a shell could expand ($, backticks, quotes, %, ...) is refused.
  return SAFE_BASE_URL.test(normalized) ? normalized : ''
}

// A value typed by the reader is kept in localStorage. A value that arrived in
// a link is kept for this tab only, so a shared link cannot change the examples
// the reader sees on later visits.
export function setGatewayBaseUrl(input: string, fromLink = false): boolean {
  const normalized = normalizeBaseUrl(input)
  if (input.trim() && !normalized) return false
  gatewayBaseUrl.value = normalized
  try {
    sessionStorage.removeItem(STORAGE_KEY)
    if (fromLink) sessionStorage.setItem(STORAGE_KEY, normalized)
    else if (normalized) localStorage.setItem(STORAGE_KEY, normalized)
    else localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Storage can be blocked; the value still applies to this page view.
  }
  applyGatewayBaseUrl()
  return true
}

export function loadGatewayBaseUrl(): void {
  const fromQuery = normalizeBaseUrl(new URLSearchParams(location.search).get(QUERY_KEY) ?? '')
  if (fromQuery) {
    setGatewayBaseUrl(fromQuery, true)
    return
  }
  try {
    gatewayBaseUrl.value = normalizeBaseUrl(sessionStorage.getItem(STORAGE_KEY) ?? localStorage.getItem(STORAGE_KEY) ?? '')
  } catch {
    gatewayBaseUrl.value = ''
  }
  applyGatewayBaseUrl()
}

const originals = new WeakMap<Text, string>()

// Only text nodes are rewritten, so a configured value can never become markup.
export function applyGatewayBaseUrl(): void {
  const root = document.querySelector('.vp-doc')
  if (!root) return
  const replacement = gatewayBaseUrl.value || PLACEHOLDER_BASE_URL
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  for (let node = walker.nextNode() as Text | null; node; node = walker.nextNode() as Text | null) {
    const original = originals.get(node) ?? node.data
    if (!original.includes(PLACEHOLDER_BASE_URL)) continue
    originals.set(node, original)
    const next = original.split(PLACEHOLDER_BASE_URL).join(replacement)
    if (node.data !== next) node.data = next
  }
}
