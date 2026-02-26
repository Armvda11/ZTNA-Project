#!/usr/bin/env bash
# =============================================================================
# demo/steps/03_issuecert.sh — Émission du device-cert X.509 par le CP
#
# Le client génère une paire ECDSA P-256 LOCALEMENT (la clé privée ne
# quitte jamais le client), construit un CSR et l'envoie au CP.
# Le CP signe le CSR avec sa Device CA et retourne le certificat PEM.
# =============================================================================
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "3" "ÉMISSION DEVICE-CERT" "Le CP signe un certificat X.509 pour ce device"

echo -e "${BOLD}Principe Zero Trust — CSR only${NC}"
print_separator
print_kv "Algorithme clé"   "ECDSA P-256 (généré localement)"
print_kv "Clé privée"       "Reste sur le client — JAMAIS transmise"
print_kv "CSR envoyé"       "POST /api/v1/credentials/device-cert"
print_kv "Auth"             "Bearer JWT (access_token étape 2)"
print_kv "CA signante"      "Device CA du Control Plane"
print_kv "TTL cert"         "7 jours (configurable)"
echo -e ""

echo -e "${INFO}Génération du device-cert sur wan-client…${NC}"
print_separator

ssh_client bash <<'ENDSSH'
set -e
source /tmp/ztna-demo/env.sh 2>/dev/null || true

ACCESS_TOKEN=$(cat /tmp/ztna-demo/access_token.txt 2>/dev/null || true)
if [[ -z "$ACCESS_TOKEN" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Token manquant — exécutez d'abord l'étape 02 (login)"
    exit 1
fi

WORK_DIR="/tmp/ztna-demo"
mkdir -p "$WORK_DIR"
KEY_FILE="$WORK_DIR/device.key"
CSR_FILE="$WORK_DIR/device.csr"
CERT_FILE="$WORK_DIR/device.crt"
CP_API=$(cat /tmp/ztna-demo/cp_api.txt 2>/dev/null || echo "https://10.10.20.30:8080")

echo -e "\033[0;36m[wan-client]\033[0m Génération clé ECDSA P-256…"
openssl ecparam -genkey -name prime256v1 -noout -out "$KEY_FILE"
echo -e "\033[0;32m[✓]\033[0m Clé privée générée (reste sur le client)"

echo -e "\033[0;36m[wan-client]\033[0m Construction du CSR…"
openssl req -new -key "$KEY_FILE" -out "$CSR_FILE" \
    -subj "/CN=ztna-client/O=ZTNA" 2>/dev/null
echo -e "\033[0;32m[✓]\033[0m CSR prêt"
echo ""
echo -e "    \033[2mCSR Subject: $(openssl req -noout -subject -in $CSR_FILE 2>/dev/null)\033[0m"

echo ""
echo -e "\033[0;36m[wan-client]\033[0m Envoi CSR au Control Plane (POST /api/v1/credentials/device-cert)…"
CSR_PEM=$(cat "$CSR_FILE")
RESPONSE=$(curl -sk --max-time 15 \
    -X POST "${CP_API}/api/v1/credentials/device-cert" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"csr\":\"$(echo "$CSR_PEM" | sed ':a;N;$!ba;s/\n/\\n/g')\"}" \
    2>&1)

CERT=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('certificate',''))" 2>/dev/null || true)

if [[ -z "$CERT" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Échec — réponse CP:"
    echo "$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESPONSE"
    exit 1
fi

echo "$CERT" > "$CERT_FILE"

EXPIRES_AT=$(echo "$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('expires_at','?'))" 2>/dev/null || echo "?")
SERIAL=$(openssl x509 -noout -serial -in "$CERT_FILE" 2>/dev/null | cut -d= -f2 || echo "?")
SUBJECT=$(openssl x509 -noout -subject -in "$CERT_FILE" 2>/dev/null || echo "?")
ISSUER=$(openssl x509 -noout -issuer  -in "$CERT_FILE" 2>/dev/null || echo "?")

echo ""
echo -e "\033[0;32m[✓]\033[0m Certificat X.509 reçu et sauvegardé!"
echo ""
printf "    \033[0;36m%-20s\033[0m %s\n" "serial:"      "$SERIAL"
printf "    \033[0;36m%-20s\033[0m %s\n" "subject:"     "$SUBJECT"
printf "    \033[0;36m%-20s\033[0m %s\n" "issuer:"      "$ISSUER"
printf "    \033[0;36m%-20s\033[0m %s\n" "expires_at:"  "$EXPIRES_AT"
echo ""
echo -e "    \033[2mCert: $CERT_FILE\033[0m"
echo -e "    \033[2mKey:  $KEY_FILE (jamais transmise)\033[0m"
ENDSSH

echo -e ""
print_ok "Device-cert obtenu — mTLS possible vers la Gateway"
