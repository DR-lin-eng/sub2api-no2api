#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH='' cd -- "$script_dir/../.." && pwd)
suffix=$((($$ % 60000) + 1000))
octet=$(((suffix % 200) + 20))
segment=$(printf '%x' "$suffix")
network_name=sub2api-he-tunnel-test-$suffix
server_name=sub2api-he-server-$suffix
client_name=sub2api-he-client-$suffix
interface=he-test0
server_ipv4=172.31.$octet.10
client_ipv4=172.31.$octet.20
server_ipv6=fd50:$segment::1
client_ipv6=fd50:$segment::2/64
config_file=$(mktemp "${TMPDIR:-/tmp}/sub2api-he-tunnel.XXXXXX")

cleanup() {
  docker rm -f "$server_name" >/dev/null 2>&1 || true
  docker rm -f "$client_name" >/dev/null 2>&1 || true
  docker network rm "$network_name" >/dev/null 2>&1 || true
  rm -f "$config_file"
}
trap cleanup EXIT INT TERM

umask 077
{
  printf '%s\n' "HE_TUNNEL_INTERFACE=$interface"
  printf '%s\n' "HE_TUNNEL_SERVER_IPV4=$server_ipv4"
  printf '%s\n' "HE_TUNNEL_LOCAL_IPV4=$client_ipv4"
  printf '%s\n' "HE_TUNNEL_CLIENT_IPV6=\"$client_ipv6\""
  printf '%s\n' "HE_TUNNEL_SERVER_IPV6=$server_ipv6"
  printf '%s\n' "HE_TUNNEL_PROBE_IPV6=$server_ipv6"
  printf '%s\n' "HE_TUNNEL_ALLOW_PRIVATE_IPV4=true"
  printf '%s\n' "HE_TUNNEL_UPDATE_ENABLED=false"
  printf '%s\n' "POSTGRES_PASSWORD=this-must-not-be-loaded"
} >"$config_file"

docker network create --subnet "172.31.$octet.0/24" "$network_name" >/dev/null
docker run -d --name "$server_name" \
  --network "$network_name" --ip "$server_ipv4" \
  --cap-add NET_ADMIN \
  golang:1.26.6-alpine sleep 300 >/dev/null
docker run -d --name "$client_name" \
  --network "$network_name" --ip "$client_ipv4" \
  -v sub2api-ipv6-go-mod:/go/pkg/mod \
  -v sub2api-ipv6-go-build:/root/.cache/go-build \
  --cap-add NET_ADMIN \
  -v "$repo_dir:/workspace:ro" \
  -v "$config_file:/he.env:ro" \
  golang:1.26.6-alpine sleep 300 >/dev/null

if private_error=$(docker exec \
  -e HE_TUNNEL_INTERFACE="$interface" \
  -e HE_TUNNEL_SERVER_IPV4="$server_ipv4" \
  -e HE_TUNNEL_LOCAL_IPV4="$client_ipv4" \
  -e HE_TUNNEL_CLIENT_IPV6="$client_ipv6" \
  -e HE_TUNNEL_SERVER_IPV6="$server_ipv6" \
  -e HE_TUNNEL_PROBE_IPV6="$server_ipv6" \
  -e HE_TUNNEL_ALLOW_PRIVATE_IPV4=false \
  "$client_name" /workspace/deploy/he-ipv6-tunnel.sh check 2>&1); then
  echo "HE tunnel accepted a private local IPv4 without explicit opt-in" >&2
  exit 1
fi
printf '%s\n' "$private_error" | grep -F "is not public" >/dev/null || {
  echo "HE private IPv4 validation returned an unexpected error: $private_error" >&2
  exit 1
}

docker exec "$server_name" ip tunnel add he-server mode sit \
  remote "$client_ipv4" local "$server_ipv4" ttl 255
docker exec "$server_name" ip link set he-server mtu 1480 up
docker exec "$server_name" ip -6 addr add "$server_ipv6/64" dev he-server

docker exec -e EXPECTED_CLIENT_IPV6="$client_ipv6" "$client_name" sh -eu -c '
  . /workspace/deploy/ipv6-egress-env.sh
  ipv6_egress_load_env_file /he.env
  [ "$HE_TUNNEL_CLIENT_IPV6" = "$EXPECTED_CLIENT_IPV6" ]
  [ "${POSTGRES_PASSWORD+x}" != x ]
'

docker exec "$client_name" /workspace/deploy/he-ipv6-tunnel.sh apply /he.env
docker exec "$client_name" /workspace/deploy/he-ipv6-tunnel.sh apply /he.env
docker exec "$client_name" /workspace/deploy/he-ipv6-tunnel.sh check /he.env
docker exec "$client_name" ping -6 -I "$interface" -c 1 -W 3 "$server_ipv6" >/dev/null
docker exec \
  -e IPV6_EGRESS_EXPECTED_TUNNEL_PREFIX="${server_ipv6%::*}::/64" \
  "$client_name" sh -eu -c 'cd /workspace/backend && go test -tags integration ./internal/platform/egress -run "^TestDockerMarksTunnelPrefixNonUsable$" -count=1 -v'

docker exec "$client_name" /workspace/deploy/he-ipv6-tunnel.sh remove /he.env
if docker exec "$client_name" ip link show "$interface" >/dev/null 2>&1; then
  echo "HE test tunnel still exists after remove" >&2
  exit 1
fi

echo "HE IPv6 tunnel Docker test passed."
