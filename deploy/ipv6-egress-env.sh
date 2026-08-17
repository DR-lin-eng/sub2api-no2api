#!/bin/sh

# Parse only the host-network settings needed by the IPv6 setup scripts.
# Values are exported literally; the file is never sourced or evaluated.

ipv6_egress_env_trim() {
  printf '%s' "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

ipv6_egress_env_key_supported() {
  case "$1" in
    IPV6_EGRESS_POOL_CIDR | \
    IPV6_EGRESS_DOCKER_NETWORK | \
    IPV6_EGRESS_CONTAINER_IP | \
    IPV6_EGRESS_CONTAINER_NAME | \
    IPV6_EGRESS_BRIDGE_INTERFACE | \
    HE_TUNNEL_ENABLED | \
    HE_TUNNEL_INTERFACE | \
    HE_TUNNEL_SERVER_IPV4 | \
    HE_TUNNEL_LOCAL_IPV4 | \
    HE_TUNNEL_CLIENT_IPV6 | \
    HE_TUNNEL_SERVER_IPV6 | \
    HE_TUNNEL_MTU | \
    HE_TUNNEL_TTL | \
    HE_TUNNEL_ROUTE_METRIC | \
    HE_TUNNEL_PROBE_IPV6 | \
    HE_TUNNEL_PROBE_TIMEOUT_SECONDS | \
    HE_TUNNEL_ALLOW_PRIVATE_IPV4 | \
    HE_TUNNEL_UPDATE_ENABLED | \
    HE_TUNNEL_ID | \
    HE_TUNNEL_USERNAME | \
    HE_TUNNEL_UPDATE_KEY | \
    HE_TUNNEL_UPDATE_URL | \
    HE_TUNNEL_RECREATE_ON_CHANGE | \
    IPV6_EGRESS_REQUEST_ID | \
    IPV6_EGRESS_REQUEST_ACTION | \
    IPV6_EGRESS_REQUESTED_AT_UNIX)
      return 0
      ;;
  esac
  return 1
}

ipv6_egress_load_env_file() {
  env_file=$1
  [ -f "$env_file" ] || {
    echo "ipv6-egress-env: file not found: $env_file" >&2
    return 1
  }
  [ -r "$env_file" ] || {
    echo "ipv6-egress-env: file is not readable: $env_file" >&2
    return 1
  }

  line_number=0
  while IFS= read -r raw_line || [ -n "$raw_line" ]; do
    line_number=$((line_number + 1))
    line=$(ipv6_egress_env_trim "$raw_line")
    case "$line" in
      '' | \#*) continue ;;
      export[[:space:]]*) line=$(ipv6_egress_env_trim "${line#export}") ;;
    esac
    case "$line" in
      *=*) ;;
      *) continue ;;
    esac

    key=$(ipv6_egress_env_trim "${line%%=*}")
    ipv6_egress_env_key_supported "$key" || continue
    value=$(ipv6_egress_env_trim "${line#*=}")
    case "$value" in
      \"*)
        case "$value" in
          *\") value=${value#\"}; value=${value%\"} ;;
          *)
            echo "ipv6-egress-env: unterminated quote at $env_file:$line_number" >&2
            return 1
            ;;
        esac
        ;;
      \'*)
        case "$value" in
          *\') value=${value#\'}; value=${value%\'} ;;
          *)
            echo "ipv6-egress-env: unterminated quote at $env_file:$line_number" >&2
            return 1
            ;;
        esac
        ;;
    esac
    export "$key=$value"
  done <"$env_file"
}
