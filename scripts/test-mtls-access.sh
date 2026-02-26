#!/usr/bin/env bash
# Test du Flux 2 : Accès via Certificat Device (mTLS) → ZTNA Gateway
#
# Flux complet :
#   wan-client
#     → (OIDC) Keycloak (token JWT)
#     → CP /api/v1/credentials/device-cert (certificat X.509 signé)
#     → mTLS:4433 → ztna-gateway
#         ‣ ConnectRequest JSON  {"resource_type":"http","resource_match":"http:lan-app:80","action":"connect"}
#         ‣ Gateway → CP PEP /api/v1/pep/authorize
#         ‣ ConnectResponse JSON {"allowed":true,...}
#         ‣ Proxy TCP → lan-app:80
#     → Réponse HTTP nginx
#
# Usage:
#   ZTNA_USER=alice ZTNA_PASS=secret ./scripts/test-mtls-access.sh [http|ssh]
#   ./scripts/test-mtls-access.sh     (interactif, HTTP par défaut)

set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────
CP_URL="https://10.10.20.30:8080"
KC_PROTO="${KC_PROTO:-https}"  # https (default) or http (legacy fallback)
if [[ "${KC_PROTO}" == "https" ]]; then
  KC_URL="https://10.10.20.30:8443"
else
  KC_URL="http://10.10.20.30:8081"
fi
KC_REALM="ztna"
KC_CLIENT="ztna-control-plane"
GW_HOST="10.10.10.20"
GW_PORT="4433"
ZTNA_USER="${ZTNA_USER:-}"
ZTNA_PASS="${ZTNA_PASS:-}"
MODE="${1:-http}"          # http | ssh
ZTNA_DIR="${HOME}/.ztna"
# ──────────────────────────────────────────────────────────────────────────────

log()  { echo "[$(date +%H:%M:%S)] $*"; }
die()  { echo "[ERREUR] $*" >&2; exit 1; }
step() { echo; echo "──── $* ────"; }

command -v openssl  >/dev/null || die "openssl requis"
command -v python3  >/dev/null || die "python3 requis"

mkdir -p "${ZTNA_DIR}"
chmod 700 "${ZTNA_DIR}"

# Credentials interactifs si absents
if [[ -z "${ZTNA_USER}" ]]; then
  read -rp "Utilisateur ZTNA : " ZTNA_USER
fi
if [[ -z "${ZTNA_PASS}" ]]; then
  read -rsp "Mot de passe     : " ZTNA_PASS
  echo
fi

DEVICE_KEY="${ZTNA_DIR}/device_${ZTNA_USER}.key"
DEVICE_CRT="${ZTNA_DIR}/device_${ZTNA_USER}.crt"

# ──────────────────────────────────────────────────────────────────────────────
step "1/4 — Obtention du token OIDC (Keycloak)"
# ──────────────────────────────────────────────────────────────────────────────
TOKEN_RESP=$(curl -sk \
  -d "client_id=${KC_CLIENT}&username=${ZTNA_USER}&password=${ZTNA_PASS}&grant_type=password" \
  "${KC_URL}/realms/${KC_REALM}/protocol/openid-connect/token")

ACCESS_TOKEN=$(echo "${TOKEN_RESP}" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('access_token',''))" 2>/dev/null || true)

[[ -z "${ACCESS_TOKEN}" ]] && { echo "Réponse : ${TOKEN_RESP}" >&2; die "Token OIDC non obtenu"; }
log "✓ Token OIDC obtenu"

# ──────────────────────────────────────────────────────────────────────────────
step "2/4 — Génération de la clé device + CSR ECDSA"
# ──────────────────────────────────────────────────────────────────────────────
DEVICE_CSR="${ZTNA_DIR}/device_${ZTNA_USER}.csr"

if [[ ! -f "${DEVICE_KEY}" ]]; then
  openssl ecparam -name prime256v1 -genkey -noout -out "${DEVICE_KEY}" 2>/dev/null
  log "✓ Clé ECDSA P-256 générée : ${DEVICE_KEY}"
fi

openssl req -new -key "${DEVICE_KEY}" \
  -subj "/CN=${ZTNA_USER}/O=ztna-admins" \
  -out "${DEVICE_CSR}" 2>/dev/null
log "✓ CSR généré"

CSR_PEM=$(cat "${DEVICE_CSR}")

# ──────────────────────────────────────────────────────────────────────────────
step "3/4 — Demande de certificat Device au Control Plane"
# ──────────────────────────────────────────────────────────────────────────────
CERT_RESP=$(curl -sk \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"csr_pem\": $(python3 -c "import json,sys; print(json.dumps(open('${DEVICE_CSR}').read()))")}" \
  "${CP_URL}/api/v1/credentials/device-cert")

