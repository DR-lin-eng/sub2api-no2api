import type {
  ChannelModelPricing,
  Group,
  GroupPlatform,
  ModelsListConfig,
  OpenAIMessagesDispatchModelConfig,
  ReasoningEffortMapping,
  SubscriptionType,
} from '@/types/group'

export interface AdminGroup extends Group {
  model_pricing?: ChannelModelPricing[]
  profit_control_enabled: boolean
  profit_min_margin: number
  profit_safety_buffer: number
  model_routing: Record<string, number[]> | null
  model_routing_enabled: boolean
  mcp_xml_inject: boolean
  supported_model_scopes?: string[]
  account_count?: number
  active_account_count?: number
  rate_limited_account_count?: number
  default_mapped_model?: string
  messages_dispatch_model_config?: OpenAIMessagesDispatchModelConfig
  models_list_config?: ModelsListConfig
  sort_order: number
}

export type CompositeRouteMatchType = 'exact' | 'prefix'

export type CompositeRouteEndpoint =
  | 'any'
  | 'messages'
  | 'count_tokens'
  | 'responses'
  | 'chat_completions'
  | 'embeddings'
  | 'images'
  | 'gemini'

export type CompositeRouteSource = 'route' | 'detector' | string

export interface CompositeModelRoute {
  id: number
  group_id: number
  public_model: string
  match_type: CompositeRouteMatchType
  target_platform: Exclude<GroupPlatform, 'composite'>
  upstream_model: string
  endpoint: CompositeRouteEndpoint
  priority: number
  enabled: boolean
  notes: string
  created_at?: string
  updated_at?: string
}

export interface CompositeModelRouteInput {
  public_model: string
  match_type: CompositeRouteMatchType
  target_platform: Exclude<GroupPlatform, 'composite'>
  upstream_model?: string
  endpoint: CompositeRouteEndpoint
  priority?: number
  enabled?: boolean
  notes?: string
}

export interface CompositeRoutePreviewRequest {
  model: string
  endpoint: CompositeRouteEndpoint
}

export interface CompositeRouteDecision {
  matched: boolean
  source: CompositeRouteSource
  group_id: number
  public_model: string
  target_platform: Exclude<GroupPlatform, 'composite'> | ''
  upstream_model: string
  endpoint: CompositeRouteEndpoint
  route?: CompositeModelRoute
  reason?: string
}

export interface CreateGroupRequest {
  name: string
  description?: string | null
  platform?: GroupPlatform
  rate_multiplier?: number
  is_exclusive?: boolean
  subscription_type?: SubscriptionType
  daily_limit_usd?: number | null
  weekly_limit_usd?: number | null
  monthly_limit_usd?: number | null
  long_context_pricing_enabled?: boolean
  model_pricing?: ChannelModelPricing[]
  allow_image_generation?: boolean
  openai_force_image_tool?: boolean
  allow_batch_image_generation?: boolean
  image_rate_independent?: boolean
  image_rate_multiplier?: number
  batch_image_discount_multiplier?: number
  batch_image_hold_multiplier?: number
  image_price_1k?: number | null
  image_price_2k?: number | null
  image_price_4k?: number | null
  video_rate_independent?: boolean
  video_rate_multiplier?: number
  video_price_480p?: number | null
  video_price_720p?: number | null
  video_price_1080p?: number | null
  web_search_price_per_call?: number | null
  peak_rate_enabled?: boolean
  peak_start?: string
  peak_end?: string
  peak_rate_multiplier?: number
  profit_control_enabled?: boolean
  profit_min_margin?: number
  profit_safety_buffer?: number
  claude_code_only?: boolean
  fallback_group_id?: number | null
  fallback_group_id_on_invalid_request?: number | null
  mcp_xml_inject?: boolean
  supported_model_scopes?: string[]
  models_list_config?: ModelsListConfig
  allow_messages_dispatch?: boolean
  allow_live?: boolean
  default_mapped_model?: string
  messages_dispatch_model_config?: OpenAIMessagesDispatchModelConfig
  model_routing?: Record<string, number[]> | null
  model_routing_enabled?: boolean
  rpm_limit?: number
  max_reasoning_effort?: string
  reasoning_effort_mappings?: ReasoningEffortMapping[]
  require_oauth_only?: boolean
  require_privacy_set?: boolean
  copy_accounts_from_group_ids?: number[]
}

export interface UpdateGroupRequest extends Partial<CreateGroupRequest> {
  status?: 'active' | 'inactive'
}

export interface LiveCapability {
  supported: boolean
  reason?: string
}

export interface ModelDefaultPricing {
  found: boolean
  input_price?: number
  output_price?: number
  cache_write_price?: number
  cache_read_price?: number
  image_input_price?: number
  image_output_price?: number
}

export interface GroupListFilters {
  platform?: GroupPlatform
  status?: 'active' | 'inactive'
  is_exclusive?: boolean
  search?: string
  sort_by?: string
  sort_order?: 'asc' | 'desc'
}

export interface GroupListOptions {
  signal?: AbortSignal
}

export interface GroupStats {
  total_api_keys: number
  active_api_keys: number
  total_requests: number
  total_cost: number
}

export interface GroupRateMultiplierEntry {
  user_id: number
  user_name: string
  user_email: string
  user_notes: string
  user_status: string
  rate_multiplier?: number | null
  rpm_override?: number | null
}

export interface GroupRPMOverrideEntry {
  user_id: number
  user_name: string
  user_email: string
  user_notes: string
  user_status: string
  rpm_override: number
}

export interface GroupRateMultiplierUpdate {
  user_id: number
  rate_multiplier: number
}

export interface GroupRPMOverrideUpdate {
  user_id: number
  rpm_override: number
}

export interface GroupSortOrderUpdate {
  id: number
  sort_order: number
}

export interface GroupUsageSummary {
  group_id: number
  today_cost: number
  yesterday_cost: number
  total_cost: number
}

export interface GroupCapacitySummary {
  group_id: number
  concurrency_used: number
  concurrency_max: number
  sessions_used: number
  sessions_max: number
  rpm_used: number
  rpm_max: number
}

export interface MessageResponse {
  message: string
}
