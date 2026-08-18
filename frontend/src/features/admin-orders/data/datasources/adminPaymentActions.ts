import { apiClient } from '@/core/networks/client'
import type { PaymentChannel, ProviderInstance, SubscriptionPlan } from '@/features/billing/paymentContracts'
import type {
  AdminRefundRequest,
  RefundResult,
  UpdatePaymentConfigRequest,
} from '../dtos/adminPaymentDtos'

export function updateConfig(data: UpdatePaymentConfigRequest) {
  return apiClient.put('/admin/payment/config', data)
}

export function cancelOrder(id: number) {
  return apiClient.post(`/admin/payment/orders/${id}/cancel`)
}

export function retryRecharge(id: number) {
  return apiClient.post(`/admin/payment/orders/${id}/retry`)
}

export function refundOrder(id: number, data: AdminRefundRequest) {
  return apiClient.post<RefundResult>(`/admin/payment/orders/${id}/refund`, data)
}

export function queryRefund(id: number) {
  return apiClient.post<RefundResult>(`/admin/payment/orders/${id}/refund/query`)
}

export function createChannel(data: Partial<PaymentChannel>) {
  return apiClient.post<PaymentChannel>('/admin/payment/channels', data)
}

export function updateChannel(id: number, data: Partial<PaymentChannel>) {
  return apiClient.put<PaymentChannel>(`/admin/payment/channels/${id}`, data)
}

export function deleteChannel(id: number) {
  return apiClient.delete(`/admin/payment/channels/${id}`)
}

export function createPlan(data: Record<string, unknown>) {
  return apiClient.post<SubscriptionPlan>('/admin/payment/plans', data)
}

export function updatePlan(id: number, data: Record<string, unknown>) {
  return apiClient.put<SubscriptionPlan>(`/admin/payment/plans/${id}`, data)
}

export function deletePlan(id: number) {
  return apiClient.delete(`/admin/payment/plans/${id}`)
}

export function createProvider(data: Partial<ProviderInstance>) {
  return apiClient.post<ProviderInstance>('/admin/payment/providers', data)
}

export function updateProvider(id: number, data: Partial<ProviderInstance>) {
  return apiClient.put<ProviderInstance>(`/admin/payment/providers/${id}`, data)
}

export function deleteProvider(id: number) {
  return apiClient.delete(`/admin/payment/providers/${id}`)
}
