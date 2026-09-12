# syntax=docker/dockerfile:1.7
# =============================================================================
# Sub2API Multi-Stage Dockerfile
# =============================================================================
# Stage 1: Build frontend
# Stage 2: Build Go backend with embedded frontend
# Stage 3: Final minimal image
# =============================================================================

ARG NODE_IMAGE=node:24-alpine
ARG GOLANG_IMAGE=golang:1.26.6-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG QUALITY_RUNTIME_IMAGE=python:3.11-slim-bookworm
ARG POSTGRES_IMAGE=postgres:18-alpine
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ARG NPM_CONFIG_REGISTRY=

# -----------------------------------------------------------------------------
# Stage 1: Frontend Builder
# -----------------------------------------------------------------------------
# --platform=$BUILDPLATFORM: the frontend output is JS (arch-neutral), so build
# it on the native host arch instead of under QEMU emulation for the target.
FROM --platform=${BUILDPLATFORM} ${NODE_IMAGE} AS frontend-builder
ARG NPM_CONFIG_REGISTRY

WORKDIR /app/frontend

# Install pnpm at the same exact version used by CI and packageManager.
RUN corepack enable && corepack prepare pnpm@11.17.0 --activate

# Install dependencies first (better caching)
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=sub2api-pnpm-store,target=/root/.local/share/pnpm/store \
    if [ -n "${NPM_CONFIG_REGISTRY}" ]; then pnpm config set registry "${NPM_CONFIG_REGISTRY}"; fi && \
    pnpm install --frozen-lockfile --prefer-offline

# Copy frontend source and build.
# LegalDocumentView.vue (admin-compliance gate) build-time imports
# ../../../../docs/legal/*.md?raw, so docs/legal/ must sit beside frontend/
# in the image (WORKDIR /app/frontend -> resolves to /app/docs/legal/*.md).
# Copy only that subtree to keep the build dependency minimal.
COPY frontend/ ./
COPY docs/legal/ /app/docs/legal/
RUN pnpm run build

# -----------------------------------------------------------------------------
# Stage 2: Backend Builder
# -----------------------------------------------------------------------------
# --platform=$BUILDPLATFORM: run the Go toolchain on the native host arch and
# cross-compile to the target arch below. The binary is CGO_ENABLED=0, so this
# is a clean pure-Go cross-compile — no QEMU emulation of go mod download / go
# build (emulated networking here was dropping module fetches with EOF).
FROM --platform=${BUILDPLATFORM} ${GOLANG_IMAGE} AS backend-builder

# Build arguments for version info (set by CI)
ARG VERSION=
ARG COMMIT=docker
ARG DATE
ARG GOPROXY
ARG GOSUMDB
# Populated by buildx from the --platform target (e.g. linux/amd64).
ARG TARGETOS
ARG TARGETARCH

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app/backend

# Copy go mod files first (better caching)
COPY backend/go.mod backend/go.sum ./
# Cache mount keeps the module cache across builds so a transient CDN blip on
# retry resumes instead of re-fetching every zip from scratch.
RUN --mount=type=cache,id=sub2api-gomod,target=/go/pkg/mod \
    go mod download

# Copy backend source first
COPY backend/ ./

# Copy frontend dist from previous stage (must be after backend copy to avoid being overwritten)
COPY --from=frontend-builder /app/backend/internal/transport/webassets/dist ./internal/transport/webassets/dist

# Build the binary (BuildType=release for CI builds, embed frontend)
# Version precedence: build arg VERSION > exact git tag > cmd/server/VERSION
RUN --mount=type=cache,id=sub2api-gomod,target=/go/pkg/mod \
    --mount=type=cache,id=sub2api-gobuild,target=/root/.cache/go-build \
    VERSION_VALUE="${VERSION}" && \
    if [ -z "${VERSION_VALUE}" ]; then VERSION_VALUE="$(./scripts/resolve-version.sh)"; fi && \
    DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build \
    -tags embed \
    -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT} -X main.Date=${DATE_VALUE} -X main.BuildType=release" \
    -trimpath \
    -o /app/sub2api \
    ./cmd/server

# -----------------------------------------------------------------------------
# Stage 3: PostgreSQL Client (version-matched with docker-compose)
# -----------------------------------------------------------------------------
FROM ${POSTGRES_IMAGE} AS pg-client
RUN mkdir -p /client && cp /usr/local/bin/pg_dump /usr/local/bin/psql /client/ \
    && cp /usr/local/lib/libpq.so.5 /client/libpq.so.5

# -----------------------------------------------------------------------------
# Stage 4: Final Runtime Image
# -----------------------------------------------------------------------------
FROM ${QUALITY_RUNTIME_IMAGE} AS quality-runtime

# Labels
LABEL maintainer="DR-lin-eng <github.com/DR-lin-eng>"
LABEL description="Sub2API - AI API Gateway Platform"
LABEL org.opencontainers.image.source="https://github.com/DR-lin-eng/sub2api-no2api"

# Build-time dependencies for the in-process quality module. No sidecar, URL,
# runtime installer, or changes to existing Compose/environment are required.
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata curl wget gosu libpq5 libldap-2.5-0 libkrb5-3 liblz4-1 libzstd1 libedit2 \
    && ln -s /usr/sbin/gosu /usr/local/bin/su-exec \
    && rm -rf /var/lib/apt/lists/*
COPY backend/resources/quality-renderer/requirements.txt /opt/quality-renderer/requirements.txt
RUN pip install --no-cache-dir torch==2.6.0 torchvision==0.21.0 --index-url https://download.pytorch.org/whl/cpu \
    && pip install --no-cache-dir -r /opt/quality-renderer/requirements.txt \
    && playwright install --with-deps chromium \
    && chmod -R a+rX /ms-playwright && rm -rf /var/lib/apt/lists/*

COPY --from=pg-client /client/pg_dump /usr/local/bin/pg_dump
COPY --from=pg-client /client/psql /usr/local/bin/psql
COPY --from=pg-client /client/libpq.so.5 /usr/local/lib/
RUN ldconfig

# Create non-root user
RUN groupadd -g 1000 sub2api && \
    useradd -u 1000 -g sub2api -m -s /bin/sh sub2api

# Set working directory
WORKDIR /app

# Copy binary/resources with ownership to avoid extra full-layer chown copy
COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api /app/sub2api
COPY --from=backend-builder --chown=sub2api:sub2api /app/backend/resources /app/resources

# Create writable runtime directories. The in-app updater stages the new
# binary beside /app/sub2api so its rename remains atomic; that requires the
# non-root service user to write the binary's parent directory as well.
RUN mkdir -p /app/data && chown sub2api:sub2api /app /app/data

# Copy entrypoint script (fixes volume permissions then drops to sub2api)
COPY deploy/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Expose port (can be overridden by SERVER_PORT env var)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/ready || exit 1

# Run the application (entrypoint fixes /app/data ownership then execs as sub2api)
ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/sub2api"]
