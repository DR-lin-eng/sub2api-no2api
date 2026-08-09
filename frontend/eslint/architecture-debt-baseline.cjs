// Each entry grants exactly one existing runtime import. Remove the entry in
// the same change that migrates the import; never add entries for new code.
module.exports = {
  legacyBarrelImports: {
    '@/api': [
      'src/features/admin-audit/presentation/pages/AuditLogPage.vue',
      'src/features/admin-backup/presentation/pages/BackupPage.vue',
      'src/features/admin-ops/presentation/widgets/OpsAlertRulesCard.vue',
      'src/features/announcements/presentation/stores/announcementsStore.ts',
      'src/features/auth/presentation/stores/authStore.ts',
      'src/features/auth/presentation/widgets/TotpStepUpDialog.vue',
      'src/features/batch-image/presentation/composables/useBatchImageGuideController.ts',
      'src/features/billing/presentation/pages/RedeemPage.vue',
      'src/features/billing/presentation/pages/StripePopupPage.vue',
      'src/features/keys/presentation/pages/KeysPage.vue',
      'src/features/profile/presentation/widgets/ProfileAvatarCard.vue',
      'src/features/profile/presentation/widgets/ProfileBalanceNotifyCard.vue',
      'src/features/profile/presentation/widgets/ProfileEditForm.vue',
      'src/features/profile/presentation/widgets/ProfilePasswordForm.vue',
      'src/features/profile/presentation/widgets/ProfileTotpCard.vue',
      'src/features/profile/presentation/widgets/TotpDisableDialog.vue',
      'src/features/profile/presentation/widgets/TotpSetupDialog.vue',
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
      'src/features/admin-cluster/presentation/pages/MultiInstancePage.vue',
      'src/features/admin-dashboard/presentation/pages/DashboardPage.vue',
      'src/features/admin-groups/presentation/widgets/CompositeRoutesDialog.vue',
      'src/features/admin-groups/presentation/widgets/GroupRPMOverridesDialog.vue',
      'src/features/admin-groups/presentation/widgets/GroupRateMultipliersDialog.vue',
      'src/features/admin-orders/presentation/pages/AdminPaymentPlansPage.vue',
      'src/features/admin-promo/presentation/pages/PromoCodesPage.vue',
      'src/features/admin-proxies/presentation/widgets/ImportDataDialog.vue',
      'src/features/admin-redeem/presentation/pages/RedeemPage.vue',
      'src/features/admin-risk-control/presentation/pages/RiskControlPage.vue',
      'src/features/admin-subscriptions/presentation/pages/SubscriptionsPage.vue',
      'src/features/admin-usage/presentation/pages/UsagePage.vue',
      'src/features/admin-usage/presentation/widgets/UsageFilters.vue',
      'src/features/admin-users/presentation/pages/UsersPage.vue',
      'src/features/admin-users/presentation/widgets/BulkEditUserDialog.vue',
      'src/features/admin-users/presentation/widgets/GroupReplaceDialog.vue',
      'src/features/admin-users/presentation/widgets/UserAllowedGroupsDialog.vue',
      'src/features/admin-users/presentation/widgets/UserApiKeysDialog.vue',
      'src/features/admin-users/presentation/widgets/UserAttributeForm.vue',
      'src/features/admin-users/presentation/widgets/UserAttributesConfigDialog.vue',
      'src/features/admin-users/presentation/widgets/UserBalanceDialog.vue',
      'src/features/admin-users/presentation/widgets/UserBalanceHistoryDialog.vue',
      'src/features/admin-users/presentation/widgets/UserCreateDialog.vue',
      'src/features/admin-users/presentation/widgets/UserEditDialog.vue',
      'src/features/admin-users/presentation/widgets/UserPlatformQuotaDialog.vue',
      'src/features/announcements/presentation/pages/AnnouncementsPage.vue',
      'src/features/announcements/presentation/widgets/AnnouncementReadStatusDialog.vue',
    ],
    '@/stores': [
      'src/App.vue',
      'src/common/widgets/data/SubscriptionProgressMini.vue',
      'src/common/widgets/data/VersionBadge.vue',
      'src/common/widgets/layout/AppHeader.vue',
      'src/common/widgets/layout/AppLayout.vue',
      'src/common/widgets/layout/AppSidebar.vue',
      'src/common/widgets/layout/AuthLayout.vue',
      'src/features/admin-audit/presentation/pages/AuditLogPage.vue',
      'src/features/admin-backup/presentation/pages/BackupPage.vue',
      'src/features/admin-ops/presentation/pages/OpsDashboardPage.vue',
      'src/features/admin-ops/presentation/widgets/OpsErrorDetailDialog.vue',
      'src/features/admin-ops/presentation/widgets/OpsRequestDetailsDialog.vue',
      'src/features/admin-ops/presentation/widgets/OpsSystemLogTable.vue',
      'src/features/auth/presentation/pages/DingTalkCallbackPage.vue',
      'src/features/auth/presentation/pages/DingTalkEmailCompletionPage.vue',
      'src/features/auth/presentation/pages/EmailVerifyPage.vue',
      'src/features/auth/presentation/pages/ForgotPasswordPage.vue',
      'src/features/auth/presentation/pages/LinuxDoCallbackPage.vue',
      'src/features/auth/presentation/pages/LoginPage.vue',
      'src/features/auth/presentation/pages/OAuthCallbackPage.vue',
      'src/features/auth/presentation/pages/OidcCallbackPage.vue',
      'src/features/auth/presentation/pages/RegisterPage.vue',
      'src/features/auth/presentation/pages/ResetPasswordPage.vue',
      'src/features/auth/presentation/pages/WechatCallbackPage.vue',
      'src/features/auth/presentation/widgets/PendingOAuthCreateAccountForm.vue',
      'src/features/auth/presentation/widgets/TotpLoginDialog.vue',
      'src/features/auth/presentation/widgets/TotpStepUpDialog.vue',
      'src/features/auth/presentation/widgets/WechatOAuthSection.vue',
      'src/features/billing/presentation/pages/PaymentPage.vue',
      'src/features/billing/presentation/pages/PaymentQRCodePage.vue',
      'src/features/billing/presentation/pages/UserOrdersPage.vue',
      'src/features/billing/presentation/pages/WechatPaymentCallbackPage.vue',
      'src/features/billing/presentation/widgets/PaymentProviderDialog.vue',
      'src/features/billing/presentation/widgets/PaymentQRDialog.vue',
      'src/features/billing/presentation/widgets/PaymentStatusPanel.vue',
      'src/features/billing/presentation/widgets/StripePaymentInline.vue',
      'src/features/channels-user/presentation/pages/CustomLandingPage.vue',
      'src/features/keys/presentation/pages/KeyUsagePage.vue',
      'src/features/profile/presentation/widgets/ProfileIdentityBindingsSection.vue',
    ],
  },
  crossFeaturePresentationImports: {
    'src/features/admin-accounts/presentation/composables/useAccountTablePresentation.ts': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/admin-accounts/presentation/pages/AccountsPage.vue': [
      '@/features/admin-settings/presentation/widgets/ErrorPassthroughRulesDialog.vue',
      '@/features/admin-settings/presentation/widgets/TLSFingerprintProfilesDialog.vue',
      '@/features/auth/presentation/stores/authStore',
      '@/features/auth/presentation/widgets/TotpStepUpDialog.vue',
    ],
    'src/features/admin-accounts/presentation/widgets/CreateAccountDialog.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/admin-accounts/presentation/widgets/EditAccountDialog.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/admin-backup/presentation/pages/BackupPage.vue': [
      '@/features/auth/presentation/widgets/TotpStepUpDialog.vue',
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
    'src/features/admin-ops/presentation/composables/useOpsRealtimeTraffic.ts': [
      '@/features/admin-settings/presentation/stores/adminSettingsStore',
    ],
    'src/features/admin-orders/presentation/pages/AdminOrdersPage.vue': [
      '@/features/billing/presentation/currencyFormatter',
      '@/features/billing/presentation/orderUtilsFormatter',
      '@/features/billing/presentation/widgets/OrderStatusBadge.vue',
      '@/features/billing/presentation/widgets/OrderTable.vue',
    ],
    'src/features/admin-orders/presentation/pages/AdminPaymentPlansPage.vue': [
      '@/features/billing/presentation/currencyFormatter',
    ],
    'src/features/admin-orders/presentation/widgets/AdminOrderDetail.vue': [
      '@/features/billing/presentation/currencyFormatter',
      '@/features/billing/presentation/orderUtilsFormatter',
    ],
    'src/features/admin-orders/presentation/widgets/AdminOrderTable.vue': [
      '@/features/billing/presentation/currencyFormatter',
      '@/features/billing/presentation/orderUtilsFormatter',
    ],
    'src/features/admin-orders/presentation/widgets/AdminRefundDialog.vue': [
      '@/features/billing/presentation/currencyFormatter',
      '@/features/billing/presentation/orderUtilsFormatter',
    ],
    'src/features/admin-orders/presentation/widgets/PlanEditDialog.vue': [
      '@/features/billing/presentation/currencyFormatter',
    ],
    'src/features/admin-risk-control/presentation/pages/RiskControlPage.vue': [
      '@/features/admin-accounts/presentation/widgets/ModelWhitelistSelector.vue',
    ],
    'src/features/admin-settings/presentation/composables/useSettingsPaymentProviders.ts': [
      '@/features/billing/presentation/paymentFlowResolver',
      '@/features/billing/presentation/widgets/PaymentProviderDialog.vue',
    ],
    'src/features/admin-settings/presentation/composables/useSettingsStructuredEditors.ts': [
      '@/features/admin-accounts/presentation/codexFingerprintSignals',
    ],
    'src/features/admin-settings/presentation/pages/SettingsPage.vue': [
      '@/features/auth/presentation/widgets/TotpStepUpDialog.vue',
      '@/features/billing/presentation/widgets/PaymentProviderDialog.vue',
    ],
    'src/features/admin-settings/presentation/widgets/settings-tabs/SettingsBackupTab.vue': [
      '@/features/admin-backup/presentation/pages/BackupPage.vue',
    ],
    'src/features/admin-settings/presentation/widgets/settings-tabs/SettingsPaymentTab.vue': [
      '@/features/billing/presentation/widgets/PaymentProviderList.vue',
    ],
    'src/features/admin-usage/presentation/pages/UsagePage.vue': [
      '@/features/admin-ops/presentation/widgets/OpsErrorDetailDialog.vue',
      '@/features/admin-ops/presentation/widgets/OpsErrorLogTable.vue',
      '@/features/admin-users/presentation/widgets/UserBalanceHistoryDialog.vue',
    ],
    'src/features/admin-users/presentation/pages/UsersPage.vue': [
      '@/features/admin-groups/presentation/apiKeyGroupFilterOptionsSignals',
      '@/features/subscriptions/presentation/widgets/PlatformCostCell.vue',
      '@/features/subscriptions/presentation/widgets/PlatformUsageBreakdown.vue',
      '@/features/subscriptions/presentation/widgets/UserPlatformQuotaCell.vue',
    ],
    'src/features/admin-users/presentation/widgets/UserCreateDialog.vue': [
      '@/features/auth/presentation/widgets/TotpStepUpDialog.vue',
    ],
    'src/features/admin-users/presentation/widgets/UserEditDialog.vue': [
      '@/features/auth/presentation/widgets/TotpStepUpDialog.vue',
    ],
    'src/features/affiliate/presentation/pages/AffiliatePage.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/affiliate/presentation/widgets/AdminAffiliateRecordsTable.vue': [
      '@/features/billing/presentation/widgets/OrderStatusBadge.vue',
    ],
    'src/features/batch-image/presentation/composables/useBatchImageAccess.ts': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/billing/presentation/pages/PaymentPage.vue': [
      '@/features/auth/presentation/stores/authStore',
      '@/features/subscriptions/presentation/stores/subscriptionsStore',
      '@/features/subscriptions/presentation/widgets/SubscriptionPlanCard.vue',
    ],
    'src/features/billing/presentation/pages/RedeemPage.vue': [
      '@/features/auth/presentation/stores/authStore',
      '@/features/subscriptions/presentation/stores/subscriptionsStore',
    ],
    'src/features/channels-user/presentation/pages/ChannelStatusPage.vue': [
      '@/features/channel-monitor-user/presentation/widgets/MonitorCardGrid.vue',
      '@/features/channel-monitor-user/presentation/widgets/MonitorDetailDialog.vue',
      '@/features/channel-monitor-user/presentation/widgets/MonitorHero.vue',
    ],
    'src/features/channels-user/presentation/pages/CustomLandingPage.vue': [
      '@/features/admin-settings/presentation/stores/adminSettingsStore',
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/dashboard-user/presentation/pages/DashboardPage.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/dashboard-user/presentation/widgets/UserDashboardQuickActions.vue': [
      '@/features/batch-image/presentation/composables/useBatchImageAccess',
    ],
    'src/features/model-plaza/presentation/pages/ModelPlazaPage.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/model-plaza/presentation/widgets/ModelPlazaContent.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/model-plaza/presentation/widgets/PlazaNavBar.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/profile/presentation/pages/ProfilePage.vue': [
      '@/features/auth/presentation/stores/authStore',
      '@/features/passkeys/presentation/widgets/ProfilePasskeyCard.vue',
    ],
    'src/features/profile/presentation/widgets/ProfileAvatarCard.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/profile/presentation/widgets/ProfileBalanceNotifyCard.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/profile/presentation/widgets/ProfileEditForm.vue': [
      '@/features/auth/presentation/stores/authStore',
    ],
    'src/features/subscriptions/presentation/widgets/SubscriptionPlanCard.vue': [
      '@/features/billing/presentation/currencyFormatter',
      '@/features/billing/presentation/utils/validity',
    ],
    'src/features/usage/presentation/pages/UsagePage.vue': [
      '@/features/admin-usage/presentation/widgets/UsageStatsCards.vue',
      '@/features/admin-usage/presentation/widgets/UsageTable.vue',
    ],
  },
}
