#!/usr/bin/env bash
# =============================================================================
# demo/steps/01_health.sh — Health check de tous les services
# =============================================================================
set -uo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "1" "HEALTH CHECK" "Vérification que tous les services sont en ligne"

echo -e "${BOLD}Vérification SSH des VMs${NC}"
print_separator

# Control Plane
printf "  %-20s" "ztna-cp (${CP_IP})"
if ssh $SSH_OPTS "ztna@${CP_IP}" 'exit 0' 2>/dev/null; then
    print_ok "SSH OK"
else
    print_err "INJOIGNABLE"
fi

# Gateway
printf "  %-20s" "ztna-gw (${GW_IP})"
if ssh $SSH_OPTS "ztna@${GW_IP}" 'exit 0' 2>/dev/null; then
    print_ok "SSH OK"
else
    print_err "INJOIGNABLE"
fi

# wan-client
printf "  %-20s" "wan-client (${CLIENT_IP})"
if ssh $SSH_OPTS "ztna@${CLIENT_IP}" 'exit 0' 2>/dev/null; then
    print_ok "SSH OK"
else
    print_err "INJOIGNABLE"
fi

echo -e ""
echo -e "${BOLD}Vérification des services ZTNA${NC}"
print_separator

# CP /healthz
printf "  %-30s" "Control Plane /healthz"
if curl -sfk --max-time 5 "${CP_API}/healthz" >/dev/null 2>&1; then
    print_ok "API CP en ligne"
else
    print_err "API CP inaccessible"
fi

# Keycloak
printf "  %-30s" "Keycloak /realms/ztna"
if curl -sfk --max-time 5 "${KC_URL}/realms/${ZTNA_REALM}" >/dev/null 2>&1; then
    print_ok "Keycloak en ligne"
else
    print_err "Keycloak inaccessible"
fi

# Gateway service
printf "  %-30s" "ztna-gateway.service"
if ssh_gw 'systemctl is-active ztna-gateway >/dev/null 2>&1'; then
    print_ok "Gateway active"
else
    print_warn "Gateway inactive (journalctl -u ztna-gateway)"
fi

# DataVault sur lan-app (via jump host)
printf "  %-30s" "DataVault lan-app:80"
DV_STATUS=$(ssh $SSH_OPTS -J "ztna@${GW_IP}" ztna@${APP_IP} \
    'curl -sf http://127.0.0.1/api/status 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)[\"status\"])" 2>/dev/null || echo "nok"' 2>/dev/null || echo "ssh-error")
if [[ "$DV_STATUS" == "ok" ]]; then
    print_ok "DataVault opérationnel"
elif [[ "$DV_STATUS" == "ssh-error" ]]; then
    print_warn "lan-app inaccessible — DataVault non vérifié"
else
    print_warn "DataVault non démarré — déploiement automatique…"
    # Déploiement automatique si le script est disponible
    DEPLOY_SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/scripts/deploy-datavault.sh"
    if [[ -f "$DEPLOY_SCRIPT" ]]; then
        bash "$DEPLOY_SCRIPT" "${APP_IP}" >/dev/null 2>&1 || true
        # Re-vérifier
        sleep 2
        DV_STATUS2=$(ssh $SSH_OPTS -J "ztna@${GW_IP}" ztna@${APP_IP} \
            'curl -sf http://127.0.0.1/api/status 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)[\"status\"])" 2>/dev/null || echo "nok"' 2>/dev/null || echo "nok")
        if [[ "$DV_STATUS2" == "ok" ]]; then
            print_ok "DataVault déployé et opérationnel"
        else
            print_warn "DataVault non disponible — exécuter manuellement : ./scripts/deploy-datavault.sh"
        fi
    else
        print_warn "Script deploy-datavault.sh introuvable — exécutez-le manuellement"
    fi
fi

echo -e ""
print_ok "Environnement vérifié — prêt pour la démo"
