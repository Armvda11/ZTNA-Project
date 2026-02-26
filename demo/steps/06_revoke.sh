#!/usr/bin/env bash
# =============================================================================
# demo/steps/06_revoke.sh — Kill session live + Révocation du device-cert
#
# Phase 1 : kill de la session active (l'administrateur coupe la session en cours)
#           La Gateway détecte le kill via le poll /pep/sessions/{id}/valid en ~5s
#
# Phase 2 : révocation du device-cert via API admin du CP
#           Le CP met à jour sa CRL (Certificate Revocation List)
#           La Gateway est redémarrée → rechargement immédiat de la CRL
# =============================================================================
set -uo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "6" "KILL SESSION + RÉVOCATION CERT" \
    "Phase 1 : kill session live (~5s)  |  Phase 2 : révocation CRL"

# =============================================================================
# PHASE 1 — Kill de la session active
# =============================================================================
echo -e ""
echo -e "${YELLOW}${BOLD}▶  PHASE 1 — L'administrateur coupe la session live${NC}"
print_separator
echo -e "${BOLD}Scénario : comportement suspect détecté — couper immédiatement l'accès${NC}"
echo -e ""
print_kv "Action admin"    "DELETE /api/v1/admin/sessions/{session_id}"
print_kv "Délai réel"      "~5s (Gateway poll IsSessionValid toutes les 5s)"
print_kv "Effet"           "Context annulé sur le tunnel TCP → proxy fermé côté Gateway"
echo -e ""

# Obtenir un token admin
echo -e "${INFO}Obtention du token admin pour le kill de session…${NC}"
ADMIN_TOKEN=$(get_oidc_token "$ZTNA_USER" "$ZTNA_PASS")
if [[ -z "$ADMIN_TOKEN" ]]; then
    print_warn "Impossible d'obtenir le token admin — Phase 1 ignorée"
else
    print_ok "Token admin obtenu"
    echo -e ""

    # Lister les sessions actives
    echo -e "${INFO}Recherche d'une session active dans le Control Plane…${NC}"
    SESSIONS_JSON=$(curl -sk --max-time 10 \
        -H "Authorization: Bearer ${ADMIN_TOKEN}" \
        "${CP_API}/api/v1/admin/sessions?limit=20" 2>/dev/null || echo "[]")

    SESSION_ID=$(echo "$SESSIONS_JSON" | python3 -c "
import json, sys
data = sys.stdin.read().strip()
try:
    sessions = json.loads(data)
except:
    sessions = []
if not isinstance(sessions, list):
    sessions = sessions.get('sessions', sessions.get('data', []))
for s in sessions:
    if not s.get('end_time') and not s.get('killed_at'):
        print(s.get('session_id', s.get('id', '')))
        break
" 2>/dev/null || true)

    if [[ -z "$SESSION_ID" ]]; then
        print_warn "Aucune session active trouvée — Phase 1 ignorée (étape 04 peut-être absente)"
    else
        print_kv "Session active trouvée" "$SESSION_ID"
        echo -e ""

        echo -e "${INFO}Kill de la session ${SESSION_ID}…${NC}"
        HTTP_STATUS=$(curl -sk --max-time 10 \
            -X DELETE \
            -H "Authorization: Bearer ${ADMIN_TOKEN}" \
            -o /tmp/ztna-demo-kill-resp.json \
            -w "%{http_code}" \
            "${CP_API}/api/v1/admin/sessions/${SESSION_ID}" 2>/dev/null)

        KILL_BODY=$(cat /tmp/ztna-demo-kill-resp.json 2>/dev/null | python3 -m json.tool 2>/dev/null || cat /tmp/ztna-demo-kill-resp.json 2>/dev/null || echo "(pas de réponse)")

        if [[ "$HTTP_STATUS" == "200" || "$HTTP_STATUS" == "204" ]]; then
            echo -e ""
            print_ok "Session killée par le CP (HTTP ${HTTP_STATUS})"
            echo -e "${DIM}${KILL_BODY}${NC}"
            echo -e ""
            echo -e "${YELLOW}⏱  La Gateway va détecter le kill dans ~5 secondes (poll /pep/sessions/{id}/valid)…${NC}"
            echo -e ""

            # Compter les 5 secondes visuellement + afficher le log wan-client
            for i in 1 2 3 4 5; do
                printf "    ${DIM}t+%ds…${NC}\n" "$i"
                sleep 1
            done

            # Afficher les dernières lignes du log du worker
            echo -e ""
            echo -e "${BOLD}  ─── Log session wan-client (dernières lignes) ───${NC}"
            WORKER_LOG=$(ssh_client "tail -8 /tmp/ztna-session.log 2>/dev/null" 2>/dev/null || true)
            if [[ -n "$WORKER_LOG" ]]; then
                echo "$WORKER_LOG" | sed 's/^/  /'
            else
                echo -e "  ${DIM}(log vide — worker non démarré ou déjà terminé)${NC}"
            fi
            echo -e ""

            # Tuer le worker si toujours actif (nettoyage)
            ssh_client "kill \$(cat /tmp/ztna-session.pid 2>/dev/null) 2>/dev/null; rm -f /tmp/ztna-session.pid" 2>/dev/null || true

            print_ok "Le tunnel TCP de la session a été coupé côté Gateway"
            echo -e "${BOLD}${RED}→ Le worker a reçu : SESSION COUPEE PAR L'ADMINISTRATEUR${NC}"
        else
            print_warn "Réponse inattendue du CP (HTTP ${HTTP_STATUS}) — poursuivi quand même"
            echo "${KILL_BODY}"
        fi
    fi
fi

echo -e ""
read -r -p "  $(echo -e "${YELLOW}Appuyez sur Entrée pour passer à la Phase 2 (révocation CRL)…${NC}")" _
echo -e ""

# =============================================================================
# PHASE 2 — Révocation du certificat (CRL)
# =============================================================================
echo -e ""
echo -e "${YELLOW}${BOLD}▶  PHASE 2 — Révocation du device-cert (CRL)${NC}"
print_separator
echo -e "${BOLD}Scénario : device compromis ou sortie d'employé — bloquer toute reconnexion${NC}"
print_separator
print_kv "Action admin"     "DELETE /api/v1/admin/device-certs/{serial}"
print_kv "Effet"            "Cert ajouté à la CRL → Gateway redémarrée → CRL chargée immédiatement"
print_kv "Portée"           "Certificat de l'étape 3"
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

# Obtenir un token admin (alice est dans ztna-admins) — réutiliser si déjà disponible
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

# Forcer le rechargement immédiat de la CRL sur la Gateway
echo -e "${INFO}Forçage du rechargement CRL sur la Gateway…${NC}"
ssh_gw "sudo systemctl restart ztna-gateway" 2>/dev/null
sleep 3
# Vérifier que la gateway a redémarré
GW_STATE=$(ssh_gw "systemctl is-active ztna-gateway 2>/dev/null" 2>/dev/null || echo "unknown")
if [[ "$GW_STATE" == "active" ]]; then
    print_ok "Gateway redémarrée — CRL rechargée immédiatement (serial blacklisté)"
else
    print_warn "État gateway : ${GW_STATE} — attente 3s supplémentaires"
    sleep 3
fi

echo -e ""
echo -e "${YELLOW}${BOLD}→ La prochaine connexion mTLS avec ce cert sera refusée (étape 07)${NC}"
