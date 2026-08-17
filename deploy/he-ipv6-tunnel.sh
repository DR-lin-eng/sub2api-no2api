#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source-path=SCRIPTDIR
# shellcheck source=ipv6-egress-env.sh
. "$script_dir/ipv6-egress-env.sh"

action=${1:-check}
env_file=${2:-}

fail() {
  echo "he-ipv6-tunnel: $*" >&2
  exit 1
}

usage() {
  fail "usage: $0 [check|apply|remove] [env-file]"
}

[ "$#" -le 2 ] || usage
case "$action" in
  check | apply | remove) ;;
  *) usage ;;
esac
[ -z "$env_file" ] || ipv6_egress_load_env_file "$env_file" || exit 1

is_true() {
  case "$1" in
    1 | true | yes | on) return 0 ;;
  esac
  return 1
}

is_false() {
  case "$1" in
    0 | false | no | off) return 0 ;;
  esac
  return 1
}

require_boolean() {
  is_true "$2" || is_false "$2" || fail "$1 must be true or false"
}

require_uint() {
  case "$2" in
    '' | *[!0-9]*) fail "$1 must be an integer" ;;
  esac
}

valid_ipv4() {
  printf '%s\n' "$1" | awk -F. '
    NF != 4 { exit 1 }
    {
      for (i = 1; i <= 4; i++) {
        if ($i !~ /^[0-9]+$/ || $i < 0 || $i > 255) exit 1
      }
    }
  '
}

non_public_ipv4() {
  printf '%s\n' "$1" | awk -F. '
    {
      a = $1 + 0; b = $2 + 0; c = $3 + 0
      if (a == 0 || a == 10 || a == 127 || a >= 224) exit 0
      if (a == 100 && b >= 64 && b <= 127) exit 0
      if (a == 169 && b == 254) exit 0
      if (a == 172 && b >= 16 && b <= 31) exit 0
      if (a == 192 && b == 168) exit 0
      if (a == 192 && b == 0 && (c == 0 || c == 2)) exit 0
      if (a == 198 && (b == 18 || b == 19)) exit 0
      if (a == 198 && b == 51 && c == 100) exit 0
      if (a == 203 && b == 0 && c == 113) exit 0
      exit 1
    }
  '
}

ensure_ipv6_enabled() {
  key=$1
  current=$(sysctl -n "$key" 2>/dev/null) || fail "kernel setting $key is unavailable"
  if [ "$current" != "0" ]; then
    sysctl -w "$key=0" >/dev/null || fail "could not enable IPv6 through $key"
  fi
}

interface=${HE_TUNNEL_INTERFACE:-he-sub2api}
case "$interface" in
  '' | *[!A-Za-z0-9_.-]*) fail "HE_TUNNEL_INTERFACE contains unsupported characters" ;;
esac
[ "${#interface}" -le 15 ] || fail "HE_TUNNEL_INTERFACE must be at most 15 characters"

command -v ip >/dev/null 2>&1 || fail "iproute2 is required"

if [ "$action" = "remove" ]; then
  [ "$(id -u)" -eq 0 ] || fail "remove must run as root"
  if ip link show "$interface" >/dev/null 2>&1; then
    ip tunnel del "$interface"
    echo "he-ipv6-tunnel: removed $interface"
  else
    echo "he-ipv6-tunnel: $interface is already absent"
  fi
  exit 0
fi

command -v sysctl >/dev/null 2>&1 || fail "sysctl is required"
command -v ping >/dev/null 2>&1 || fail "ping with IPv6 support is required"

server_ipv4=${HE_TUNNEL_SERVER_IPV4:-}
local_ipv4=${HE_TUNNEL_LOCAL_IPV4:-}
client_ipv6=${HE_TUNNEL_CLIENT_IPV6:-}
server_ipv6=${HE_TUNNEL_SERVER_IPV6:-}
mtu=${HE_TUNNEL_MTU:-1480}
ttl=${HE_TUNNEL_TTL:-255}
route_metric=${HE_TUNNEL_ROUTE_METRIC:-2048}
probe_ipv6=${HE_TUNNEL_PROBE_IPV6-2606:4700:4700::1111}
probe_timeout=${HE_TUNNEL_PROBE_TIMEOUT_SECONDS:-5}
allow_private_ipv4=${HE_TUNNEL_ALLOW_PRIVATE_IPV4:-false}
update_enabled=${HE_TUNNEL_UPDATE_ENABLED:-false}
recreate_on_change=${HE_TUNNEL_RECREATE_ON_CHANGE:-false}

