#!/usr/bin/env bash
# Generates the TLS certificate the gateway serves.
#
# Locally: signed by the mkcert CA, so browsers on this machine trust it. Run
# `mkcert -install` once first. Without mkcert (e.g. on EC2) it falls back to a
# self-signed certificate, which browsers warn about; `make tls` replaces it
# with a Let's Encrypt certificate.
#
# The cert covers localhost, this machine's hostname, and every IPv4 address
# it currently has, so phones on the same LAN can open https://<lan-ip>:<port>.
# Extra names/IPs can be passed as arguments:
#
#   ./gen-cert.sh                  # defaults only
#   ./gen-cert.sh 192.168.1.50 capstone.local
#
# Re-run it whenever your LAN IP changes, then restart the gateway.
set -euo pipefail

CERT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/certs"
mkdir -p "$CERT_DIR"

NAMES=("localhost" "$(hostname)" "127.0.0.1")
for ip in $(hostname -I); do
    [[ "$ip" == *:* ]] && continue # skip IPv6
    NAMES+=("$ip")
done
NAMES+=("$@")

if command -v mkcert >/dev/null; then
    mkcert \
        -cert-file "$CERT_DIR/capstone.crt" \
        -key-file "$CERT_DIR/capstone.key" \
        "${NAMES[@]}"
else
    echo "mkcert not found; making a self-signed certificate instead" >&2
    san=""
    for name in "${NAMES[@]}"; do
        if [[ "$name" =~ ^[0-9.]+$ ]]; then kind=IP; else kind=DNS; fi
        san+="${san:+,}$kind:$name"
    done
    openssl req -x509 -newkey rsa:2048 -nodes -days 825 \
        -keyout "$CERT_DIR/capstone.key" -out "$CERT_DIR/capstone.crt" \
        -subj "/CN=${NAMES[0]}" -addext "subjectAltName=$san" 2>/dev/null
fi

# The gateway container reads these as a different user than the host's.
chmod 644 "$CERT_DIR/capstone.key" "$CERT_DIR/capstone.crt"

echo "Wrote $CERT_DIR/capstone.crt and capstone.key"
echo "Names: ${NAMES[*]}"
