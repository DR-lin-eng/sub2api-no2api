export interface AdminPaymentConfig {
  enabled: boolean
  min_amount: number
  max_amount: number
  daily_limit: number
  order_timeout_minutes: number
  max_pending_orders: number
  enabled_payment_types: string[]
  balance_disabled: boolean
  balance_recharge_multiplier: number
  subscription_usd_to_cny_rate: number
  recharge_fee_rate: number
  load_balance_strategy: string
  product_name_prefix: string
  product_name_suffix: string
  help_image_url: string
  help_text: string
}

export interface UpdatePaymentConfigRequest {
  enabled?: boolean
  min_amount?: number
  max_amount?: number
  daily_limit?: number
  order_timeout_minutes?: number
  max_pending_orders?: number
  enabled_payment_types?: string[]
  balance_disabled?: boolean
  balance_recharge_multiplier?: number
  subscription_usd_to_cny_rate?: number
  recharge_fee_rate?: number
  load_balance_strategy?: string
  product_name_prefix?: string
  product_name_suffix?: string
  help_image_url?: string
  help_text?: string
}

export interface AdminPaymentOrderQuery {
  page?: number
  page_size?: number
  status?: string
  payment_type?: string
  user_id?: number
  keyword?: string
  start_date?: string
  end_date?: string
  order_type?: string
}

export interface AdminRefundRequest {
  amount: number
  reason: string
  deduct_balance?: boolean
  force?: boolean
}

export interface RefundResult {
  success: boolean
  warning?: string
  require_force?: boolean
  balance_deducted?: number
  subscription_days_deducted?: number
}
