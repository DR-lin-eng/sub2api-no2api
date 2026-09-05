export interface CheckinRewardForm {
 day: number
 reward_type: 'balance' | 'concurrency' | 'subscription'
 value: string
 reward_group_id: number | null
 label: string
}
export function completeCheckinCalendar(cycle: string, existing: CheckinRewardForm[] = []): CheckinRewardForm[] {
 const days = cycle === 'monthly' ? 30 : cycle === 'biweekly' ? 14 : 7
 return Array.from({ length: days }, (_, i) => {
  const saved = existing.find(reward => reward.day === i + 1)
  return saved ? { ...saved } : { day: i + 1, reward_type: 'balance', value: '', reward_group_id: null, label: '' }
 })
}
