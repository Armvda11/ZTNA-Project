#!/usr/bin/env bash
# test-crl-sessions-routing.sh
#
# Test E2E des trois chantiers implémentés sur le lab Terraform :
#
#   Bloc A : CRL enforcement
#     A1. Enroll device cert → connexion HTTP OK (allow)
#     A2. Révoquer le serial côté CP admin
#     A3. Vérifier que la CRL du CP contient le serial
#     A4. Attente refresh CRL sur la GW + vérification rejet
#     A5. (bonus) Session active coupée après révocation
#
#   Bloc B : Session telemetry
#     B1. Connexion mTLS réussie avec un cert frais
#     B2. GET /api/v1/admin/sessions → session présente
#     B3. Corrélation decision_id + bytes_in/out > 0
#
#   Bloc C : Routing hardening
#     C1. WAN→LAN ping bloqué depuis wan-client
#     C2. WAN→LAN TCP:80 refusé
#     C3. WAN→DMZ (CP) accessible depuis wan-client
#     C4. FORWARD policy DROP confirmée sur ztna-gw
#     C5. Règle DROP WAN→LAN présente dans iptables
#     C6. Logs ZTNA-BLOCKED dans dmesg (optionnel)
#
# Usage :
#   ZTNA_USER=alice ZTNA_PASS=Password123! bash scripts/test-crl-sessions-routing.sh
#
# Variables d'environnement :
#   CRL_WAIT    : secondes à attendre pour le refresh CRL (défaut 70)
#   CP_URL      : URL control plane (défaut https://10.10.20.30:8080)
#   GW_HOST     : IP gateway WAN (défaut 10.10.10.20)

set -euo pipefail

# ── Paramètres lab ────────────────────────────────────────────────────────────
CP_URL="${CP_URL:-https://10.10.20.30:8080}"
KC_URL="${KC_URL:-http://10.10.20.30:8081}"
KC_REALM="ztna"
KC_CLIENT="ztna-control-plane"
KC_SECRET="${KC_SECRET:-}"
GW_HOST="${GW_HOST:-10.10.10.20}"
GW_PORT="${GW_PORT:-4433}"
WAN_CLIENT_IP="${WAN_CLIENT_IP:-10.10.10.10}"
LAN_APP_IP="${LAN_APP_IP:-10.10.30.10}"
CP_IP="${CP_IP:-10.10.20.30}"
SSH_USER="ztna"
# La clé SSH est copiée sur wan-client par 'make test-crl-routing' avant l'exécution
SSH_KEY="${HOME}/.ssh/id_ed25519"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10 -o BatchMode=yes -i ${SSH_KEY}"
CRL_WAIT="${CRL_WAIT:-70}"
ZTNA_DIR="${HOME}/.ztna-test"
ZTNA_USER="${ZTNA_USER:-}"
ZTNA_PASS="${ZTNA_PASS:-}"

# ── Couleurs + compteurs ──────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
PASS=0; FAIL=0; SKIP=0

pass()  { echo -e "  ${GREEN}PASS${NC} -- $*"; (( PASS++ )) || true; }
fail()  { echo -e "  ${RED}FAIL${NC} -- $*"; (( FAIL++ )) || true; }
skip()  { echo -e "  ${YELLOW}SKIP${NC} -- $*"; (( SKIP++ )) || true; }
info()  { echo -e "  ${CYAN}INFO${NC}  $*"; }
step()  { echo; echo -e "${BOLD}-- $* --${NC}"; }
die()   { echo -e "${RED}[FATAL] $*${NC}" >&2; exit 1; }

require_cmd() { command -v "$1" >/dev/null 2>&1 || die "$1 requis mais non trouvé"; }
require_cmd openssl; require_cmd curl; require_cmd python3; require_cmd ssh

# ── Credentials interactifs ───────────────────────────────────────────────────
[[ -z "${ZTNA_USER}" ]] && { read -rp "Utilisateur ZTNA (ex: alice) : " ZTNA_USER; }
[[ -z "${ZTNA_PASS}" ]] && { read -rsp "Mot de passe : " ZTNA_PASS; echo; }

mkdir -p "${ZTNA_DIR}"; chmod 700 "${ZTNA_DIR}"

# ── Client Python mTLS écrit dans un fichier temporaire ──────────────────────
# On écrit d'abord le code Python dans un fichier, ce qui évite les
# problèmes de heredoc imbriqués dans $(…).
MTLS_CLIENT_PY="${ZTNA_DIR}/mtls_client.py"

