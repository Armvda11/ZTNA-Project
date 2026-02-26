#!/usr/bin/env bash
# =============================================================================
# demo/lib/ssh.sh — Fonctions SSH utilitaires pour la démo ZTNA
# =============================================================================

SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 -i ${SSH_KEY}"

# IP des VMs (peut être surchargé via variables d'environnement)
CP_IP="${CP_IP:-10.10.20.30}"
GW_IP="${GW_IP:-10.10.10.20}"
CLIENT_IP="${CLIENT_IP:-10.10.10.10}"
APP_IP="${APP_IP:-10.10.30.10}"

# Keycloak
KC_PROTO="${KC_PROTO:-https}"
KC_PORT="${KC_PORT:-8443}"
KC_URL="${KC_PROTO}://${CP_IP}:${KC_PORT}"
CP_API="https://${CP_IP}:8080"

# Utilisateur de démo (par défaut alice)
ZTNA_USER="${ZTNA_USER:-alice}"
ZTNA_PASS="${ZTNA_PASS:-Password123!}"
ZTNA_REALM="${ZTNA_REALM:-ztna}"
ZTNA_CLIENT_ID="${ZTNA_CLIENT_ID:-ztna-control-plane}"

# ─── Helpers SSH de base ──────────────────────────────────────────────────────

# ssh_run HOST COMMAND — Exécute une commande sur un hôte VM
ssh_run() {
    local host="$1"; shift
    ssh $SSH_OPTS "ztna@${host}" "$@"
}

# ssh_run_jump HOST COMMAND — Via jump host ztna-gw (pour LAN)
ssh_run_jump() {
    local host="$1"; shift
    ssh $SSH_OPTS -J "ztna@${GW_IP}" "ztna@${host}" "$@"
}

# ssh_cp COMMAND — Raccourci vers ztna-cp
ssh_cp()     { ssh_run "$CP_IP" "$@"; }
ssh_gw()     { ssh_run "$GW_IP" "$@"; }
ssh_client() { ssh_run "$CLIENT_IP" "$@"; }
ssh_app()    { ssh_run_jump "$APP_IP" "$@"; }

# ─── Attente SSH ──────────────────────────────────────────────────────────────

# wait_ssh HOST [RETRIES=15] [DELAY=2] — Attend que SSH réponde
wait_ssh() {
    local host="$1"
    local retries="${2:-15}"
    local delay="${3:-2}"
    local count=0

    while ! ssh $SSH_OPTS -o ConnectTimeout=3 "ztna@${host}" 'exit 0' 2>/dev/null; do
        count=$((count+1))
        if [[ $count -ge $retries ]]; then
            echo -e "\033[0;31m[✗]\033[0m SSH injoignable sur ${host} après ${retries} tentatives"
            return 1
        fi
        echo -e "\033[1;33m[…]\033[0m Attente SSH ${host} (${count}/${retries})…"
        sleep "$delay"
    done
    echo -e "\033[0;32m[✓]\033[0m SSH OK sur ${host}"
}

# ─── Token OIDC ───────────────────────────────────────────────────────────────

# get_oidc_token [USER] [PASS] — Obtient un access_token via ROPC Keycloak
# Retourne le token ou quitte avec erreur
get_oidc_token() {
    local user="${1:-$ZTNA_USER}"
    local pass="${2:-$ZTNA_PASS}"
    local kc_base="${KC_URL}/realms/${ZTNA_REALM}/protocol/openid-connect/token"

    local response
    response=$(curl -sk --max-time 10 \
        -d "grant_type=password" \
        -d "client_id=${ZTNA_CLIENT_ID}" \
        -d "username=${user}" \
        -d "password=${pass}" \
        "$kc_base" 2>&1)

    local token
    token=$(echo "$response" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['access_token'])" 2>/dev/null || true)

    if [[ -z "$token" ]]; then
        echo -e "\033[0;31m[✗]\033[0m Échec d'obtention du token OIDC" >&2
        echo -e "\033[2m    Réponse: ${response}${NC}" >&2
        return 1
    fi
    echo "$token"
}

# ─── Utilitaires ──────────────────────────────────────────────────────────────

# require_cmd CMD — Vérifie qu'une commande est disponible
require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo -e "\033[0;31m[✗]\033[0m Commande manquante: $1"
        return 1
    fi
}

# check_prereqs — Vérifie les prérequis de la démo
check_prereqs() {
    local ok=true
    for cmd in gnome-terminal ssh curl python3 jq; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            echo -e "\033[0;31m[✗]\033[0m Prérequis manquant: ${cmd}"
            ok=false
        fi
    done
    $ok
}

# short_token TOKEN — Tronque un token JWT pour l'affichage
short_token() {
    local tok="$1"
    echo "${tok:0:40}…"
}
