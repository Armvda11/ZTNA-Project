#!/usr/bin/env bash
# ============================================================================
# ZTNA Full-Demo Bootstrap
# ============================================================================
#
# Prend en charge un poste qui vient de "git pull", vérifie chaque couche
# de l'infrastructure et déploie ce qui manque, puis lance demo-interactive.
#
# Étapes :
#   [0] Prérequis hôte          → commandes, libvirt, SSH key
#   [1] VMs                     → créées ? démarrées ? SSH joignable ?
#   [2] Control Plane           → service répondant ? sinon make deploy
#   [3] Gateway                 → service actif ? sinon make deploy-gw
#   [4] PostgreSQL (optionnel)  → déployé ? sinon proposer make deploy-db
#   [5] Validation              → double-check complet avant le lancement
#   [6] Lancement               → make demo-interactive
#
# Comportement clé :
#   - Chaque étape est idempotente (vérifie avant d'agir)
#   - Aucune vérification d'espace disque / RAM
#   - Terraform NON lancé automatiquement (trop destructif)
#     → si les VMs n'existent pas, l'utilisateur est guidé
#   - Ctrl+C à tout moment pour annuler proprement
#
# Usage :
#   make full-demo
#   # ou directement :
#   bash scripts/full-demo.sh
#   bash scripts/full-demo.sh --no-db    # sauter PostgreSQL
#   bash scripts/full-demo.sh --yes      # valider toutes les prompts auto
# ============================================================================

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

# ── Options ──────────────────────────────────────────────────────────────────
SKIP_DB=false
AUTO_YES=false
for arg in "$@"; do
  case "$arg" in
    --no-db)  SKIP_DB=true ;;
    --yes|-y) AUTO_YES=true ;;
  esac
done

# ── Variables d'infrastructure ────────────────────────────────────────────────
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10"
SSH="ssh ${SSH_OPTS} -i ${SSH_KEY}"
VIRSH="bash ${ROOT_DIR}/scripts/virsh-lab"

CP_IP="10.10.20.30"
GW_IP="10.10.10.20"
CLIENT_IP="10.10.10.10"
APP_IP="10.10.30.10"
ADMIN_IP="10.10.30.11"

CP_API="https://${CP_IP}:8080"
KC_URL="https://${CP_IP}:8443"

keycloak_realm_up() {
  curl -sfk --max-time 5 "${KC_URL}/realms/ztna" >/dev/null 2>&1 || \
  curl -sfk --max-time 5 "${KC_URL}/auth/realms/ztna" >/dev/null 2>&1
}

VMS=(wan-client ztna-gw ztna-cp lan-app lan-admin)
VM_IPS=("${CLIENT_IP}" "${GW_IP}" "${CP_IP}" "${APP_IP}" "${ADMIN_IP}")

# ── Compteurs ─────────────────────────────────────────────────────────────────
WARNINGS=0
ERRORS=0

# ── Couleurs ──────────────────────────────────────────────────────────────────
RST='\033[0m'
BOLD='\033[1m'
DIM='\033[2m'
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'

# ── Helpers ───────────────────────────────────────────────────────────────────
ok()    { echo -e "  ${GREEN}✓${RST}  $*"; }
skip()  { echo -e "  ${CYAN}–${RST}  $*  ${DIM}(déjà OK)${RST}"; }
warn()  { echo -e "  ${YELLOW}⚠${RST}  $*"; WARNINGS=$((WARNINGS + 1)); }
fail()  { echo -e "  ${RED}✗${RST}  $*"; ERRORS=$((ERRORS + 1)); }
info()  { echo -e "\n  ${BLUE}●${RST}  ${BOLD}$*${RST}"; }
action(){ echo -e "  ${YELLOW}→${RST}  $*"; }
sep()   { echo -e "  ${DIM}────────────────────────────────────────────────────────${RST}"; }

header() {
  local step="$1" title="$2"
  echo ""
  echo -e "${BOLD}${CYAN}  ╔══ Étape ${step} ══ ${title} ══${RST}"
  sep
}