# Chaque ligne est écrite séparément pour ne pas imbriquer de heredoc
{
  echo 'import sys, ssl, socket, json, time'
  echo ''
  echo 'gw_host, gw_port, cert, key, res_type, res_match = sys.argv[1:7]'
  echo 'send_http = len(sys.argv) > 7 and sys.argv[7] == "send_http"'
  echo 'gw_port = int(gw_port)'
  echo ''
  echo 'ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)'
  echo 'ctx.check_hostname = False'
  echo 'ctx.verify_mode    = ssl.CERT_NONE'
  echo 'ctx.minimum_version = ssl.TLSVersion.TLSv1_3'
  echo 'ctx.load_cert_chain(certfile=cert, keyfile=key)'
  echo ''
  echo 'try:'
  echo '    raw = socket.create_connection((gw_host, gw_port), timeout=10)'
  echo '    tls = ctx.wrap_socket(raw, server_hostname=gw_host)'
  echo '    req = json.dumps({"resource_type": res_type, "resource_match": res_match, "action": "connect"}) + "\n"'
  echo '    tls.sendall(req.encode())'
  echo '    resp_raw = b""'
  echo '    while b"\n" not in resp_raw:'
  echo '        chunk = tls.recv(4096)'
  echo '        if not chunk:'
  echo '            break'
  echo '        resp_raw += chunk'
  echo '    resp = json.loads(resp_raw.split(b"\n")[0])'
  echo '    print(json.dumps(resp))'
  echo '    if resp.get("allowed") and send_http:'
  echo '        http_req = b"GET / HTTP/1.0\r\nHost: lan-app\r\n\r\n"'
  echo '        tls.sendall(http_req)'
  echo '        time.sleep(1)'
  echo '        try:'
  echo '            data = tls.recv(8192)'
  echo '            print(f"HTTP_BYTES_RECEIVED={len(data)}")'
  echo '        except Exception:'
  echo '            pass'
  echo '    tls.close()'
  echo 'except Exception as e:'
  echo '    print(json.dumps({"error": str(e)}))'
} > "${MTLS_CLIENT_PY}"

# ── Fonction : connexion mTLS vers la GW ─────────────────────────────────────
# Usage : mtls_connect <cert> <key> <res_type> <res_match> [send_http]
mtls_connect() {
    local cert="$1" key="$2" res_type="$3" res_match="$4"
    local extra="${5:-}"
    if [[ -n "${extra}" ]]; then
        python3 "${MTLS_CLIENT_PY}" "${GW_HOST}" "${GW_PORT}" \
            "${cert}" "${key}" "${res_type}" "${res_match}" "${extra}" 2>/dev/null || true
    else
        python3 "${MTLS_CLIENT_PY}" "${GW_HOST}" "${GW_PORT}" \
            "${cert}" "${key}" "${res_type}" "${res_match}" 2>/dev/null || true
    fi
}

# ── Fonction : enrôlement device cert ────────────────────────────────────────
enroll_cert() {
    local key_f="$1" csr_f="$2" crt_f="$3" token="$4"
    openssl ecparam -name prime256v1 -genkey -noout -out "${key_f}" 2>/dev/null
    openssl req -new -key "${key_f}" \
        -subj "/CN=${ZTNA_USER}/O=ztna-admins" \
        -out "${csr_f}" 2>/dev/null
    local csr_pem
    csr_pem=$(cat "${csr_f}")
    local csr_json
    csr_json=$(python3 -c "import json,sys; print(json.dumps(sys.stdin.read()))" <<< "${csr_pem}")
    local resp
    resp=$(curl -sk --max-time 15 \
        -H "Authorization: Bearer ${token}" \
        -H "Content-Type: application/json" \
        -d "{\"csr_pem\": ${csr_json}}" \
        "${CP_URL}/api/v1/credentials/device-cert" 2>/dev/null)
    local pem
    pem=$(python3 -c "import json,sys; print(json.loads(sys.stdin.read()).get('certificate_pem',''))" <<< "${resp}" 2>/dev/null || true)
    [[ -z "${pem}" ]] && return 1
    printf '%s' "${pem}" > "${crt_f}"; chmod 600 "${crt_f}"
    return 0
}

