// Each entry grants exactly one existing runtime import. Remove the entry in
// the same change that migrates the import; never add entries for new code.
module.exports = {
  legacyBarrelImports: {
    '@/api': [
      'src/features/admin-backup/presentation/pages/BackupPage.vue',
      'src/features/batch-image/presentation/composables/useBatchImageGuideController.ts',
      'src/features/usage/presentation/pages/UsagePage.vue',
    ],
    '@/api/admin': [
      'src/common/widgets/data/ProxySelector.vue',
      'src/features/admin-audit/presentation/pages/AuditLogPage.vue',
      'src/features/admin-channel-monitor/presentation/pages/ChannelMonitorPage.vue',
      'src/features/admin-channel-monitor/presentation/widgets/MonitorFormDialog.vue',
      'src/features/admin-channel-monitor/presentation/widgets/MonitorTemplateApplyPickerDialog.vue',
      'src/features/admin-channel-monitor/presentation/widgets/MonitorTemplateManagerDialog.vue',
      'src/features/admin-channels/presentation/pages/ChannelsPage.vue',
      'src/features/admin-dashboard/presentation/pages/DashboardPage.vue',
      'src/features/admin-promo/presentation/pages/PromoCodesPage.vue',
      'src/features/admin-proxies/presentation/widgets/ImportDataDialog.vue',
      'src/features/admin-redeem/presentation/pages/RedeemPage.vue',
      'src/features/admin-risk-control/presentation/pages/RiskControlPage.vue',
      'src/features/admin-subscriptions/presentation/pages/SubscriptionsPage.vue',
      'src/features/announcements/presentation/pages/AnnouncementsPage.vue',
      'src/features/announcements/presentation/widgets/AnnouncementReadStatusDialog.vue',
    ],
    '@/stores': [
      'src/common/widgets/data/SubscriptionProgressMini.vue',
      'src/common/widgets/data/VersionBadge.vue',
      'src/common/widgets/layout/AppHeader.vue',
      'src/common/widgets/layout/AppLayout.vue',
      'src/common/widgets/layout/AppSidebar.vue',
      'src/features/admin-audit/presentation/pages/AuditLogPage.vue',
      'src/features/admin-backup/presentation/pages/BackupPage.vue',
      'src/features/billing/presentation/pages/PaymentPage.vue',
      'src/features/billing/presentation/pages/PaymentQRCodePage.vue',
      'src/features/billing/presentation/pages/UserOrdersPage.vue',
      'src/features/billing/presentation/pages/WechatPaymentCallbackPage.vue',
      'src/features/billing/presentation/widgets/PaymentProviderDialog.vue',
      'src/features/billing/presentation/widgets/PaymentQRDialog.vue',
      'src/features/billing/presentation/widgets/PaymentStatusPanel.vue',
      'src/features/billing/presentation/widgets/StripePaymentInline.vue',
      'src/features/keys/presentation/pages/KeyUsagePage.vue',
    ],
  },
  crossFeaturePresentationImports: {
    'src/features/admin-accounts/presentation/pages/AccountsPage.vue': [
      '@/features/admin-settings/presentation/widgets/ErrorPassthroughRulesDialog.vue',
      '@/features/admin-settings/presentation/widgets/TLSFingerprintProfilesDialog.vue',
    ],
    'src/features/admin-channel-monitor/presentation/pages/ChannelMonitorPage.vue': [
      '@/features/channel-monitor-user/presentation/composables/useChannelMonitorFormat',
    ],
    'src/features/admin-channel-monitor/presentation/widgets/MonitorFormDialog.vue': [
      '@/features/admin-channels/presentation/adminChannelSignals',
      '@/features/admin-channels/presentation/widgets/ModelTagInput.vue',
      '@/features/channel-monitor-user/presentation/composables/useChannelMonitorFormat',
      '@/features/channel-monitor-user/presentation/widgets/ProviderIcon.vue',
    ],
    'src/features/admin-channel-monitor/presentation/widgets/MonitorPrimaryModelCell.vue': [
      '@/features/channel-monitor-user/presentation/composables/useChannelMonitorFormat',
    ],
    'src/features/admin-channel-monitor/presentation/widgets/MonitorRunResultDialog.vue': [
      '@/features/channel-monitor-user/presentation/composables/useChannelMonitorFormat',
    ],
    'src/features/admin-channel-monitor/presentation/widgets/MonitorTemplateManagerDialog.vue': [
      '@/features/channel-monitor-user/presentation/composables/useChannelMonitorFormat',
    ],
    'src/features/admin-dashboard/presentation/pages/DashboardPage.vue': [
      '@/features/batch-image/presentation/composables/useBatchImageAccess',
    ],
    'src/features/admin-risk-control/presentation/pages/RiskControlPage.vue': [
      '@/features/admin-accounts/presentation/widgets/ModelWhitelistSelector.vue',
    ],
    'src/features/admin-settings/presentation/composables/useSettingsStructuredEditors.ts': [
      '@/features/admin-accounts/presentation/codexFingerprintSignals',
    ],
    'src/features/admin-settings/presentation/widgets/settings-tabs/SettingsBackupTab.vue': [
      '@/features/admin-backup/presentation/pages/BackupPage.vue',
    ],
    'src/features/admin-users/presentation/pages/UsersPage.vue': [
      '@/features/admin-groups/presentation/apiKeyGroupFilterOptionsSignals',
      '@/features/subscriptions/presentation/widgets/PlatformCostCell.vue',
      '@/features/subscriptions/presentation/widgets/PlatformUsageBreakdown.vue',
      '@/features/subscriptions/presentation/widgets/UserPlatformQuotaCell.vue',
    ],
    'src/features/channels-user/presentation/pages/ChannelStatusPage.vue': [
      '@/features/channel-monitor-user/presentation/widgets/MonitorCardGrid.vue',
      '@/features/channel-monitor-user/presentation/widgets/MonitorDetailDialog.vue',
      '@/features/channel-monitor-user/presentation/widgets/MonitorHero.vue',
    ],
    'src/features/dashboard-user/presentation/widgets/UserDashboardQuickActions.vue': [
      '@/features/batch-image/presentation/composables/useBatchImageAccess',
    ],
  },
}
