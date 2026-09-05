import { readFileSync } from 'node:fs'
import { relative, resolve } from 'node:path'
import ts from 'typescript'
import { describe, expect, it } from 'vitest'

const srcRoot = resolve(process.cwd(), 'src')

function runtimeSourceFiles(): string[] {
  return ts.sys.readDirectory(
    srcRoot,
    ['.ts', '.vue'],
    ['**/__tests__/**', '**/core/i18n/locales/**', '**/*.spec.ts', '**/*.test.ts'],
    ['**/*'],
  )
}

function normalizeExpression(value: string): string {
  return value.replace(/\s+/g, ' ').trim()
}

function dynamicLocaleCalls(): Map<string, number> {
  const findings = new Map<string, number>()
  const callee = String.raw`(?:\b(?:t|\$t)|\b[A-Za-z_$][\w$]*\.t)`
  const patterns = [
    new RegExp(`${callee}\\s*\\(\\s*(\`[^\`\\n]*\\$\\{[^\`\\n]*\`)`, 'g'),
    new RegExp(`${callee}\\s*\\(\\s*((?:'[^'\\n]*'|"[^"\\n]*")\\s*\\+[^,\\n)]+)`, 'g'),
  ]

  for (const file of runtimeSourceFiles()) {
    const source = readFileSync(file, 'utf8')
    for (const pattern of patterns) {
      for (const match of source.matchAll(pattern)) {
        const signature = `${relative(srcRoot, file)} :: ${normalizeExpression(match[1])}`
        findings.set(signature, (findings.get(signature) ?? 0) + 1)
      }
    }
  }
  return findings
}

