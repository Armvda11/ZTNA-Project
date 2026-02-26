#!/usr/bin/env bash
# =============================================================================
# demo/steps/04_connect.sh — Connexion mTLS (Flux 2) : client → GW → CP → lan-app
#
# Le client utilise son device-cert pour s'authentifier auprès de la Gateway.
# La Gateway extrait son identité du cert, consulte le CP (PEP/authorize)
# et proxifie le TCP vers lan-app si la décision est "allow".
# =============================================================================
set -uo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "4" "CONNEXION mTLS — FLUX 2" "Client → Gateway (mTLS) → CP ( allow ) → lan-app"

echo -e "${BOLD}Flux Zero Trust — mTLS + Policy Engine${NC}"
print_separator
print_kv "Protocole"    "mTLS TLS 1.3 — cert X.509 de l'étape 3"
print_kv "Gateway"      "${GW_IP}:9443"
print_kv "Ressource"    "http:lan-app:80"
print_kv "CP consulté"  "POST /api/v1/pep/authorize"
print_kv "Décision"     "Politique RBAC — group:ztna-admins → allow"
echo -e ""

echo -e "${INFO}Connexion mTLS via Python (simulation du protocole CONNECT)…${NC}"
print_separator

ssh_client bash <<ENDSSH
set -e
WORK_DIR="/tmp/ztna-demo"
KEY_FILE="\$WORK_DIR/device.key"
CERT_FILE="\$WORK_DIR/device.crt"
GW_ADDR="${GW_IP}:9443"

if [[ ! -f "\$CERT_FILE" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Certificat manquant — exécutez d'abord l'étape 03"
    exit 1
fi

echo -e "\033[0;36m[wan-client]\033[0m Connexion mTLS vers \$GW_ADDR…"
echo ""

python3 - <<'PYEOF'
import ssl, socket, struct, json, sys

GW_HOST = "${GW_IP}"
GW_PORT = 9443
CERT_FILE = "/tmp/ztna-demo/device.crt"
KEY_FILE  = "/tmp/ztna-demo/device.key"

ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE   # lab: CA auto-signée

connect_req = json.dumps({
    "protocol_version": 1,
    "action": "connect",
    "resource": {"type": "http", "host": "lan-app", "port": 80},
    "context": {"src_ip": "", "device_info": {}}
}).encode()

try:
    with socket.create_connection((GW_HOST, GW_PORT), timeout=10) as raw:
        with ctx.wrap_socket(raw) as conn:
            # Afficher les infos TLS
            ci = conn.cipher()
            peer = conn.getpeercert(binary_form=False) or {}
            print(f"  TLS: {ci[1] if ci else '?'}  cipher={ci[0] if ci else '?'}")

            # Envoyer ConnectRequest length-prefixed
            length = struct.pack(">I", len(connect_req))
            conn.sendall(length + connect_req)
            print(f"\n\033[0;36m  → ConnectRequest envoyé:\033[0m")
            req_obj = json.loads(connect_req)
            print(f"    action:   {req_obj['action']}")
            print(f"    resource: {req_obj['resource']}")

            # Lire ConnectResponse
            raw_len = b""
            while len(raw_len) < 4:
                chunk = conn.recv(4 - len(raw_len))
                if not chunk:
                    print("\033[0;31m  Connexion fermée avant réponse\033[0m"); sys.exit(1)
                raw_len += chunk
            msg_len = struct.unpack(">I", raw_len)[0]
            payload = b""
            while len(payload) < msg_len:
                chunk = conn.recv(msg_len - len(payload))
                if not chunk:
                    print("\033[0;31m  Connexion fermée pendant lecture réponse\033[0m"); sys.exit(1)
                payload += chunk

            resp = json.loads(payload)
            print(f"\n\033[0;36m  ← ConnectResponse reçu:\033[0m")
            if resp.get("decision") == "allow":
                print(f"\033[0;32m    decision:    ALLOW ✅\033[0m")
                print(f"    decision_id: {resp.get('decision_id','?')}")
                print(f"    ttl_seconds: {resp.get('ttl_seconds','?')}")
                # Envoyer une requête HTTP basique pour montrer que ça passe
                http_req = b"GET / HTTP/1.0\r\nHost: lan-app\r\n\r\n"
                conn.sendall(http_req)
                http_resp = b""
                try:
                    while True:
                        chunk = conn.recv(4096)
                        if not chunk: break
                        http_resp += chunk
                except: pass
                if http_resp:
                    print(f"\n\033[0;32m  HTTP réponse de lan-app:\033[0m")
                    print(f"  {http_resp.decode(errors='replace').splitlines()[0]}")
            else:
                print(f"\033[0;31m    decision: DENY 🚫\033[0m")
                print(f"    reason:   {resp.get('reason','?')}")
                sys.exit(1)
except Exception as e:
    print(f"\033[0;31m  Erreur: {e}\033[0m")
    sys.exit(1)
PYEOF
ENDSSH

echo -e ""
print_allow
echo -e ""
print_ok "Flux 2 complet — mTLS → Gateway → CP → proxy TCP → lan-app"
