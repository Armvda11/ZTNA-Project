#!/usr/bin/env bash
# =============================================================================
# demo/steps/04_connect.sh — Accès réel à la ressource + session ZTNA live
#
# Phase 1 : accès direct LAN impossible (isolement réseau)
# Phase 2 : worker Python lancé en arrière-plan sur wan-client
#           Il reconnecte automatiquement (HTTP/1.0 → nouvelle connexion ZTNA)
#           La session reste ACTIVE — l'étape 06 la coupera via l'admin API
# =============================================================================
set -uo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "4" "ACCES A LA RESSOURCE — SESSION ZTNA LIVE" \
    "wan-client ──mTLS──> GW (PEP) ──authorize──> CP ──proxy──> lan-app"

echo -e "${BOLD}Zero Trust Network Access — Session persistante${NC}"
print_separator
print_kv "Protocole"   "mTLS TLS 1.3 — device-cert X.509 (étape 3)"
print_kv "Gateway"     "${GW_IP}:4433"
print_kv "Ressource"   "ACME Corp Internal API  lan-app:80"
print_kv "CP"          "POST /api/v1/pep/authorize  (PEP)"
print_kv "Politique"   "group:ztna-admins -> allow"
print_kv "Réseau LAN"  "${APP_IP}:80  (non routable depuis WAN)"
echo -e ""

# ─── Phase 1 : accès direct impossible ───────────────────────────────────────
echo -e "${BOLD}Phase 1 — Sans ZTNA : tentative d'accès direct${NC}"
print_separator

ssh_client bash <<'TRY_DIRECT'
if timeout 3 bash -c 'echo >/dev/tcp/10.10.30.10/80' 2>/dev/null; then
    echo -e "\033[0;33m  Accès direct possible (inattendu dans ce lab)\033[0m"
else
    echo -e "\033[0;31m  [BLOCKE] Connexion TCP refusée — 10.10.30.10:80 injoignable depuis WAN\033[0m"
    echo -e "\033[2m  La ressource est isolée sur le réseau LAN (10.10.30.0/24).\033[0m"
    echo -e "\033[2m  Aucune route, aucun accès sans passer par la Gateway ZTNA.\033[0m"
fi
TRY_DIRECT

echo -e ""

# ─── Phase 2 : session ZTNA persistante en arrière-plan ──────────────────────
echo -e "${BOLD}Phase 2 — Avec ZTNA : worker de session en arrière-plan${NC}"
print_separator
echo -e "${DIM}  Un tunnel mTLS est ouvert vers lan-app via la Gateway à chaque requête.${NC}"
echo -e "${DIM}  Le worker tourne en arrière-plan — l'étape 06 coupera sa session en direct.${NC}"
echo -e ""

# Copier le script Python worker sur wan-client
ssh_client "cat > /tmp/ztna-worker.py" << 'PYEOF'
#!/usr/bin/env python3
"""
ZTNA session worker — tourne en arrière-plan sur wan-client.
- Ouvre un tunnel mTLS vers la Gateway (CONNECT handshake)
- Envoie GET /api/status toutes les 3s, reconnecte sur HTTP/1.0 EOF
- Ecrit dans /tmp/ztna-session.log
- Quitte sur ConnectionResetError/SSLError (tunnel tué par Gateway)
- Timeout max 120s (sécurité auto-stop demo)
"""
import ssl, socket, struct, json, sys, time

GW_HOST   = "10.10.10.20"
GW_PORT   = 4433
TARGET    = "lan-app"
CERT_FILE = "/tmp/ztna-demo/device.crt"
KEY_FILE  = "/tmp/ztna-demo/device.key"
MAX_SEC   = 120
LOG_FILE  = "/tmp/ztna-session.log"

import os
os.makedirs("/tmp/ztna-demo", exist_ok=True)

def log(msg):
    with open(LOG_FILE, "a") as f:
        f.write(msg + "\n")
    print(msg, flush=True)

def new_ctx():
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    ctx.load_cert_chain(certfile=CERT_FILE, keyfile=KEY_FILE)
    ctx.check_hostname = False
    ctx.verify_mode    = ssl.CERT_NONE
    return ctx

