/**
 * UI preview for the backend-computed group score.
 *
 * The real score is produced by the backend from recent requests (first-token
 * latency, success rate, error rate and other dimensions). Until that API is
 * wired, derive a stable sample score from the group identity so the admin UI
 * can render the real layout.
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
}

const SCORE_WINDOW_SECONDS = 900

function sampleRatio(groupID: number, name: string): number {
  const input = `${groupID}:${name}`
  let hash = 2166136261
  for (let i = 0; i < input.length; i += 1) {
    hash ^= input.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  return ((hash >>> 0) % 1000) / 1000
}

export function previewGroupScore(groupID: number, name = ''): RoutingGroupScore {
  const ratio = sampleRatio(groupID, name)
  const successRate = Math.round((95.5 + ratio * 4.4) * 10) / 10
  const sampleCount = 60 + Math.round(ratio * 500)
  return {
    overall: Math.round(68 + ratio * 30),
    ttft_seconds: Math.round((0.6 + (1 - ratio) * 3.4) * 10) / 10,
    success_rate: successRate,
    error_rate: Math.round((100 - successRate) * 10) / 10,
    error_count: Math.round(sampleCount * (100 - successRate) / 100),
    sample_count: sampleCount,
    window_seconds: SCORE_WINDOW_SECONDS,
  }
}

export function scorePercent(score: number, scores: number[]): number {
  const total = scores.reduce((sum, value) => sum + value, 0)
  if (total <= 0) return 0
  return Math.round((score / total) * 100)
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
