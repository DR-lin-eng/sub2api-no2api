export const usageObject = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}

export const usageRows = (value: unknown): Record<string, unknown>[] =>
  Array.isArray(value) ? value.map(usageObject) : []

export const usageNumber = (value: unknown): number | null =>
  typeof value === 'number' && Number.isFinite(value) ? value : null

export function dailyModelUsage(data: Record<string, unknown> | null) {
  return usageRows(data?.data).flatMap(day => usageRows(day.groups ?? day.models).map(model => {
    const uncached = usageNumber(model.uncached_text_input_tokens)
    const cached = usageNumber(model.cached_text_input_tokens)
    const output = usageNumber(model.text_output_tokens)
    const total = usageNumber(model.text_total_tokens) ?? (uncached !== null || cached !== null || output !== null ? (uncached ?? 0) + (cached ?? 0) + (output ?? 0) : data?.units === 'tokens' ? usageNumber(model.credits) : null)
    return {
      date: String(day.date ?? ''),
      model: String(model.model ?? usageObject(model.dimensions).model ?? (model.is_other ? 'other' : '—')),
      uncached, cached, output, total,
      credits: data?.units === 'tokens' ? null : usageNumber(model.credits)
    }
  }))
}

// History is approximate: carry the last observation forward between samples,
// and estimate full allowance only where a reset boundary can be inferred.
export function sampleUsageBalance(points: Record<string, unknown>[], maxPoints = 64) {
  const sorted = points.filter(point => Number.isFinite(Date.parse(String(point.observed_at)))).slice().sort((a, b) => Date.parse(String(a.observed_at)) - Date.parse(String(b.observed_at)))
  if (!sorted.length) return []
  const start = Date.parse(String(sorted[0].observed_at))
  const end = Date.parse(String(sorted[sorted.length - 1].observed_at))
  const count = sorted.length === 1 ? 1 : maxPoints
  let index = 0
  return Array.from({ length: count }, (_, i) => {
    const timestamp = count === 1 ? start : start + (end - start) * i / (count - 1)
    while (index + 1 < sorted.length && Date.parse(String(sorted[index + 1].observed_at)) <= timestamp) index++
    const point = sorted[index]
    const next = sorted[index + 1]
    const reset = Date.parse(String(point.estimated_reset_at))
    const nextReset = next ? Date.parse(String(next.estimated_reset_at)) : Number.NaN
    const inferredReset = next && next.plan_type === point.plan_type && reset > Date.parse(String(point.observed_at)) && reset <= timestamp && nextReset - reset > 60_000
    const remaining = usageNumber(point.remaining_basis_points)
    return { timestamp, remaining: inferredReset ? 100 : remaining === null ? null : remaining / 100 }
  })
}

export function threadUsageRows(data: Record<string, unknown> | null) {
  return usageRows(data?.threads).map(thread => {
    const result = { ...thread }
    const groups = usageRows(thread.groups)
    for (const key of ['net_new_input_tokens', 'cached_input_tokens', 'output_tokens']) {
      if (usageNumber(result[key]) !== null) continue
      const reported = groups.map(group => usageNumber(group[key])).filter((value): value is number => value !== null)
      result[key] = reported.length ? reported.reduce((sum, value) => sum + value, 0) : null
    }
    return result
  })
}

export function usageScalarFields(value: unknown, prefix = ''): { key: string; value: unknown }[] {
  return Object.entries(usageObject(value)).flatMap(([key, item]) => {
    const path = prefix ? `${prefix}.${key}` : key
    if (item !== null && typeof item === 'object') return Array.isArray(item) ? [] : usageScalarFields(item, path)
    return [{ key: path, value: item }]
  })
}