def read_msg(conn):
    buf = b""
    while len(buf) < 4:
        chunk = conn.recv(4 - len(buf))
        if not chunk:
            raise RuntimeError("connexion fermée pendant lecture handshake")
        buf += chunk
    size = struct.unpack(">I", buf)[0]
    data = b""
    while len(data) < size:
        chunk = conn.recv(size - len(data))
        if not chunk:
            raise RuntimeError("connexion fermée pendant lecture message")
        data += chunk
    return json.loads(data)

def open_tunnel():
    raw = socket.create_connection((GW_HOST, GW_PORT), timeout=15)
    conn = new_ctx().wrap_socket(raw)
    req_body = json.dumps({
        "protocol_version": 1, "action": "connect",
        "resource": {"type": "http", "host": TARGET, "port": 80},
        "context": {}
    }).encode()
    conn.sendall(struct.pack(">I", len(req_body)) + req_body)
    resp = read_msg(conn)
    if resp.get("decision") != "allow":
        raise PermissionError(f"DENY: {resp.get('reason','?')}")
    return conn, resp

# ── Ouverture initiale ────────────────────────────────────────────────────────
log(f"  Connexion mTLS → {GW_HOST}:{GW_PORT} …")
try:
    conn, resp = open_tunnel()
except PermissionError as e:
    log(f"\033[0;31m  [DENY]\033[0m {e}")
    sys.exit(1)
except Exception as e:
    log(f"\033[0;31m  [ERREUR TUNNEL]\033[0m {e}")
    sys.exit(1)

dec_id  = resp.get("decision_id", "?")
ttl_sec = resp.get("ttl_seconds", 0)
log(f"\033[0;32m  [ALLOW]\033[0m decision_id={dec_id[:32]}  ttl={ttl_sec}s")
log(f"\033[0;32m  Tunnel TCP ouvert :\033[0m {GW_HOST}:{GW_PORT} → {TARGET}:80")
log(f"\033[0;33m  Session active — requêtes toutes les 3s (Ctrl+C ou étape 06 pour couper)…\033[0m")
log("")

http_req = (
    f"GET /api/status HTTP/1.1\r\nHost: {TARGET}\r\n"
    "Connection: keep-alive\r\n\r\n"
).encode()

iteration = 0
start     = time.time()

while time.time() - start < MAX_SEC:
    iteration += 1
    try:
        conn.settimeout(10)
        conn.sendall(http_req)
        raw_resp = b""
        conn.settimeout(8)
        while True:
            chunk = conn.recv(4096)
            if not chunk:
                # EOF — HTTP/1.0 ferme après réponse → reconnecter
                raise EOFError("HTTP/1.0 EOF")
            raw_resp += chunk
            if b"\r\n\r\n" in raw_resp:
                break

        status = raw_resp.split(b"\r\n")[0].decode(errors="replace")
        body_raw = raw_resp.split(b"\r\n\r\n", 1)[1] if b"\r\n\r\n" in raw_resp else b""
        try:
            data     = json.loads(body_raw)
            host_v   = data.get("host", TARGET)
            uptime_v = data.get("uptime_s", "?")
            log(f"  \033[0;32m[{iteration:03d}]\033[0m {status:<25s}  host={host_v}  uptime={uptime_v}s  t={time.strftime('%H:%M:%S')}")
        except Exception:
            log(f"  \033[0;32m[{iteration:03d}]\033[0m {status}  t={time.strftime('%H:%M:%S')}")

    except EOFError:
        # HTTP/1.0 : le serveur ferme après chaque réponse → reconnecter silencieusement
        try:
            conn.close()
        except Exception:
            pass
        try:
            conn, resp = open_tunnel()
        except PermissionError as e:
            log(f"\n\033[0;31m  ══════════════════════════════════════════════════════════\033[0m")
            log(f"\033[0;31m  SESSION COUPEE PAR L'ADMINISTRATEUR  (accès refusé)\033[0m")
            log(f"\033[0;31m  ══════════════════════════════════════════════════════════\033[0m")
            log(f"  Raison : {e}")
            sys.exit(0)
        except Exception as e:
            log(f"  \033[0;33m[reconnect]\033[0m {e}")
            time.sleep(5)
            continue
        time.sleep(3)
        continue

    except (ssl.SSLError, ConnectionResetError, BrokenPipeError, OSError) as e:
        log(f"\n\033[0;31m  ══════════════════════════════════════════════════════════\033[0m")
        log(f"\033[0;31m  SESSION COUPEE PAR L'ADMINISTRATEUR\033[0m")
        log(f"\033[0;31m  ══════════════════════════════════════════════════════════\033[0m")
        log(f"  Raison : {type(e).__name__} — {e}")
        log(f"  Requêtes effectuées avant la coupure : {iteration}")
        sys.exit(0)

    except socket.timeout:
        log(f"  \033[0;33m[{iteration:03d}]\033[0m timeout — reconnexion…")
        try:
            conn.close()
        except Exception:
            pass
        try:
            conn, resp = open_tunnel()
        except Exception as e:
            log(f"  \033[0;31m[ERREUR reconnect]\033[0m {e}")
            time.sleep(5)
        time.sleep(3)
        continue

    except Exception as e:
        log(f"  \033[0;31m[ERREUR]\033[0m {type(e).__name__}: {e}")
        sys.exit(1)

    time.sleep(3)