[ -n "$server_ipv4" ] || fail "HE_TUNNEL_SERVER_IPV4 is required"
valid_ipv4 "$server_ipv4" || fail "HE_TUNNEL_SERVER_IPV4 must be IPv4"
if [ -z "$local_ipv4" ]; then
  route_output=$(ip -4 route get "$server_ipv4" 2>/dev/null) || \
    fail "cannot find an IPv4 route to the HE server"
  local_ipv4=$(printf '%s\n' "$route_output" | awk '
    { for (i = 1; i <= NF; i++) if ($i == "src" && i < NF) { print $(i + 1); exit } }
  ')
fi
valid_ipv4 "$local_ipv4" || fail "HE_TUNNEL_LOCAL_IPV4 must be IPv4"
ip -o -4 addr show | awk -v expected="$local_ipv4" '
  { split($4, address, "/"); if (address[1] == expected) found = 1 }
  END { exit found ? 0 : 1 }
' || fail "HE_TUNNEL_LOCAL_IPV4 is not assigned to this host"

require_boolean HE_TUNNEL_ALLOW_PRIVATE_IPV4 "$allow_private_ipv4"
if non_public_ipv4 "$local_ipv4" && ! is_true "$allow_private_ipv4"; then
  fail "HE_TUNNEL_LOCAL_IPV4 is not public; protocol 41 needs a directly assigned public IPv4 (set HE_TUNNEL_ALLOW_PRIVATE_IPV4=true only for verified 1:1 NAT)"
fi

case "$client_ipv6" in
  *:*/*) ;;
  *) fail "HE_TUNNEL_CLIENT_IPV6 must include its IPv6 prefix length" ;;
esac
client_prefix=${client_ipv6##*/}
require_uint HE_TUNNEL_CLIENT_IPV6_PREFIX "$client_prefix"
[ "$client_prefix" -ge 1 ] && [ "$client_prefix" -le 128 ] || \
  fail "HE_TUNNEL_CLIENT_IPV6 prefix must be between 1 and 128"
