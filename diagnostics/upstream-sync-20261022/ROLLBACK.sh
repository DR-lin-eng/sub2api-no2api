#!/bin/sh
set -eu
if [ "$#" -ne 1 ] || [ ! -d "$1" ]; then echo "usage: ROLLBACK.sh TARGET_COPY_DIRECTORY" >&2; exit 64; fi
role_dir=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
target=$(CDPATH= cd -P "$1" && pwd)
case "$target" in
  /|/Users/lin/.codex/worktrees/f89c/sub2api-no2api|/private/tmp/sub2api-upstream-sync-20261022-ieZnRf/MODIFIED_FILE) echo "refusing original workspace or candidate checkout" >&2; exit 64;;
esac
[ -f "$role_dir/BASELINE_SOURCE_FILES.tar.gz" ] || { echo "missing sibling baseline archive" >&2; exit 66; }
while IFS= read -r name; do [ -z "$name" ] || rm -f "$target/$name"; done <<'ADDED_SOURCE_PATHS'
backend/internal/application/service/account_model_mapping_signature.go
backend/internal/application/service/account_model_mapping_signature_test.go
backend/internal/application/service/account_quota_service_account_test.go
backend/internal/application/service/account_quota_support.go
backend/internal/application/service/gateway_forward_compat_client_disconnect_test.go
backend/internal/application/service/gateway_forward_compat_idle.go
backend/internal/application/service/gateway_forward_compat_stream.go
backend/internal/application/service/gateway_stream_error_failover.go
backend/internal/application/service/gemini_image_generation_gate.go
backend/internal/application/service/gemini_image_generation_gate_test.go
backend/internal/application/service/gemini_tool_schema.go
backend/internal/application/service/gemini_tool_schema_test.go
backend/internal/application/service/ratelimit_cloudflare.go
backend/internal/application/service/ratelimit_cloudflare_test.go
backend/internal/application/service/request_metadata_single_account.go
backend/internal/application/service/single_account_retry_probe_test.go
backend/internal/infrastructure/repository/usage_billing_account_quota.go
backend/internal/transport/http/handler/gateway_compat_stream_usage_test.go
backend/internal/transport/http/handler/gemini_image_generation_gate.go
backend/internal/transport/http/handler/gemini_image_generation_gate_test.go
backend/internal/transport/http/handler/upstream_sync_fixture_test.go
backend/migrations/250_enable_gemini_image_generation_groups.sql
docs/UPSTREAM_SYNC_20261022.md
frontend/src/features/admin-accounts/__tests__/accountFilterState.spec.ts
frontend/src/features/admin-accounts/presentation/accountFilterState.ts
frontend/src/features/admin-accounts/presentation/accountQuota.ts
ADDED_SOURCE_PATHS
tar -xzf "$role_dir/BASELINE_SOURCE_FILES.tar.gz" -C "$target"
printf 'Restored pristine source bytes in %s\n' "$target"
