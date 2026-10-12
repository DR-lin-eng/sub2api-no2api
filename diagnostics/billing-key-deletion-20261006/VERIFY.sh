#!/usr/bin/env bash
set -euo pipefail

audit_dir=$(cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(cd "$audit_dir/../.." && pwd)
test_image=sub2api-billing-audit-test:20261006
project=sub2api-billing-audit-20261006
module_cache=${AUDIT_MODULE_CACHE:-/Users/lin/go/pkg/mod}
docker_socket=${AUDIT_DOCKER_SOCKET:-/Users/lin/.docker/run/docker.sock}
compose=(docker compose -f "$audit_dir/compose.yaml" -p "$project")
runner=(docker run --rm
  -v "$repo_dir/backend:/workspace:ro"
  -v "$audit_dir:/evidence"
  -v "$module_cache:/go/pkg/mod"
  -v sub2api-sync-gocache:/root/.cache/go-build)
integration=(-v "$docker_socket:/var/run/docker.sock"
  -e CI=true -e DOCKER_HOST=unix:///var/run/docker.sock
  -e TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal
  -e TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock)

normalize_logs() {
  python3 - "$audit_dir" <<'PY'
import pathlib
import sys
for path in pathlib.Path(sys.argv[1]).glob("*.log"):
    lines = [line.expandtabs(4).rstrip() for line in path.read_text().splitlines()]
    path.write_text("\n".join(lines).rstrip() + ("\n" if lines else ""))
PY
}

docker build -t "$test_image" -f "$audit_dir/Dockerfile.test" "$audit_dir" > "$audit_dir/docker-test-image.log" 2>&1
"${runner[@]}" "$test_image" go test -tags=unit ./internal/application/service ./internal/infrastructure/repository \
  -run '^(TestApplyUsageBillingEffects|TestDeductUsageBillingBalance|TestApiKeyService_Delete|TestAPIKeyService_Delete|Test.*UsageBilling)' \
  -count=1 -v > "$audit_dir/docker-unit.log" 2>&1
"${runner[@]}" "${integration[@]}" "$test_image" go test -tags=integration ./internal/infrastructure/repository \
  -run '^(TestUsageBillingKeyDeletion|TestUsageBillingRepositoryApply|TestDurableUsageBillingQueue)' \
  -count=1 -v > "$audit_dir/docker-integration.log" 2>&1
"${runner[@]}" "${integration[@]}" "$test_image" go test -race -tags=integration ./internal/infrastructure/repository \
  -run '^(TestUsageBillingKeyDeletion|TestUsageBillingRepositoryApply_DeletedAPIKeyStillBillsBalance|TestDurableUsageBillingQueueSurvivesRedisLoss)' \
  -count=1 -v > "$audit_dir/docker-race.log" 2>&1

# Historical negative control: restoring this one pre-fix source file must fail
# every deletion regression. Production source is unchanged by the Go overlay.
if "${runner[@]}" "${integration[@]}" "$test_image" go test -overlay=/evidence/pre-fix-overlay.json \
  -tags=integration ./internal/infrastructure/repository -run '^TestUsageBillingKeyDeletion' \
  -count=1 -v > "$audit_dir/docker-pre-fix-replay.log" 2>&1; then
  echo 'ERROR: pre-fix negative control unexpectedly passed' >&2
  exit 1
fi
test "$(rg -c '^    --- FAIL: TestUsageBillingKeyDeletion' "$audit_dir/docker-pre-fix-replay.log")" = 4

commit=$(git -C "$repo_dir" rev-parse HEAD)
"${runner[@]}" -e CGO_ENABLED=0 "$test_image" go build \
  -ldflags="-X main.Commit=$commit -X main.BuildType=debug" \
  -o /evidence/server ./cmd/server > "$audit_dir/docker-build.log" 2>&1

# Only this disposable audit project is removed; no other Docker resources.
trap '"${compose[@]}" down -v > "$audit_dir/docker-runtime-cleanup.log" 2>&1; normalize_logs' EXIT
"${compose[@]}" down -v > "$audit_dir/docker-runtime-reset.log" 2>&1
BILLING_AUDIT_QUEUE_ENABLED=false "${compose[@]}" up -d > "$audit_dir/docker-runtime-start.log" 2>&1
python3 "$audit_dir/runtime_check.py" direct > "$audit_dir/runtime-direct.log" 2>&1
BILLING_AUDIT_QUEUE_ENABLED=true "${compose[@]}" up -d --force-recreate app > "$audit_dir/docker-runtime-queued-start.log" 2>&1
python3 "$audit_dir/runtime_check.py" queued > "$audit_dir/runtime-queued.log" 2>&1
"${compose[@]}" logs --no-color app upstream > "$audit_dir/runtime-app.log" 2>&1
"${compose[@]}" exec -T postgres psql -U audit -d audit -v ON_ERROR_STOP=1 -f - \
  < "$audit_dir/audit.sql" > "$audit_dir/runtime-audit-sql.log"
echo 'PASS: current billing, historical negative control and 24 HTTP scenarios'
