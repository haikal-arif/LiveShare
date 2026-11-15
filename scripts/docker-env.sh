#!/usr/bin/env bash
# Writes or updates the Docker stack's .env. Called by `make docker-setup`.
#
# Keys already in .env keep their value unless an option changes them. Options
# come in as KEY=VALUE arguments; an empty value means "not given":
#
#   HOST_IP=1.2.3.4          public address clients reach (APP_HOST_IP)
#   EC2=1                    read the public IP from the EC2 instance metadata
#   DOMAIN=name.duckdns.org  hostname for links and TLS (APP_PUBLIC_HOST)
#   HTTPS_PORT=443           gateway HTTPS port on the host
#   HTTP_PORT=80             gateway HTTP redirect port on the host
#   TURN=1|0                 run the TURN relay or not
#
# Derived values (TURN URLs, COMPOSE_PROFILES) are rewritten on every run so
# they always match APP_PUBLIC_HOST and TURN.
set -euo pipefail

ENV_FILE="${ENV_FILE:-.env}"
IMDS=http://169.254.169.254/latest

opt_host_ip="" opt_ec2="" opt_domain="" opt_https="" opt_http="" opt_turn=""
for arg in "$@"; do
    case "$arg" in
        HOST_IP=*) opt_host_ip="${arg#HOST_IP=}" ;;
        EC2=*) opt_ec2="${arg#EC2=}" ;;
        DOMAIN=*) opt_domain="${arg#DOMAIN=}" ;;
        HTTPS_PORT=*) opt_https="${arg#HTTPS_PORT=}" ;;
        HTTP_PORT=*) opt_http="${arg#HTTP_PORT=}" ;;
        TURN=*) opt_turn="${arg#TURN=}" ;;
        *) echo "docker-env: unknown option '$arg'" >&2; exit 1 ;;
    esac
done

has_key() { [ -f "$ENV_FILE" ] && grep -q "^$1=" "$ENV_FILE"; }

get() {
    if has_key "$1"; then grep -m1 "^$1=" "$ENV_FILE" | cut -d= -f2-; fi
}

set_key() {
    touch "$ENV_FILE"
    if has_key "$1"; then
        sed -i "s|^$1=.*|$1=$2|" "$ENV_FILE"
    else
        echo "$1=$2" >> "$ENV_FILE"
    fi
}

# Sets a key only when it is missing, so existing secrets are kept.
ensure() {
    if ! has_key "$1"; then set_key "$1" "$2"; fi
}

ec2_public_ip() {
    local token ip
    token="$(curl -fsS -m 3 -X PUT "$IMDS/api/token" \
        -H 'X-aws-ec2-metadata-token-ttl-seconds: 60')" \
        || { echo "docker-env: EC2 metadata not reachable; pass HOST_IP=" >&2; exit 1; }
    ip="$(curl -fsS -m 3 -H "X-aws-ec2-metadata-token: $token" "$IMDS/meta-data/public-ipv4")" \
        || { echo "docker-env: this instance has no public IPv4; pass HOST_IP=" >&2; exit 1; }
    echo "$ip"
}

default_ip() {
    ip route get 1.1.1.1 | awk '{for (i = 1; i <= NF; i++) if ($i == "src") print $(i + 1)}'
}

ec2=0
if [ -n "$opt_ec2" ] && [ "$opt_ec2" != 0 ]; then ec2=1; fi

# --- address -----------------------------------------------------------------
old_ip="$(get APP_HOST_IP)"
new_ip="$opt_host_ip"
if [ -z "$new_ip" ] && [ "$ec2" = 1 ]; then new_ip="$(ec2_public_ip)"; fi
if [ -z "$new_ip" ] && [ -z "$old_ip" ]; then new_ip="$(default_ip)"; fi
if [ -z "$new_ip" ]; then new_ip="$old_ip"; fi

if [ "$new_ip" != "$old_ip" ]; then
    # The public host follows the IP until a domain is set.
    if [ -z "$(get APP_PUBLIC_HOST)" ] || [ "$(get APP_PUBLIC_HOST)" = "$old_ip" ]; then
        set_key APP_PUBLIC_HOST "$new_ip"
    fi
    set_key APP_HOST_IP "$new_ip"
fi

if [ -n "$opt_domain" ]; then set_key APP_PUBLIC_HOST "$opt_domain"; fi
if [ -n "$opt_https" ]; then set_key APP_HTTPS_PORT "$opt_https"; fi
if [ -n "$opt_http" ]; then set_key APP_HTTP_PORT "$opt_http"; fi

case "$opt_turn" in
    "") ;;
    1|on|yes) set_key APP_TURN_ENABLED 1 ;;
    0|off|no) set_key APP_TURN_ENABLED 0 ;;
    *) echo "docker-env: TURN must be 1 or 0" >&2; exit 1 ;;
esac

# --- defaults for missing keys ----------------------------------------------
ensure APP_HOST_IP "$new_ip"
ensure APP_PUBLIC_HOST "$(get APP_HOST_IP)"
ensure APP_ROOM_PASSCODE changeme
ensure APP_JWT_SECRET "$(openssl rand -hex 32)"
ensure APP_HTTPS_PORT 8443
ensure APP_HTTP_PORT 8088
ensure APP_MAX_PARTICIPANTS 0
ensure APP_TURN_ENABLED 0
ensure APP_TURN_USER capstone
ensure APP_TURN_PASSWORD "$(openssl rand -hex 16)"

# --- derived -----------------------------------------------------------------
host="$(get APP_PUBLIC_HOST)"
if [ "$(get APP_TURN_ENABLED)" = 1 ]; then
    set_key APP_TURN_URLS "turn:$host:3478?transport=udp,turn:$host:3478?transport=tcp,turns:$host:5349?transport=tcp"
    set_key COMPOSE_PROFILES turn
    turn_note="on (3478 udp/tcp, 5349 tls)"
else
    set_key APP_TURN_URLS ""
    set_key COMPOSE_PROFILES ""
    turn_note="off"
fi

echo "docker-env: $ENV_FILE ready: host=$host ip=$(get APP_HOST_IP) https=$(get APP_HTTPS_PORT) turn=$turn_note"
