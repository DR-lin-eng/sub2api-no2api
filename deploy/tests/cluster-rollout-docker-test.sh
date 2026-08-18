#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_id="sub2api-cluster-rollout-$$"
network_name="$test_id"
postgres_name="$test_id-postgres"
redis_name="$test_id-redis"
app_a_name="$test_id-app-a"
app_b_name="$test_id-app-b"
postgres_volume="$test_id-postgres-data"
app_a_volume="$test_id-app-a-data"
app_b_volume="$test_id-app-b-data"
source_image="$test_id:source"
target_image="$test_id:target"
source_version=${CLUSTER_TEST_SOURCE_VERSION:-0.0.0-cluster-test}
target_version=${CLUSTER_TEST_TARGET_VERSION:-$($repo_root/backend/scripts/resolve-version.sh)}
image_replacement=${CLUSTER_TEST_IMAGE_REPLACEMENT:-0}
initial_rollout_poll_seconds=1
if [ "$image_replacement" = "1" ]; then
  initial_rollout_poll_seconds=30
fi
admin_key=admin-cluster-rollout-test-key
admin_key_hash=2879e03aa0b2c375f81a39e9b1ca2428160c07f310f0c0567f38fc9fc1ae3417
postgres_password=cluster-rollout-postgres-password
jwt_secret=cluster-rollout-jwt-secret-0123456789abcdef
totp_key=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef

cleanup() {
  if [ "${KEEP_CLUSTER_TEST:-0}" = "1" ]; then
    printf 'cluster rollout test resources retained with prefix %s\n' "$test_id"
    return
  fi
  docker rm -f "$app_a_name" "$app_b_name" "$postgres_name" "$redis_name" >/dev/null 2>&1 || true
  docker volume rm "$app_a_volume" "$app_b_volume" "$postgres_volume" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
  docker image rm "$source_image" "$target_image" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

for command_name in docker curl jq; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'required command not found: %s\n' "$command_name" >&2
    exit 1
  fi
done

wait_container_command() {
  container_name=$1
  shift
  attempts=0
  until docker exec "$container_name" "$@" >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 90 ]; then
      printf 'container did not become ready: %s\n' "$container_name" >&2
      docker logs "$container_name" >&2 || true
      exit 1
    fi
    sleep 1
  done
}

container_url() {
  container_name=$1
  address=$(docker port "$container_name" 8080/tcp | sed -n '1p')
  printf 'http://%s' "$address"
}

wait_http_status() {
  url=$1
  expected=$2
  container_name=${3:-}
  attempts=0
  while :; do
    status=$(curl -sS -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)
    if [ "$status" = "$expected" ]; then
      return
    fi
    if [ -n "$container_name" ] && [ "$(docker inspect -f '{{.State.Running}}' "$container_name" 2>/dev/null || true)" != "true" ]; then
      printf 'container exited while waiting for %s: %s\n' "$url" "$container_name" >&2
      docker logs "$container_name" >&2 || true
      return 1
    fi
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 180 ]; then
      printf 'timed out waiting for %s to return %s; last status=%s\n' "$url" "$expected" "$status" >&2
      return 1
    fi
    sleep 1
  done
}

wait_internal_http_status() {
  container_name=$1
  path=$2
  expected=$3
  attempts=0
  while :; do
    probe=$(docker exec "$postgres_name" wget -S -O /dev/null -T 2 \
      "http://$container_name:8080$path" 2>&1 || true)
    status=$(printf '%s\n' "$probe" | sed -n 's/.*HTTP\/[0-9.]* \([0-9][0-9][0-9]\).*/\1/p' | tail -n 1)
    if [ "$status" = "$expected" ]; then
      return
    fi
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 180 ]; then
      printf 'timed out waiting for internal http://%s:8080%s to return %s; last status=%s\n' \
        "$container_name" "$path" "$expected" "$status" >&2
      docker logs "$container_name" >&2 || true
      return 1
    fi
    sleep 1
  done
}

