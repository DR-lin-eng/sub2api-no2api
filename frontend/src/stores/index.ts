/**
 * Transitional store compatibility barrel.
 * New code should import from the owning core or feature module directly.
 */

export { useAuthStore } from '@/features/auth'
export { useAppStore } from '@/core/stores/appStore'
export { useAdminSettingsStore } from '@/features/admin-settings/adminSettingsStore'
export { useSubscriptionStore } from '@/features/subscriptions/presentation/stores/subscriptionsStore'
export { useOnboardingStore } from '@/core/stores/onboardingStore'
export { useAnnouncementStore } from '@/features/announcements/presentation/stores/announcementsStore'
export { usePaymentStore } from '@/features/billing/paymentStore'
export { useAdminComplianceStore } from '@/features/admin-settings/presentation/stores/adminComplianceStore'

// Re-export types for convenience
export type { User, LoginRequest, RegisterRequest, AuthResponse } from '@/types'
export type { Toast, ToastType, AppState } from '@/types'
