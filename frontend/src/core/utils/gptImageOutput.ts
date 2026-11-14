export interface GPTImageOutputOptions {
  output_format?: 'png' | 'jpeg' | 'webp' | 'gif' | 'svg'
  frame_count?: number
  frame_duration_ms?: number
  svg?: { model: string; reasoning?: { effort: string } }
}

export function supportsGPTImage26Output(model: string): boolean {
  return /^gpt-image-2\.6-(flare|sunburst)$/i.test(model.trim())
}

export function normalizeGPTImageOutput(value: unknown): GPTImageOutputOptions {
  if (!value || typeof value !== 'object') return {}
  const options = value as GPTImageOutputOptions
  const format = options.output_format
  if (!format || !['png', 'jpeg', 'webp', 'gif', 'svg'].includes(format)) return {}
  if (format === 'gif') {
    const integer = (n: unknown, fallback: number, min: number, max: number) => typeof n === 'number' && Number.isInteger(n) && n >= min && n <= max ? n : fallback
    return { output_format: format, frame_count: integer(options.frame_count, 9, 2, 16), frame_duration_ms: integer(options.frame_duration_ms, 200, 20, 2000) }
  }
  const result: GPTImageOutputOptions = { output_format: format }
  if (format === 'svg' && typeof options.svg?.model === 'string' && options.svg.model.trim()) {
    result.svg = { model: options.svg.model.trim().slice(0, 256) }
    const effort = options.svg.reasoning?.effort
    if (effort && ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'].includes(effort)) result.svg.reasoning = { effort }
  }
  return result
}

export function imageOutputExtension(mimeType = ''): string {
  if (mimeType.includes('svg')) return 'svg'
  if (mimeType.includes('gif')) return 'gif'
  if (mimeType.includes('jpeg')) return 'jpg'
  if (mimeType.includes('webp')) return 'webp'
  return 'png'
}