case "$server_ipv6" in
  *:*/*) fail "HE_TUNNEL_SERVER_IPV6 must not include a prefix length" ;;
  *:*) ;;
  *) fail "HE_TUNNEL_SERVER_IPV6 is required" ;;
esac

require_uint HE_TUNNEL_MTU "$mtu"
[ "$mtu" -ge 1280 ] && [ "$mtu" -le 1480 ] || fail "HE_TUNNEL_MTU must be between 1280 and 1480"
require_uint HE_TUNNEL_TTL "$ttl"
[ "$ttl" -ge 1 ] && [ "$ttl" -le 255 ] || fail "HE_TUNNEL_TTL must be between 1 and 255"
require_uint HE_TUNNEL_ROUTE_METRIC "$route_metric"
[ "$route_metric" -ge 1 ] || fail "HE_TUNNEL_ROUTE_METRIC must be positive"
require_uint HE_TUNNEL_PROBE_TIMEOUT_SECONDS "$probe_timeout"
[ "$probe_timeout" -ge 1 ] || fail "HE_TUNNEL_PROBE_TIMEOUT_SECONDS must be positive"
require_boolean HE_TUNNEL_UPDATE_ENABLED "$update_enabled"
require_boolean HE_TUNNEL_RECREATE_ON_CHANGE "$recreate_on_change"

if [ "$action" = "apply" ]; then
  [ "$(id -u)" -eq 0 ] || fail "apply must run as root"
  ensure_ipv6_enabled net.ipv6.conf.all.disable_ipv6
  ensure_ipv6_enabled net.ipv6.conf.default.disable_ipv6

  if is_true "$update_enabled"; then
    tunnel_id=${HE_TUNNEL_ID:-}
    username=${HE_TUNNEL_USERNAME:-}
    update_key=${HE_TUNNEL_UPDATE_KEY:-}
    update_url=${HE_TUNNEL_UPDATE_URL:-https://ipv4.tunnelbroker.net/nic/update}
    [ -n "$tunnel_id" ] || fail "HE_TUNNEL_ID is required when endpoint updates are enabled"
    [ -n "$username" ] || fail "HE_TUNNEL_USERNAME is required when endpoint updates are enabled"
    [ -n "$update_key" ] || fail "HE_TUNNEL_UPDATE_KEY is required when endpoint updates are enabled"
    command -v curl >/dev/null 2>&1 || fail "curl is required when HE endpoint updates are enabled"
    update_response=$(curl -4 --fail --silent --show-error --max-time 15 \
      --user "$username:$update_key" --get --data-urlencode "hostname=$tunnel_id" "$update_url") || \
      fail "HE endpoint update request failed"
    case "$update_response" in
      good* | nochg*) ;;
      *) fail "HE endpoint update failed: $update_response" ;;
    esac
  fi

  create_tunnel=true
  if ip link show "$interface" >/dev/null 2>&1; then
    tunnel_state=$(ip tunnel show "$interface") || fail "cannot inspect existing tunnel $interface"
    if printf '%s\n' "$tunnel_state" | grep -F "remote $server_ipv4" >/dev/null && \
      printf '%s\n' "$tunnel_state" | grep -F "local $local_ipv4" >/dev/null && \
      printf '%s\n' "$tunnel_state" | grep -F "ttl $ttl" >/dev/null; then
      create_tunnel=false
    elif is_true "$recreate_on_change"; then
      ip tunnel del "$interface"
    else
      fail "$interface already exists with different endpoints or TTL; remove it first"
    fi
  fi
  if [ "$create_tunnel" = "true" ]; then
    command -v modprobe >/dev/null 2>&1 && modprobe sit >/dev/null 2>&1 || true
    ip tunnel add "$interface" mode sit remote "$server_ipv4" local "$local_ipv4" ttl "$ttl" || \
      fail "could not create SIT tunnel (verify public IPv4 and protocol 41 support)"
  fi

  ip link set "$interface" mtu "$mtu" up
  ensure_ipv6_enabled "net.ipv6.conf.$interface.disable_ipv6"
  if ! ip -6 addr show dev "$interface" | awk -v expected="$client_ipv6" '
    $1 == "inet6" && $2 == expected { found = 1 }
    END { exit found ? 0 : 1 }
  '; then
    ip -6 addr add "$client_ipv6" dev "$interface" || fail "could not assign HE client IPv6"
  fi
  ip -6 route replace default via "$server_ipv6" dev "$interface" metric "$route_metric" || \
    fail "could not install the HE IPv6 default route"
fi

tunnel_state=$(ip tunnel show "$interface" 2>/dev/null) || fail "tunnel $interface is missing"
printf '%s\n' "$tunnel_state" | grep -F "remote $server_ipv4" >/dev/null || fail "tunnel remote IPv4 is incorrect"
printf '%s\n' "$tunnel_state" | grep -F "local $local_ipv4" >/dev/null || fail "tunnel local IPv4 is incorrect"
ip link show "$interface" | grep -F "UP" >/dev/null || fail "tunnel $interface is down"
ip -6 addr show dev "$interface" | awk -v expected="$client_ipv6" '
  $1 == "inet6" && $2 == expected { found = 1 }
  END { exit found ? 0 : 1 }
' || fail "HE client IPv6 is missing from $interface"
default_route=$(ip -6 route show default dev "$interface")
printf '%s\n' "$default_route" | grep -F "via $server_ipv6" >/dev/null || fail "HE default route is missing"

ping -6 -I "$interface" -c 1 -W "$probe_timeout" "$server_ipv6" >/dev/null 2>&1 || \
  fail "HE server IPv6 is unreachable; verify protocol 41 is permitted"
if [ -n "$probe_ipv6" ] && [ "$probe_ipv6" != "$server_ipv6" ]; then
  ping -6 -I "$interface" -c 1 -W "$probe_timeout" "$probe_ipv6" >/dev/null 2>&1 || \
    fail "external IPv6 probe $probe_ipv6 is unreachable through $interface"
fi

echo "he-ipv6-tunnel: ready ($client_ipv6 via $server_ipv6 on $interface)"
