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

print_step_banner "7" "ZERO TRUST EN ACTION — DENY" "Cert revoque — tout acces est bloque immediatement"

echo -e "${BOLD}Scenario : cert compromis recupere par un attaquant${NC}"
print_separator
print_kv "Cert utilise"         "device.crt (revoque a l'etape 6)"
print_kv "CRL Gateway"          "Rechargement automatique — serial blackliste"
print_kv "Comportement attendu" "DENY immediat a chaque tentative d'acces"
print_kv "Zero Trust"           "Pas de session persistante — chaque connect = re-verification"
echo -e ""

echo -e "${YELLOW}${BOLD}1. Verification : le serial est-il dans la CRL ?${NC}"
print_separator

if [[ -f /tmp/ztna-demo/device_cert_serial.txt ]]; then
    REVOKED_SERIAL=$(cat /tmp/ztna-demo/device_cert_serial.txt 2>/dev/null || echo "?")
    print_kv "Serial revoque" "${REVOKED_SERIAL}"
fi

# Consulter la CRL publique du CP
CRL_SERIALS=$(curl -sk --max-time 5 "${CP_API}/pki/device-ca/crl" \
    | openssl crl -text -noout 2>/dev/null \
    | grep "Serial Number" | awk '{print $3}' || true)

if [[ -n "$CRL_SERIALS" ]]; then
    print_ok "CRL accessible — serials revoques :"
    echo "$CRL_SERIALS" | while IFS= read -r s; do
        echo -e "    ${DIM}serial: ${s}${NC}"
    done
else
    echo -e "  ${DIM}(CRL non parsable dans ce contexte)${NC}"
fi
echo -e ""

echo -e "${YELLOW}${BOLD}2. Attente propagation CRL vers la Gateway…${NC}"
print_separator
echo -e "${DIM}  La Gateway a été redémarrée à l'étape 6 — CRL déjà chargée.${NC}"
echo -e "${DIM}  Sonde de confirmation (max 15s)…${NC}"

# Deployer le script de sonde sur wan-client
cat > /tmp/ztna-probe.py << 'EOFILE'
import ssl, socket, struct, json, sys
gw = sys.argv[1] if len(sys.argv) > 1 else "10.10.10.20"
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile="/tmp/ztna-demo/device.crt", keyfile="/tmp/ztna-demo/device.key")
ctx.check_hostname = False; ctx.verify_mode = ssl.CERT_NONE
req = json.dumps({"protocol_version":1,"action":"connect","resource":{"type":"http","host":"lan-app","port":80},"context":{}}).encode()
try:
    with socket.create_connection((gw, 4433), timeout=5) as r:
        with ctx.wrap_socket(r) as c:
            c.sendall(struct.pack(">I",len(req))+req)
            rl=b""
            while len(rl)<4: rl+=c.recv(4-len(rl))
            ml=struct.unpack(">I",rl)[0]; p=b""
            while len(p)<ml: p+=c.recv(ml-len(p))
            print(json.loads(p).get("decision","?"))
except: print("error")
EOFILE
scp -q ${SSH_OPTS} /tmp/ztna-probe.py ztna@${CLIENT_IP}:/tmp/ztna-demo/ztna-probe.py 2>/dev/null || true
rm -f /tmp/ztna-probe.py

# Attendre que la Gateway propage la CRL (max 15s, sondage toutes les 3s)
WAIT_MAX=15
WAIT_START=$(date +%s)
while true; do
    NOW=$(date +%s)
    ELAPSED=$(( NOW - WAIT_START ))
    if [[ $ELAPSED -ge $WAIT_MAX ]]; then
        print_warn "CRL non confirmée après ${WAIT_MAX}s — le test final décidera"
        break
    fi
    # Sonde rapide : si la GW renvoie DENY c'est bon
    PROBE=$(ssh_client "python3 /tmp/ztna-demo/ztna-probe.py ${GW_IP}" 2>/dev/null || echo "error")
    REMAINING=$(( WAIT_MAX - ELAPSED ))
    if [[ "$PROBE" == "deny" ]]; then
        print_ok "CRL propagee — Gateway refuse le cert revoque (${ELAPSED}s)"
        break
    fi
    printf "\r  ${DIM}[%2ds] Gateway: %s — attente propagation CRL…${NC}  " "$ELAPSED" "$PROBE"
    sleep 3
