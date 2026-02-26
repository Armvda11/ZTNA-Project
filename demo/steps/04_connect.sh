#!/usr/bin/env bash
# =============================================================================
# demo/steps/04_connect.sh — Accès à la ressource interne via ZTNA (Flux 2)
#
# Démontre le principe fondamental du Zero Trust :
#   1. Accès DIRECT impossible — lan-app:80 n'est pas routable depuis WAN
#   2. Accès via ZTNA — chaque requête passe par une autorisation mTLS + PEP
#   3. Trois appels API réels (status, assets, secrets) — chacun = 1 cycle ZTNA
#
# ACCÈS MANUEL depuis le terminal wan-client :
#   ztna whoami
#   ztna get /api/status
#   ztna get /api/assets
#   ztna get /api/secrets
# =============================================================================
set -uo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "4" "ACCES A LA RESSOURCE — FLUX ZTNA" "wan-client --> GW (mTLS) --> CP (authorize) --> lan-app"

echo -e "${BOLD}Zero Trust Network Access — Preuve par l'exemple${NC}"
print_separator
print_kv "Protocole"   "mTLS TLS 1.3 — device-cert X.509 (etape 3)"
print_kv "Gateway"     "${GW_IP}:4433"
print_kv "Ressource"   "ACME Corp Internal API  lan-app:80"
print_kv "CP consulte" "POST /api/v1/pep/authorize  (PEP)"
print_kv "Politique"   "group:ztna-admins -> allow"
print_kv "Acces LAN"   "${APP_IP}:80  (non routable depuis WAN)"
echo -e ""

# ─── Phase 1 : prouver que l'accès direct LAN est impossible ─────────────────
echo -e "${BOLD}Phase 1 — Sans ZTNA : tentative d'acces direct${NC}"
print_separator

echo -e "${INFO}[wan-client] Tentative d'acces direct a ${APP_IP}:80 (reseau LAN interne)…${NC}"
ssh_client bash <<'TRY_DIRECT'
if timeout 3 bash -c 'echo >/dev/tcp/10.10.30.10/80' 2>/dev/null; then
    echo -e "\033[0;33m  Acces direct possible (inattendu dans ce lab)\033[0m"
else
    echo -e "\033[0;31m  [BLOQUE] Connexion TCP refusee — 10.10.30.10:80 injoignable depuis WAN\033[0m"
    echo -e "\033[2m  La ressource est isolee sur le reseau LAN (10.10.30.0/24).\033[0m"
    echo -e "\033[2m  Aucune route, aucun acces sans passer par la Gateway ZTNA.\033[0m"
fi
TRY_DIRECT

echo -e ""

# ─── Phase 2 : accès via ZTNA — 3 appels réels avec autorisation à chaque fois ──
echo -e "${BOLD}Phase 2 — Avec ZTNA : acces autorise apres verification d'identite${NC}"
print_separator
echo -e "${DIM}  Chaque appel API = 1 cycle complet : mTLS + identite cert + PEP authorize${NC}"
echo -e ""

ssh_client bash <<ENDSSH
set -e
CERT_FILE="/tmp/ztna-demo/device.crt"
KEY_FILE="/tmp/ztna-demo/device.key"