const auditedDynamicCalls: Record<string, number> = {
  // Activity enums and the fixed legacy-text allowlist are covered in activitySafety.spec.ts.
  'features/activity-center/presentation/activityCenterText.ts :: `${namespace}.${key}`': 1,
  'features/activity-center/presentation/pages/ActivityCenterDetailPage.vue :: `activityCenter.prizeTypes.${type}`': 1,
  'features/activity-center/presentation/pages/ActivityCenterDetailPage.vue :: `activityCenter.types.${type}`': 1,
  'features/activity-center/presentation/pages/ActivityCenterPage.vue :: `activityCenter.types.${type}`': 1,
  'features/activity-center/presentation/pages/ActivityRecordsPage.vue :: `activityCenter.types.${type}`': 1,
  'features/activity-center/presentation/pages/AdminActivityCenterPage.vue :: `admin.activityCenter.records.rewardStatus.${status}`': 1,
  'features/activity-center/presentation/pages/AdminActivityCenterPage.vue :: `admin.activityCenter.types.${value}`': 1,
  'features/activity-center/presentation/pages/AdminActivityRecordsPage.vue :: `admin.activityCenter.records.rewardStatus.${value}`': 1,
  'features/activity-center/presentation/pages/AdminActivityRecordsPage.vue :: `admin.activityCenter.types.${value}`': 1,

  'features/admin-account-inspection/presentation/widgets/QuotaUsageDistributionChart.vue :: `admin.accountInspection.quotaUsage.buckets.${bucket.key}`': 1,
  'features/admin-accounts/presentation/accountEditUpdatePayload.ts :: `admin.accounts.headerOverride.${headerError}`': 2,
  'features/admin-accounts/presentation/widgets/BulkEditAccountDialog.vue :: `admin.accounts.headerOverride.${headerError}`': 1,
  'features/admin-accounts/presentation/widgets/CreateAccountDialog.vue :: `admin.accounts.headerOverride.${headerError}`': 2,
  'features/admin-accounts/presentation/widgets/GrokBaseUrlPresets.vue :: `admin.accounts.grokCustomBaseUrl.presets.${preset.labelKey}`': 1,
  "features/admin-accounts/presentation/widgets/QuotaDimensionRow.vue :: 'admin.accounts.dayOfWeek.' + d.key": 1,
  "features/admin-accounts/presentation/widgets/QuotaLimitCard.vue :: 'admin.accounts.dayOfWeek.' + dayKey": 1,
  'features/admin-channels/presentation/adminChannelSignals.ts :: `admin.channels.intervalValidation.${key}`': 1,
  'features/admin-channels/presentation/adminChannelSignals.ts :: `admin.channels.intervalValidation.price.${key}`': 1,
  'features/admin-groups/presentation/composables/useCreateGroupController.ts :: `admin.groups.profitControl.${profitControlError}`': 1,
  'features/admin-groups/presentation/composables/useEditGroupController.ts :: `admin.groups.profitControl.${profitControlError}`': 1,
  'features/admin-groups/presentation/widgets/GroupModelPricingEntry.vue :: `admin.channels.form.${field.label}`': 1,
  'features/admin-groups/presentation/widgets/ReasoningEffortPolicyFields.vue :: `admin.groups.form.${code}`': 1,
  'features/admin-ops/presentation/widgets/OpsAlertRulesCard.vue :: `admin.ops.alertRules.metricGroups.${group}`': 1,
  "features/admin-orders/presentation/pages/AdminOrdersPage.vue :: 'payment.methods.' + selectedOrder.payment_type": 1,
  "features/admin-orders/presentation/pages/AdminPaymentDashboardPage.vue :: 'payment.methods.' + method.type": 1,
  "features/admin-orders/presentation/widgets/AdminOrderDetail.vue :: 'payment.methods.' + order.payment_type": 1,
  "features/admin-orders/presentation/widgets/AdminOrderTable.vue :: 'payment.methods.' + value": 1,
  "features/admin-orders/presentation/widgets/PaymentMethodChart.vue :: 'payment.methods.' + method.type": 1,
  'features/admin-risk-control/presentation/pages/IngressRiskPage.vue :: `admin.ingressRisk.health.${auth.subscriber.connected ? \'connected\' : \'disconnected\'}`': 2,
  'features/admin-risk-control/presentation/pages/IngressRiskPage.vue :: `admin.ingressRisk.health.${collector.accepting ? \'running\' : \'stopped\'}`': 1,
  'features/admin-risk-control/presentation/pages/IngressRiskPage.vue :: `admin.ingressRisk.health.${overallHealth}Description`': 1,
  'features/admin-risk-control/presentation/pages/IngressRiskPage.vue :: `admin.ingressRisk.health.${overallHealth}`': 1,
  'features/admin-risk-control/presentation/pages/IngressRiskPage.vue :: `admin.ingressRisk.runtime.${auth.invalid_abuse.enabled ? \'enabled\' : \'disabled\'}`': 1,
  'features/admin-risk-control/presentation/pages/IngressRiskPage.vue :: `admin.ingressRisk.timeRanges.${filters.time_range}`': 1,
  'features/admin-risk-control/presentation/pages/IngressRiskPage.vue :: `admin.ingressRisk.timeRanges.${value}`': 1,
  'features/admin-settings/presentation/composables/useSettingsPaymentProviders.ts :: `payment.methods.${conflict.method}`': 1,
  'features/admin-settings/presentation/pages/SettingsPage.vue :: `admin.settings.tabs.${tab.key}`': 1,
  'features/admin-users/presentation/pages/UsersPage.vue :: `admin.users.schedulingTiers.${schedulingTierKey(value)}`': 1,
  'features/admin-users/presentation/widgets/BulkEditUserDialog.vue :: `admin.users.schedulingTiers.${schedulingTierValue.value === 0 ? \'priority\' : schedulingTierValue.value === 2 ? \'low\' : \'normal\'}`': 1,
  'features/admin-users/presentation/widgets/UserPlatformQuotaDialog.vue :: `admin.users.platformQuota.window${quotaWindow.charAt(0).toUpperCase() + quotaWindow.slice(1)}`': 1,
  "features/affiliate/presentation/widgets/AdminAffiliateRecordsTable.vue :: 'payment.methods.' + row.payment_type": 1,
  "features/billing/presentation/widgets/OrderTable.vue :: 'payment.methods.' + value": 1,
  'features/billing/presentation/widgets/PaymentMethodSelector.vue :: `payment.methods.${method.type}`': 1,
  'features/billing/presentation/widgets/PaymentProviderDialog.vue :: `admin.settings.payment.field_${f.key}`': 2,
  'features/billing/presentation/widgets/PaymentProviderDialog.vue :: `payment.methods.${opt.value}`': 1,
  'features/billing/presentation/widgets/PaymentProviderList.vue :: `payment.methods.${opt.value}`': 1,
  'features/channel-monitor-user/presentation/widgets/MonitorCard.vue :: `channelStatus.windowTab.${props.window}`': 1,
  'features/dashboard-user/presentation/widgets/UserDashboardStats.vue :: `dashboard.platformQuota.${w}`': 2,
  'features/media-studio/presentation/widgets/MediaStudioCanvas.vue :: `mediaStudio.modeItems.${mode.id}.title`': 1,
  'features/media-studio/presentation/widgets/MediaStudioCanvas.vue :: `mediaStudio.modeItems.${selectedMode.id}.title`': 1,
  'features/prompt-audit/presentation/widgets/EventDetailDialog.vue :: `admin.promptAudit.events.tabs.${tab}`': 1,
  'features/prompt-audit/presentation/widgets/FilterDeleteDialog.vue :: `admin.promptAudit.events.timePresets.${option.id}`': 1,
  'features/prompt-audit/presentation/widgets/PolicyPanel.vue :: `admin.promptAudit.scanners.${id}`': 1,
}

describe('dynamic locale mapping gate', () => {
  it('requires every dynamic translation key construction to be explicitly reviewed', () => {
    expect(Object.fromEntries([...dynamicLocaleCalls()].sort(([left], [right]) => left.localeCompare(right))))
      .toEqual(auditedDynamicCalls)
  })
})
