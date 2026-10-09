#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
audit_dir="$(mktemp -d "${TMPDIR:-/tmp}/sub2api-responses-ssrf.XXXXXX")"
resource_prefix="sub2api-responses-ssrf-$$"
private_network="${resource_prefix}-private"
metadata_network="${resource_prefix}-metadata"
public_network="${resource_prefix}-public"
container="${resource_prefix}-test"
baseline_ref="${SUB2API_SSRF_BASELINE_REF:-9600705ea6a6ce9fc02512582e86ef57f9242f63}"

cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  for network in "$public_network" "$metadata_network" "$private_network"; do
    docker network rm "$network" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT

docker network create --internal --subnet 10.253.77.0/24 "$private_network" >/dev/null
docker network create --internal --subnet 169.254.169.0/24 "$metadata_network" >/dev/null
docker network create --internal --subnet 93.184.216.0/24 "$public_network" >/dev/null
git -C "$repo_root" archive -o "$audit_dir/baseline.tar" "$baseline_ref" backend

run_suite() {
  local stage="$1"
  local source_root="$repo_root/backend"
  local -a command=(go test -tags ssrf_docker ./internal/infrastructure/repository -run '^TestResponsesImageSSRFDocker$' -count=1 -v -timeout=3m)
  local -a extra_mounts=()
  local vulnerable=0
  if [[ "$stage" == baseline ]]; then
    vulnerable=1
    extra_mounts=(--mount "type=bind,src=$audit_dir/baseline.tar,dst=/audit/baseline.tar,readonly"
      --mount "type=bind,src=$repo_root/backend/scripts/responses-image-ssrf-baseline-overlay.json,dst=/audit/overlay.json,readonly"
      --mount "type=bind,src=$repo_root/backend/internal/infrastructure/repository/http_upstream_public_docker_test.go,dst=/audit/e2e_test.go,readonly")
    command=(sh -c 'mkdir -p /workspace; tar -xf /audit/baseline.tar -C /workspace; cd /workspace/backend; go test -overlay=/audit/overlay.json -tags ssrf_docker ./internal/infrastructure/repository -run "^TestResponsesImageSSRFDocker$" -count=1 -v -timeout=3m')
  else
    extra_mounts=(--mount "type=bind,src=$source_root,dst=/workspace/backend,readonly")
  fi
  docker create --name "$container" --network "$private_network" --ip 10.253.77.20 \
    --cap-drop ALL --security-opt no-new-privileges \
    --add-host private-image.test:10.253.77.20 --add-host public-image.test:93.184.216.34 \
    --mount "type=volume,src=${SUB2API_SSRF_GOMOD_VOLUME:-sub2api-go-mod-cache},dst=/go/pkg/mod,readonly" \
    --mount "type=volume,src=${SUB2API_SSRF_GOBUILD_VOLUME:-sub2api-go-build-cache},dst=/root/.cache/go-build" \
    "${extra_mounts[@]}" --workdir /workspace/backend \
    --env GOTOOLCHAIN=local --env GOPROXY=off --env RUN_RESPONSES_IMAGE_SSRF_DOCKER=1 \
    --env "SSRF_EXPECT_VULNERABLE=$vulnerable" \
    golang:1.26.9-bookworm "${command[@]}" >/dev/null
  docker network connect --ip 169.254.169.254 "$metadata_network" "$container"
  docker network connect --ip 93.184.216.34 "$public_network" "$container"
  docker start -a "$container" | tee "$audit_dir/$stage.log"
  if ! rg -q '^--- PASS: TestResponsesImageSSRFDocker' "$audit_dir/$stage.log"; then
    printf 'SSRF Docker suite did not execute; evidence: %s\n' "$audit_dir" >&2
    return 1
  fi
  local status
  status="$(docker inspect --format '{{.State.ExitCode}}' "$container")"
  docker rm "$container" >/dev/null
  if [[ "$status" != 0 ]]; then
    printf 'SSRF %s suite failed; evidence: %s\n' "$stage" "$audit_dir" >&2
    return "$status"
  fi
}

printf 'SSRF Docker evidence directory: %s\n' "$audit_dir"
run_suite baseline
run_suite fixed
printf 'Baseline reproduction and fixed regression passed. Evidence: %s\n' "$audit_dir"
