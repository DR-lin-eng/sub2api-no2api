#!/bin/sh
set -eu
HERE=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
MODE=${1:?usage: VERIFY.sh baseline|modified|rollback /absolute/repository}
REPO=${2:?repository is required}
case "$MODE" in baseline|rollback) EXPECT=NULL;; modified) EXPECT=0;; *) exit 64;; esac
TMP=$(mktemp -d /private/tmp/sub2api-metrics-probe.XXXXXX)
trap 'rm -rf "$TMP"' EXIT HUP INT TERM
cp -R "$REPO/backend" "$TMP/backend"
cp "$HERE/METRICS_PROBE.go" "$TMP/backend/internal/infrastructure/repository/upstream_sync_probe_test.go"
set +e
docker run --rm -e SYNC_EXPECT="$EXPECT" -e GOMAXPROCS=2 -e GOCACHE=/root/.cache/go-build \
 -v "$TMP/backend:/src/backend" -v codex-sub2api-go-mod:/go/pkg/mod \
 -v codex-sub2api-go-build:/root/.cache/go-build -w /src/backend sub2api-pr31:go-test \
 go test -vet=off -p=1 -tags=unit -count=1 ./internal/infrastructure/repository -run '^TestUpstreamSyncMetricsProbe$' -v > "$TMP/probe.log" 2>&1
RESULT=$?
set -e
if [ "$RESULT" -ne 0 ]; then cat "$TMP/probe.log"; exit "$RESULT"; fi
grep '^DBConnActive' "$TMP/probe.log"
printf '%s test_exit=%s\n' "$MODE" "$RESULT"