confirm() {
  local msg="$1" default="${2:-y}"
  if $AUTO_YES; then
    echo -e "  ${DIM}[auto-yes] ${msg}${RST}"
    return 0
  fi
  printf "  %b ?  [Y/n]  " "$msg"
  local answer
  read -r answer
  answer="${answer:-y}"
  [[ "${answer,,}" == "y" ]]
}

run_make() {
  local target="$1"
  action "make ${target}"
  echo ""
  if ! make -C "${ROOT_DIR}" "${target}"; then
    fail "make ${target} a échoué"
    return 1
  fi
  return 0
}

summary_box() {
  local title="$1" color="$2"
  echo ""
  echo -e "${color}${BOLD}"
  echo "  ╔═══════════════════════════════════════════════════════════╗"
  printf "  ║  %-57s║\n" "$title"
  echo "  ╚═══════════════════════════════════════════════════════════╝"
  echo -e "${RST}"
}

# ── Trap ──────────────────────────────────────────────────────────────────────
trap 'echo -e "\n\n  ${YELLOW}[Ctrl+C]${RST} Arrêt demandé.\n"; exit 130' INT TERM

# ============================================================================
# BANNER
# ============================================================================

clear
echo ""
echo -e "${BOLD}${CYAN}"
echo "  ╔══════════════════════════════════════════════════════════════╗"
echo "  ║         ZTNA Full-Demo Bootstrap — One-Shot Setup           ║"
echo "  ║                                                              ║"
echo "  ║  • Vérifie chaque couche de l'infrastructure                ║"
echo "  ║  • Déploie automatiquement ce qui manque                    ║"
echo "  ║  • Lance la démo interactive 100% réelle à la fin           ║"
echo "  ╚══════════════════════════════════════════════════════════════╝"
echo -e "${RST}"
echo -e "  ${DIM}Root: ${ROOT_DIR}${RST}"
echo -e "  ${DIM}SSH key: ${SSH_KEY}${RST}"
$SKIP_DB  && echo -e "  ${DIM}PostgreSQL: ignoré (--no-db)${RST}"
$AUTO_YES && echo -e "  ${DIM}Mode auto-yes actif${RST}"

# ============================================================================
# ÉTAPE 0 — Prérequis hôte
# ============================================================================

header "0/5" "Prérequis hôte"

# SSH key
if [[ -f "${SSH_KEY}" ]]; then
  ok "Clé SSH trouvée : ${SSH_KEY}"
else
  fail "Clé SSH introuvable : ${SSH_KEY}"
  echo -e "  ${DIM}  → Créez-la : ssh-keygen -t ed25519 -f ${SSH_KEY} -N ''${RST}"
  ERRORS=$((ERRORS + 1))
fi

# Commandes obligatoires (sans RAM/disque)
for entry in \
  "virsh:Libvirt CLI" \
  "ssh:SSH client" \
  "curl:curl" \
  "go:Go compiler" \
  "openssl:OpenSSL" \
  "python3:Python 3"
do
  cmd="${entry%%:*}"
  label="${entry##*:}"
  if command -v "$cmd" >/dev/null 2>&1; then
    ok "$label ($cmd)"
  else
    fail "$label requis : $cmd introuvable"
    [[ "$cmd" == "go" ]] && echo -e "  ${DIM}  → https://go.dev/dl/${RST}"
    [[ "$cmd" == "virsh" ]] && echo -e "  ${DIM}  → sudo apt install libvirt-clients${RST}"
  fi
done

# Groupe libvirt
if id -Gn 2>/dev/null | grep -q '\blibvirt\b'; then
  ok "Utilisateur dans le groupe libvirt"
else
  warn "Utilisateur hors groupe libvirt → sudo usermod -aG libvirt,kvm \$USER && newgrp libvirt"
fi

# libvirtd
if systemctl is-active --quiet libvirtd 2>/dev/null || \
   systemctl is-active --quiet virtqemud 2>/dev/null; then
  ok "Service libvirtd/virtqemud actif"
