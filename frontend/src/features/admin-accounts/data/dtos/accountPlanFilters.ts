export const ACCOUNT_OAUTH_PLAN_FILTER = {
  free: 'free',
  pro: 'pro',
  pro5x: 'pro5x',
  pro20x: 'pro20x',
  pro50x: 'pro50x',
  team: 'team',
  plus: 'plus',
  enterprise: 'enterprise',
  edu: 'edu'
} as const

export type AccountOAuthPlanFilter = typeof ACCOUNT_OAUTH_PLAN_FILTER[keyof typeof ACCOUNT_OAUTH_PLAN_FILTER]
