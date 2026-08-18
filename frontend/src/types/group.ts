import type { BillingMode } from '@/core/constants/channel'

export type GroupPlatform = 'anthropic' | 'openai' | 'gemini' | 'antigravity' | 'grok' | 'composite'

export type SubscriptionType = 'standard' | 'subscription'

export interface OpenAIMessagesDispatchModelConfig {
  opus_mapped_model?: string
  sonnet_mapped_model?: string
  haiku_mapped_model?: string
  exact_model_mappings?: Record<string, string>
}

export interface ReasoningEffortMapping {
  from: string
  to: string
}

export interface PricingInterval {
  id?: number
  min_tokens: number
  max_tokens: number | null
  tier_label: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
  sort_order: number
}

export interface ChannelModelPricing {
  id?: number
  platform: string
  models: string[]
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_input_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: PricingInterval[]
}

export interface ModelsListConfig {
  enabled: boolean
  models: string[]
}

export interface Group {
  id: number
  name: string
  description: string | null
  platform: GroupPlatform
  rate_multiplier: number
  rpm_limit?: number
  max_reasoning_effort?: string
  reasoning_effort_mappings?: ReasoningEffortMapping[]
  is_exclusive: boolean
  status: 'active' | 'inactive'
  subscription_type: SubscriptionType
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
  long_context_pricing_enabled?: boolean
  allow_image_generation: boolean
  openai_force_image_tool: boolean
  allow_batch_image_generation: boolean
  image_rate_independent: boolean
  image_rate_multiplier: number
  batch_image_discount_multiplier: number
  batch_image_hold_multiplier: number
  image_price_1k: number | null
  image_price_2k: number | null
  image_price_4k: number | null
  video_rate_independent: boolean
  video_rate_multiplier: number
  video_price_480p: number | null
  video_price_720p: number | null
  video_price_1080p: number | null
  web_search_price_per_call: number | null
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  claude_code_only: boolean
  fallback_group_id: number | null
  fallback_group_id_on_invalid_request: number | null
  allow_messages_dispatch?: boolean
  allow_live: boolean
  default_mapped_model?: string
  messages_dispatch_model_config?: OpenAIMessagesDispatchModelConfig
  require_oauth_only: boolean
  require_privacy_set: boolean
  created_at: string
  updated_at: string
}