CERT_PEM=$(echo "${CERT_RESP}" | \
  python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('certificate_pem',''))" 2>/dev/null || true)

if [[ -z "${CERT_PEM}" ]]; then
  echo "Réponse CP : ${CERT_RESP}" >&2
  die "Certificat device non obtenu."
fi

echo "${CERT_PEM}" > "${DEVICE_CRT}"
chmod 600 "${DEVICE_CRT}"
log "✓ Certificat device obtenu → ${DEVICE_CRT}"
log "  Subject : $(openssl x509 -noout -subject -in "${DEVICE_CRT}" 2>/dev/null)"
log "  Expiry  : $(openssl x509 -noout -enddate -in "${DEVICE_CRT}" 2>/dev/null)"

# ──────────────────────────────────────────────────────────────────────────────
step "4/4 — Connexion mTLS vers le gateway (${GW_HOST}:${GW_PORT})"
# ──────────────────────────────────────────────────────────────────────────────
case "${MODE}" in
  http)
    RESOURCE_TYPE="http"
    RESOURCE_MATCH="http:lan-app:80"
    ;;
  ssh)
    RESOURCE_TYPE="ssh"
    RESOURCE_MATCH="ssh:lan-app:22"
    ;;
  *)
    die "Mode inconnu '${MODE}'. Utiliser 'http' ou 'ssh'."
    ;;
esac

CONNECT_REQ="{\"resource_type\":\"${RESOURCE_TYPE}\",\"resource_match\":\"${RESOURCE_MATCH}\",\"action\":\"connect\"}"
log "ConnectRequest : ${CONNECT_REQ}"
echo

# Python client mTLS : handshake + ConnectRequest + ConnectResponse + proxy HTTP
python3 - "${GW_HOST}" "${GW_PORT}" "${DEVICE_CRT}" "${DEVICE_KEY}" \
           "${RESOURCE_TYPE}" "${RESOURCE_MATCH}" "${MODE}" << 'PYEOF'
import sys, ssl, socket, json, http.client

gw_host, gw_port, cert, key, res_type, res_match, mode = sys.argv[1:]
gw_port = int(gw_port)

# Contexte mTLS : on ne vérifie pas le cert serveur (lab self-signed)
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=cert, keyfile=key)

print(f"[mTLS] Connexion vers {gw_host}:{gw_port} ...")
raw = socket.create_connection((gw_host, gw_port), timeout=15)
tls = ctx.wrap_socket(raw, server_hostname=gw_host)
print(f"[mTLS] Handshake OK — protocole : {tls.version()}")

connect_req = json.dumps({
    "resource_type":  res_type,
    "resource_match": res_match,
    "action":         "connect",
})
tls.sendall((connect_req + "\n").encode())
print(f"[mTLS] ConnectRequest envoyé : {connect_req}")

# Lecture de la réponse JSON
buf = b""
while b"\n" not in buf:
    chunk = tls.recv(4096)
    if not chunk:
        break
    buf += chunk

resp = json.loads(buf.split(b"\n")[0])
print()
print("┌─ ConnectResponse ─────────────────────────────────────────")
print(f"│  allowed     : {resp.get('allowed')}")
print(f"│  decision_id : {resp.get('decision_id','—')}")
print(f"│  reason      : {resp.get('reason','—')}")
print("└───────────────────────────────────────────────────────────")

if not resp.get("allowed"):
    print("\n[REFUSÉ] Accès non autorisé par le PEP.")
    sys.exit(1)

print(f"\n[AUTORISÉ] Tunnel TCP ouvert vers {res_match}")

if mode == "http":
    # Envoyer une requête HTTP simple
    http_req = (
        f"GET / HTTP/1.0\r\n"
        f"Host: lan-app\r\n"
        f"\r\n"
    )
    tls.sendall(http_req.encode())
    # Lire la réponse (HTTP/1.0 ferme après la réponse)
    tls.settimeout(10.0)
    response = b""
    try:
        while True:
            data = tls.recv(4096)
            if not data:
                break
            response += data
    except Exception:
        pass  # EOF ou timeout = fin normale

    if not response:
        print("[ERREUR] Aucune réponse HTTP reçue depuis lan-app.")
        sys.exit(1)

    print("\n─── Réponse HTTP depuis lan-app ────────────────────────────")
    print(response.decode(errors="replace")[:2000])
    print("────────────────────────────────────────────────────────────")
else:
    print("[INFO] Mode SSH : le tunnel est ouvert.")
    print("       Pour SSH interactif, utilisez test-ssh-cert-access.sh")
    print("       (le gateway proxyfie SSH mais nécessite un client SSH complet)")

tls.close()
print("\n✓ Test mTLS terminé avec succès.")
PYEOF
