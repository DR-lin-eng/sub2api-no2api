#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source-path=SCRIPTDIR
# shellcheck source=ipv6-egress-env.sh
. "$script_dir/ipv6-egress-env.sh"

action=${1:-check}
env_file=${2:-}

fail() {
  echo "ipv6-egress-host: $*" >&2
  exit 1
}

[ "$#" -le 2 ] || fail "usage: $0 [check|apply] [env-file]"
[ -z "$env_file" ] || ipv6_egress_load_env_file "$env_file" || exit 1

network_name=${IPV6_EGRESS_DOCKER_NETWORK:-sub2api-ipv6-egress}
pool_cidr=${IPV6_EGRESS_POOL_CIDR:-}
container_ip=${IPV6_EGRESS_CONTAINER_IP:-fd42:5355:4232::10}
container_name=${IPV6_EGRESS_CONTAINER_NAME:-sub2api}

command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v ip >/dev/null 2>&1 || fail "iproute2 is required"
command -v nsenter >/dev/null 2>&1 || fail "util-linux nsenter is required"
command -v sysctl >/dev/null 2>&1 || fail "sysctl is required"
[ -n "$pool_cidr" ] || fail "IPV6_EGRESS_POOL_CIDR is required"
case "$pool_cidr" in
  *:*/[0-9]*) ;;
  *) fail "IPV6_EGRESS_POOL_CIDR must be an IPv6 CIDR" ;;
esac
case "$container_ip" in
  *:*) ;;
  *) fail "IPV6_EGRESS_CONTAINER_IP must be IPv6" ;;
esac

network_id=$(docker network inspect -f '{{.Id}}' "$network_name" 2>/dev/null) || \
  fail "Docker network $network_name does not exist; create the Compose stack first"
bridge_suffix=$(printf '%s' "$network_id" | cut -c1-12)
bridge_name=${IPV6_EGRESS_BRIDGE_INTERFACE:-br-$bridge_suffix}
ip link show "$bridge_name" >/dev/null 2>&1 || fail "Docker bridge $bridge_name was not found"
container_pid=$(docker inspect -f '{{.State.Pid}}' "$container_name" 2>/dev/null) || \
  fail "application container $container_name does not exist"
[ "$container_pid" -gt 0 ] 2>/dev/null || fail "application container $container_name is not running"

if [ "$action" = "apply" ]; then
  [ "$(id -u)" -eq 0 ] || fail "apply must run as root"
  sysctl -w net.ipv6.conf.all.forwarding=1 >/dev/null
  ip -6 route replace "$pool_cidr" via "$container_ip" dev "$bridge_name"
  nsenter -t "$container_pid" -n ip -6 route replace local "$pool_cidr" dev lo
elif [ "$action" != "check" ]; then
  fail "usage: $0 [check|apply] [env-file]"
fi

[ "$(sysctl -n net.ipv6.conf.all.forwarding)" = "1" ] || fail "IPv6 forwarding is disabled"
ip -6 route show "$pool_cidr" | grep -F "via $container_ip" | grep -F "dev $bridge_name" >/dev/null || \
  fail "route $pool_cidr via $container_ip dev $bridge_name is missing"
nsenter -t "$container_pid" -n ip -6 route show table local "$pool_cidr" | grep -F "local $pool_cidr dev lo" >/dev/null || \
  fail "container local route $pool_cidr dev lo is missing"

cap_add=$(docker inspect -f '{{json .HostConfig.CapAdd}}' "$container_name")
case "$cap_add" in
  *NET_ADMIN*) fail "main application container must not have NET_ADMIN" ;;
esac

ip -6 route get 2606:4700:4700::1111 >/dev/null 2>&1 || fail "host has no usable external IPv6 route"
echo "ipv6-egress-host: ready ($pool_cidr via $container_ip on $bridge_name; local in $container_name)"
