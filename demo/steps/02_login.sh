#!/usr/bin/env bash
# =============================================================================
# demo/steps/02_login.sh — Authentification OIDC via Keycloak
#
# Le client s'authentifie auprès de Keycloak (realm ztna) avec ROPC.
# En production : Device Flow ou Authorization Code + PKCE.
# Keycloak retourne un JWT signé RS256 que le CP validera offline.
# =============================================================================
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "2" "AUTHENTIFICATION OIDC" "Le client prouve son identité auprès de Keycloak"

echo -e "${BOLD}Utilisateur de démonstration${NC}"
print_separator
print_kv "Utilisateur"   "${ZTNA_USER}"
print_kv "IDP"           "${KC_URL}/realms/${ZTNA_REALM}"
print_kv "Protocole"     "OpenID Connect — Resource Owner Password Grant (lab)"
print_kv "Algorithme"    "RS256 (Keycloak)"
echo -e ""

echo -e "${CYAN}Exécution du login OIDC sur wan-client…${NC}"
print_separator
print_cmd "curl -sk -d grant_type=password -d client_id=${ZTNA_CLIENT_ID} -d username=${ZTNA_USER} \\\n    \"${KC_URL}/realms/${ZTNA_REALM}/protocol/openid-connect/token\""

ssh_client bash <<ENDSSH
set -e
echo -e "\033[0;36m[wan-client]\033[0m Appel Keycloak OIDC endpoint…"
echo ""

TOKEN_RESPONSE=\$(curl -sk --max-time 10 \\
    -d "grant_type=password" \\
    -d "client_id=${ZTNA_CLIENT_ID}" \\
    -d "username=${ZTNA_USER}" \\
    -d "password=${ZTNA_PASS}" \\
    "${KC_URL}/realms/${ZTNA_REALM}/protocol/openid-connect/token")

ACCESS_TOKEN=\$(echo "\$TOKEN_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])" 2>/dev/null)
EXPIRES_IN=\$(echo "\$TOKEN_RESPONSE"   | python3 -c "import sys,json; print(json.load(sys.stdin).get('expires_in','?'))" 2>/dev/null)
TOKEN_TYPE=\$(echo "\$TOKEN_RESPONSE"   | python3 -c "import sys,json; print(json.load(sys.stdin).get('token_type','?'))" 2>/dev/null)

if [[ -z "\$ACCESS_TOKEN" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Échec — réponse Keycloak:"
    echo "\$TOKEN_RESPONSE" | python3 -m json.tool 2>/dev/null || echo "\$TOKEN_RESPONSE"
    exit 1
fi

# Décoder le payload JWT (sans vérification)
PAYLOAD=\$(echo "\$ACCESS_TOKEN" | cut -d. -f2 | base64 -d 2>/dev/null | python3 -m json.tool 2>/dev/null || echo "")

echo -e "\033[0;32m[✓]\033[0m Authentification réussie!"
echo ""
printf "    \033[0;36m%-20s\033[0m %s\n" "token_type:"  "\$TOKEN_TYPE"
printf "    \033[0;36m%-20s\033[0m %s s\n" "expires_in:" "\$EXPIRES_IN"
printf "    \033[0;36m%-20s\033[0m %s…\n" "access_token:" "\${ACCESS_TOKEN:0:50}"
echo ""

if [[ -n "\$PAYLOAD" ]]; then
    SUB=\$(echo "\$PAYLOAD" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('sub','?'))" 2>/dev/null || echo "?")
    USERNAME=\$(echo "\$PAYLOAD" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('preferred_username','?'))" 2>/dev/null || echo "?")
    GROUPS=\$(echo "\$PAYLOAD" | python3 -c "import sys,json; d=json.load(sys.stdin); print(','.join(d.get('groups',[])))" 2>/dev/null || echo "?")
    EXP=\$(echo "\$PAYLOAD" | python3 -c "import sys,json,datetime; d=json.load(sys.stdin); print(datetime.datetime.fromtimestamp(d.get('exp',0)).strftime('%H:%M:%S'))" 2>/dev/null || echo "?")

    echo -e "    \033[1m--- Payload JWT ---\033[0m"
    printf "    \033[0;36m%-20s\033[0m %s\n" "sub:"      "\$SUB"
    printf "    \033[0;36m%-20s\033[0m %s\n" "username:" "\$USERNAME"
    printf "    \033[0;36m%-20s\033[0m %s\n" "groups:"   "\$GROUPS"
    printf "    \033[0;36m%-20s\033[0m %s\n" "exp:"      "\$EXP"
fi

# Sauvegarder le token pour les étapes suivantes
mkdir -p /tmp/ztna-demo
echo "\$ACCESS_TOKEN" > /tmp/ztna-demo/access_token.txt
echo -e ""
echo -e "    \033[2mToken sauvegardé dans /tmp/ztna-demo/access_token.txt\033[0m"
ENDSSH

echo -e ""
print_ok "Login OIDC réussi — JWT disponible pour les étapes suivantes"