run_app() {
  container_name=$1
  volume_name=$2
  image_name=$3
  configured_name=$4
  rollout_poll_seconds=${5:-1}
  docker run --detach \
    --name "$container_name" \
    --restart unless-stopped \
    --network "$network_name" \
    --publish 127.0.0.1::8080 \
    --volume "$volume_name:/app/data" \
    --env AUTO_SETUP=true \
    --env SERVER_HOST=0.0.0.0 \
    --env SERVER_PORT=8080 \
    --env SERVER_MODE=release \
    --env DEPLOYMENT_MODE=multi_instance \
    --env DEPLOYMENT_NODE_ID_FILE=/app/data/.cluster-node-id \
    --env "NODE_NAME=$configured_name" \
    --env WORKER_ENABLED=auto \
    --env DEPLOYMENT_HEARTBEAT_INTERVAL_SECONDS=2 \
    --env DEPLOYMENT_STALE_AFTER_SECONDS=6 \
    --env DEPLOYMENT_TASK_LEASE_SECONDS=15 \
    --env "DEPLOYMENT_ROLLOUT_POLL_SECONDS=$rollout_poll_seconds" \
    --env DEPLOYMENT_ROLLOUT_DRAIN_GRACE_SECONDS=0 \
    --env DEPLOYMENT_ROLLOUT_DRAIN_TIMEOUT_SECONDS=30 \
    --env DEPLOYMENT_ROLLOUT_VERIFY_HEARTBEATS=3 \
    --env "DATABASE_HOST=$postgres_name" \
    --env DATABASE_PORT=5432 \
    --env DATABASE_USER=sub2api \
    --env "DATABASE_PASSWORD=$postgres_password" \
    --env DATABASE_DBNAME=sub2api \
    --env DATABASE_SSLMODE=disable \
    --env DATABASE_MAX_OPEN_CONNS=20 \
    --env DATABASE_MAX_IDLE_CONNS=5 \
    --env "REDIS_HOST=$redis_name" \
    --env REDIS_PORT=6379 \
    --env REDIS_POOL_SIZE=50 \
    --env REDIS_MIN_IDLE_CONNS=5 \
    --env ADMIN_EMAIL=admin@cluster.test \
    --env ADMIN_PASSWORD=cluster-rollout-admin-password \
    --env "JWT_SECRET=$jwt_secret" \
    --env "TOTP_ENCRYPTION_KEY=$totp_key" \
    --env TZ=UTC \
    "$image_name" >/dev/null
}

api_call() (
  method=$1
  base_url=$2
  path=$3
  body=${4:-}
  response_file=$(mktemp "${TMPDIR:-/tmp}/sub2api-cluster-api.XXXXXX")
  if [ -n "$body" ]; then
    status=$(curl -sS \
      --output "$response_file" \
      --write-out '%{http_code}' \
      --request "$method" \
      --header "x-api-key: $admin_key" \
      --header 'Content-Type: application/json' \
      --data "$body" \
      "$base_url$path") || {
        curl_status=$?
        printf 'API request failed: %s %s%s (curl exit %s)\n' "$method" "$base_url" "$path" "$curl_status" >&2
        cat "$response_file" >&2
        rm -f "$response_file"
        return "$curl_status"
      }
  else
    status=$(curl -sS \
      --output "$response_file" \
      --write-out '%{http_code}' \
      --request "$method" \
      --header "x-api-key: $admin_key" \
      "$base_url$path") || {
        curl_status=$?
        printf 'API request failed: %s %s%s (curl exit %s)\n' "$method" "$base_url" "$path" "$curl_status" >&2
        cat "$response_file" >&2
        rm -f "$response_file"
        return "$curl_status"
      }
  fi
  case "$status" in
    2??)
      cat "$response_file"
      rm -f "$response_file"
      ;;
    *)
      printf 'API request failed: %s %s%s (HTTP %s)\n' "$method" "$base_url" "$path" "$status" >&2
      cat "$response_file" >&2
      rm -f "$response_file"
      return 1
      ;;
  esac
)

api_get_internal() (
  container_name=$1
  path=$2
  response_file=$(mktemp "${TMPDIR:-/tmp}/sub2api-cluster-api.XXXXXX")
  if ! docker exec "$postgres_name" wget -q -O - \
    --header "x-api-key: $admin_key" \
    "http://$container_name:8080$path" >"$response_file"; then
    printf 'internal API request failed: GET http://%s:8080%s\n' "$container_name" "$path" >&2
    cat "$response_file" >&2
    rm -f "$response_file"
    return 1
  fi
  cat "$response_file"
  rm -f "$response_file"
)