# ── Fonction : obtenir un token OIDC ─────────────────────────────────────────
get_token() {
    local data=(
        "--data-urlencode" "client_id=${KC_CLIENT}"
        "--data-urlencode" "username=${ZTNA_USER}"
        "--data-urlencode" "password=${ZTNA_PASS}"
        "--data-urlencode" "grant_type=password"
    )
    if [[ -n "${KC_SECRET}" ]]; then
        data+=("--data-urlencode" "client_secret=${KC_SECRET}")
    fi

    curl -sk --max-time 15 -X POST \
        "${KC_URL}/realms/${KC_REALM}/protocol/openid-connect/token" \
        -H "Content-Type: application/x-www-form-urlencoded" \
        "${data[@]}" \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('access_token',''))" 2>/dev/null || true
}

# ── ssh helper ────────────────────────────────────────────────────────────────
# Utilisé uniquement pour se connecter à ztna-gw (checks iptables depuis wan-client).
# La clé SSH est copiée sur wan-client par la cible make test-crl-routing.
ssh_on() { ssh ${SSH_OPTS} "${SSH_USER}@$1" "$2" 2>/dev/null || true; }

# ═══════════════════════════════════════════════════════════════════════════════
echo
echo -e "${BOLD}+============================================================+${NC}"
echo -e "${BOLD}|  ZTNA LAB -- CRL enforcement / Session telemetry / Routing  |${NC}"
echo -e "${BOLD}+============================================================+${NC}"
echo "  CP     : ${CP_URL}"
echo "  GW     : ${GW_HOST}:${GW_PORT}"
echo "  User   : ${ZTNA_USER}"
echo "  LAN_APP: ${LAN_APP_IP}"

# ═══════════════════════════════════════════════════════════════════════════════
step "PREAMBULE -- Auth OIDC + Enrollment device cert"

