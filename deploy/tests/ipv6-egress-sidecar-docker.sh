#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH='' cd -- "$script_dir/../.." && pwd)
backend_dir=$repo_dir/backend
suffix=$((($$ % 60000) + 1000))
octet=$(((suffix % 200) + 20))
segment=$(printf '%x' "$suffix")
network_name=sub2api-he-sidecar-test-$suffix
server_name=sub2api-he-sidecar-server-$suffix
app_name=sub2api-he-sidecar-app-$suffix
sidecar_name=sub2api-he-sidecar-agent-$suffix
image=${IPV6_EGRESS_SIDECAR_IMAGE:-sub2api-ipv6-egress-sidecar:local}
server_ipv4=172.30.$octet.10
client_ipv4=172.30.$octet.20
server_ipv6=fd50:$segment::1
client_ipv6=fd50:$segment::2
routed_pool=fd51:$segment::/120
source_a=fd51:$segment::20
source_b=fd51:$segment::21
control_dir=$(mktemp -d "${TMPDIR:-/tmp}/sub2api-he-sidecar.XXXXXX")

cleanup() {
  docker rm -f "$sidecar_name" >/dev/null 2>&1 || true
  docker rm -f "$app_name" >/dev/null 2>&1 || true
  docker rm -f "$server_name" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
  rm -rf -- "$control_dir"
}
trap cleanup EXIT INT TERM

chmod 0777 "$control_dir"
if ! docker image inspect "$image" >/dev/null 2>&1; then
  docker build -f "$repo_dir/deploy/Dockerfile.ipv6-egress-sidecar" -t "$image" "$repo_dir" >/dev/null
fi

docker network create --subnet "172.30.$octet.0/24" "$network_name" >/dev/null
docker run -d --name "$server_name" \
  --network "$network_name" --ip "$server_ipv4" \
  --cap-add NET_ADMIN \
  -v sub2api-ipv6-go-mod:/go/pkg/mod \
  -v sub2api-ipv6-go-build:/root/.cache/go-build \
  -v "$backend_dir:/app" -w /app \
  golang:1.26.6-alpine \
  go run ./internal/platform/egress/testdata/echo_server.go >/dev/null
docker run -d --name "$app_name" \
  --network "$network_name" --ip "$client_ipv4" \
  -v sub2api-ipv6-go-mod:/go/pkg/mod \
  -v sub2api-ipv6-go-build:/root/.cache/go-build \
  -v "$backend_dir:/app" -w /app \
  -v "$control_dir:/control" \
  golang:1.26.6-alpine sleep 300 >/dev/null

docker exec "$server_name" ip tunnel add he-server mode sit \
  remote "$client_ipv4" local "$server_ipv4" ttl 255
docker exec "$server_name" ip link set he-server mtu 1480 up
docker exec "$server_name" ip -6 addr add "$server_ipv6/64" dev he-server
docker exec "$server_name" ip -6 route replace "$routed_pool" via "$client_ipv6" dev he-server

cat >"$control_dir/desired.env" <<EOF
HE_TUNNEL_ENABLED=true
HE_TUNNEL_SERVER_IPV4=$server_ipv4
HE_TUNNEL_LOCAL_IPV4=$client_ipv4
HE_TUNNEL_CLIENT_IPV6=$client_ipv6/64
HE_TUNNEL_SERVER_IPV6=$server_ipv6
IPV6_EGRESS_POOL_CIDR=$routed_pool
HE_TUNNEL_MTU=1480
HE_TUNNEL_TTL=255
HE_TUNNEL_ROUTE_METRIC=2048
HE_TUNNEL_PROBE_IPV6=$server_ipv6
HE_TUNNEL_PROBE_TIMEOUT_SECONDS=3
HE_TUNNEL_ALLOW_PRIVATE_IPV4=true
HE_TUNNEL_UPDATE_ENABLED=false
HE_TUNNEL_ID=
HE_TUNNEL_USERNAME=
HE_TUNNEL_UPDATE_KEY=
EOF

write_request() {
  request_id=$1
  request_action=$2
  cat >"$control_dir/request.env" <<EOF
IPV6_EGRESS_REQUEST_ID=$request_id
IPV6_EGRESS_REQUEST_ACTION=$request_action
IPV6_EGRESS_REQUESTED_AT_UNIX=$(date +%s)
EOF
}

run_sidecar_once() {
  docker run --rm --name "$sidecar_name" \
    --network "container:$app_name" \
    --cap-add NET_ADMIN --cap-add NET_RAW \
    --security-opt no-new-privileges:true \
    -v "$control_dir:/control" \
    "$image" once
}

write_request 11111111111111111111111111111111 apply
run_sidecar_once
grep -F 'IPV6_EGRESS_STATUS_STATE=succeeded' "$control_dir/status.env" >/dev/null
docker exec "$app_name" ip link show he-sub2api >/dev/null
docker exec "$app_name" ip -6 route show table local "$routed_pool" | \
  grep -F "local $routed_pool dev lo" >/dev/null

cap_add=$(docker inspect -f '{{json .HostConfig.CapAdd}}' "$app_name")
case "$cap_add" in
  *NET_ADMIN* | *NET_RAW*) echo "main application test container unexpectedly has network capabilities" >&2; exit 1 ;;
esac

ready=false
attempt=0
while [ "$attempt" -lt 30 ]; do
  if docker logs "$server_name" 2>&1 | grep -q 'IPv6 echo ready'; then
    ready=true
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
[ "$ready" = true ] || {
  docker logs "$server_name" >&2
  exit 1
}

docker exec \
  -e IPV6_EGRESS_ECHO_URL="http://[$server_ipv6]:8080" \
  -e IPV6_EGRESS_SOURCE_A="$source_a" \
  -e IPV6_EGRESS_SOURCE_B="$source_b" \
  "$app_name" go test -tags integration ./internal/platform/egress \
  -run '^TestDockerIPv6SourceIsolation$' -count=1 -v

write_request 22222222222222222222222222222222 check
run_sidecar_once
grep -F 'IPV6_EGRESS_STATUS_STATE=succeeded' "$control_dir/status.env" >/dev/null

write_request 33333333333333333333333333333333 remove
run_sidecar_once
grep -F 'IPV6_EGRESS_STATUS_STATE=succeeded' "$control_dir/status.env" >/dev/null
if docker exec "$app_name" ip link show he-sub2api >/dev/null 2>&1; then
  echo "HE sidecar tunnel still exists after remove" >&2
  exit 1
fi

echo "Container-only HE sidecar Docker test passed."
