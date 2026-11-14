#!/bin/sh
set -eu
go test -json -tags=unit -p 2 -count=1 -timeout=10m ./internal/application/service -run 'DesktopPlan|DesktopJWT|DesktopRefreshPlan|DesktopNewer|DesktopExpired|DesktopExpiry|DesktopNewVersions|DesktopWaitedToken|OAuthPrincipalChanges|RefreshPrincipalChange|TeamGenericUnauthorized|TokenExpired|CodexDesktop|DesktopAlignment' > /results/focused-final.jsonl 2>&1
go test -json -tags=unit -p 2 -count=1 -timeout=15m ./internal/application/service/... ./internal/transport/http/... ./internal/shared/openai/... > /results/regression-final.jsonl 2>&1
go build -p 2 -o /tmp/sub2api ./cmd/server > /results/build-final.log 2>&1
