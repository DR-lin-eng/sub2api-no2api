import { sanitizeChatHtml } from '@/features/support-chat/presentation/utils/sanitizeChatHtml'

export interface SupportStickerPayload {
  id: string
  name: string
  emoji?: string
  url?: string
}

export interface ParsedSupportMessageContent {
  html: string
  sticker: SupportStickerPayload | null
}

const STICKER_MARKER_PATTERN = /\[\[support-sticker:([^\]]+)\]\]/
const LEGACY_STICKER_PATTERN = /^<strong\s+title="([^"]{1,80})">([^<]{1,120})<\/strong>$/i

const LEGACY_STICKER_EMOJI_BY_NAME: Record<string, string> = {
  收到: '🫡',
  Received: '🫡',
  稍等: '⏳',
  Wait: '⏳',
}

export function escapeChatHtml(value: string): string {
  return value.replace(/[&<>"']/g, (character) => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  })[character] || character)
}

export function buildImageMessageContent(url: string, name: string, fallbackName: string): string {
  const safeName = name || fallbackName
  return `<img src="${escapeChatHtml(url)}" alt="${escapeChatHtml(safeName)}" title="${escapeChatHtml(safeName)}" />`
}

export function buildStickerMessageContent(sticker: SupportStickerPayload): string {
  const payload: SupportStickerPayload = {
    id: sticker.id,
    name: sticker.name,
    emoji: sticker.emoji,
    url: sticker.url,
  }
  return `[[support-sticker:${encodeURIComponent(JSON.stringify(payload))}]]`
}

export function appendTextContent(prefix: string, content: string): string {
  const text = prefix.trim()
  if (!text) return content
  return `${escapeChatHtml(text)}\n${content}`
}

export function parseSupportMessageContent(content: string): ParsedSupportMessageContent {
  const markerMatch = content.match(STICKER_MARKER_PATTERN)
  if (markerMatch) {
    const sticker = parseStickerPayload(markerMatch[1])
    const textContent = `${content.slice(0, markerMatch.index)}${content.slice((markerMatch.index ?? 0) + markerMatch[0].length)}`.trim()
    return {
      html: textContent ? sanitizeChatHtml(textContent) : '',
      sticker,
    }
  }

  const legacySticker = parseLegacySticker(content)
  if (legacySticker) {
    return {
      html: '',
      sticker: legacySticker,
    }
  }

  return {
    html: sanitizeChatHtml(content),
    sticker: null,
  }
}

function parseStickerPayload(encodedPayload: string): SupportStickerPayload | null {
  try {
    const parsed = JSON.parse(decodeURIComponent(encodedPayload)) as Partial<SupportStickerPayload>
    if (!parsed || typeof parsed.name !== 'string') return null
    const emoji = typeof parsed.emoji === 'string' ? parsed.emoji : ''
    const url = typeof parsed.url === 'string' ? parsed.url : ''
    if (!emoji && !url) return null
    return {
      id: typeof parsed.id === 'string' ? parsed.id : 'custom',
      name: parsed.name,
      emoji: normalizeStickerEmoji(parsed.name, emoji),
      url,
    }
  } catch {
    return null
  }
}

function parseLegacySticker(content: string): SupportStickerPayload | null {
  const match = content.trim().match(LEGACY_STICKER_PATTERN)
  if (!match) return null
  const name = decodeHtmlEntity(match[1]).trim()
  const body = decodeHtmlEntity(match[2]).trim()
  if (!name || !body) return null
  const firstToken = body.split(/\s+/)[0] || body
  return {
    id: `legacy-${name}`,
    name,
    emoji: normalizeStickerEmoji(name, firstToken),
  }
}

function normalizeStickerEmoji(name: string, emoji: string): string {
  const trimmedEmoji = emoji.trim()
  const mapped = LEGACY_STICKER_EMOJI_BY_NAME[name]
  if (mapped && (!trimmedEmoji || trimmedEmoji === name)) return mapped
  return trimmedEmoji || mapped || '💬'
}

function decodeHtmlEntity(value: string): string {
  const textarea = document.createElement('textarea')
  textarea.innerHTML = value
  return textarea.value
}
