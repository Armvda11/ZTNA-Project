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
print_kv "Gateway"      "${GW_IP}:4433"
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
GW_ADDR="${GW_IP}:4433"  # port réel de la gateway (config /etc/ztna/gateway.yaml)

if [[ ! -f "\$CERT_FILE" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Certificat manquant — exécutez d'abord l'étape 03"
    exit 1
fi

echo -e "\033[0;36m[wan-client]\033[0m Connexion mTLS vers \$GW_ADDR…"
echo ""

python3 - <<'PYEOF'
import ssl, socket, struct, json, sys

GW_HOST = "${GW_IP}"
GW_PORT = 4433
CERT_FILE = "/tmp/ztna-demo/device.crt"
KEY_FILE  = "/tmp/ztna-demo/device.key"

# Ressource cible : ACME Corp Internal API sur lan-app:80
TARGET_HOST = "lan-app"
TARGET_PORT = 80

ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE   # lab: CA auto-signée

connect_req = json.dumps({
    "protocol_version": 1,
    "action": "connect",
    "resource": {"type": "http", "host": TARGET_HOST, "port": TARGET_PORT},
    "context": {"src_ip": "", "device_info": {}}
}).encode()

try:
    with socket.create_connection((GW_HOST, GW_PORT), timeout=10) as raw:
        with ctx.wrap_socket(raw) as conn:
            ci = conn.cipher()
            print(f"  TLS: {ci[1] if ci else '?'}  cipher={ci[0] if ci else '?'}")

            # --- 1. Envoi du ConnectRequest ---
            conn.sendall(struct.pack(">I", len(connect_req)) + connect_req)
            req_obj = json.loads(connect_req)
            print(f"\n\033[0;36m  --> ConnectRequest:\033[0m")
            print(f"    action:   {req_obj['action']}")
            print(f"    resource: {req_obj['resource']}")

            # --- 2. Lecture ConnectResponse ---
            raw_len = b""
            while len(raw_len) < 4:
                chunk = conn.recv(4 - len(raw_len))
                if not chunk:
                    print("\033[0;31m  Connexion fermee avant reponse\033[0m"); sys.exit(1)
                raw_len += chunk
            msg_len = struct.unpack(">I", raw_len)[0]
            payload = b""
            while len(payload) < msg_len:
                chunk = conn.recv(msg_len - len(payload))
                if not chunk:
                    print("\033[0;31m  Connexion fermee pendant lecture reponse\033[0m"); sys.exit(1)
                payload += chunk

            resp = json.loads(payload)
            print(f"\n\033[0;36m  <-- ConnectResponse:\033[0m")

            if resp.get("decision") != "allow":
                print(f"\033[0;31m    decision: DENY\033[0m")
                print(f"    reason:   {resp.get('reason','?')}")
                sys.exit(1)

            print(f"\033[0;32m    decision:    ALLOW\033[0m")
            print(f"    decision_id: {resp.get('decision_id','?')}")
            print(f"    ttl_seconds: {resp.get('ttl_seconds','?')}")

            # --- 3. La connexion est maintenant un tunnel TCP transparent vers lan-app:80 ---
            print(f"\n\033[0;36m  [ZTNA] Tunnel actif  wan-client --[mTLS]--> ztna-gw --[TCP]--> {TARGET_HOST}:{TARGET_PORT}\033[0m")
            print(f"\033[0;36m  --> HTTP GET /api/assets (via tunnel ZTNA)\033[0m")

            http_req = (
                f"GET /api/assets HTTP/1.0\r\n"
                f"Host: {TARGET_HOST}\r\n"
                f"Accept: application/json\r\n"
                f"\r\n"
            ).encode()
            conn.sendall(http_req)

            # Lire la reponse HTTP complete
            http_resp = b""
            try:
                while True:
                    chunk = conn.recv(4096)
                    if not chunk:
                        break
                    http_resp += chunk
            except Exception:
                pass

            if not http_resp:
                print("\033[0;31m  Aucune reponse HTTP de lan-app\033[0m")
                sys.exit(1)

            # Separer headers et body
            parts = http_resp.split(b"\r\n\r\n", 1)
            status_line = parts[0].splitlines()[0].decode(errors="replace")
            body_raw = parts[1] if len(parts) > 1 else b""

            print(f"\n\033[0;32m  <-- Reponse de {TARGET_HOST} (via tunnel ZTNA):\033[0m")
            print(f"  Status  : \033[1m{status_line}\033[0m")
            print(f"  Serveur : LAN-RESTRICTED — {TARGET_HOST}:80")
            print()

            # Afficher le JSON de facon lisible
            try:
                data = json.loads(body_raw)
                print(f"  \033[1mACME Corp — Inventaire des assets internes\033[0m")
                print(f"  Nombre d'assets : {data.get('count', '?')}")
                print()
                for asset in data.get("assets", []):
                    status_color = "\033[0;32m" if asset.get("status") in ("running","active") else "\033[0;33m"
                    print(f"  [{asset['id']:8s}] {asset['name']:20s}  type={asset['type']:12s}  env={asset['env']:12s}  {status_color}status={asset['status']}\033[0m")
                print()
                print(f"  \033[2mrestricted={data.get('restricted')}  queried_by={data.get('queried_by','?')}\033[0m")
            except Exception:
                print(body_raw.decode(errors="replace")[:800])

except Exception as e:
    print(f"\033[0;31m  Erreur: {e}\033[0m")
    sys.exit(1)
PYEOF
ENDSSH

echo -e ""
print_allow
echo -e ""
print_ok "Flux 2 complet — mTLS → Gateway → CP → proxy TCP → lan-app"