assert_json() {
  json=$1
  filter=$2
  message=$3
  if ! printf '%s' "$json" | jq -e "$filter" >/dev/null; then
    printf 'assertion failed: %s\n' "$message" >&2
    printf '%s\n' "$json" | jq . >&2 || true
    exit 1
  fi
}

wait_rollout_target_status() (
  container_name=$1
  rollout_id=$2
  node_id=$3
  expected=$4
  attempts=0
  while :; do
    rollout_json=$(api_get_internal "$container_name" "/api/v1/admin/cluster/rollouts/$rollout_id")
    target_status=$(printf '%s' "$rollout_json" | jq -r --arg node_id "$node_id" '.data.targets[] | select(.node_id == $node_id) | .status')
    if [ "$target_status" = "$expected" ]; then
      return
    fi
    case "$target_status" in
      failed|cancelled)
        printf 'rollout target entered terminal status %s while waiting for %s\n' "$target_status" "$expected" >&2
        printf '%s\n' "$rollout_json" | jq . >&2 || true
        return 1
        ;;
    esac
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 180 ]; then
      printf 'timed out waiting for rollout %s node %s to reach %s; last status=%s\n' "$rollout_id" "$node_id" "$expected" "$target_status" >&2
      printf '%s\n' "$rollout_json" | jq . >&2 || true
      return 1
    fi
    sleep 1
  done
)

