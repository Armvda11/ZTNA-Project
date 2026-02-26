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

echo -e ""
print_ok "Environnement vérifié — prêt pour la démo"
