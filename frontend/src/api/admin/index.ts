/**
 * Transitional admin API compatibility barrel.
 * New code should import the owning feature datasource directly.
 */

import dashboardAPI from '@/features/admin-dashboard/data/datasources/adminDashboardDatasource'
import usersAPI from '@/features/admin-users/data/datasources/adminUsersDatasource'
import groupsAPI from '@/features/admin-groups/data/datasources/adminGroupsDatasource'
import accountsAPI from '@/features/admin-accounts/data/datasources/adminAccountsDatasource'
import proxiesAPI from '@/features/admin-proxies/data/datasources/adminProxiesDatasource'
import redeemAPI from '@/features/admin-redeem/data/datasources/adminRedeemDatasource'
import promoAPI from '@/features/admin-promo/data/datasources/adminPromoDatasource'
import announcementsAPI from '@/features/announcements/data/datasources/adminAnnouncementsDatasource'
import settingsAPI from '@/features/admin-settings/data/datasources/adminSettingsDatasource'
import systemAPI from '@/features/admin-settings/data/datasources/systemDatasource'
import subscriptionsAPI from '@/features/admin-subscriptions/data/datasources/adminSubscriptionsDatasource'
import usageAPI from '@/features/admin-usage/data/datasources/adminUsageDatasource'
import geminiAPI from '@/features/admin-accounts/data/datasources/geminiDatasource'
import antigravityAPI from '@/features/admin-accounts/data/datasources/antigravityDatasource'
import grokAPI from '@/features/admin-accounts/data/datasources/grokDatasource'
import userAttributesAPI from '@/features/admin-users/data/datasources/userAttributesDatasource'
import opsAPI from '@/features/admin-ops/data/datasources/adminOpsDatasource'
import errorPassthroughAPI from '@/features/admin-settings/data/datasources/errorPassthroughDatasource'
import dataManagementAPI from '@/features/admin-backup/data/datasources/dataManagementDatasource'
import apiKeysAPI from '@/features/admin-usage/data/datasources/apiKeysDatasource'
import scheduledTestsAPI from '@/features/admin-accounts/data/datasources/scheduledTestsDatasource'
import backupAPI from '@/features/admin-backup/data/datasources/adminBackupDatasource'
import tlsFingerprintProfileAPI from '@/features/admin-settings/data/datasources/tlsFingerprintProfileDatasource'
import channelsAPI from '@/features/admin-channels/data/datasources/adminChannelsDatasource'
import channelMonitorAPI from '@/features/admin-channel-monitor/data/datasources/adminChannelMonitorDatasource'
import channelMonitorTemplateAPI from '@/features/admin-channel-monitor/data/datasources/adminChannelMonitorTemplateDatasource'
import adminPaymentAPI from '@/features/admin-orders/data/datasources/adminPaymentDatasource'
import affiliatesAPI from '@/features/affiliate/data/datasources/adminAffiliatesDatasource'
import riskControlAPI from '@/features/admin-risk-control/data/datasources/adminRiskControlDatasource'
import adminComplianceAPI from '@/features/admin-settings/data/datasources/complianceDatasource'
import auditAPI from '@/features/admin-audit/data/datasources/adminAuditDatasource'
import clusterAPI from '@/features/admin-cluster/data/datasources/adminClusterDatasource'
import ingressRiskAPI from '@/features/admin-risk-control/data/datasources/ingressRiskDatasource'

/**
 * Unified admin API object for convenient access
 */
export const adminAPI = {
  dashboard: dashboardAPI,
  users: usersAPI,
  groups: groupsAPI,
  accounts: accountsAPI,
  proxies: proxiesAPI,
  redeem: redeemAPI,
  promo: promoAPI,
  announcements: announcementsAPI,
  settings: settingsAPI,
  system: systemAPI,
  subscriptions: subscriptionsAPI,
  usage: usageAPI,
  gemini: geminiAPI,
  antigravity: antigravityAPI,
  grok: grokAPI,
  userAttributes: userAttributesAPI,
  ops: opsAPI,
  errorPassthrough: errorPassthroughAPI,
  dataManagement: dataManagementAPI,
  apiKeys: apiKeysAPI,
  scheduledTests: scheduledTestsAPI,
  backup: backupAPI,
  tlsFingerprintProfiles: tlsFingerprintProfileAPI,
  channels: channelsAPI,
  channelMonitor: channelMonitorAPI,
  channelMonitorTemplate: channelMonitorTemplateAPI,
  payment: adminPaymentAPI,
  affiliates: affiliatesAPI,
  riskControl: riskControlAPI,
  compliance: adminComplianceAPI,
  audit: auditAPI,
  cluster: clusterAPI,
  ingressRisk: ingressRiskAPI
}

export {
  dashboardAPI,
  usersAPI,
  groupsAPI,
  accountsAPI,
  proxiesAPI,
  redeemAPI,
  promoAPI,
  announcementsAPI,
  settingsAPI,
  systemAPI,
  subscriptionsAPI,
  usageAPI,
  geminiAPI,
  antigravityAPI,
  grokAPI,
  userAttributesAPI,
  opsAPI,
  errorPassthroughAPI,
  dataManagementAPI,
  apiKeysAPI,
  scheduledTestsAPI,
  backupAPI,
  tlsFingerprintProfileAPI,
  channelsAPI,
  channelMonitorAPI,
  channelMonitorTemplateAPI,
  adminPaymentAPI,
  affiliatesAPI,
  riskControlAPI,
  adminComplianceAPI,
  auditAPI,
  clusterAPI,
  ingressRiskAPI
}

export default adminAPI

// Re-export types used by components
export type { AuditLog, AuditLogQuery, AuditLogListResponse } from '@/features/admin-audit/data/datasources/adminAuditDatasource'
export type { BalanceHistoryItem } from '@/features/admin-users/data/dtos/adminUserDtos'
export type { ErrorPassthroughRule, CreateRuleRequest, UpdateRuleRequest } from '@/features/admin-settings/data/datasources/errorPassthroughDatasource'
export type { BackupAgentHealth, DataManagementConfig } from '@/features/admin-backup/data/datasources/dataManagementDatasource'
export type { TLSFingerprintProfile, CreateProfileRequest, UpdateProfileRequest } from '@/features/admin-settings/data/datasources/tlsFingerprintProfileDatasource'
export type { ContentModerationConfig, ContentModerationLog, ModerationMode } from '@/features/admin-risk-control/data/datasources/adminRiskControlDatasource'
export type {
  AuthCacheHealth,
  IngressCollectorHealth,
  IngressRejection,
  IngressRejectionList,
  IngressRejectionQuery,
  IngressRiskTimeRange,
} from '@/features/admin-risk-control/data/datasources/ingressRiskDatasource'