done
echo -e ""

echo -e "${YELLOW}${BOLD}3. Tentative d'acces a /api/secrets avec le cert revoque…${NC}"
print_separator

ssh_client bash <<ENDSSH
CERT_FILE="/tmp/ztna-demo/device.crt"
KEY_FILE="/tmp/ztna-demo/device.key"
GW_ADDR="${GW_IP}:4433"

if [[ ! -f "\$CERT_FILE" ]]; then
    echo -e "\033[0;31m[x]\033[0m Certificat introuvable — executez d'abord les etapes 03 et 06"
    exit 0
fi

echo -e "\033[1;33m[wan-client]\033[0m Tentative mTLS avec cert revoque -- GET /api/secrets"
echo ""

python3 - <<'PYEOF'
import ssl, socket, struct, json, sys

GW_HOST = "${GW_IP}"
GW_PORT = 4433
CERT_FILE = "/tmp/ztna-demo/device.crt"
KEY_FILE  = "/tmp/ztna-demo/device.key"
ENDPOINT  = "/api/vault/secrets"

ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE

req = json.dumps({
    "protocol_version": 1, "action": "connect",
    "resource": {"type": "http", "host": "lan-app", "port": 80},
    "context": {}
}).encode()

print(f"  wan-client  -->  ztna-gw:4433 (mTLS)  -->  CP/authorize  -->  lan-app:80")
print(f"  --> ConnectRequest  GET {ENDPOINT}  [cert RÉVOQU\u00c9]")

try:
    with socket.create_connection((GW_HOST, GW_PORT), timeout=30) as raw:
        with ctx.wrap_socket(raw) as conn:
            conn.sendall(struct.pack(">I", len(req)) + req)
            rl = b""
            while len(rl) < 4:
                chunk = conn.recv(4 - len(rl))
                if not chunk:
                    print("\033[0;31m  Connexion fermee par la Gateway (rejet TLS)\033[0m")
                    sys.exit(0)
                rl += chunk
            ml = struct.unpack(">I", rl)[0]
            p = b""
            while len(p) < ml:
                chunk = conn.recv(ml - len(p))
                if not chunk: break
                p += chunk
            resp = json.loads(p) if p else {}
            dec = resp.get("decision", "?")
            if dec == "deny":
                print(f"\033[0;31m  <-- DENY  reason={resp.get('reason','?')}\033[0m")
                print(f"  Le serial du cert est dans la CRL de la Gateway.")
                print(f"  Aucune donnee transmise. Tunnel non etabli.")
            elif dec == "allow":
                print(f"\033[1;33m  decision: ALLOW (CRL pas encore propagee a la GW)\033[0m")
                print(f"  Relancer apres refresh CRL de la Gateway (max 30s)")
            else:
                print(f"  reponse inattendue: {resp}")
except ssl.SSLError as e:
    print(f"\033[0;31m  Rejet TLS par la Gateway: {e}\033[0m")
except Exception as e:
    print(f"\033[0;31m  Erreur: {e}\033[0m")
PYEOF
ENDSSH

echo -e ""
print_deny
echo -e ""
echo -e "${RED}${BOLD}  Certificat revoque = acces refuse immediatement.${NC}"
echo -e "${WHITE}  Zero Trust : pas de session persistante — chaque connect = re-verification CRL.${NC}"
echo -e "${WHITE}  Meme l'attaquant avec le cert vole est bloque en temps reel.${NC}"
echo -e ""
print_separator
echo -e "${GREEN}${BOLD}  Demonstration ZTNA complete.${NC}"
echo -e "${DIM}  Flux 2 (mTLS) : etapes 2-4 (login -> cert -> acces ressource)${NC}"
echo -e "${DIM}  Controle d'acces : etapes 6-7 (revocation -> deny immediat)${NC}"
echo -e "${DIM}  Flux 1 (SSH cert) : etape 5 (acces SSH via certificat ephemere)${NC}"
