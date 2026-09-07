#!/bin/sh
set -eu
phase=${1:?phase required}
root=${2:-$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)}
image=sha256:64e6355fc108df54c0f9751c91a24b593dceebf01cebebe2f2e5658849e8fa4a
printf '%s\n' "$phase input: TestOpenAIStreamControl* and TestOpenAIStreamPoolRecovery*; native/passthrough; HTTP/1.1/HTTP/2; sanitized SSE fixture; five-account recovery"
set +e
docker run --rm --network none --entrypoint go \
  -e GOMAXPROCS=2 -e GOGC=20 -e GOMEMLIMIT=1200MiB \
  -v sub2api-stream-control-gobuild-20260907:/root/.cache/go-build \
  -v "$root/backend:/app/backend:ro" -w /app/backend \
  "$image" test -tags=unit -p 1 ./internal/application/service ./internal/transport/http/handler \
  -run '^TestOpenAIStream(Control|PoolRecovery)' -count=1 -timeout=180s
status=$?
set -e
printf '%s exit=%s\n' "$phase" "$status"
exit "$status"