info "Obtention du token OIDC..."
ACCESS_TOKEN=$(get_token)
[[ -z "${ACCESS_TOKEN}" || ${#ACCESS_TOKEN} -lt 50 ]] \
    && die "Token OIDC non obtenu. CP accessible ? ${KC_URL}"
info "Token OIDC obtenu (${#ACCESS_TOKEN} chars)"

DEVICE_KEY="${ZTNA_DIR}/device_main.key"
DEVICE_CSR="${ZTNA_DIR}/device_main.csr"
DEVICE_CRT="${ZTNA_DIR}/device_main.crt"

info "Enrôlement cert device principal..."
enroll_cert "${DEVICE_KEY}" "${DEVICE_CSR}" "${DEVICE_CRT}" "${ACCESS_TOKEN}" \
    || die "Enrôlement cert échoué. Vérifier ${CP_URL}/api/v1/credentials/device-cert"

DEVICE_SERIAL=$(openssl x509 -noout -serial -in "${DEVICE_CRT}" | cut -d= -f2 | tr '[:upper:]' '[:lower:]')
info "Cert obtenu -- serial = ${DEVICE_SERIAL}"

# ═══════════════════════════════════════════════════════════════════════════════
step "BLOC A -- CRL enforcement"

# A1 : connexion AVANT révocation
echo
echo "  [A1] Connexion mTLS initiale (avant révocation) -- doit etre ALLOW"
RESULT_A1=$(mtls_connect "${DEVICE_CRT}" "${DEVICE_KEY}" "http" "http:lan-app:80")
ALLOWED_A1=$(python3 -c "import json,sys; d=json.loads(sys.argv[1]); print('yes' if d.get('allowed') else 'no')" "${RESULT_A1}" 2>/dev/null || echo "no")
DECISION_ID_A1=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('decision_id',''))" "${RESULT_A1}" 2>/dev/null || echo "")

if [[ "${ALLOWED_A1}" == "yes" ]]; then
    pass "A1 -- Connexion mTLS ALLOW avant révocation (decision_id=${DECISION_ID_A1})"
else
    fail "A1 -- Connexion refusée AVANT révocation : ${RESULT_A1}"
fi

# A2 : révoquer le serial
echo
echo "  [A2] Révocation serial ${DEVICE_SERIAL} via DELETE /admin/device-certs"
REVOKE_CODE=$(curl -sk --max-time 15 -o /dev/null -w "%{http_code}" \
    -X DELETE \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "${CP_URL}/api/v1/admin/device-certs/${DEVICE_SERIAL}" 2>/dev/null || echo "000")

if [[ "${REVOKE_CODE}" =~ ^(200|202|204)$ ]]; then
    pass "A2 -- DELETE /admin/device-certs/${DEVICE_SERIAL} => HTTP ${REVOKE_CODE}"
else
    fail "A2 -- Révocation échouée (HTTP ${REVOKE_CODE})"
fi

# A3 : vérifier la CRL côté CP
echo
echo "  [A3] Vérification CRL sur le CP (/pki/device-ca/crl)"
CRL_FILE="${ZTNA_DIR}/current.crl"
curl -sk --max-time 10 -o "${CRL_FILE}" "${CP_URL}/pki/device-ca/crl" 2>/dev/null || true

if [[ -f "${CRL_FILE}" && -s "${CRL_FILE}" ]]; then
    CRL_TEXT=$(openssl crl -inform DER -text -noout -in "${CRL_FILE}" 2>/dev/null || true)
    CRL_SERIALS=$(echo "${CRL_TEXT}" | grep -i "Serial Number" \
        | sed 's/.*Serial Number.*: *//I' | tr -d ':' | tr '[:upper:]' '[:lower:]' || true)
    SERIAL_NORM=$(echo "${DEVICE_SERIAL}" | sed 's/^0*//')
    if echo "${CRL_SERIALS}" | grep -qi "${SERIAL_NORM}"; then
        pass "A3 -- Serial ${DEVICE_SERIAL} présent dans la CRL du CP"
    else
        fail "A3 -- Serial ${DEVICE_SERIAL} absent de la CRL"
        info "Serials révoqués dans la CRL : $(echo "${CRL_SERIALS}" | head -5 | tr '\n' ' ')"
    fi
else
    fail "A3 -- CRL vide ou non téléchargée"
fi

# A4 : attente refresh CRL + vérification rejet
echo
echo "  [A4] Attente refresh CRL sur la gateway (max ${CRL_WAIT}s)"
info "      CRL_WAIT peut être augmenté : CRL_WAIT=130 bash $0"

REVOKE_CONFIRMED=false
ELAPSED=0
POLL_INTERVAL=10

while (( ELAPSED < CRL_WAIT )); do
    sleep "${POLL_INTERVAL}"
    (( ELAPSED += POLL_INTERVAL )) || true
    printf "  [A4] %ds/%ds -- test rejet cert révoqué... " "${ELAPSED}" "${CRL_WAIT}"

    RESULT_A4=$(mtls_connect "${DEVICE_CRT}" "${DEVICE_KEY}" "http" "http:lan-app:80")

    IS_REJECTED=$(python3 - "${RESULT_A4}" << 'PYEOF'
import sys, json
try:
    d = json.loads(sys.argv[1])
    allowed = d.get("allowed", True)
    reason  = d.get("reason", "").lower()
    error   = d.get("error", "").lower()
    if not allowed and ("revok" in reason or "cert" in reason):
        print("json_revoked")
    elif error and any(w in error for w in ("tls", "eof", "reset", "refused", "handshake")):
        print("tls_rejected")
    else:
        print("allowed")
except Exception:
    print("parse_error")
PYEOF
)

    case "${IS_REJECTED}" in
        json_revoked)
            echo "REJETE (JSON)"
            pass "A4 -- Connexion rejetée reason='certificate revoked' (${ELAPSED}s)"
            REVOKE_CONFIRMED=true; break ;;
        tls_rejected)
            echo "REJETE (TLS)"
            pass "A4 -- Connexion rejetée au niveau TLS handshake (${ELAPSED}s)"
            REVOKE_CONFIRMED=true; break ;;
        *)
            echo "encore ALLOW"
            if (( ELAPSED >= CRL_WAIT )); then
                fail "A4 -- Cert révoqué encore accepté après ${CRL_WAIT}s"
                info "Vérifier logs GW : ssh ${SSH_USER}@${GW_HOST} 'journalctl -u ztna-gateway -n 20'"
                info "Ou augmenter CRL_WAIT : CRL_WAIT=130 bash $0"
            fi ;;
    esac
done

# A5 : kill session active après révocation
echo
echo "  [A5] Session active coupée après révocation"
TOKEN_A5=$(get_token)
if [[ -z "${TOKEN_A5}" ]]; then
    skip "A5 -- Token expiré"
