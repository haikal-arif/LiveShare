#!/usr/bin/env bash
# Opens/closes the ufw ports that let other devices on the same LAN use the
# app through nginx:
#
#   TCP 80, 443          nginx (website, /api, /ws signaling)
#   UDP $SFU_UDP_PORTS   SFU WebRTC media (default 50000-50199; must match
#                        the SFU's SFU_UDP_PORTS)
#
# Rules only allow the current LAN subnet and are tagged with a ufw comment,
# so `close` removes exactly what `open` added even if the subnet changed.
#
#   ./firewall.sh open      # allow LAN access
#   ./firewall.sh close     # remove those rules
#   ./firewall.sh status    # show the rules this script manages
set -euo pipefail

TAG="capstone"
UDP_PORTS="${SFU_UDP_PORTS:-50000-50199}"

if [ "$(id -u)" -ne 0 ]; then
    exec sudo SFU_UDP_PORTS="$UDP_PORTS" "$0" "$@"
fi

# Subnet of the interface holding the default route, e.g. 10.20.191.0/24.
lan_subnet() {
    local iface
    iface="$(ip route show default | awk '{print $5; exit}')"
    [ -n "$iface" ] || { echo "No default route; are you connected to a network?" >&2; exit 1; }
    ip -4 route show dev "$iface" proto kernel scope link | awk '{print $1; exit}'
}

# ufw rule numbers tagged with $TAG, highest first so deleting doesn't
# shift the numbers still to be deleted.
tagged_rules() {
    ufw status numbered | grep "# $TAG\$" | sed -E 's/^\[ *([0-9]+)\].*/\1/' | sort -rn
}

case "${1:-}" in
    open)
        subnet="$(lan_subnet)"
        ufw allow from "$subnet" to any port 80,443 proto tcp comment "$TAG"
        ufw allow from "$subnet" to any port "${UDP_PORTS/-/:}" proto udp comment "$TAG"
        echo "Opened for $subnet. Others can now visit https://$(hostname -I | awk '{print $1}')"
        ;;
    close)
        rules="$(tagged_rules)"
        [ -n "$rules" ] || { echo "No $TAG rules to remove."; exit 0; }
        for n in $rules; do
            ufw --force delete "$n"
        done
        echo "Closed."
        ;;
    status)
        ufw status numbered | grep "# $TAG\$" || echo "No $TAG rules (closed)."
        ;;
    *)
        echo "Usage: $0 {open|close|status}" >&2
        exit 1
        ;;
esac
