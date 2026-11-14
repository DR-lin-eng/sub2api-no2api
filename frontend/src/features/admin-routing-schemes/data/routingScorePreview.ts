/**
 * Read-only presentation helpers for the backend-computed group score.
 *
 * The score itself comes from `/admin/routing-schemes`; this module only keeps
 * the shared shape and the percentage/color mapping used by the admin UI.
 */
export interface RoutingGroupScore {
  /** 0-100 composite score used to normalize the scheduling share. */
  overall: number
  ttft_seconds: number
  success_rate: number
  error_rate: number
  error_count: number
  sample_count: number
  window_seconds: number
  cold_start?: boolean
}

/**
 * Normalizes scores into integer percentages that always sum to 100.
 * Largest-remainder rounding avoids the "99%" / "101%" drift of rounding each
 * share independently.
 */
export function scorePercents(scores: number[]): number[] {
  const total = scores.reduce((sum, value) => sum + value, 0)
  if (total <= 0) return scores.map(() => 0)
  const exact = scores.map(value => (value / total) * 100)
  const result = exact.map(value => Math.floor(value))
  let remainder = 100 - result.reduce((sum, value) => sum + value, 0)
  const byFraction = exact
    .map((value, index) => ({ index, fraction: value - result[index] }))
    .sort((left, right) => right.fraction - left.fraction)
  for (let i = 0; i < byFraction.length && remainder > 0; i += 1) {
    result[byFraction[i].index] += 1
    remainder -= 1
  }
  return result
}

export function scoreToneClass(score: number): string {
  if (score >= 90) return 'text-emerald-600 dark:text-emerald-400'
  if (score >= 78) return 'text-amber-600 dark:text-amber-400'
  return 'text-red-600 dark:text-red-400'
}

export function scoreBarClass(score: number): string {
  if (score >= 90) return 'bg-emerald-500'
  if (score >= 78) return 'bg-amber-400'
  return 'bg-red-500'
}