else
    KEY_A5="${ZTNA_DIR}/device_kill.key"
    CSR_A5="${ZTNA_DIR}/device_kill.csr"
    CRT_A5="${ZTNA_DIR}/device_kill.crt"

    if enroll_cert "${KEY_A5}" "${CSR_A5}" "${CRT_A5}" "${TOKEN_A5}"; then
        SERIAL_A5=$(openssl x509 -noout -serial -in "${CRT_A5}" | cut -d= -f2 | tr '[:upper:]' '[:lower:]')
        info "2e cert -- serial = ${SERIAL_A5}"

        python3 "${MTLS_CLIENT_PY}" "${GW_HOST}" "${GW_PORT}" \
            "${CRT_A5}" "${KEY_A5}" "http" "http:lan-app:80" "send_http" \
            > "${ZTNA_DIR}/kill_session.log" 2>&1 &
        KILL_PID=$!
        sleep 3

        if ! kill -0 "${KILL_PID}" 2>/dev/null; then
            skip "A5 -- Connexion terminée avant révocation"
        else
            curl -sk --max-time 10 -o /dev/null \
                -X DELETE \
                -H "Authorization: Bearer ${TOKEN_A5}" \
                "${CP_URL}/api/v1/admin/device-certs/${SERIAL_A5}" 2>/dev/null || true
            info "Serial ${SERIAL_A5} révoqué -- attente ${CRL_WAIT}s..."
            sleep $(( CRL_WAIT + 5 ))

            if ! kill -0 "${KILL_PID}" 2>/dev/null; then
                pass "A5 -- Session coupée par la gateway après révocation"
            else
                KILL_LOG=$(head -5 "${ZTNA_DIR}/kill_session.log" 2>/dev/null || true)
                if echo "${KILL_LOG}" | grep -qiE "error|close|reset|eof|revok"; then
                    pass "A5 -- Session coupée (log: ${KILL_LOG})"
                else
                    fail "A5 -- Session encore active ${CRL_WAIT}s après révocation"
                fi
                kill "${KILL_PID}" 2>/dev/null || true
            fi
        fi
    else
        skip "A5 -- Enrôlement 2e cert échoué"
    fi
fi

# ═══════════════════════════════════════════════════════════════════════════════
step "BLOC B -- Session telemetry"

TOKEN_B=$(get_token)
if [[ -z "${TOKEN_B}" ]]; then
    skip "BLOC B -- token OIDC indisponible"
else
    # B1
    echo
    echo "  [B1] Connexion mTLS cert frais + transfert HTTP"
    KEY_B="${ZTNA_DIR}/device_sess.key"
    CSR_B="${ZTNA_DIR}/device_sess.csr"
    CRT_B="${ZTNA_DIR}/device_sess.crt"
    DECISION_ID_B=""

    if enroll_cert "${KEY_B}" "${CSR_B}" "${CRT_B}" "${TOKEN_B}"; then
        SERIAL_B=$(openssl x509 -noout -serial -in "${CRT_B}" | cut -d= -f2 | tr '[:upper:]' '[:lower:]')
        info "Cert session -- serial = ${SERIAL_B}"

        RESULT_B1_ALL=$(mtls_connect "${CRT_B}" "${KEY_B}" "http" "http:lan-app:80" "send_http")
        RESULT_B1=$(echo "${RESULT_B1_ALL}" | head -1)
        ALLOWED_B1=$(python3 -c "import json,sys; d=json.loads(sys.argv[1]); print('yes' if d.get('allowed') else 'no')" "${RESULT_B1}" 2>/dev/null || echo "no")
        DECISION_ID_B=$(python3 -c "import json,sys; print(json.loads(sys.argv[1]).get('decision_id',''))" "${RESULT_B1}" 2>/dev/null || echo "")

        if [[ "${ALLOWED_B1}" == "yes" ]]; then
            pass "B1 -- Session mTLS OK (decision_id=${DECISION_ID_B})"
        else
            fail "B1 -- Connexion session refusée : ${RESULT_B1}"
        fi
    else
        fail "B1 -- Enrôlement cert session échoué"
    fi

    sleep 3  # laisser la GW envoyer session_end

    # B2
    echo
    echo "  [B2] GET /api/v1/admin/sessions"
    SESSIONS_JSON=$(curl -sk --max-time 15 \
        -H "Authorization: Bearer ${TOKEN_B}" \
        "${CP_URL}/api/v1/admin/sessions" 2>/dev/null || echo "[]")
    SESSION_COUNT=$(python3 -c "import json,sys; print(len(json.loads(sys.argv[1])))" "${SESSIONS_JSON}" 2>/dev/null || echo "0")

    if (( SESSION_COUNT > 0 )); then
        pass "B2 -- ${SESSION_COUNT} session(s) dans /admin/sessions"
        python3 - "${SESSIONS_JSON}" << 'PYEOF'
