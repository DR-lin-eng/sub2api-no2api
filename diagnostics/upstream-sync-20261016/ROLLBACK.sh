#!/bin/sh
# Restore only the source paths in this transaction; evidence is retained.
set -eu
if [ "$#" -ne 1 ] || [ ! -d "$1" ]; then
  echo "usage: ROLLBACK.sh TARGET_COPY_DIRECTORY" >&2
  exit 64
fi
role_dir=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
target=$(CDPATH= cd -P "$1" && pwd)
case "$target" in
  /|/Users/lin/.codex/worktrees/76ea/sub2api-no2api) echo "refusing to modify the original workspace or root" >&2; exit 64 ;;
esac
baseline="$role_dir/BASELINE_SOURCE_FILES.tar.gz"
[ -f "$baseline" ] || { echo "missing sibling BASELINE_SOURCE_FILES.tar.gz" >&2; exit 66; }
while IFS= read -r name; do
  [ -n "$name" ] && rm -f "$target/$name"
done <<'ADDED_SOURCE_PATHS'
backend/internal/application/service/payment_notification_confirm_test.go
backend/internal/application/service/payment_order_type_test.go
backend/internal/application/service/payment_refund_pending_hold_test.go
backend/internal/application/service/payment_refund_subscription_test.go
backend/internal/infrastructure/repository/payment_refund_hold_integration_test.go
backend/internal/modules/payment/provider/easypay_confirm_test.go
docs/UPSTREAM_SYNC_20261016.md
ADDED_SOURCE_PATHS
tar -xzf "$baseline" -C "$target"
echo "Restored pristine source bytes in $target"