log(f"\n  [worker] Durée max ({MAX_SEC}s) atteinte — arrêt automatique.")
PYEOF

echo -e "${INFO}Lancement du worker ZTNA en arrière-plan sur wan-client…${NC}"

# Lancer en background (nohup + disown)
ssh_client bash <<'LAUNCH'
rm -f /tmp/ztna-session.log
nohup python3 /tmp/ztna-worker.py > /tmp/ztna-session.log 2>&1 &
echo $! > /tmp/ztna-session.pid
disown
sleep 0.5
echo "[✓] Worker lancé (PID=$(cat /tmp/ztna-session.pid 2>/dev/null || echo ?))"
LAUNCH

# Attendre le premier [ALLOW] avec timeout 10s
echo -e "${INFO}Attente de l'autorisation PEP (max 10s)…${NC}"
ALLOWED=0
for i in $(seq 1 10); do
    FIRST_LINE=$(ssh_client "grep -m1 '\[ALLOW\]\|DENY\|ERREUR' /tmp/ztna-session.log 2>/dev/null" 2>/dev/null || true)
    if [[ -n "$FIRST_LINE" ]]; then
        ALLOWED=1
        break
    fi
    sleep 1
done

echo -e ""
echo -e "${BOLD}  ─── Session ZTNA — log en direct (10s) ───${NC}"
echo -e ""

# Afficher le log en direct pendant 10s (poll toutes les 2s)
PREV_LINES=0
for i in $(seq 1 5); do
    sleep 2
    NEW_LINES=$(ssh_client "wc -l < /tmp/ztna-session.log 2>/dev/null" 2>/dev/null | tr -d ' ' || echo "0")
    if [[ "$NEW_LINES" -gt "$PREV_LINES" ]]; then
        ssh_client "tail -n $((NEW_LINES - PREV_LINES)) /tmp/ztna-session.log 2>/dev/null" 2>/dev/null | sed 's/^/  /' || true
        PREV_LINES="$NEW_LINES"
    fi
done

echo -e ""

if [[ "$ALLOWED" -eq 0 ]]; then
    print_warn "Aucune autorisation détectée — vérifier Gateway/CP"
else
    print_ok "Session ZTNA active en arrière-plan sur wan-client"
fi

echo -e ""
echo -e "${YELLOW}${BOLD}  → Le worker continue de tourner — l'étape 06 le coupera en direct.${NC}"
echo -e "${DIM}  Log visible sur wan-client : tail -f /tmp/ztna-session.log${NC}"
echo -e "${DIM}  Accès manuel : ztna connect http:lan-app:80 --local-port 18080${NC}"
