#!/usr/bin/env bash
# =============================================================================
# demo/steps/03_issuecert.sh — Émission du device-cert X.509 par le CP
#
# Le client génère une paire ECDSA P-256 LOCALEMENT (la clé privée ne
# quitte jamais le client), construit un CSR et l'envoie au CP.
# Le CP signe le CSR avec sa Device CA et retourne le certificat PEM.
# =============================================================================
set -uo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "3" "ÉMISSION DEVICE-CERT" "Le CP signe un certificat X.509 pour ce device"

echo -e "${BOLD}Principe Zero Trust — CSR only${NC}"
print_separator
print_kv "Algorithme clé"   "ECDSA P-256 (généré localement)"
print_kv "Clé privée"       "Reste sur le client — JAMAIS transmise"
print_kv "CSR envoyé"       "POST /api/v1/credentials/device-cert"
print_kv "Auth"             "Bearer JWT (access_token étape 2)"
print_kv "CA signante"      "Device CA du Control Plane"
print_kv "TTL cert"         "7 jours (configurable)"
echo -e ""

echo -e "${INFO}Génération du device-cert sur wan-client…${NC}"
print_separator

ssh_client bash <<'ENDSSH'
set -e
source /tmp/ztna-demo/env.sh 2>/dev/null || true

ACCESS_TOKEN=$(cat /tmp/ztna-demo/access_token.txt 2>/dev/null || true)
if [[ -z "$ACCESS_TOKEN" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Token manquant — exécutez d'abord l'étape 02 (login)"
    exit 1
fi

WORK_DIR="/tmp/ztna-demo"
mkdir -p "$WORK_DIR"
KEY_FILE="$WORK_DIR/device.key"
CSR_FILE="$WORK_DIR/device.csr"
CERT_FILE="$WORK_DIR/device.crt"
CP_API=$(cat /tmp/ztna-demo/cp_api.txt 2>/dev/null || echo "https://10.10.20.30:8080")

echo -e "\033[0;36m[wan-client]\033[0m Génération clé ECDSA P-256…"
openssl ecparam -genkey -name prime256v1 -noout -out "$KEY_FILE"
echo -e "\033[0;32m[✓]\033[0m Clé privée générée (reste sur le client)"

echo -e "\033[0;36m[wan-client]\033[0m Construction du CSR…"
openssl req -new -key "$KEY_FILE" -out "$CSR_FILE" \
    -subj "/CN=ztna-client/O=ZTNA" 2>/dev/null
echo -e "\033[0;32m[✓]\033[0m CSR prêt"
echo ""
echo -e "    \033[2mCSR Subject: $(openssl req -noout -subject -in $CSR_FILE 2>/dev/null)\033[0m"

echo ""
echo -e "\033[0;36m[wan-client]\033[0m Envoi CSR au Control Plane (POST /api/v1/credentials/device-cert)…"
# Le CP attend le champ "csr_pem" (PEM complet avec sauts de ligne)
CSR_PEM=$(cat "$CSR_FILE")
RESPONSE=$(curl -sk --max-time 15 \
    -X POST "${CP_API}/api/v1/credentials/device-cert" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"csr_pem\": $(python3 -c "import json,sys; print(json.dumps(open('$CSR_FILE').read()))") }" \
    2>&1)

# La réponse CP a le champ "certificate_pem" (pas "certificate")
CERT=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('certificate_pem',''))" 2>/dev/null || true)

if [[ -z "$CERT" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Échec — réponse CP:"
    echo "$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESPONSE"
    exit 1
fi

echo "$CERT" > "$CERT_FILE"

EXPIRES_AT=$(echo "$RESPONSE" | python3 -c "import sys,json; v=json.load(sys.stdin).get('expires_at','?'); print(str(v)[:16] if v != '?' else '?')" 2>/dev/null || echo "?")
SERIAL_FROM_RESP=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('serial','?'))" 2>/dev/null || echo "?")
echo "$SERIAL_FROM_RESP" > "$WORK_DIR/device_cert_serial.txt"
SERIAL=$(openssl x509 -noout -serial -in "$CERT_FILE" 2>/dev/null | cut -d= -f2 || echo "?")
SUBJECT=$(openssl x509 -noout -subject -in "$CERT_FILE" 2>/dev/null || echo "?")
ISSUER=$(openssl x509 -noout -issuer  -in "$CERT_FILE" 2>/dev/null || echo "?")

echo ""
echo -e "\033[0;32m[✓]\033[0m Certificat X.509 reçu et sauvegardé!"
echo ""
printf "    \033[0;36m%-20s\033[0m %s\n" "serial:"      "$SERIAL"
printf "    \033[0;36m%-20s\033[0m %s\n" "subject:"     "$SUBJECT"
printf "    \033[0;36m%-20s\033[0m %s\n" "issuer:"      "$ISSUER"
printf "    \033[0;36m%-20s\033[0m %s\n" "expires_at:"  "$EXPIRES_AT"
echo ""
echo -e "    \033[2mCert: $CERT_FILE\033[0m"
echo -e "    \033[2mKey:  $KEY_FILE (jamais transmise)\033[0m"
ENDSSH

echo -e ""
print_ok "Device-cert obtenu — mTLS possible vers la Gateway"

# ─── Déployer le script d'accès manuel sur wan-client ────────────────────────
echo -e "${DIM}  Deploiement de l'outil d'acces manuel sur wan-client…${NC}"

cat > /tmp/ztna-connect.py << 'PYEOF'
import ssl, socket, struct, json, sys

GW_HOST   = open("/tmp/ztna-demo/gw_addr.txt").read().split(":")[0].strip()
GW_PORT   = 4433
CERT_FILE = "/tmp/ztna-demo/device.crt"
KEY_FILE  = "/tmp/ztna-demo/device.key"
TARGET    = "lan-app"
ENDPOINT  = sys.argv[1] if len(sys.argv) > 1 else "/api/status"

ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
ctx.check_hostname = False
ctx.verify_mode    = ssl.CERT_NONE

req = json.dumps({"protocol_version":1,"action":"connect",
    "resource":{"type":"http","host":TARGET,"port":80},
    "context":{"src_ip":"","device_info":{}}}).encode()

try:
    with socket.create_connection((GW_HOST, GW_PORT), timeout=10) as raw:
        with ctx.wrap_socket(raw) as conn:
            conn.sendall(struct.pack(">I", len(req)) + req)
            rl = b""
            while len(rl) < 4: rl += conn.recv(4 - len(rl))
            ml = struct.unpack(">I", rl)[0]
            p = b""
            while len(p) < ml: p += conn.recv(ml - len(p))
            resp = json.loads(p)
            if resp.get("decision") != "allow":
                print(f"DENY  reason={resp.get('reason','?')}")
                sys.exit(1)
            print(f"ALLOW  decision_id={resp.get('decision_id','?')}")
            conn.sendall(f"GET {ENDPOINT} HTTP/1.0\r\nHost: {TARGET}\r\n\r\n".encode())
            data = b""
            while True:
                chunk = conn.recv(4096)
                if not chunk: break
                data += chunk
            header, body = data.split(b"\r\n\r\n", 1)
            print(header.splitlines()[0].decode())
            print(json.dumps(json.loads(body), indent=2, ensure_ascii=False))
except Exception as e:
    print(f"Erreur: {e}"); sys.exit(1)
PYEOF

scp -q ${SSH_OPTS} /tmp/ztna-connect.py ztna@${CLIENT_IP}:/tmp/ztna-demo/ztna-connect.py 2>/dev/null || true
rm -f /tmp/ztna-connect.py
echo -e "${DIM}  Script disponible: python3 /tmp/ztna-demo/ztna-connect.py /api/assets${NC}"
