#!/bin/sh
set -eu

artifact_dir=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$artifact_dir/../.." && pwd)

restore_file() {
  relative_path=$1
  source_path="$artifact_dir/original/$relative_path"
  target_path="$repo_root/$relative_path"
  mkdir -p "$(dirname "$target_path")"
  cp "$source_path" "$target_path"
}

restore_file "frontend/src/App.vue"
restore_file "frontend/src/features/support-chat/presentation/pages/AdminSupportChatPage.vue"
restore_file "frontend/src/features/support-chat/README.md"
restore_file "frontend/src/core/i18n/locales/en/supportChat.ts"
restore_file "frontend/src/core/i18n/locales/zh/supportChat.ts"
restore_file "docs/SUPPORT_CHAT.md"

for relative_path in \
  "frontend/src/features/support-chat/presentation/composables/useSupportBrowserNotifications.ts" \
  "frontend/src/features/support-chat/presentation/composables/__tests__/useSupportBrowserNotifications.spec.ts" \
  "frontend/src/features/support-chat/presentation/stores/supportChatNotificationStore.ts" \
  "frontend/src/features/support-chat/presentation/utils/supportBrowserNotificationContent.ts" \
  "frontend/src/features/support-chat/presentation/widgets/SupportBrowserNotificationButton.vue" \
  "frontend/src/features/support-chat/__tests__/SupportBrowserNotificationButton.spec.ts"; do
  rm -f "$repo_root/$relative_path"
done

printf '%s\n' 'ROLLBACK_RESULT=restored original support-chat behavior and removed browser-notification sources'
