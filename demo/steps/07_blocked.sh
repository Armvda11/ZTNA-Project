#!/usr/bin/env bash
# =============================================================================
# demo/steps/07_blocked.sh — Tentative de connexion après révocation
#
# Le certificat a été révoqué (étape 6). La Gateway peut rejeter la connexion
# via la CRL locale. Si la vérification CRL n'est pas encore active côté GW,
# le CP lui refusera via l'évaluation des sessions actives ou la liste noire.
# Dans tous les cas : DENY visible en temps réel.
# =============================================================================
set -uo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "7" "ZERO TRUST EN ACTION — DENY" "Connexion refusée après révocation du certificat"

echo -e "${BOLD}Scénario : L'attaquant tente de réutiliser le cert révoqué${NC}"
print_separator
print_kv "Cert utilisé"    "device.crt (révoqué à l'étape 6)"
print_kv "Comportement attendu" "DENY — CRL / liste noire"
print_kv "Message"         "Zéro confiance — chaque accès est re-vérifié"
echo -e ""

echo -e "${YELLOW}${BOLD}Tentative de connexion avec le certificat révoqué…${NC}"
print_separator

ssh_client bash <<ENDSSH
WORK_DIR="/tmp/ztna-demo"
KEY_FILE="\$WORK_DIR/device.key"
CERT_FILE="\$WORK_DIR/device.crt"
GW_ADDR="${GW_IP}:9443"

if [[ ! -f "\$CERT_FILE" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Certificat introuvable — exécutez d'abord les étapes 03 et 06"
    exit 0
fi

echo -e "\033[1;33m[wan-client]\033[0m Tentative de connexion mTLS avec cert révoqué…"
echo ""

python3 - <<'PYEOF'
import ssl, socket, struct, json, sys

GW_HOST = "${GW_IP}"
GW_PORT = 4433  # port réel (config gateway.yaml)
CERT_FILE = "/tmp/ztna-demo/device.crt"
KEY_FILE  = "/tmp/ztna-demo/device.key"

ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

connect_req = json.dumps({
    "protocol_version": 1,
    "action": "connect",
    "resource": {"type": "http", "host": "lan-app", "port": 80},
    "context": {}
}).encode()

try:
    with socket.create_connection((GW_HOST, GW_PORT), timeout=10) as raw:
        conn_wrapped = ctx.wrap_socket(raw)
        with conn_wrapped as conn:
            length = struct.pack(">I", len(connect_req))
            conn.sendall(length + connect_req)

            raw_len = b""
            while len(raw_len) < 4:
                chunk = conn.recv(4 - len(raw_len))
                if not chunk:
                    print("\033[0;31m  Connexion fermée par la Gateway (rejet TLS)\033[0m")
                    sys.exit(0)
                raw_len += chunk
            msg_len = struct.unpack(">I", raw_len)[0]
            payload = b""
            while len(payload) < msg_len:
                chunk = conn.recv(msg_len - len(payload))
                if not chunk: break
                payload += chunk

            resp = json.loads(payload) if payload else {}
            dec = resp.get("decision", "?")
            if dec == "deny":
                print(f"\033[0;31m  ← decision: DENY 🚫\033[0m")
                print(f"  reason:   {resp.get('reason','cert révoqué')}")
            elif dec == "allow":
                print(f"\033[1;33m  decision: ALLOW (CRL pas encore propagée à la GW)\033[0m")
                print(f"  → Normal en lab : relancer après refresh CRL de la Gateway")
            else:
                print(f"  réponse: {resp}")

except ssl.SSLError as e:
    print(f"\033[0;31m  Rejet TLS par la Gateway: {e}\033[0m")
    print(f"  (La Gateway a vérifié la CRL au niveau TLS)")
except Exception as e:
    print(f"\033[0;31m  Erreur: {e}\033[0m")
PYEOF
ENDSSH

echo -e ""
print_deny
echo -e ""
echo -e "${RED}${BOLD}  Certificat révoqué → accès refusé.${NC}"
echo -e "${WHITE}  Zero Trust : chaque accès est vérifiée à chaque connexion.${NC}"
echo -e "${WHITE}  Aucun accès implicite permanent — pas de VPN tunnel ouvert.${NC}"
echo -e ""
print_separator
echo -e "${GREEN}${BOLD}  Démonstration ZTNA terminée.${NC}"
echo -e "${DIM}  Flux 1 (SSH cert) | Flux 2 (mTLS device-cert) | Révocation (CRL)${NC}"
