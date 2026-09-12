import type { AccountQualityPublicPoint } from '../data/datasources/accountQualityPublicDatasource'

export function qualityVerdict(point: AccountQualityPublicPoint): 'passed' | 'wrong' | 'uncertain' | 'error' {
  if (point.status === 'wrong' || point.status === 'degraded') return 'wrong'
  if (point.status === 'healthy' || (point.status === 'ready' && point.label === 'normal')) return 'passed'
  if (point.status === 'ready') {
    const stages = [point.details?.stage1, point.details?.stage2].filter((stage): stage is NonNullable<typeof stage> => !!stage)
    if (stages.length > 0 && stages.every(stage => stage.status === 'passed')) return 'passed'
    if (stages.some(stage => stage.status === 'wrong')) return 'wrong'
    if (stages.some(stage => stage.status === 'error')) return 'error'
  }
  if (point.status === 'ready' && point.label === 'unnormal') return 'wrong'
  if (point.status === 'uncertain' || point.status === 'ready') return 'uncertain'
  return 'error'
}
export const verdictColor = {
  passed: 'bg-emerald-500', wrong: 'bg-rose-500', uncertain: 'bg-amber-400', error: 'bg-slate-400'
}
export function previewURL(point: AccountQualityPublicPoint): string {
  return `/api/v1/account-quality-share/image/${encodeURIComponent(point.id)}?format=webp`
}
export function hasPreview(point: AccountQualityPublicPoint): boolean {
  return !!point.id && (point.has_preview ?? ['ready', 'wrong'].includes(point.status))
}
export function qualityTime(value?: string): string {
  if (!value || Number.isNaN(Date.parse(value))) return '—'
  return new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value)).replace(/\//g, '-')
}