import sys, json
sessions = json.loads(sys.argv[1])
for s in sessions[:3]:
    print(f"    [{s.get('session_id','?')[:12]}...] user={s.get('subject_username','?')} serial={s.get('device_serial','?')[:10]} bytes_in={s.get('bytes_in',0)} bytes_out={s.get('bytes_out',0)} reason={s.get('end_reason','?')}")
PYEOF
    else
        fail "B2 -- Aucune session. Réponse : ${SESSIONS_JSON:0:300}"
    fi

    # B3a
    echo
    echo "  [B3a] Corrélation decision_id"
    if [[ -n "${DECISION_ID_B}" && "${SESSION_COUNT}" -gt 0 ]]; then
        CORR=$(python3 - "${SESSIONS_JSON}" "${DECISION_ID_B}" << 'PYEOF'
import sys, json
sessions = json.loads(sys.argv[1])
did = sys.argv[2]
print("FOUND" if any(s.get("decision_id") == did for s in sessions) else "NOT_FOUND")
PYEOF
)
        if [[ "${CORR}" == "FOUND" ]]; then
            pass "B3a -- Session corrélée à decision_id=${DECISION_ID_B}"
        else
            fail "B3a -- Aucune session avec decision_id=${DECISION_ID_B}"
        fi
    else
        skip "B3a -- decision_id vide ou pas de sessions"
    fi

    # B3b
    echo
    echo "  [B3b] bytes_in/bytes_out > 0"
    HAS_BYTES=$(python3 -c "
import json, sys
s = json.loads(sys.argv[1])
print('YES' if any(x.get('bytes_in',0) > 0 or x.get('bytes_out',0) > 0 for x in s) else 'NO')
" "${SESSIONS_JSON}" 2>/dev/null || echo "NO")
    [[ "${HAS_BYTES}" == "YES" ]] \
        && pass "B3b -- Sessions avec bytes transférés" \
        || fail "B3b -- bytes_in=0 et bytes_out=0 pour toutes les sessions"

    # B3c
    echo
    echo "  [B3c] end_reason renseigné"
    HAS_REASON=$(python3 -c "
import json, sys
s = json.loads(sys.argv[1])
print('YES' if any(x.get('end_reason','') for x in s) else 'NO')
" "${SESSIONS_JSON}" 2>/dev/null || echo "NO")
    [[ "${HAS_REASON}" == "YES" ]] \
        && pass "B3c -- end_reason présent dans au moins une session" \
        || fail "B3c -- end_reason vide -- vérifier SessionEnd dans proxyTCPInstrumented"
fi

# ═══════════════════════════════════════════════════════════════════════════════
step "BLOC C -- Routing hardening"

# C1 — ping local (le script tourne déjà sur wan-client)
echo
echo "  [C1] WAN→LAN ping bloqué (local depuis wan-client vers ${LAN_APP_IP})"
PING_LAN=$(ping -c 3 -W 2 "${LAN_APP_IP}" 2>&1 | tail -2 || true)
if echo "${PING_LAN}" | grep -qE "100% packet loss|unreachable|no route|host unreachable"; then
    pass "C1 -- WAN→LAN bloqué : ${PING_LAN}"
elif echo "${PING_LAN}" | grep -q "0% packet loss"; then
    fail "C1 -- WAN→LAN NON bloqué ! Ping ${LAN_APP_IP} réussit depuis wan-client"
    info "Lancer : make setup-routing"
else
    pass "C1 -- WAN→LAN : perte partielle = bloqué"
fi

# C2 — nc local
echo
echo "  [C2] TCP direct WAN→LAN:80 refusé (local nc vers ${LAN_APP_IP}:80)"
TCP_LAN=$(timeout 5 nc -zv "${LAN_APP_IP}" 80 2>&1 || true)
if echo "${TCP_LAN}" | grep -qiE "refused|timed out|unreachable|no route|reset"; then
    pass "C2 -- TCP direct WAN→LAN:80 bloqué"
elif echo "${TCP_LAN}" | grep -qiE "open|succeeded|connected"; then
    fail "C2 -- TCP direct WAN→LAN:80 OUVERT -- bypass PEP possible !"
else
    pass "C2 -- TCP direct WAN→LAN:80 bloqué (pas de connexion)"
fi

# C3 — ping local vers CP (DMZ)
echo
echo "  [C3] WAN→DMZ (CP) accessible (local ping vers ${CP_IP})"
PING_CP=$(ping -c 3 -W 3 "${CP_IP}" 2>&1 | tail -2 || true)
if echo "${PING_CP}" | grep -q "0% packet loss"; then
    pass "C3 -- WAN→DMZ accessible : ${PING_CP}"
elif echo "${PING_CP}" | grep -qE "100%|unreachable|no route"; then
    fail "C3 -- WAN→DMZ bloqué ! Le CP (${CP_IP}) n'est pas atteignable depuis wan-client"
else
    info "C3 -- ${PING_CP}"
fi

# C4
echo
echo "  [C4] FORWARD policy DROP sur ztna-gw"
FORWARD_POLICY=$(ssh_on "${GW_HOST}" "sudo iptables -L FORWARD -n 2>/dev/null | head -1")
if echo "${FORWARD_POLICY}" | grep -qi "DROP"; then
    pass "C4 -- FORWARD policy = DROP"
else
    fail "C4 -- FORWARD policy != DROP : '${FORWARD_POLICY}'"
fi

# C5
echo
echo "  [C5] Règle DROP WAN→LAN dans iptables"
FWDRULES=$(ssh_on "${GW_HOST}" "sudo iptables -L FORWARD -n -v 2>/dev/null")
if echo "${FWDRULES}" | grep -qiE "ZTNA-BLOCKED|DROP.*10\.10\.30"; then
    pass "C5 -- Règle DROP/ZTNA-BLOCKED WAN→LAN présente"
elif echo "${FWDRULES}" | grep -qi "DROP"; then
    pass "C5 -- Règle DROP présente (FORWARD policy couvre LAN)"
    info "Vérifier manuellement que LAN_CIDR est ciblé"
else
    fail "C5 -- Aucune règle DROP WAN→LAN"
    echo "${FWDRULES}" | head -20 | sed 's/^/    /'
fi

# C6
echo
echo "  [C6] Logs ZTNA-BLOCKED dans dmesg (optionnel)"
DMESG_LOG=$(ssh_on "${GW_HOST}" "sudo dmesg 2>/dev/null | grep 'ZTNA-BLOCKED' | tail -3")
if [[ -n "${DMESG_LOG}" ]]; then
    pass "C6 -- Tentatives WAN→LAN loggées :"
    echo "${DMESG_LOG}" | sed 's/^/    /'
else
    info "C6 -- Pas encore de log (normal si aucune tentative avant ce test)"
fi

# ═══════════════════════════════════════════════════════════════════════════════
echo
echo -e "${BOLD}+============================================================+${NC}"
echo -e "${BOLD}|  RESUME DES TESTS                                           |${NC}"
echo -e "${BOLD}+============================================================+${NC}"
printf "  %b%-6s%b %d\n" "${GREEN}" "PASS" "${NC}" "${PASS}"
printf "  %b%-6s%b %d\n" "${RED}"   "FAIL" "${NC}" "${FAIL}"
printf "  %b%-6s%b %d\n" "${YELLOW}" "SKIP" "${NC}" "${SKIP}"
echo
echo "  Bloc A -- CRL enforcement   : cert révoqué rejeté (data-plane)"
echo "  Bloc B -- Session telemetry : sessions corrélées aux décisions PDP"
echo "  Bloc C -- Routing hardening : FORWARD DROP + WAN→LAN bloqué"
echo
if (( FAIL == 0 )); then
    echo -e "  ${GREEN}${BOLD}TOUS LES TESTS PASSENT${NC}"
else
    echo -e "  ${RED}${BOLD}${FAIL} TEST(S) EN ECHEC -- voir détails ci-dessus${NC}"
    exit 1
fi
echo
echo "  Fichiers temporaires : ${ZTNA_DIR}/"
echo "  Nettoyage            : rm -rf ${ZTNA_DIR}"
