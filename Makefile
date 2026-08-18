.PHONY: build build-backend build-frontend test test-backend test-frontend test-frontend-critical check-docs

FRONTEND_CRITICAL_VITEST := \
	src/common/widgets/data/__tests__/GroupOptionItem.spec.ts \
	src/common/widgets/layout/__tests__/AppHeaderResponsive.spec.ts \
	src/core/i18n/__tests__/routeLocaleCoverage.spec.ts \
	src/__tests__/dynamicLocaleMapping.spec.ts \
	src/core/routes/__tests__/adminRouteAccess.spec.ts \
	src/features/auth/__tests__/authProfileLocaleScopes.spec.ts \
	src/features/billing/__tests__/paymentLocaleScopes.spec.ts \
	src/features/subscriptions/__tests__/subscriptionStatus.spec.ts \
	src/features/admin-backup/presentation/pages/__tests__/backupStatus.spec.ts \
	src/core/utils/__tests__/usageLocale.spec.ts \
	src/core/utils/__tests__/embedded-url.spec.ts \
	src/core/utils/__tests__/homeContent.spec.ts \
	src/__tests__/dynamicHtmlSecurity.spec.ts \
	src/common/pages/__tests__/HomePageCompact.spec.ts \
	src/features/channels-user/__tests__/customPageHtml.spec.ts \
	src/features/channel-monitor-user/__tests__/channelMonitorLocale.spec.ts \
	src/features/channels-user/__tests__/CustomLandingPage.security.spec.ts \
	src/features/prompt-audit/__tests__/promptAuditLocale.spec.ts \
	src/features/prompt-audit/__tests__/RuntimeOverview.spec.ts \
	src/features/admin-usage/__tests__/UsageDetailDialog.spec.ts \
	src/features/admin-usage/__tests__/UsageStatsCards.spec.ts \
	src/features/admin-usage/__tests__/UsageTable.spec.ts \
	src/features/auth/__tests__/LinuxDoCallbackView.spec.ts \
	src/features/auth/__tests__/WechatCallbackView.spec.ts \
	src/features/billing/__tests__/PaymentPage.spec.ts \
	src/features/billing/__tests__/PaymentResultPage.spec.ts \
	src/features/profile/__tests__/ProfileInfoCard.spec.ts \
	src/features/admin-settings/__tests__/SettingsPage.spec.ts

# 一键编译前后端
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
	@$(MAKE) -C backend build

# 编译前端（需要已安装依赖）
build-frontend:
	@pnpm --dir frontend run build

# 运行测试（后端 + 前端）
test: check-docs test-backend test-frontend

check-docs:
	@./tools/check-docs.sh

test-backend:
	@$(MAKE) -C backend test

test-frontend:
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck
	@$(MAKE) test-frontend-critical

test-frontend-critical:
	@pnpm --dir frontend exec vitest run $(FRONTEND_CRITICAL_VITEST)
