#!/usr/bin/env bash
# =============================================================================
# demo/steps/06_revoke.sh — Révocation du device-cert via API admin
#
# Un administrateur révoque le certificat X.509 du device via l'API admin du CP.
# Le CP met à jour sa CRL (Certificate Revocation List).
# La Gateway rechargera la CRL lors de la prochaine connexion.
# =============================================================================
set -uo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "6" "RÉVOCATION DU CERT (CRL)" "Un admin révoque le device-cert → CRL mise à jour"

echo -e "${BOLD}Scénario : device compromis ou sortie d'employé${NC}"
print_separator
print_kv "Action admin"    "DELETE /api/v1/admin/device-certs/{serial}"
print_kv "Effet"           "Cert ajouté à la CRL — aucune nouvelle session"
print_kv "Portée"          "Certificat de l'étape 3"
print_kv "Authentification" "Bearer JWT + groupe ztna-admins"
echo -e ""

echo -e "${INFO}Révocation via API admin du Control Plane…${NC}"
print_separator

# Récupérer le serial du cert depuis wan-client (openssl retourne en majuscules)
# Le CP stocke en minuscules (fmt.Sprintf("%x",...)) → conversion nécessaire
SERIAL=$(ssh_client "openssl x509 -noout -serial -in /tmp/ztna-demo/device.crt 2>/dev/null | cut -d= -f2 | tr '[:upper:]' '[:lower:]'" 2>/dev/null || true)

if [[ -z "$SERIAL" ]]; then
    print_err "Certificat introuvable sur wan-client — exécutez d'abord l'étape 03"
    exit 1
fi

print_kv "Serial à révoquer" "$SERIAL"
echo -e ""

# Obtenir un token admin (alice est dans ztna-admins)
echo -e "${INFO}Obtention du token admin (alice / ztna-admins)…${NC}"
ADMIN_TOKEN=$(get_oidc_token "$ZTNA_USER" "$ZTNA_PASS")
if [[ -z "$ADMIN_TOKEN" ]]; then
    print_err "Impossible d'obtenir le token admin"
    exit 1
fi
print_ok "Token admin obtenu"
echo -e ""

# Appel API révocation
echo -e "${INFO}Appel DELETE /api/v1/admin/device-certs/${SERIAL}…${NC}"
HTTP_STATUS=$(curl -sk --max-time 10 -o /tmp/ztna-demo-revoke-resp.json -w "%{http_code}" \
    -X DELETE \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    "${CP_API}/api/v1/admin/device-certs/${SERIAL}" 2>/dev/null)

RESP_BODY=$(cat /tmp/ztna-demo-revoke-resp.json 2>/dev/null | python3 -m json.tool 2>/dev/null || cat /tmp/ztna-demo-revoke-resp.json 2>/dev/null)

if [[ "$HTTP_STATUS" == "200" || "$HTTP_STATUS" == "204" ]]; then
    echo -e ""
    print_ok "Certificat révoqué avec succès (HTTP ${HTTP_STATUS})"
    echo -e ""
    echo -e "${DIM}${RESP_BODY}${NC}"
    echo -e ""

    # Vérifier que la CRL a été mise à jour
    echo -e "${INFO}Vérification de la CRL publique…${NC}"
    CRL_RESP=$(curl -sk --max-time 5 "${CP_API}/pki/device-ca/crl" | openssl crl -text -noout 2>/dev/null | grep -E "Serial Number|Last Update|Next Update" | head -6 || true)
    if [[ -n "$CRL_RESP" ]]; then
        print_ok "CRL mise à jour"
        echo "$CRL_RESP" | while IFS= read -r line; do
            echo -e "    ${DIM}${line}${NC}"
        done
    fi
else
    print_err "Échec de la révocation (HTTP ${HTTP_STATUS})"
    echo "${RESP_BODY}"
    exit 1
fi

echo -e ""
echo -e "${YELLOW}${BOLD}→ La prochaine connexion mTLS avec ce cert sera refusée (étape 07)${NC}"
