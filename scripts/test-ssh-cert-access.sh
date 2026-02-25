#!/usr/bin/env bash
# Test du Flux 1 : Accès SSH via Certificat SSH signé par le CP
#
# Flux complet :
#   wan-client
#     → (OIDC) Keycloak (token JWT)
#     → CP /api/v1/credentials/ssh-cert (certificat SSH signé)
#     → SSH -J ztna-gw:22 (jump host, cert accepté via TrustedUserCAKeys)
#     → lan-app:22 (cert accepté via TrustedUserCAKeys)
#
# Usage:
#   ZTNA_USER=alice ZTNA_PASS=secret ./scripts/test-ssh-cert-access.sh [lan-app|lan-admin]
#   ./scripts/test-ssh-cert-access.sh  (interactif)

set -euo pipefail

# ── Configuration ─────────────────────────────────────────────────────────────
CP_URL="https://10.10.20.30:8080"
KC_URL="http://10.10.20.30:8081"
KC_REALM="ztna"
KC_CLIENT="ztna-control-plane"
GW_HOST="10.10.10.20"             # WAN IP de ztna-gw
LAN_APP_IP="10.10.30.10"
LAN_ADMIN_IP="10.10.30.11"
ZTNA_USER="${ZTNA_USER:-}"
ZTNA_PASS="${ZTNA_PASS:-}"
TARGET="${1:-lan-app}"
ZTNA_DIR="${HOME}/.ztna"
SSH_TEST_CMD="${SSH_TEST_CMD:-}"
# ──────────────────────────────────────────────────────────────────────────────

log()  { echo "[$(date +%H:%M:%S)] $*"; }
die()  { echo "[ERREUR] $*" >&2; exit 1; }
step() { echo; echo "──── $* ────"; }

mkdir -p "${ZTNA_DIR}"
chmod 700 "${ZTNA_DIR}"

# Sélection de la cible
case "${TARGET}" in
  lan-app)   TARGET_IP="${LAN_APP_IP}" ;;
  lan-admin) TARGET_IP="${LAN_ADMIN_IP}" ;;
  *)         die "Cible inconnue '${TARGET}'. Usage: $0 [lan-app|lan-admin]" ;;
esac

# Credentials interactifs si absents
if [[ -z "${ZTNA_USER}" ]]; then
  read -rp "Utilisateur ZTNA : " ZTNA_USER
fi
if [[ -z "${ZTNA_PASS}" ]]; then
  read -rsp "Mot de passe     : " ZTNA_PASS
  echo
fi

# ──────────────────────────────────────────────────────────────────────────────
step "1/4 — Obtention du token OIDC (Keycloak)"
# ──────────────────────────────────────────────────────────────────────────────
TOKEN_RESP=$(curl -sk \
  -d "client_id=${KC_CLIENT}&username=${ZTNA_USER}&password=${ZTNA_PASS}&grant_type=password" \
  "${KC_URL}/realms/${KC_REALM}/protocol/openid-connect/token")

ACCESS_TOKEN=$(echo "${TOKEN_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('access_token',''))" 2>/dev/null || true)

if [[ -z "${ACCESS_TOKEN}" ]]; then
  echo "Réponse Keycloak : ${TOKEN_RESP}" >&2
  die "Impossible d'obtenir le token OIDC. Vérifiez les credentials et l'URL Keycloak."
fi
log "✓ Token OIDC obtenu (${#ACCESS_TOKEN} caractères)"

# ──────────────────────────────────────────────────────────────────────────────
step "2/4 — Génération de la clé SSH (ou réutilisation)"
# ──────────────────────────────────────────────────────────────────────────────
KEY_FILE="${ZTNA_DIR}/id_ztna_${ZTNA_USER}"
CERT_FILE="${KEY_FILE}-cert.pub"

if [[ ! -f "${KEY_FILE}" ]]; then
  ssh-keygen -t ed25519 -f "${KEY_FILE}" -N "" -C "ztna-${ZTNA_USER}" -q
  log "✓ Nouvelle clé Ed25519 générée : ${KEY_FILE}"
else
  log "✓ Clé existante réutilisée : ${KEY_FILE}"
fi

PUB_KEY=$(cat "${KEY_FILE}.pub")

# ──────────────────────────────────────────────────────────────────────────────
step "3/4 — Demande de certificat SSH au Control Plane"
# ──────────────────────────────────────────────────────────────────────────────
CERT_RESP=$(curl -sk \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"${PUB_KEY}\", \"principals\": [\"ztna\", \"${ZTNA_USER}\"]}" \
  "${CP_URL}/api/v1/credentials/ssh-cert")

CERT=$(echo "${CERT_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('certificate',''))" 2>/dev/null || true)

if [[ -z "${CERT}" ]]; then
  echo "Réponse CP : ${CERT_RESP}" >&2
  die "Certificat SSH non obtenu. Vérifiez que le CP est démarré et les policies."
fi

echo "${CERT}" > "${CERT_FILE}"
chmod 600 "${CERT_FILE}"
log "✓ Certificat SSH obtenu → ${CERT_FILE}"
log "  $(ssh-keygen -L -f "${CERT_FILE}" 2>/dev/null | grep -E 'Type|Key ID|Valid|Principals' | sed 's/^/    /')"

# ──────────────────────────────────────────────────────────────────────────────
step "4/4 — Connexion SSH via jump host ztna-gw → ${TARGET} (${TARGET_IP})"
# ──────────────────────────────────────────────────────────────────────────────
log "Connexion : SSH -J ztna@${GW_HOST} ztna@${TARGET_IP}"

if [[ -n "${SSH_TEST_CMD}" ]]; then
  log "Mode non-interactif activé (SSH_TEST_CMD='${SSH_TEST_CMD}')"
  ssh \
    -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -i "${KEY_FILE}" \
    -i "${CERT_FILE}" \
    -J "ztna@${GW_HOST}" \
    "ztna@${TARGET_IP}" \
    "${SSH_TEST_CMD}"
  log "✓ Test SSH cert non-interactif réussi"
  exit 0
fi

echo
echo "─────────────────────────────────────────────────────────────"
echo "  Session SSH interactive — tapez 'exit' pour terminer"
echo "─────────────────────────────────────────────────────────────"
echo

exec ssh \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -o LogLevel=ERROR \
  -i "${KEY_FILE}" \
  -i "${CERT_FILE}" \
  -J "ztna@${GW_HOST}" \
  "ztna@${TARGET_IP}"
