#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH='' cd -- "$script_dir/../.." && pwd)
backend_dir=$repo_dir/backend
suffix=$(( $$ % 65535 ))
segment=$(printf '%x' "$suffix")
network_name=sub2api-ipv6-egress-test-$suffix
echo_name=sub2api-ipv6-echo-$suffix
client_name=sub2api-ipv6-client-$suffix
subnet=fd42:5355:$segment::/64
echo_ip=fd42:5355:$segment::10
client_ip=fd42:5355:$segment::20
routed_pool=fd43:5355:$segment::/120
source_a=fd43:5355:$segment::20
source_b=fd43:5355:$segment::21

cleanup() {
  docker rm -f "$echo_name" >/dev/null 2>&1 || true
  docker rm -f "$client_name" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker network create --ipv6 --subnet "$subnet" "$network_name" >/dev/null
docker run -d --name "$echo_name" \
  --network "$network_name" --ip6 "$echo_ip" \
  -e PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/sbin \
  -v "$backend_dir:/app" -w /app \
  golang:1.26.6-alpine \
  /usr/local/go/bin/go run ./internal/platform/egress/testdata/echo_server.go >/dev/null

# Keep the application process in an unprivileged container. Short-lived init
# containers modify only the two test network namespaces, mirroring the host
# setup script without granting NET_ADMIN to the application container.
docker run -d --name "$client_name" \
  --network "$network_name" --ip6 "$client_ip" \
  -e PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin:/sbin \
  -v sub2api-ipv6-go-mod:/go/pkg/mod \
  -v sub2api-ipv6-go-build:/root/.cache/go-build \
  -v "$backend_dir:/app" -w /app \
  golang:1.26.6-alpine sleep 300 >/dev/null

docker run --rm --network "container:$echo_name" --cap-add NET_ADMIN \
  -e ROUTED_POOL="$routed_pool" -e CLIENT_IP="$client_ip" \
  golang:1.26.6-alpine sh -c \
  'ip -6 route replace "$ROUTED_POOL" via "$CLIENT_IP" dev eth0'
docker run --rm --network "container:$client_name" --cap-add NET_ADMIN \
  -e ROUTED_POOL="$routed_pool" \
  golang:1.26.6-alpine sh -c \
  'ip -6 route replace local "$ROUTED_POOL" dev lo'

cap_add=$(docker inspect -f '{{json .HostConfig.CapAdd}}' "$client_name")
case "$cap_add" in
  *NET_ADMIN*) echo "test application container unexpectedly has NET_ADMIN" >&2; exit 1 ;;
esac

ready=false
attempt=0
while [ "$attempt" -lt 30 ]; do
  if docker logs "$echo_name" 2>&1 | grep -q 'IPv6 echo ready'; then
    ready=true
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
[ "$ready" = true ] || {
  docker logs "$echo_name" >&2
  exit 1
}

docker exec \
  -e IPV6_EGRESS_ECHO_URL="http://[$echo_ip]:8080" \
  -e IPV6_EGRESS_SOURCE_A="$source_a" \
  -e IPV6_EGRESS_SOURCE_B="$source_b" \
  "$client_name" /usr/local/go/bin/go test -tags integration ./internal/platform/egress \
  -run '^TestDockerIPv6SourceIsolation$' -count=1 -v
