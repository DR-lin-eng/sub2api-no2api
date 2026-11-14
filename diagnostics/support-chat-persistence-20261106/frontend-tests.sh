#!/bin/sh
set -eu
mkdir -p /results /app/docs/legal
cd /app/frontend
rm -rf src vite.config.js vite.config.d.ts tsconfig.tsbuildinfo tsconfig.node.tsbuildinfo
tar -C /src/frontend --exclude=node_modules --exclude='*.tsbuildinfo' --exclude=vite.config.js --exclude=vite.config.d.ts --exclude='.env*' -cf - . | tar -xf -
cp -a /src/docs/legal/. /app/docs/legal/
pnpm install --frozen-lockfile --prefer-offline > /results/frontend-install.txt 2>&1
pnpm exec vitest run src/features/support-chat > /results/frontend-tests.txt 2>&1
pnpm exec eslint src/features/support-chat/presentation/pages/SupportChatPage.vue src/features/support-chat/presentation/pages/AdminSupportChatPage.vue src/features/support-chat/presentation/composables/supportChatHistory.ts src/features/support-chat/presentation/composables/__tests__/supportChatHistory.spec.ts src/features/support-chat/__tests__/SupportChatHistoryLifecycle.spec.ts > /results/frontend-eslint.txt 2>&1
pnpm run typecheck > /results/frontend-typecheck.txt 2>&1
pnpm run build > /results/frontend-build.txt 2>&1
echo 'PASS Docker frontend tests, ESLint, typecheck and production build'
