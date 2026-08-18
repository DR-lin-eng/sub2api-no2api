import { apiClient } from '@/core/networks/client'
import type { BasePaginationResponse } from '@/types'
import type {
  DashboardStats,
  PaymentChannel,
  PaymentOrder,
  ProviderInstance,
  SubscriptionPlan,
} from '@/features/billing/paymentContracts'
import type { AdminPaymentConfig, AdminPaymentOrderQuery } from '../dtos/adminPaymentDtos'

export function getConfig() {
  return apiClient.get<AdminPaymentConfig>('/admin/payment/config')
}

export function getDashboard(days?: number) {
  return apiClient.get<DashboardStats>('/admin/payment/dashboard', {
    params: days ? { days } : undefined,
  })
}

export function getOrders(params?: AdminPaymentOrderQuery) {
  return apiClient.get<BasePaginationResponse<PaymentOrder>>('/admin/payment/orders', { params })
}

export function getOrder(id: number) {
  return apiClient.get<PaymentOrder>(`/admin/payment/orders/${id}`)
}

export function getChannels() {
  return apiClient.get<PaymentChannel[]>('/admin/payment/channels')
}

export function getPlans() {
  return apiClient.get<SubscriptionPlan[]>('/admin/payment/plans')
}

export function getProviders() {
  return apiClient.get<ProviderInstance[]>('/admin/payment/providers')
}
