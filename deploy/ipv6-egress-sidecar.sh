#!/bin/sh
set -eu

script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
# shellcheck source-path=SCRIPTDIR
# shellcheck source=ipv6-egress-env.sh
. "$script_dir/ipv6-egress-env.sh"

mode=${1:-run}
control_dir=${IPV6_EGRESS_CONTROL_DIR:-/control}
poll_seconds=${IPV6_EGRESS_CONTROL_POLL_SECONDS:-2}
desired_file=$control_dir/desired.env
request_file=$control_dir/request.env
status_file=$control_dir/status.env
applied_file=$control_dir/applied.env

case "$mode" in
  run | once) ;;
  *) echo "ipv6-egress-sidecar: usage: $0 [run|once]" >&2; exit 1 ;;
esac
case "$poll_seconds" in
  '' | *[!0-9]*) echo "ipv6-egress-sidecar: poll interval must be an integer" >&2; exit 1 ;;
esac
[ "$poll_seconds" -ge 1 ] && [ "$poll_seconds" -le 60 ] || {
  echo "ipv6-egress-sidecar: poll interval must be between 1 and 60 seconds" >&2
  exit 1
}

mkdir -p "$control_dir"

status_value() {
  key=$1
  file=$2
  [ -f "$file" ] || return 0
  awk -F= -v expected="$key" '$1 == expected { sub(/^[^=]*=/, ""); print; exit }' "$file"
}

sanitize_status_message() {
  printf '%s' "$1" | tr '\r\n=' '   ' | cut -c1-500
}

status_request_id=$(status_value IPV6_EGRESS_STATUS_REQUEST_ID "$status_file")
status_state=$(status_value IPV6_EGRESS_STATUS_STATE "$status_file")
status_action=$(status_value IPV6_EGRESS_STATUS_ACTION "$status_file")
status_message=$(status_value IPV6_EGRESS_STATUS_MESSAGE "$status_file")
status_updated_at=$(status_value IPV6_EGRESS_STATUS_UPDATED_AT_UNIX "$status_file")
[ -n "$status_state" ] || status_state=idle
[ -n "$status_updated_at" ] || status_updated_at=0

write_status() {
  now=$(date +%s)
  tmp_file=$control_dir/.status.$$
  umask 022
  {
    printf 'IPV6_EGRESS_STATUS_REQUEST_ID=%s\n' "$status_request_id"
    printf 'IPV6_EGRESS_STATUS_STATE=%s\n' "$status_state"
    printf 'IPV6_EGRESS_STATUS_ACTION=%s\n' "$status_action"
    printf 'IPV6_EGRESS_STATUS_MESSAGE=%s\n' "$(sanitize_status_message "$status_message")"
    printf 'IPV6_EGRESS_STATUS_UPDATED_AT_UNIX=%s\n' "$status_updated_at"
    printf 'IPV6_EGRESS_STATUS_HEARTBEAT_UNIX=%s\n' "$now"
  } >"$tmp_file"
  chmod 0644 "$tmp_file"
  mv -f "$tmp_file" "$status_file"
}

valid_pool_cidr() {
  case "$1" in
    *:*/*)
      prefix_length=${1##*/}
      case "$prefix_length" in
        '' | *[!0-9]*) return 1 ;;
      esac
      [ "$prefix_length" -ge 48 ] && [ "$prefix_length" -le 120 ]
      ;;
    *) return 1 ;;
  esac
}

read_applied_pool() {
  status_value IPV6_EGRESS_APPLIED_POOL_CIDR "$applied_file"
}

write_applied_pool() {
  tmp_file=$control_dir/.applied.$$
  umask 077
  printf 'IPV6_EGRESS_APPLIED_POOL_CIDR=%s\n' "$1" >"$tmp_file"
  mv -f "$tmp_file" "$applied_file"
}

remove_local_pool_route() {
  pool=$1
  valid_pool_cidr "$pool" || return 0
  ip -6 route del local "$pool" dev lo >/dev/null 2>&1 || true
}

