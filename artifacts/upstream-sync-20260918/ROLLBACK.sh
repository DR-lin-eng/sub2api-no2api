#!/bin/sh
set -eu

# Usage: ROLLBACK.sh /path/to/independent-copy
TARGET=${1:?target checkout required}
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
BASELINE="$SCRIPT_DIR/baseline/all"

tracked='backend/internal/application/service/cn_provider_sync_test.go
backend/internal/application/service/openai_gateway_chat_completions_raw.go
backend/internal/application/service/openai_gateway_request_body.go
backend/internal/application/service/openai_gateway_response_handling.go
backend/internal/application/service/openai_gateway_service_test.go
backend/internal/application/service/token_refresh_service_candidates_test.go
backend/internal/infrastructure/repository/account_repo_integration_test.go
backend/internal/infrastructure/repository/account_repo_list.go
backend/internal/infrastructure/repository/account_repo_temp_unsched_test.go
backend/internal/infrastructure/repository/usage_log_repo_group_rollup.go
backend/internal/infrastructure/repository/usage_log_repo_group_rollup_test.go
backend/internal/shared/antigravity/request_transformer.go
frontend/src/common/composables/__tests__/useClipboard.spec.ts
frontend/src/common/composables/useClipboard.ts
frontend/src/common/widgets/data/Pagination.vue
frontend/src/common/widgets/data/ProxySelector.vue
frontend/src/common/widgets/feedback/BaseDialog.vue
frontend/src/features/admin-channels/presentation/widgets/ModelTagInput.vue
frontend/src/features/admin-orders/presentation/widgets/AdminRefundDialog.vue
frontend/src/features/announcements/presentation/stores/announcementsStore.ts
frontend/src/features/auth/presentation/pages/RegisterPage.vue
frontend/src/features/billing/presentation/pages/UserOrdersPage.vue
frontend/src/features/billing/presentation/stores/paymentStore.ts
frontend/src/features/billing/presentation/widgets/AmountInput.vue
frontend/src/features/profile/presentation/widgets/TotpDisableDialog.vue
frontend/src/features/profile/presentation/widgets/TotpSetupDialog.vue
frontend/src/features/subscriptions/__tests__/subscriptionsStore.spec.ts
frontend/src/features/subscriptions/presentation/stores/subscriptionsStore.ts'

printf '%s\n' "$tracked" | while IFS= read -r path; do
  [ -n "$path" ] || continue
  mkdir -p "$TARGET/$(dirname "$path")"
  cp "$BASELINE/$path" "$TARGET/$path"
done

added='backend/internal/application/service/openai_chat_roles.go
backend/internal/application/service/openai_chat_roles_test.go
backend/internal/shared/antigravity/attribution_test.go
backend/internal/shared/apicompat/responses_tool_output_media.go
backend/internal/shared/apicompat/responses_tool_output_media_test.go
frontend/src/common/widgets/data/__tests__/Pagination.jump.spec.ts
frontend/src/common/widgets/feedback/__tests__/BaseDialog.ids.spec.ts
frontend/src/features/admin-channels/__tests__/ModelTagInput.keyboard.spec.ts
frontend/src/features/announcements/__tests__/announcementsStore.spec.ts
frontend/src/features/billing/__tests__/AmountInput.invalid.spec.ts
frontend/src/features/billing/__tests__/paymentStore.spec.ts'
printf '%s\n' "$added" | while IFS= read -r path; do
  [ -n "$path" ] || continue
  rm -f "$TARGET/$path"
done

echo "ROLLBACK_RESTORED=$TARGET"
echo "TRACKED_FILES_RESTORED=28"
echo "ADDED_FILES_REMOVED=11"
