/**
 * Stable compatibility entrypoint for shared frontend types.
 *
 * Domain declarations live in bounded files so feature code can keep using
 * '@/types' while new ownership boundaries remain explicit.
 */
export * from './common'
export * from './gateway'
export * from './usage'

// Payment types
export type { SubscriptionPlan, PaymentOrder, CheckoutInfoResponse } from './payment'

export type {
  PlatformQuotaItem,
  PlatformQuotaUpdateItem,
  PlatformQuotaPlatform,
  PlatformQuotaWindow,
  PlatformQuotasResponse,
} from '@/features/admin-users/data/dtos/adminUserDtos'

export type {
  CheckMixedChannelRequest,
  CheckMixedChannelResponse,
  ClaudeModel,
  CodexSessionImportItem,
  CodexSessionImportMessage,
  CodexSessionImportRequest,
  CodexSessionImportResult,
  CreateAccountRequest,
  MixedChannelWarningDetails,
  OpenAICodexPATCreateRequest,
  TempUnschedulableRule,
  TempUnschedulableState,
  TempUnschedulableStatus,
  UpdateAccountRequest,
} from '@/features/admin-accounts/data/dtos/adminAccountDtos'