apply_tunnel() {
  [ -f "$desired_file" ] || {
    echo "HE tunnel configuration has not been saved" >&2
    return 1
  }
  ipv6_egress_load_env_file "$desired_file"
  [ "${HE_TUNNEL_ENABLED:-false}" = "true" ] || {
    echo "HE tunnel configuration is disabled" >&2
    return 1
  }
  valid_pool_cidr "${IPV6_EGRESS_POOL_CIDR:-}" || {
    echo "IPV6_EGRESS_POOL_CIDR must use a /48 to /120 IPv6 prefix" >&2
    return 1
  }
  HE_TUNNEL_RECREATE_ON_CHANGE=true
  export HE_TUNNEL_RECREATE_ON_CHANGE
  "$script_dir/he-ipv6-tunnel.sh" apply
  previous_pool=$(read_applied_pool)
  if [ -n "$previous_pool" ] && [ "$previous_pool" != "$IPV6_EGRESS_POOL_CIDR" ]; then
    remove_local_pool_route "$previous_pool"
  fi
  ip -6 route replace local "$IPV6_EGRESS_POOL_CIDR" dev lo
  write_applied_pool "$IPV6_EGRESS_POOL_CIDR"
  echo "HE tunnel and routed pool are ready"
}

check_tunnel() {
  [ -f "$desired_file" ] || {
    echo "HE tunnel configuration has not been saved" >&2
    return 1
  }
  ipv6_egress_load_env_file "$desired_file"
  valid_pool_cidr "${IPV6_EGRESS_POOL_CIDR:-}" || {
    echo "IPV6_EGRESS_POOL_CIDR must use a /48 to /120 IPv6 prefix" >&2
    return 1
  }
  "$script_dir/he-ipv6-tunnel.sh" check
  ip -6 route show table local "$IPV6_EGRESS_POOL_CIDR" | \
    grep -F "local $IPV6_EGRESS_POOL_CIDR dev lo" >/dev/null || {
      echo "local routed-pool route is missing" >&2
      return 1
    }
  echo "HE tunnel and routed pool check passed"
}

remove_tunnel() {
  if [ -f "$desired_file" ]; then
    ipv6_egress_load_env_file "$desired_file"
    remove_local_pool_route "${IPV6_EGRESS_POOL_CIDR:-}"
  fi
  previous_pool=$(read_applied_pool)
  remove_local_pool_route "$previous_pool"
  "$script_dir/he-ipv6-tunnel.sh" remove
  rm -f "$applied_file"
  echo "HE tunnel and routed pool were removed"
}

process_request() {
  [ -f "$request_file" ] || return 0
  IPV6_EGRESS_REQUEST_ID=
  IPV6_EGRESS_REQUEST_ACTION=
  export IPV6_EGRESS_REQUEST_ID IPV6_EGRESS_REQUEST_ACTION
  ipv6_egress_load_env_file "$request_file"
  request_id=${IPV6_EGRESS_REQUEST_ID:-}
  request_action=${IPV6_EGRESS_REQUEST_ACTION:-}
  case "$request_id" in
    ????????????????????????????????) ;;
    *) return 0 ;;
  esac
  case "$request_id" in
    *[!0-9a-f]*) return 0 ;;
  esac
  [ "$request_id" != "$status_request_id" ] || return 0
  case "$request_action" in
    apply | check | remove) ;;
    *) return 0 ;;
  esac

  status_request_id=$request_id
  status_action=$request_action
  status_state=applying
  status_message=
  status_updated_at=$(date +%s)
  write_status

  output_file=$control_dir/.action.$$
  if "$request_action"_tunnel >"$output_file" 2>&1; then
    status_state=succeeded
  else
    status_state=failed
  fi
  status_message=$(tail -n 1 "$output_file" 2>/dev/null || true)
  rm -f "$output_file"
  status_updated_at=$(date +%s)
  write_status
}

while :; do
  process_request
  write_status
  [ "$mode" = "run" ] || exit 0
  sleep "$poll_seconds"
done
