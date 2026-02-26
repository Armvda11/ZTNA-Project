#!/usr/bin/env bash
# =============================================================================
# scripts/deploy-datavault.sh — Déploiement du serveur DataVault sur lan-app
#
# Déploie l'API DataVault (ressource protégée de la démo ZTNA) sur la VM lan-app.
# Remplace le nginx basique par un vrai serveur d'API JSON simulant des données
# confidentielles d'entreprise.
#
# Usage:
#   ./scripts/deploy-datavault.sh [LAN_APP_IP]
#
# Prérequis:
#   - VM lan-app démarrée et accessible via SSH
#   - Clé SSH configurée (défaut: ~/.ssh/id_rsa)
#   - Python3 disponible sur lan-app (installé par cloud-init)
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "${SCRIPT_DIR}")"

LAN_APP_IP="${1:-10.10.30.10}"
SSH_USER="${SSH_USER:-ztna}"
SSH_KEY="${SSH_KEY:-${HOME}/.ssh/id_rsa}"
SSH_OPTS="-o StrictHostKeyChecking=no -o ConnectTimeout=10 -i ${SSH_KEY}"

DATAVAULT_SRC="${PROJECT_DIR}/demo/app/datavault_server.py"
DATAVAULT_SVC="${PROJECT_DIR}/demo/app/datavault.service"

# ── Couleurs ─────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()      { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()    { echo -e "${RED}[FAIL]${NC}  $*" >&2; exit 1; }

# ── Vérifications préalables ──────────────────────────────────────────────────
echo -e ""
echo -e "${BOLD}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}║       Déploiement DataVault sur lan-app               ║${NC}"
echo -e "${BOLD}╚══════════════════════════════════════════════════════╝${NC}"
echo -e ""

[[ -f "${DATAVAULT_SRC}" ]] || fail "Script DataVault introuvable : ${DATAVAULT_SRC}"
[[ -f "${DATAVAULT_SVC}" ]] || fail "Service systemd introuvable : ${DATAVAULT_SVC}"
[[ -f "${SSH_KEY}" ]]       || fail "Clé SSH introuvable : ${SSH_KEY}"

info "Cible    : ${SSH_USER}@${LAN_APP_IP}"
info "Script   : ${DATAVAULT_SRC}"

# ── Attente SSH ───────────────────────────────────────────────────────────────
info "Attente SSH sur ${LAN_APP_IP} (max 60s)…"
for i in $(seq 1 30); do
    if ssh ${SSH_OPTS} "${SSH_USER}@${LAN_APP_IP}" "echo ok" >/dev/null 2>&1; then
        ok "SSH disponible"
        break
    fi
    if [[ $i -eq 30 ]]; then
        fail "SSH non disponible après 60s sur ${LAN_APP_IP}"
    fi
    sleep 2
done

# ── Arrêt nginx si présent ────────────────────────────────────────────────────
info "Arrêt de nginx (si actif)…"
ssh ${SSH_OPTS} "${SSH_USER}@${LAN_APP_IP}" \
    "sudo systemctl stop nginx 2>/dev/null || true; sudo systemctl disable nginx 2>/dev/null || true"
ok "nginx désactivé"

# ── Création du répertoire ────────────────────────────────────────────────────
info "Création de /opt/datavault…"
ssh ${SSH_OPTS} "${SSH_USER}@${LAN_APP_IP}" "sudo mkdir -p /opt/datavault"

# ── Copie du script ───────────────────────────────────────────────────────────
info "Copie de datavault_server.py…"
scp ${SSH_OPTS} "${DATAVAULT_SRC}" "${SSH_USER}@${LAN_APP_IP}:/tmp/datavault_server.py"
ssh ${SSH_OPTS} "${SSH_USER}@${LAN_APP_IP}" \
    "sudo cp /tmp/datavault_server.py /opt/datavault/ && sudo chmod 755 /opt/datavault/datavault_server.py"
ok "Script copié"

# ── Installation du service systemd ──────────────────────────────────────────
info "Installation du service systemd…"
scp ${SSH_OPTS} "${DATAVAULT_SVC}" "${SSH_USER}@${LAN_APP_IP}:/tmp/datavault.service"
ssh ${SSH_OPTS} "${SSH_USER}@${LAN_APP_IP}" "
    sudo cp /tmp/datavault.service /etc/systemd/system/datavault.service
    sudo systemctl daemon-reload
    sudo systemctl enable datavault
    sudo systemctl restart datavault
"
ok "Service datavault installé et démarré"

# ── Vérification santé ────────────────────────────────────────────────────────
info "Vérification santé du serveur (max 15s)…"
sleep 3
for i in $(seq 1 5); do
    STATUS=$(ssh ${SSH_OPTS} "${SSH_USER}@${LAN_APP_IP}" \
        "curl -sf http://127.0.0.1/api/status 2>/dev/null | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d[\"status\"])' 2>/dev/null || echo 'nok'")
    if [[ "${STATUS}" == "ok" ]]; then
        ok "DataVault répond : status=ok"
        break
    fi
    if [[ $i -eq 5 ]]; then
        warn "Santé non confirmée — vérifier : ssh ${SSH_USER}@${LAN_APP_IP} systemctl status datavault"
    fi
    sleep 3
done

# ── Résumé ────────────────────────────────────────────────────────────────────
echo -e ""
echo -e "${GREEN}${BOLD}✓ DataVault déployé sur lan-app (${LAN_APP_IP})${NC}"
echo -e ""
echo -e "  Endpoints disponibles (via tunnel ZTNA uniquement) :"
echo -e "  ${BOLD}GET${NC} http://lan-app/api/status          → santé"
echo -e "  ${BOLD}GET${NC} http://lan-app/api/vault/records   → enregistrements confidentiels"
echo -e "  ${BOLD}GET${NC} http://lan-app/api/vault/secrets   → secrets TOP SECRET"
echo -e "  ${BOLD}GET${NC} http://lan-app/api/whoami          → identité connexion"
echo -e ""
echo -e "  Logs : ${DIM:-}ssh ${SSH_USER}@${LAN_APP_IP} journalctl -fu datavault${NC}"
echo -e ""
