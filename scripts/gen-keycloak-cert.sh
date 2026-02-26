#!/usr/bin/env bash
# Génère un certificat TLS pour Keycloak, signé par la CA du lab.
#
# Usage: ./scripts/gen-keycloak-cert.sh [HOST_IP]
#   HOST_IP : IP du serveur Keycloak (défaut: 10.10.20.30)
#
# Prérequis : la CA doit déjà exister dans control-plane/certs/
# Sortie    : control-plane/keycloak/certs/keycloak.{crt,key}

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CA_DIR="${ROOT_DIR}/control-plane/certs"
KC_CERT_DIR="${ROOT_DIR}/control-plane/keycloak/certs"
HOST_IP="${1:-10.10.20.30}"

log() { echo "[$(date +%H:%M:%S)] $*"; }

# Vérifier la présence de la CA
if [[ ! -f "${CA_DIR}/ca.key" || ! -f "${CA_DIR}/ca.crt" ]]; then
  log "✗ CA introuvable dans ${CA_DIR}/"
  log "  Lancez d'abord deploy-control-plane.sh ou générez la CA manuellement."
  exit 1
fi

mkdir -p "${KC_CERT_DIR}"

# Générer la clé privée du serveur Keycloak
log "Génération de la clé privée Keycloak..."
openssl genrsa -out "${KC_CERT_DIR}/keycloak.key" 2048 2>/dev/null

# Créer la configuration d'extensions pour les SAN
cat > "${KC_CERT_DIR}/keycloak-ext.cnf" <<EOF
[req]
distinguished_name = req_dn
req_extensions     = v3_req
prompt             = no

[req_dn]
CN = keycloak

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = keycloak
IP.1  = 127.0.0.1
IP.2  = ${HOST_IP}
EOF

# Générer le CSR
log "Génération du CSR Keycloak (SAN: localhost, keycloak, 127.0.0.1, ${HOST_IP})..."
openssl req -new \
  -key "${KC_CERT_DIR}/keycloak.key" \
  -subj "/CN=keycloak" \
  -config "${KC_CERT_DIR}/keycloak-ext.cnf" \
  -out "${KC_CERT_DIR}/keycloak.csr" 2>/dev/null

# Signer avec la CA du lab
log "Signature du certificat par la CA du lab..."
openssl x509 -req \
  -in "${KC_CERT_DIR}/keycloak.csr" \
  -CA "${CA_DIR}/ca.crt" \
  -CAkey "${CA_DIR}/ca.key" \
  -CAcreateserial \
  -out "${KC_CERT_DIR}/keycloak.crt" \
  -days 365 \
  -sha256 \
  -extensions v3_req \
  -extfile "${KC_CERT_DIR}/keycloak-ext.cnf" 2>/dev/null

# Nettoyage fichiers temporaires
rm -f "${KC_CERT_DIR}/keycloak.csr" "${KC_CERT_DIR}/keycloak-ext.cnf"

log "✓ Certificat Keycloak généré dans ${KC_CERT_DIR}/"
log "  - keycloak.key (clé privée)"
log "  - keycloak.crt (certificat signé par CA lab)"
log "  SAN: DNS:localhost, DNS:keycloak, IP:127.0.0.1, IP:${HOST_IP}"

# Vérification rapide
log ""
log "Vérification du certificat :"
openssl x509 -in "${KC_CERT_DIR}/keycloak.crt" -noout -subject -issuer -dates 2>/dev/null
openssl verify -CAfile "${CA_DIR}/ca.crt" "${KC_CERT_DIR}/keycloak.crt" 2>/dev/null || true
