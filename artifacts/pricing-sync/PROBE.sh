#!/bin/sh
set -eu
artifact_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
case "${1-}" in
  BASELINE) source_dir=/private/tmp/pricing-baseline ;;
  MODIFIED) source_dir=$(CDPATH= cd -- "$artifact_dir/../.." && pwd) ;;
  ROLLBACK) source_dir=/private/tmp/pricing-sync-rollback-31dd ;;
  *) echo 'usage: PROBE.sh BASELINE|MODIFIED|ROLLBACK' >&2; exit 2 ;;
esac
exec docker run --rm \
  -v "$source_dir":/workspace:ro \
  -v "$artifact_dir":/evidence:ro \
  -v /Users/lin/go/pkg/mod:/go/pkg/mod \
  -v /private/tmp/pricing-go-cache:/root/.cache/go-build \
  -w /workspace/backend -e GOMAXPROCS=4 golang:1.26.6-bookworm \
  go run -overlay /evidence/probe-overlay.json ./cmd/server/main.go "$1"
