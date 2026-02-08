#!/usr/bin/env bash
set -euo pipefail

CERT_DIR="/etc/ztna/tls"
HOST_IP="${1:-10.10.20.30}"
HOST_DNS="${2:-ztna-cp.local}"

sudo mkdir -p "$CERT_DIR"

cfg_file="$(mktemp)"
cat >"$cfg_file" <<EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
distinguished_name = dn
req_extensions = req_ext

[dn]
CN = ztna-control-plane

[req_ext]
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = ${HOST_DNS}
IP.1 = 127.0.0.1
IP.2 = ${HOST_IP}
EOF

sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout "$CERT_DIR/server.key" \
  -out "$CERT_DIR/server.crt" \
  -config "$cfg_file"

sudo chmod 600 "$CERT_DIR/server.key"
rm -f "$cfg_file"

echo "Generated: $CERT_DIR/server.crt and $CERT_DIR/server.key"