cd "$repo_root"
printf 'building Docker images for source=%s target=%s\n' "$source_version" "$target_version"
build_image() {
  version=$1
  tag=$2
  goproxy=${CLUSTER_TEST_GOPROXY:-https://goproxy.cn,direct}
  if [ "${CLUSTER_TEST_NO_CACHE:-0}" = "1" ]; then
    docker build --no-cache --file Dockerfile --build-arg "VERSION=$version" --build-arg "GOPROXY=$goproxy" --tag "$tag" .
  else
    docker build --file Dockerfile --build-arg "VERSION=$version" --build-arg "GOPROXY=$goproxy" --tag "$tag" .
  fi
}
build_image "$source_version" "$source_image"
build_image "$target_version" "$target_image"

docker network create "$network_name" >/dev/null
docker volume create "$postgres_volume" >/dev/null
docker volume create "$app_a_volume" >/dev/null
docker volume create "$app_b_volume" >/dev/null

docker run --detach \
  --name "$postgres_name" \
  --network "$network_name" \
  --volume "$postgres_volume:/var/lib/postgresql/data" \
  --env POSTGRES_USER=sub2api \
  --env "POSTGRES_PASSWORD=$postgres_password" \
  --env POSTGRES_DB=sub2api \
  --env PGDATA=/var/lib/postgresql/data \
  postgres:18.1-alpine3.23 >/dev/null
docker run --detach \
  --name "$redis_name" \
  --network "$network_name" \
  redis:8.4-alpine >/dev/null

wait_container_command "$postgres_name" pg_isready -U sub2api -d sub2api
wait_container_command "$redis_name" redis-cli ping

run_app "$app_a_name" "$app_a_volume" "$source_image" alpha "$initial_rollout_poll_seconds"
app_a_url=$(container_url "$app_a_name")
wait_http_status "$app_a_url/ready" 200 "$app_a_name"
run_app "$app_b_name" "$app_b_volume" "$source_image" alpha "$initial_rollout_poll_seconds"
app_b_url=$(container_url "$app_b_name")
wait_http_status "$app_b_url/ready" 200 "$app_b_name"

admin_key_json="{\"version\":1,\"keys\":[{\"id\":\"cluster-rollout-test\",\"name\":\"Cluster rollout test\",\"key_prefix\":\"admin-clus\",\"last_four\":\"-key\",\"scopes\":[\"admin.read\",\"admin.write\"],\"status\":\"active\",\"created_by\":1,\"created_at\":\"2026-08-09T00:00:00Z\",\"updated_at\":\"2026-08-09T00:00:00Z\",\"key_hash\":\"$admin_key_hash\"}]}"
compliance_json='{"version":"v2026.06.10","document_zh":"docs/legal/admin-compliance.zh.md","document_en":"docs/legal/admin-compliance.en.md","admin_user_id":1,"accepted_at":"2026-08-09T00:00:00Z"}'
docker exec "$postgres_name" psql -U sub2api -d sub2api -v ON_ERROR_STOP=1 -c \
  "INSERT INTO settings (key, value) VALUES ('admin_api_keys', '$admin_key_json') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()" >/dev/null
docker exec "$postgres_name" psql -U sub2api -d sub2api -v ON_ERROR_STOP=1 -c \
  "INSERT INTO settings (key, value) VALUES ('admin_compliance_acknowledgement:1', '$compliance_json') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()" >/dev/null

status_a=$(api_call GET "$app_a_url" /api/v1/admin/cluster/status)
status_b=$(api_call GET "$app_b_url" /api/v1/admin/cluster/status)
assert_json "$status_a" ".data.instances | length == 2" "node A must see exactly two logical nodes"
assert_json "$status_b" ".data.instances | length == 2" "node B must see exactly two logical nodes"
assert_json "$status_a" ".data.instances | all(.version == \"$source_version\")" "all nodes must report the source version"
assert_json "$status_a" ".data.instances | map(.node_name) | unique | length == 2" "duplicate configured names must receive distinct initial display names"

node_a_id=$(printf '%s' "$status_a" | jq -r '.data.instances[] | select(.current == true) | .node_id')
node_b_id=$(printf '%s' "$status_a" | jq -r '.data.instances[] | select(.current == false) | .node_id')
node_b_runner=$(printf '%s' "$status_a" | jq -r '.data.instances[] | select(.current == false) | .runner_id')
if [ -z "$node_a_id" ] || [ -z "$node_b_id" ]; then
  printf 'failed to resolve initial logical node identities\n' >&2
  exit 1
fi

api_call PUT "$app_a_url" "/api/v1/admin/cluster/nodes/$node_b_id" '{"name":"bravo-renamed"}' >/dev/null
docker rm -f "$app_b_name" >/dev/null
run_app "$app_b_name" "$app_b_volume" "$source_image" alpha "$initial_rollout_poll_seconds"
app_b_url=$(container_url "$app_b_name")
wait_http_status "$app_b_url/ready" 200 "$app_b_name"

status_after_recreate=$(api_call GET "$app_a_url" /api/v1/admin/cluster/status)
assert_json "$status_after_recreate" ".data.instances | length == 2" "recreating a container must not add a logical node"
assert_json "$status_after_recreate" ".data.instances[] | select(.node_id == \"$node_b_id\") | .node_name == \"bravo-renamed\"" "renamed node alias must survive recreation"
new_node_b_runner=$(printf '%s' "$status_after_recreate" | jq -r ".data.instances[] | select(.node_id == \"$node_b_id\") | .runner_id")
if [ "$new_node_b_runner" = "$node_b_runner" ]; then
  printf 'runner_id did not change after container recreation\n' >&2
  exit 1
fi

rollout=$(api_call POST "$app_b_url" /api/v1/admin/cluster/rollouts "{\"target_version\":\"$target_version\",\"confirm\":true}")
rollout_id=$(printf '%s' "$rollout" | jq -r '.data.id')
assert_json "$rollout" ".data.targets | length == 2" "rollout created through node B must target both nodes"
assert_json "$rollout" ".data.targets | all(.target_version == \"$target_version\")" "every target must use one exact version"
assert_json "$rollout" ".data.targets[0].node_id == \"$node_a_id\"" "alpha must be the first serial target"

status_after_create=$(api_get_internal "$app_b_name" /api/v1/admin/cluster/status)
assert_json "$status_after_create" ".data.release.state.desired_version == \"$target_version\"" "candidate version must be announced before the first restart"
assert_json "$status_after_create" ".data.release.state.locked_version == \"\" or .data.release.state.locked_version == null" "candidate version must remain unlocked during rollout"
wait_internal_http_status "$app_a_name" /ready 200

if [ "$image_replacement" = "1" ]; then
  api_call POST "$app_b_url" "/api/v1/admin/cluster/rollouts/$rollout_id/pause" >/dev/null
  docker rm -f "$app_a_name" "$app_b_name" >/dev/null

  run_app "$app_a_name" "$app_a_volume" "$target_image" alpha 1
  app_a_url=$(container_url "$app_a_name")
  wait_http_status "$app_a_url/ready" 200 "$app_a_name"
  api_call POST "$app_a_url" "/api/v1/admin/cluster/rollouts/$rollout_id/resume" >/dev/null
  wait_rollout_target_status "$app_a_name" "$rollout_id" "$node_a_id" succeeded

  run_app "$app_b_name" "$app_b_volume" "$target_image" alpha 1
  app_b_url=$(container_url "$app_b_name")
  wait_http_status "$app_b_url/ready" 200 "$app_b_name"
  wait_rollout_target_status "$app_a_name" "$rollout_id" "$node_b_id" succeeded
else
  wait_rollout_target_status "$app_b_name" "$rollout_id" "$node_a_id" succeeded
  wait_internal_http_status "$app_a_name" /ready 200

  rollout_after_a=$(api_get_internal "$app_a_name" "/api/v1/admin/cluster/rollouts/$rollout_id")
  assert_json "$rollout_after_a" ".data.targets[] | select(.node_id == \"$node_a_id\") | .status == \"succeeded\"" "the first target must verify before the second replacement"

  wait_rollout_target_status "$app_a_name" "$rollout_id" "$node_b_id" succeeded
  wait_internal_http_status "$app_b_name" /ready 200
fi

final_status=$(api_get_internal "$app_a_name" /api/v1/admin/cluster/status)
assert_json "$final_status" ".data.instances | length == 2" "completed rollout must still expose two logical nodes"
assert_json "$final_status" ".data.release.active_rollout.status == \"awaiting_confirmation\"" "verified rollout must wait for operator confirmation"
assert_json "$final_status" ".data.release.state.active_rollout_id == \"$rollout_id\"" "active rollout must remain visible until confirmation"
assert_json "$final_status" ".data.release.consistent == true" "cluster versions must converge before confirmation"
assert_json "$final_status" ".data.release.state.desired_version == \"$target_version\"" "desired version must advance after verification"
assert_json "$final_status" ".data.release.version_counts == [{\"version\":\"$target_version\",\"nodes\":2}]" "both nodes must report the target version"
assert_json "$final_status" ".data.instances[] | select(.node_id == \"$node_b_id\") | .node_name == \"bravo-renamed\"" "node alias must survive the rollout"

api_call POST "$app_a_url" "/api/v1/admin/cluster/rollouts/$rollout_id/confirm" '{"confirm":true}' >/dev/null
confirmed_status=$(api_get_internal "$app_a_name" /api/v1/admin/cluster/status)
assert_json "$confirmed_status" ".data.release.state.active_rollout_id == null or .data.release.state.active_rollout_id == \"\"" "manual confirmation must clear the active rollout"
assert_json "$confirmed_status" ".data.release.state.locked_version == \"$target_version\"" "manual confirmation must lock the target version"

docker rm -f "$app_a_name" >/dev/null
run_app "$app_a_name" "$app_a_volume" "$source_image" alpha 1
wait_internal_http_status "$app_a_name" /health 200
wait_internal_http_status "$app_a_name" /ready 503

status_with_old_node=$(api_get_internal "$app_a_name" /api/v1/admin/cluster/status)
assert_json "$status_with_old_node" ".data.instances | length == 2" "a non-ready node must retain access to the authenticated cluster recovery API"
assert_json "$status_with_old_node" ".data.instances[] | select(.node_id == \"$node_a_id\") | .version == \"$source_version\"" "inventory must expose the regressed version"

docker rm -f "$app_a_name" >/dev/null
run_app "$app_a_name" "$app_a_volume" "$target_image" alpha 1
wait_internal_http_status "$app_a_name" /ready 200

printf 'cluster rollout Docker test passed: nodes=2 target=%s rollout=%s\n' "$target_version" "$rollout_id"