if [[ ! -f "\$CERT_FILE" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Certificat manquant — executez d'abord l'etape 03"
    exit 1
fi

python3 - <<'PYEOF'
import ssl, socket, struct, json, sys, time

GW_HOST   = "${GW_IP}"
GW_PORT   = 4433
CERT_FILE = "/tmp/ztna-demo/device.crt"
KEY_FILE  = "/tmp/ztna-demo/device.key"
TARGET    = "lan-app"

def new_ctx():
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
    ctx.check_hostname = False
    ctx.verify_mode    = ssl.CERT_NONE
    return ctx

def ztna_get(endpoint):
    """Ouvre une connexion ZTNA (mTLS + autorisation PEP) puis envoie le GET HTTP."""
    req = json.dumps({
        "protocol_version": 1, "action": "connect",
        "resource": {"type": "http", "host": TARGET, "port": 80},
        "context": {"src_ip": "", "device_info": {}}
    }).encode()
    raw = socket.create_connection((GW_HOST, GW_PORT), timeout=30)
    with new_ctx().wrap_socket(raw) as conn:
        # Handshake ZTNA (timeout 30s — CP call inclus)
        conn.sendall(struct.pack(">I", len(req)) + req)
        rl = b""
        while len(rl) < 4: rl += conn.recv(4 - len(rl))
        ml = struct.unpack(">I", rl)[0]
        p = b""
        while len(p) < ml: p += conn.recv(ml - len(p))
        resp = json.loads(p)
        if resp.get("decision") != "allow":
            return None, resp.get("reason", "?"), resp.get("decision_id", "")
        decision_id = resp.get("decision_id", "?")
        # Requete HTTP via tunnel proxifie (HTTP/1.0 ferme la conn apres reponse)
        conn.settimeout(8)   # court timeout : la reponse arrive vite, on n'attend pas close_notify
        conn.sendall(f"GET {endpoint} HTTP/1.0\r\nHost: {TARGET}\r\n\r\n".encode())
        data = b""
        try:
            while True:
                chunk = conn.recv(4096)
                if not chunk: break
                data += chunk
        except Exception:
            pass  # TLS close_notify absent => reponse deja recue dans data
        if not data:
            raise RuntimeError(f"Aucune reponse HTTP de {TARGET}")
        header, body = data.split(b"\r\n\r\n", 1)
        return (header.splitlines()[0].decode(), json.loads(body), decision_id), None, "allow"

CALLS = [
    ("/api/status",  "Etat du service interne"),
    ("/api/assets",  "Inventaire des assets (serveurs, postes, reseau)"),
    ("/api/secrets", "Secrets & credentials  [CONFIDENTIEL]"),
]

print(f"  Certificat : {CERT_FILE}")
ci_info = None
try:
    import subprocess, re
    out = subprocess.check_output(
        ["openssl","x509","-noout","-subject","-serial","-in",CERT_FILE],
        stderr=subprocess.DEVNULL).decode()
    subj   = re.search(r"subject=(.+)", out)
    serial = re.search(r"serial=(.+)", out)
    if subj:   print(f"  Identite   : {subj.group(1).strip()}")
    if serial: print(f"  Serial     : {serial.group(1).strip().lower()}")
except Exception: pass
print()

ok_count = 0
for i, (endpoint, label) in enumerate(CALLS, 1):
    print(f"\033[0;36m  [{i}/{len(CALLS)}] {label}\033[0m")
    print(f"         wan-client  -->  ztna-gw:4433 (mTLS)  -->  CP/authorize  -->  {TARGET}:80")
    result, reason, decision = ztna_get(endpoint)
    if result is None:
        print(f"\033[0;31m         <-- DENY  reason={reason}\033[0m")
        sys.exit(1)
    status_line, body, dec_id = result
    print(f"\033[0;32m         <-- ALLOW  {status_line}  decision={dec_id[:16]}...\033[0m")
    print()

    if endpoint == "/api/status":
        svc = body.get("services", {})
        print(f"         host={body.get('host')}  uptime={body.get('uptime_s')}s")
        print(f"         services: " + "  ".join(f"{k}={v}" for k,v in svc.items()))

    elif endpoint == "/api/assets":
        print(f"         {body.get('count')} assets — reseau LAN prive :")
        for a in body.get("assets", []):
            col = "\033[0;32m" if a.get("status") in ("running","active") else "\033[0;33m"
            print(f"         {col}  {a['id']:8s}  {a['name']:22s}  {a['type']:14s}  {a['status']}\033[0m")

    elif endpoint == "/api/secrets":
        print(f"\033[0;33m         {body.get('notice')}\033[0m")
        print(f"         {body.get('count')} secrets — accessibles uniquement via ZTNA :")
        for s in body.get("secrets", []):
            print(f"           {s['key']:35s}  rotation={s['rotation']}  owner={s['owner']}")

    print()
    ok_count += 1
    if i < len(CALLS):
        time.sleep(0.5)

print(f"\033[0;32m  {ok_count}/{len(CALLS)} appels API autorises et executes avec succes.\033[0m")
print(f"  Chaque appel a traverse le cycle complet : mTLS -> PEP -> proxy -> lan-app")
PYEOF
ENDSSH

echo -e ""
print_allow
echo -e ""
print_ok "Flux ZTNA complet — identite verifiee, ressource interne accessible"
echo -e ""
echo -e "${BOLD}  Acces manuel depuis le terminal wan-client :${NC}"
echo -e "${CYAN}    ztna whoami${NC}"
echo -e "${CYAN}    ztna status${NC}"
echo -e "${CYAN}    ztna get /api/assets${NC}"
echo -e "${CYAN}    ztna get /api/secrets${NC}"

