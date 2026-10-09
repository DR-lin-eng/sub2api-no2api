FROM sub2api:securityfix-frontend-20261007
RUN apk add --no-cache chromium font-noto-cjk
RUN mkdir -p /opt/qa && cd /opt/qa && pnpm add playwright-core@1.56.1
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml /app/frontend/
RUN pnpm install --frozen-lockfile
COPY frontend/ /app/frontend/
COPY docs/legal/ /app/docs/legal/