else
  warn "libvirtd inactif → sudo systemctl start libvirtd"
fi

# Virtualisation CPU
if grep -qE 'vmx|svm' /proc/cpuinfo 2>/dev/null; then
  ok "Virtualisation CPU détectée (VT-x/AMD-V)"
else
  warn "Virtualisation CPU non détectée (les VMs peuvent ne pas se lancer)"
fi

# Outils fenêtres (non bloquants)
for entry in "xdotool:xdotool" "wmctrl:wmctrl"; do
  cmd="${entry%%:*}"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    warn "${cmd} absent → positionnement automatique des fenêtres désactivé (sudo apt install ${cmd})"
  fi
done

if [[ $ERRORS -gt 0 ]]; then
  summary_box "⚠ ${ERRORS} prérequis manquant(s) — corrigez puis relancez" "$RED"
  exit 1
fi

ok "Tous les prérequis hôte sont satisfaits"

# ============================================================================
# ÉTAPE 1 — VMs
# ============================================================================

header "1/5" "Machines Virtuelles KVM"

# Vérifier si les VMs existent dans libvirt
VMS_FOUND=0
VMS_TOTAL=${#VMS[@]}
for vm in "${VMS[@]}"; do
  if ${VIRSH} domstate "${vm}" >/dev/null 2>&1; then
    VMS_FOUND=$((VMS_FOUND + 1))
  fi
done

if [[ $VMS_FOUND -eq 0 ]]; then
  fail "Aucune VM ZTNA trouvée dans libvirt"
  echo ""
  echo -e "  ${YELLOW}Les VMs ne sont pas encore créées.${RST}"
  echo -e "  Il faut lancer Terraform pour les créer :"
  echo -e "  ${DIM}    cd ${ROOT_DIR}/lab/terraform${RST}"
  echo -e "  ${DIM}    terraform init && terraform apply${RST}"
  echo -e "  ${DIM}    # ou : make up (depuis la racine du projet)${RST}"
  echo ""
  if confirm "Lancer make up maintenant (terraform apply, ~5-10 min)"; then
    run_make "up" || { fail "Terraform apply a échoué"; exit 1; }
    echo ""
    ok "VMs créées via Terraform"
  else
    fail "VMs manquantes — arrêt"
    exit 1
  fi
elif [[ $VMS_FOUND -lt $VMS_TOTAL ]]; then
  warn "Seulement ${VMS_FOUND}/${VMS_TOTAL} VMs trouvées dans libvirt"
else
  ok "${VMS_FOUND}/${VMS_TOTAL} VMs présentes dans libvirt"
fi

# Démarrer les VMs qui ne tournent pas
VMS_DOWN=()
for vm in "${VMS[@]}"; do
  state=$(${VIRSH} domstate "${vm}" 2>/dev/null || echo "absent")
  if [[ "$state" != "running" && "$state" != "absent" ]]; then
    VMS_DOWN+=("$vm")
  fi
done

if [[ ${#VMS_DOWN[@]} -gt 0 ]]; then
  action "VMs à démarrer : ${VMS_DOWN[*]}"
  run_make "lab-start" || warn "Certaines VMs n'ont pas pu démarrer"
else
  skip "Toutes les VMs sont déjà en état running"
fi

# Attendre la connectivité SSH sur les VMs WAN (accès direct)
info "Vérification SSH"
WAN_VMS=("wan-client:${CLIENT_IP}" "ztna-gw:${GW_IP}" "ztna-cp:${CP_IP}")
ALL_SSH_OK=true
for entry in "${WAN_VMS[@]}"; do
  name="${entry%%:*}"; ip="${entry##*:}"
  echo -ne "  ${DIM}  SSH ${name} (${ip}) ...${RST} "
  if timeout 12 ${SSH} ztna@"${ip}" 'echo ok' >/dev/null 2>&1; then
    echo -e "${GREEN}✓${RST}"
  else
    echo -e "${YELLOW}⚠ inaccessible${RST}"
    ALL_SSH_OK=false
  fi
done

# Jump host vers LAN
echo -ne "  ${DIM}  SSH lan-app (${APP_IP}) via jump ztna-gw ...${RST} "
if timeout 15 ${SSH} -J ztna@"${GW_IP}" ztna@"${APP_IP}" 'echo ok' >/dev/null 2>&1; then
  echo -e "${GREEN}✓${RST}"
else
  echo -e "${YELLOW}⚠ inaccessible${RST}"
  warn "lan-app inaccessible (le LAN est isolé — vérifiez que ztna-gw est joignable)"
fi

if ! $ALL_SSH_OK; then
  warn "Certaines VMs WAN sont inaccessibles — les déploiements suivants peuvent échouer"
  if ! confirm "Continuer quand même"; then
    echo ""
    echo -e "  ${DIM}  Attendez que les VMs finissent leur boot (cloud-init ~90s) puis relancez.${RST}"
    exit 1
  fi
fi

# ============================================================================
# ÉTAPE 2 — Control Plane
# ============================================================================

header "2/5" "Control Plane"

CP_OK=false
echo -ne "  ${DIM}  GET ${CP_API}/healthz ...${RST} "
if curl -sfk --max-time 5 "${CP_API}/healthz" >/dev/null 2>&1; then
  echo -e "${GREEN}✓ UP${RST}"
  CP_OK=true
else
  echo -e "${YELLOW}pas de réponse${RST}"
fi

echo -ne "  ${DIM}  GET ${KC_URL}/realms/ztna ...${RST} "
if keycloak_realm_up; then
  echo -e "${GREEN}✓ Keycloak UP${RST}"
  KC_OK=true
else
  echo -e "${YELLOW}pas de réponse${RST}"
  KC_OK=false
fi

if $CP_OK && $KC_OK; then
  skip "Control Plane + Keycloak déjà opérationnels"
else
  if $CP_OK && ! $KC_OK; then
    warn "CP répond mais Keycloak non — Keycloak démarre peut-être encore (docker)"
    echo -e "  ${DIM}  Vérifiez avec : ssh ztna@${CP_IP} 'docker ps'${RST}"
  fi

  action "Déploiement du Control Plane..."
  if ! confirm "Lancer make deploy (build Go + copie sur ztna-cp + démarrage Keycloak, ~2-3 min)"; then
    warn "Control Plane non déployé — la démo peut échouer"
  else
    run_make "deploy" || { fail "make deploy a échoué"; exit 1; }

    # Attendre que Keycloak soit UP (max 300s)
    echo ""
    action "Attente de Keycloak (max 300s)..."
    KC_WAIT=0
    until keycloak_realm_up; do
      sleep 5
      KC_WAIT=$((KC_WAIT + 5))
      printf "    %ds..." "$KC_WAIT"
      if [[ $((KC_WAIT % 30)) -eq 0 ]]; then
        printf "\n"
        echo -e "  ${DIM}  état container keycloak:${RST}"
        timeout 8 ${SSH} ztna@"${CP_IP}" "docker ps --format '{{.Names}}  {{.Status}}' | grep -i keycloak || true" 2>/dev/null || true
      fi
      if [[ $KC_WAIT -ge 300 ]]; then
        echo ""
        warn "Keycloak n'a pas répondu en 300s"
        echo -e "  ${DIM}  Vérifiez avec : ssh ztna@${CP_IP} 'cd ztna/control-plane/keycloak && docker compose ps && docker compose logs --tail=80 keycloak'${RST}"
        break
      fi
    done
    echo ""

    if keycloak_realm_up; then
      ok "Control Plane + Keycloak opérationnels"
    else
      warn "Keycloak toujours absent — la démo peut échouer au step OIDC"
    fi
  fi
fi

# ============================================================================
# ÉTAPE 3 — Gateway
# ============================================================================

header "3/5" "Gateway"

GW_OK=false
echo -ne "  ${DIM}  service ztna-gateway sur ${GW_IP} ...${RST} "
if timeout 8 ${SSH} ztna@"${GW_IP}" 'systemctl is-active ztna-gateway' >/dev/null 2>&1; then
  echo -e "${GREEN}✓ active${RST}"
  GW_OK=true
else
  echo -e "${YELLOW}absent ou inactif${RST}"
fi

if $GW_OK; then
  skip "Gateway déjà déployé et actif"
else
  action "Déploiement du Gateway..."
  if ! confirm "Lancer make deploy-gw (build Go + déploiement sur ztna-gw, ~1-2 min)"; then
    warn "Gateway non déployé — la démo échouera aux étapes mTLS/SSH"
  else
    run_make "deploy-gw" || { fail "make deploy-gw a échoué"; exit 1; }

    # Vérification post-deploy
    sleep 3
    if timeout 8 ${SSH} ztna@"${GW_IP}" 'systemctl is-active ztna-gateway' >/dev/null 2>&1; then
      ok "Gateway opérationnel"
    else
      warn "Gateway ne répond toujours pas — vérifiez : ssh ztna@${GW_IP} 'journalctl -u ztna-gateway -n 30'"
    fi
  fi
fi

# ============================================================================
# ÉTAPE 4 — PostgreSQL (optionnel)
# ============================================================================

header "4/5" "PostgreSQL (optionnel — scénario DB)"

if $SKIP_DB; then
  skip "PostgreSQL ignoré (--no-db)"
else
  PG_OK=false
  echo -ne "  ${DIM}  pg_isready sur lan-app:5432 (via jump) ...${RST} "
  if timeout 12 ${SSH} -J ztna@"${GW_IP}" ztna@"${APP_IP}" 'pg_isready -q' >/dev/null 2>&1; then
    echo -e "${GREEN}✓ opérationnel${RST}"
    PG_OK=true
  else
    echo -e "${YELLOW}absent${RST}"
  fi

  if $PG_OK; then
    skip "PostgreSQL déjà déployé sur lan-app"
  else
    echo ""
    echo -e "  ${DIM}  PostgreSQL permet de démonter le scénario 3 (accès DB via mTLS ZTNA).${RST}"
    echo -e "  ${DIM}  Sans lui les scénarios SSH et HTTP fonctionnent parfaitement.${RST}"
    echo ""
    if confirm "Déployer PostgreSQL sur lan-app maintenant (make deploy-db, ~2 min)"; then
      if ! run_make "deploy-db"; then
        warn "make deploy-db a échoué — scénario DB indisponible"
        echo -e "  ${DIM}Diagnostic rapide lan-app (DNS/routage) :${RST}"
        timeout 15 ${SSH} -J ztna@"${GW_IP}" ztna@"${APP_IP}" "
          echo '  --- /etc/resolv.conf ---'; cat /etc/resolv.conf || true;
          echo '  --- ip route ---'; ip route || true;
          echo '  --- DNS archive.ubuntu.com ---'; getent hosts archive.ubuntu.com || echo DNS_FAIL;
          echo '  --- DNS security.ubuntu.com ---'; getent hosts security.ubuntu.com || echo DNS_FAIL;
        " 2>/dev/null || true
      fi

      sleep 3
      if timeout 12 ${SSH} -J ztna@"${GW_IP}" ztna@"${APP_IP}" 'pg_isready -q' >/dev/null 2>&1; then
        ok "PostgreSQL opérationnel sur lan-app"
      else
        warn "PostgreSQL toujours absent — scénario DB non disponible"
      fi
    else
      skip "PostgreSQL ignoré — scénarios SSH et HTTP disponibles"
    fi
  fi
fi

# ============================================================================
# ÉTAPE 5 — Validation finale
# ============================================================================

header "5/5" "Validation finale"

FINAL_ERRORS=0

check_final() {
  local label="$1" pass="$2"
  if $pass; then
    ok "$label"
  else
    fail "$label"
    FINAL_ERRORS=$((FINAL_ERRORS + 1))
  fi
}

# SSH WAN
for entry in "wan-client:${CLIENT_IP}" "ztna-gw:${GW_IP}" "ztna-cp:${CP_IP}"; do
  name="${entry%%:*}"; ip="${entry##*:}"
  ok_flag=false
  timeout 8 ${SSH} ztna@"${ip}" 'echo ok' >/dev/null 2>&1 && ok_flag=true
  check_final "SSH $name (${ip})" $ok_flag
done

# Control Plane API
cp_up=false
curl -sfk --max-time 5 "${CP_API}/healthz" >/dev/null 2>&1 && cp_up=true
check_final "Control Plane API (${CP_API}/healthz)" $cp_up

# Keycloak
kc_up=false
keycloak_realm_up && kc_up=true
check_final "Keycloak OIDC (${KC_URL}/realms/ztna)" $kc_up

# Gateway service
gw_active=false
timeout 8 ${SSH} ztna@"${GW_IP}" 'systemctl is-active ztna-gateway' >/dev/null 2>&1 && gw_active=true
check_final "Gateway service (ztna-gateway)" $gw_active

# Token OIDC test réel
echo -ne "  ${DIM}  Test token OIDC alice@keycloak ...${RST} "
TOKEN_RESP=$(curl -sk --max-time 10 \
  -d "client_id=ztna-control-plane&username=alice&password=Password123!&grant_type=password" \
  "${KC_URL}/realms/ztna/protocol/openid-connect/token" 2>/dev/null || echo "")
if echo "${TOKEN_RESP}" | python3 -c "import sys,json; d=json.load(sys.stdin); assert d.get('access_token')" >/dev/null 2>&1; then
  echo -e "${GREEN}✓ token obtenu${RST}"
  ok_flag=true
else
  echo -e "${YELLOW}⚠ échec${RST}"
  ok_flag=false
fi
check_final "Token OIDC (alice / ztna-control-plane)" $ok_flag

# PostgreSQL (non bloquant)
if ! $SKIP_DB; then
  pg_up=false
  timeout 12 ${SSH} -J ztna@"${GW_IP}" ztna@"${APP_IP}" 'pg_isready -q' >/dev/null 2>&1 && pg_up=true
  if $pg_up; then
    ok "PostgreSQL opérationnel (scénario 3 disponible)"
  else
    warn "PostgreSQL absent — scénario 3 (DB) non disponible (scénarios 1+2 fonctionnent)"
  fi
fi

echo ""
sep

# Résumé
if [[ $FINAL_ERRORS -gt 0 ]]; then
  summary_box "⚠ ${FINAL_ERRORS} vérification(s) échouée(s)" "$RED"
  echo -e "  ${RED}Les vérifications essentielles ont échoué.${RST}"
  echo -e "  ${DIM}  Corrigez les points indiqués ci-dessus puis relancez.${RST}"
  echo ""
  if ! confirm "Lancer quand même la démo (peut échouer en cours de route)"; then
    exit 1
  fi
else
  summary_box "✓ Infrastructure ZTNA opérationnelle — prête pour la démo" "$GREEN"
fi

# ============================================================================
# LANCEMENT DE LA DÉMO
# ============================================================================

echo ""
echo -e "  ${BOLD}${CYAN}Lancement de la démonstration interactive...${RST}"
echo ""
echo -e "  ${DIM}  5 fenêtres vont s'ouvrir sur votre bureau :${RST}"
echo -e "  ${DIM}    • CONTROLLER   → navigation étape par étape${RST}"
echo -e "  ${DIM}    • CLIENT       → exécution des vraies commandes${RST}"
echo -e "  ${DIM}    • FLUX         → diagrammes d'architecture${RST}"
echo -e "  ${DIM}    • GATEWAY      → logs journalctl en direct${RST}"
echo -e "  ${DIM}    • CTRL PLANE   → logs journalctl en direct${RST}"
echo ""
echo -e "  ${DIM}  Scénarios disponibles : [1] SSH  [2] HTTP  [3] PostgreSQL${RST}"
echo ""
sleep 1

exec bash "${SCRIPT_DIR}/demo-interactive.sh"
