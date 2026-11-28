#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
output_dir=${1:-/private/tmp/sub2api-ws-image-compression-20261127}
mod_cache=${GO_MOD_CACHE:-sub2api-go-modcache-20261125}
build_cache=${GO_BUILD_CACHE:-sub2api-go-buildcache-20261125}

mkdir -p "$output_dir"

set -o pipefail

docker run --rm --network none --cpus=2 --memory=4g \
	-e GOTOOLCHAIN=local \
	-e GOPROXY=off \
	-e GOSUMDB=off \
	-e CGO_ENABLED=0 \
	-e GOMAXPROCS=2 \
	-v "$repo_root/backend:/src:ro" \
	-v "$mod_cache:/go/pkg/mod:ro" \
	-v "$build_cache:/root/.cache/go-build" \
	-w /src \
	golang:1.26.11-alpine \
	sh -ec '
	go test ./internal/platform/config -count=1
	go test ./internal/application/service -run "TestOpenAIWS|TestPrepareOpenAIWSImagePayload|TestOpenAIWSRoutingUsesHardLimitAfterImagePolicy" -count=1
	go test -tags=unit ./internal/transport/http/handler -run "TestOpenAIResponsesWSSizeHTTPRecovery" -count=1
	go test ./internal/application/service -run "^$" -bench "^BenchmarkPrepareOpenAIWSImagePayload$" -benchmem -benchtime=3x
	' 2>&1 | tee "$output_dir/docker.log"

cat > "$output_dir/RESULTS.json" <<EOF
{
  "status": "pass",
  "date": "2026-11-27",
  "image": "golang:1.26.11-alpine",
  "cpus": 2,
  "memory": "4g",
  "gomaxprocs": 2,
  "cgo_enabled": 0,
  "network": "none",
  "tests": [
    "platform config defaults and validation",
    "OpenAI WS image compression and routing",
    "HTTP handler WS size and streaming recovery",
    "image compression benchmark"
  ]
}
EOF

echo "Docker image compression verification passed"
