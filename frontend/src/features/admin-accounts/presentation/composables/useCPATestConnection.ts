import { ref, type Ref } from 'vue'
import type { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import { extractApiErrorMessage } from '@/core/utils/apiError'
import { testCPAConnection as requestCPATestConnection } from '@/features/admin-accounts/data/datasources/adminAccountActions'

interface CPATestConnectionOptions {
  account: () => Account | null
  cpaConcurrencyPerCredential: Ref<number>
  cpaExcludeAbnormalCredentials: Ref<boolean>
  cpaManagementKey: Ref<string>
  cpaManagementUrl: Ref<string>
  cpaUseBaseUrl: Ref<boolean>
  editBaseUrl: Ref<string>
  notifications: {
    showError: (message: string) => void
    showSuccess: (message: string) => void
  }
  t: ReturnType<typeof useI18n>['t']
}

export function useCPATestConnection(options: CPATestConnectionOptions) {
  const isTestingCPA = ref(false)

  const testCPAConnection = async () => {
    const account = options.account()
    if (!account || isTestingCPA.value) return
    isTestingCPA.value = true
    try {
      const result = await requestCPATestConnection(account.id, {
        use_account_base_url: options.cpaUseBaseUrl.value,
        base_url: options.editBaseUrl.value.trim(),
        management_url: options.cpaUseBaseUrl.value ? undefined : options.cpaManagementUrl.value.trim(),
        management_password: options.cpaManagementKey.value.trim() || undefined,
        concurrency_per_credential: Math.trunc(Number(options.cpaConcurrencyPerCredential.value)),
        exclude_abnormal_credentials: options.cpaExcludeAbnormalCredentials.value,
      })
      options.notifications.showSuccess(options.t('admin.accounts.cpaTestSuccess', {
        capacity: result.capacity_credentials ?? result.available_credentials,
        concurrency: result.effective_concurrency,
        latency: result.latency_ms,
      }))
    } catch (error: unknown) {
      options.notifications.showError(extractApiErrorMessage(error, options.t('admin.accounts.cpaTestFailed')))
    } finally {
      isTestingCPA.value = false
    }
  }

  return { isTestingCPA, testCPAConnection }
}
