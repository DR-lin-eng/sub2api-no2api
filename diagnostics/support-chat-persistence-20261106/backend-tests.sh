#!/bin/sh
set -eu
mkdir -p /work /results
cp -a /src/backend /work/backend
cd /work/backend
go test -json ./internal/modules/chat -count=1 > /results/backend-chat-unit.jsonl
go test -json ./internal/application/service -run 'Test.*SupportChat' -count=1 > /results/backend-chat-service.jsonl
go test -json ./internal/transport/http/handler ./internal/transport/http/handler/admin -run 'Test.*Chat|TestLimitChatSearch' -count=1 > /results/backend-chat-handlers.jsonl
go test -json -tags=integration ./internal/infrastructure/repository -run '^TestChat' -count=1 -timeout=8m > /results/backend-chat-integration.jsonl
rm -rf internal/transport/webassets/dist
cp -a /qa/dist internal/transport/webassets/dist
CGO_ENABLED=0 go build -tags embed -trimpath -ldflags '-s -w -X main.Version=support-chat-qa -X main.Commit=22335a0-chat-persistence -X main.BuildType=release' -o /qa/sub2api ./cmd/server
echo 'PASS Docker chat module, service, handler, PostgreSQL integration tests and production backend build'
