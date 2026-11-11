import type { AccountUsageInfo } from '@/types'

interface AntigravityUsageResult { utilization: number; resetTime: string | null }

export function resolveAntigravityQuotaGroupBars(groups: AccountUsageInfo['antigravity_quota_groups'], t: (key: string) => string) {
  if (!groups || groups.length === 0) return []
  const bars: Array<{
    key: string
    label: string
    title: string
    utilization: number
    resetsAt: string | null
    color: 'indigo' | 'emerald' | 'purple' | 'amber'
  }> = []
  for (const group of groups) {
    // 'gemini' = Gemini models, 'gemini-argon-wtu' = the separate Argon quota of Pro plans,
    // '3p' (or anything else) = Antigravity's Claude and GPT models.
    const family = group.key === 'gemini' ? 'Gemini' : group.key.startsWith('gemini-argon') ? 'Argon' : 'Other'
    for (const window of group.windows || []) {
      if (window.window !== '5h' && window.window !== 'weekly') continue
      const weekly = window.window === 'weekly'
      const labels = {
        Gemini: { fiveHour: t('admin.accounts.usageWindow.antigravityGemini5h'), weekly: t('admin.accounts.usageWindow.antigravityGeminiWeekly') },
        Argon: { fiveHour: t('admin.accounts.usageWindow.antigravityArgon5h'), weekly: t('admin.accounts.usageWindow.antigravityArgonWeekly') },
        Other: { fiveHour: t('admin.accounts.usageWindow.antigravityOther5h'), weekly: t('admin.accounts.usageWindow.antigravityOtherWeekly') }
      }
      const color: 'indigo' | 'emerald' | 'purple' | 'amber' =
        family === 'Gemini' ? (weekly ? 'indigo' : 'emerald') : family === 'Argon' ? 'indigo' : weekly ? 'purple' : 'amber'
      bars.push({
        key: `${group.key}-${window.window}`,
        label: labels[family][weekly ? 'weekly' : 'fiveHour'],
        title: group.name,
        utilization: Math.round(window.utilization ?? 0),
        resetsAt: window.resets_at ?? null,
        color
      })
    }
  }
  return bars
}

export function resolveAntigravityModelUsage(
  quota: AccountUsageInfo['antigravity_quota'],
  modelNames: string[]
): AntigravityUsageResult | null {
  if (!quota) return null

  let maxUtilization = 0
  let earliestReset: string | null = null

  for (const model of modelNames) {
    const modelQuota = quota[model]
    if (!modelQuota) continue

    if (modelQuota.utilization > maxUtilization) {
      maxUtilization = modelQuota.utilization
    }
    if (modelQuota.reset_time) {
      if (!earliestReset || modelQuota.reset_time < earliestReset) {
        earliestReset = modelQuota.reset_time
      }
    }
  }

  // 如果没有找到任何匹配的模型
  if (maxUtilization === 0 && earliestReset === null) {
    const hasAnyData = modelNames.some((m) => quota[m])
    if (!hasAnyData) return null
  }

  return {
    utilization: maxUtilization,
    resetTime: earliestReset
  }
}
