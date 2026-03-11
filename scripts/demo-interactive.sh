#!/usr/bin/env bash
# ============================================================================
# ZTNA Interactive Demo — REAL Operations on Real Infrastructure
# ============================================================================
#
# Ouvre 5 fenêtres séparées sur le bureau :
#
#   ┌───────────────────────┐  ┌───────────────────────┐
#   │  🖥️  CONTROLLER       │  │  📊 FLUX / DIAGRAMME  │
#   │  (navigation)         │  │  (architecture)       │
#   └───────────────────────┘  └───────────────────────┘
#   ┌───────────────┐ ┌───────────────┐ ┌───────────────┐
#   │ 👤 CLIENT     │ │ 🌐 GATEWAY    │ │ 🔒 CTRL PLANE │
#   │ REAL OPS      │ │ LIVE LOGS     │ │ LIVE LOGS     │
#   └───────────────┘ └───────────────┘ └───────────────┘
#
# TOUT EST RÉEL :
#   - Vrais tokens OIDC depuis Keycloak
#   - Vrais certificats signés par le Control Plane
#   - Vrais tunnels mTLS vers le Gateway
#   - Vrais logs journalctl en temps réel
#   - Vrai accès aux ressources (SSH, HTTP, PostgreSQL)
#
# Usage:
#   bash scripts/demo-interactive.sh
#
# Modes internes (lancés automatiquement) :
#   --display flow     Boucle d'affichage pour diagrammes (fenêtre FLUX)
#   --controller       Fenêtre de contrôle interactive
#   --client           Exécution des commandes réelles (fenêtre CLIENT)
#   --live-logs <cp|gw> Logs journalctl en direct
#
# Prérequis :
#   - Lab démarré (make lab-start)
#   - CP + GW déployés (make deploy && make deploy-gw)
#   - Pour PostgreSQL : make deploy-db
# ============================================================================

set -uo pipefail

# ============================================================================
# CONSTANTS
# ============================================================================

SCRIPT_PATH="$(readlink -f "$0")"
SCRIPT_DIR="$(dirname "$SCRIPT_PATH")"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DEMO_DIR="/tmp/ztna-demo"
PID_FILE="$DEMO_DIR/pids"

# Network
CP_IP="10.10.20.30"
GW_IP="10.10.10.20"
CLIENT_IP="10.10.10.10"
APP_IP="10.10.30.10"
ADMIN_IP="10.10.30.11"

# SSH (for VM access — NOT ZTNA, just lab management)
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10"
SSH_CMD="ssh ${SSH_OPTS} -i ${SSH_KEY}"

# OIDC / Keycloak
KC_URL="https://${CP_IP}:8443"
KC_REALM="ztna"
KC_CLIENT="ztna-control-plane"
ZTNA_USER="alice"
ZTNA_PASS="Password123!"

# Control Plane API
CP_API="https://${CP_IP}:8080"

# Gateway
GW_PORT="4433"

# Colors (ANSI)
RST='\033[0m'
BOLD='\033[1m'
DIM='\033[2m'
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
WHITE='\033[1;37m'
BG_GREEN='\033[42m'
BG_BLUE='\033[44m'
BG_GRAY='\033[100m'
BG_RED='\033[41m'
FG_BLACK='\033[30m'

# Scenario defaults
SCENARIO=1
RES_TYPE="ssh"
RES_NAME="ssh-dev-01"
RES_DESC="Serveur SSH Backend"
RES_BACKEND=""
LOCAL_PORT=2222

# ============================================================================
# UTILITY FUNCTIONS
# ============================================================================

trigger() { echo "$RANDOM$RANDOM" > "$DEMO_DIR/epoch-flow"; }

set_flow() {
  printf '%b\n' "$@" > "$DEMO_DIR/pane-flow"
  trigger
}

load_scenario() {
  [[ -f "$DEMO_DIR/scenario.env" ]] && source "$DEMO_DIR/scenario.env"
}

save_scenario() {
  cat > "$DEMO_DIR/scenario.env" << EOF
SCENARIO=$SCENARIO
RES_TYPE=$RES_TYPE
RES_NAME=$RES_NAME
RES_DESC="$RES_DESC"
RES_BACKEND="$RES_BACKEND"
LOCAL_PORT=$LOCAL_PORT
EOF
}

signal_client() { echo "$1" > "$DEMO_DIR/client-step"; }

progress_bar() {
  local step=$1 total=${2:-8}
  local pct=$((step * 100 / total))
  local filled=$((step * 30 / total))
  local empty=$((30 - filled))
  local bar=""
  for ((i=0; i<filled; i++)); do bar+="▓"; done
  for ((i=0; i<empty; i++)); do bar+="░"; done
  printf '%s %d%%' "$bar" "$pct"
}

# ============================================================================
# FLOW DIAGRAMS — Educational architecture diagrams (FLOW window)
# ============================================================================

flow_step_0() {
  set_flow \
    "" \
    "  ${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════════════════════╗${RST}" \
    "  ${BOLD}${CYAN}║              ARCHITECTURE ZTNA — ZERO TRUST NETWORK ACCESS                 ║${RST}" \
    "  ${BOLD}${CYAN}║                     ★ Opérations 100% Réelles ★                            ║${RST}" \
    "  ${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════════════════════╝${RST}" \
    "" \
    "  ${GREEN}  CLIENT${RST}              ${YELLOW}  GATEWAY${RST}                  ${CYAN}  CONTROL PLANE${RST}        ${MAGENTA}  RESOURCES${RST}" \
    "  ${GREEN}  ┌──────┐${RST}            ${YELLOW}  ┌──────────┐${RST}              ${CYAN}  ┌────────────┐${RST}      ${MAGENTA}  ┌────────┐${RST}" \
    "  ${GREEN}  │ CLI  │${RST}            ${YELLOW}  │  PEP     │${RST}              ${CYAN}  │  PDP       │${RST}      ${MAGENTA}  │ SSH    │${RST}" \
    "  ${GREEN}  │      │${RST}   mTLS     ${YELLOW}  │  Proxy   │${RST}   authz      ${CYAN}  │  PKI       │${RST}      ${MAGENTA}  │ HTTP   │${RST}" \
    "  ${GREEN}  │ OIDC │${RST}  ════════  ${YELLOW}  │  CRL     │${RST}  ═════════   ${CYAN}  │  Policy    │${RST}      ${MAGENTA}  │ PgSQL  │${RST}" \
    "  ${GREEN}  │ Cert │${RST}   TLS1.3   ${YELLOW}  │  SSRF    │${RST}   PEP auth   ${CYAN}  │  OIDC      │${RST}      ${MAGENTA}  │        │${RST}" \
    "  ${GREEN}  └──────┘${RST}            ${YELLOW}  └──────────┘${RST}              ${CYAN}  └────────────┘${RST}      ${MAGENTA}  └────────┘${RST}" \
    "  ${DIM}  localhost${RST}            ${DIM}  ${GW_IP}${RST}                ${DIM}  ${CP_IP}${RST}        ${DIM}  ${APP_IP}${RST}" \
    "  ${DIM}  (dev machine)${RST}        ${DIM}  (WAN/DMZ/LAN)${RST}              ${DIM}  (DMZ)${RST}              ${DIM}  (LAN isolé)${RST}" \
    "" \
    "  ${GREEN}✓${RST} Vrais tokens OIDC    ${GREEN}✓${RST} Vrais certificats X.509    ${GREEN}✓${RST} Vrais logs journalctl" \
    "  ${GREEN}✓${RST} Vrai tunnel mTLS     ${GREEN}✓${RST} Vraie autorisation PEP     ${GREEN}✓${RST} Vrai accès ressources"
}

flow_step_1() {
  load_scenario

  local pg_status="${RED}✗ non déployé (make deploy-db)${RST}"
  if timeout 5 ${SSH_CMD} -J ztna@${GW_IP} ztna@${APP_IP} "pg_isready -q" >/dev/null 2>&1; then
    pg_status="${GREEN}✓ opérationnel${RST}"
  fi

  set_flow \
    "" \
    "  ${BOLD}Scénarios de démonstration — Ressources RÉELLES${RST}" \
    "" \
    "  ${BOLD}${GREEN}  [1]${RST}  ${BOLD}Accès SSH${RST} — Flux 1 (certificat SSH signé par le CP)" \
    "       OIDC → SSH Cert → Jump Host ztna-gw → lan-app:22" \
    "       ${GREEN}✓ opérationnel${RST}" \
    "" \
    "  ${BOLD}${BLUE}  [2]${RST}  ${BOLD}Accès HTTP${RST} — Flux 2 (certificat Device mTLS)" \
    "       OIDC → Device Cert → mTLS Gateway:${GW_PORT} → nginx lan-app:80" \
    "       ${GREEN}✓ opérationnel${RST}" \
    "" \
    "  ${BOLD}${YELLOW}  [3]${RST}  ${BOLD}Accès PostgreSQL${RST} — Flux 2 (mTLS TCP tunnel)" \
    "       OIDC → Device Cert → mTLS Gateway:${GW_PORT} → pg lan-app:5432" \
    "       ${pg_status}" \
    "" \
    "  ${DIM}  Appuyez sur 1, 2 ou 3 dans la fenêtre CONTROLLER${RST}" \
    "" \
    "  ${DIM}  Infrastructure : 5 VMs KVM/libvirt, 3 zones réseau (WAN/DMZ/LAN)${RST}" \
    "  ${DIM}  LAN isolé (mode=none) — accessible uniquement via le gateway ZTNA${RST}"
}

flow_step_2() {
  set_flow \
    "" \
    "  ${BOLD}Flux OIDC — Resource Owner Password Grant (RÉEL)${RST}" \
    "" \
    "    ${GREEN}CLIENT${RST}                                              ${CYAN}KEYCLOAK (IdP)${RST}" \
    "    ┌──────┐                                            ┌────────────────┐" \
    "    │      │                                             │  Realm: ztna   │" \
    "    │      │ ─── POST /token ─────────────────────────► │                │" \
    "    │      │     client_id=ztna-control-plane            │  Authentifier  │" \
    "    │      │     username=alice                          │  alice         │" \
    "    │      │     password=****                           │                │" \
    "    │      │     grant_type=password                     │  Vérifier...   │" \
    "    │      │                                             │  ✓             │" \
    "    │      │ ◄── access_token (JWT signé RS256) ──────── │                │" \
    "    │      │     + refresh_token                         │  Claims :      │" \
    "    └──────┘                                            │  sub, groups,  │" \
    "                                                        │  exp, iss      │" \
    "    ${DIM}Endpoint réel :${RST}                                    └────────────────┘" \
    "    ${DIM}${KC_URL}/realms/${KC_REALM}/protocol/openid-connect/token${RST}" \
    "" \
    "    ${YELLOW}Le token JWT contient les groupes Keycloak (ztna-admins, ztna-dba)${RST}" \
    "    ${YELLOW}utilisés ensuite pour l'autorisation ABAC par le Control Plane.${RST}"
}

flow_step_3() {
  load_scenario
  if [[ "$RES_TYPE" == "ssh" ]]; then
    set_flow \
      "" \
      "  ${BOLD}Émission de Certificat SSH — PKI ZTNA (RÉEL)${RST}" \
      "" \
      "    ${GREEN}CLIENT${RST}                                          ${CYAN}CONTROL PLANE (SSH CA)${RST}" \
      "    ┌──────┐                                        ┌──────────────────┐" \
      "    │      │ ── 1. ssh-keygen -t ed25519             │                  │" \
      "    │      │                                        │                  │" \
      "    │ key  │ ── 2. POST /credentials/ssh-cert ────► │  Valider JWT     │" \
      "    │ pub  │      {public_key, principals}           │  Extraire groups │" \
      "    │      │      + Authorization: Bearer JWT        │  Signer cert     │" \
      "    │      │                                        │  (SSH CA key)    │" \
      "    │      │ ◄── 3. {certificate: ssh-ed25519-cert} │  TTL: 15min      │" \
      "    └──────┘                                        └──────────────────┘" \
      "" \
      "    ${YELLOW}Le certificat SSH est signé par la CA du CP (ssh_ca).${RST}" \
      "    ${YELLOW}Les VMs cibles (ztna-gw, lan-app) ont TrustedUserCAKeys configuré.${RST}" \
      "    ${YELLOW}Aucune distribution de clés nécessaire — trust par CA.${RST}"
  else
    set_flow \
      "" \
      "  ${BOLD}Émission de Certificat Device X.509 — PKI ZTNA (RÉEL)${RST}" \
      "" \
      "    ${GREEN}CLIENT${RST}                                          ${CYAN}CONTROL PLANE (Device CA)${RST}" \
      "    ┌──────┐                                        ┌──────────────────┐" \
      "    │      │ ── 1. openssl ecparam P-256 genkey      │                  │" \
      "    │      │ ── 2. openssl req -new (CSR)            │                  │" \
      "    │      │                                        │                  │" \
      "    │ CSR  │ ── 3. POST /credentials/device-cert ──►│  Valider JWT     │" \
      "    │      │      {csr_pem} + Bearer JWT             │  Extraire groups │" \
      "    │      │                                        │  Signer CSR      │" \
      "    │      │                                        │  (Device CA key) │" \
      "    │      │ ◄── 4. {certificate_pem: X.509 PEM} ── │  TTL: 24h        │" \
      "    └──────┘                                        └──────────────────┘" \
      "" \
      "    ${YELLOW}Le certificat X.509 encode les groupes OIDC dans le champ Organization.${RST}" \
      "    ${YELLOW}CN=alice, O=ztna-admins — utilisé pour TLS 1.3 client auth (mTLS).${RST}"
  fi
}

flow_step_4() {
  set_flow \
    "" \
    "  ${BOLD}Découverte des Ressources Publiées (RÉEL)${RST}" \
    "" \
    "    ${GREEN}CLIENT${RST}                                          ${CYAN}CONTROL PLANE${RST}" \
    "    ┌──────┐                                        ┌──────────────────┐" \
    "    │      │ ── GET /api/v1/resources ─────────────► │  Valider JWT     │" \
    "    │      │    Authorization: Bearer JWT             │  Filtrer par     │" \
    "    │      │                                        │  groupes user    │" \
    "    │ list │ ◄── [{name, type, access_mode, ...}] ── │  Retourner       │" \
    "    └──────┘                                        └──────────────────┘" \
    "" \
    "    ${BOLD}Endpoint réel :${RST} ${CP_API}/api/v1/resources" \
    "" \
    "    ${YELLOW}Les ressources publiées sont définies côté CP (resources.yaml)${RST}" \
    "    ${YELLOW}et filtrées par les groupes de l'utilisateur authentifié.${RST}"
}

flow_step_5() {
  load_scenario
  if [[ "$RES_TYPE" == "ssh" ]]; then
    set_flow \
      "" \
      "  ${BOLD}Connexion SSH via Jump Host ZTNA (RÉEL)${RST}" \
      "" \
      "    ${GREEN}CLIENT${RST}         ${DIM}SSH Cert Auth${RST}        ${YELLOW}ZTNA-GW (Jump)${RST}       ${MAGENTA}LAN-APP${RST}" \
      "    ┌──────┐                          ┌──────────┐         ┌────────┐" \
      "    │      │ ══ SSH + Cert ══════════► │ Verify   │         │        │" \
      "    │ ssh  │    ed25519-cert           │ CA Trust │ SSH     │ sshd   │" \
      "    │  -J  │    principals: [ztna]     │ ✓        │ ═════► │ ztna@  │" \
      "    │      │                          │ Forward  │         │        │" \
      "    │      │ ◄═══════════════════════ │ ◄═══════ │ ◄═════ │        │" \
      "    └──────┘                          └──────────┘         └────────┘" \
      "    ${DIM}localhost${RST}                        ${DIM}${GW_IP}${RST}            ${DIM}${APP_IP}${RST}" \
      "" \
      "    ${YELLOW}ssh -i id_ztna_alice -i id_ztna_alice-cert.pub -J ztna@${GW_IP} ztna@${APP_IP}${RST}" \
      "    ${DIM}TrustedUserCAKeys sur ztna-gw ET lan-app → trust par CA ZTNA${RST}"
  else
    set_flow \
      "" \
      "  ${BOLD}Tunnel mTLS + Autorisation PEP + TCP Proxy (RÉEL)${RST}" \
      "" \
      "    ${GREEN}CLIENT${RST}          ${DIM}TLS 1.3 mTLS${RST}       ${YELLOW}GATEWAY (PEP)${RST}        ${CYAN}CP (PDP)${RST}" \
      "    ┌──────┐                         ┌──────────┐        ┌────────────┐" \
      "    │      │ ═ 1. ClientHello ══════►│ Verify   │        │            │" \
      "    │ cert │ ═ 2. Client Cert ══════►│ Chain ✓  │        │            │" \
      "    │ key  │                         │ CRL ✓    │        │            │" \
      "    │      │ ═ 3. ConnectRequest ═══►│          │        │            │" \
      "    │      │    {resource, action}    │ 4. authz │══════►│  ABAC eval │" \
      "    │      │                         │          │◄══════│  allow/deny│" \
      "    │      │ ◄═ 5. ConnectResponse ═ │ 5. cache │        │            │" \
      "    │      │    {allowed, decision}   │ 6. proxy │═══► ${MAGENTA}BACKEND${RST}        │" \
      "    └──────┘                         └──────────┘        └────────────┘" \
      "    ${DIM}localhost${RST}                       ${DIM}${GW_IP}:${GW_PORT}${RST}        ${DIM}${CP_IP}:8443${RST}" \
      "" \
      "    ${YELLOW}Protocole : JSON newline-delimited sur TLS 1.3 (TLS_AES_256_GCM)${RST}" \
      "    ${YELLOW}Après ConnectResponse(allowed), le gateway fait io.Copy bidirectionnel${RST}"
  fi
}

flow_step_6() {
  load_scenario
  set_flow \
    "" \
    "  ${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════════════════════╗${RST}" \
    "  ${BOLD}${CYAN}║          DÉMONSTRATION TERMINÉE — Opérations 100% Réelles ✓                ║${RST}" \
    "  ${BOLD}${CYAN}╠══════════════════════════════════════════════════════════════════════════════╣${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Token OIDC réel depuis Keycloak (${CP_IP}:8443)                       ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Certificat réel signé par le Control Plane (${CP_IP}:8080)            ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Ressources publiées réelles du CP                                    ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Accès réel à ${RES_NAME} (${RES_TYPE}) via ZTNA                      ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Logs réels journalctl des services CP + Gateway                      ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}╠══════════════════════════════════════════════════════════════════════════════╣${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${BOLD}Scénario :${RST} ${RES_DESC}                                              ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${BOLD}Flux :${RST} OIDC → Cert → mTLS/SSH → PEP Authorize → Proxy → Access       ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}╠══════════════════════════════════════════════════════════════════════════════╣${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${DIM}Infrastructure : 5 VMs KVM, 3 zones réseau, LAN isolé${RST}                    ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${DIM}Aucune simulation — tout est exécuté sur l'infrastructure réelle${RST}         ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════════════════════╝${RST}" \
    "" \
    "  ${DIM}  [1] Autre scénario   [b] Retour   [q] Quitter${RST}"
}

# ============================================================================
# CLIENT EXECUTION — Real commands (CLIENT window)
# ============================================================================

client_header() {
  echo -e "\n  ${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RST}"
  echo -e "  ${BOLD}${CYAN}  $1${RST}"
  echo -e "  ${BOLD}${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RST}"
}

client_step_0() {
  clear
  echo -e "${BOLD}${CYAN}"
  echo "    ╔══════════════════════════════════════════════╗"
  echo "    ║  ZTNA — Client Terminal (Real Operations)   ║"
  echo "    ╚══════════════════════════════════════════════╝"
  echo -e "${RST}"

  client_header "Pre-flight — Vérification de l'infrastructure"
  echo ""

  local all_ok=true

  for entry in "${CLIENT_IP}:wan-client" "${GW_IP}:ztna-gw" "${CP_IP}:ztna-cp"; do
    local ip="${entry%%:*}" name="${entry##*:}"
    echo -ne "  [${name}] SSH ${ip} ... "
    if timeout 5 ${SSH_CMD} ztna@"${ip}" 'echo ok' >/dev/null 2>&1; then
      echo -e "${GREEN}✓ accessible${RST}"
    else
      echo -e "${RED}✗ inaccessible${RST}"
      all_ok=false
    fi
  done

  echo -ne "  [lan-app] SSH ${APP_IP} (via jump host) ... "
  if timeout 8 ${SSH_CMD} -J ztna@"${GW_IP}" ztna@"${APP_IP}" 'echo ok' >/dev/null 2>&1; then
    echo -e "${GREEN}✓ accessible${RST}"
  else
    echo -e "${YELLOW}✗ inaccessible${RST}"
  fi

  echo ""
  echo -ne "  [control-plane] ${CP_API}/healthz ... "
  if curl -sfk --max-time 3 "${CP_API}/healthz" >/dev/null 2>&1; then
    echo -e "${GREEN}✓ UP${RST}"
  else
    echo -e "${RED}✗ DOWN${RST}"
    all_ok=false
  fi

  echo -ne "  [keycloak] ${KC_URL}/realms/${KC_REALM} ... "
  if curl -sfk --max-time 5 "${KC_URL}/realms/${KC_REALM}" >/dev/null 2>&1; then
    echo -e "${GREEN}✓ UP${RST}"
  else
    echo -e "${RED}✗ DOWN${RST}"
    all_ok=false
  fi

  echo -ne "  [gateway] ztna-gateway.service ... "
  if timeout 5 ${SSH_CMD} ztna@"${GW_IP}" "systemctl is-active ztna-gateway" >/dev/null 2>&1; then
    echo -e "${GREEN}✓ active${RST}"
  else
    echo -e "${RED}✗ inactive${RST}"
    all_ok=false
  fi

  echo -ne "  [postgresql] lan-app:5432 ... "
  if timeout 8 ${SSH_CMD} -J ztna@"${GW_IP}" ztna@"${APP_IP}" "pg_isready -q" >/dev/null 2>&1; then
    echo -e "${GREEN}✓ opérationnel${RST}"
  else
    echo -e "${DIM}— non déployé (optionnel : make deploy-db)${RST}"
  fi

  echo ""
  if $all_ok; then
    echo -e "  ${GREEN}${BOLD}✓ Infrastructure ZTNA opérationnelle — prête pour la démo${RST}"
  else
    echo -e "  ${RED}${BOLD}⚠ Certains composants sont inaccessibles${RST}"
    echo -e "  ${DIM}  Vérifiez avec : make check${RST}"
  fi
}

client_step_1() {
  clear
  echo -e "${BOLD}${CYAN}"
  echo "    ╔══════════════════════════════════════════════╗"
  echo "    ║  ZTNA — Client Terminal (Real Operations)   ║"
  echo "    ╚══════════════════════════════════════════════╝"
  echo -e "${RST}"
  echo -e "  ${DIM}En attente de la sélection du scénario dans CONTROLLER...${RST}"
}

client_step_2() {
  load_scenario
  clear
  client_header "Étape 2 — Authentification OIDC (RÉELLE)"
  echo ""
  echo -e "  ${BOLD}\$ ztna login --provider keycloak${RST}"
  echo -e "  ${DIM}Authentification via OIDC Resource Owner Password Grant${RST}"
  echo ""
  echo -e "  ${DIM}→ POST ${KC_URL}/realms/${KC_REALM}/protocol/openid-connect/token${RST}"
  echo -e "  ${DIM}  client_id=${KC_CLIENT}  username=${ZTNA_USER}  grant_type=password${RST}"
  echo ""

  local TOKEN_RESP
  TOKEN_RESP=$(curl -sk \
    -d "client_id=${KC_CLIENT}&username=${ZTNA_USER}&password=${ZTNA_PASS}&grant_type=password" \
    "${KC_URL}/realms/${KC_REALM}/protocol/openid-connect/token" 2>&1)

  local ACCESS_TOKEN
  ACCESS_TOKEN=$(echo "$TOKEN_RESP" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('access_token',''))" 2>/dev/null || true)

  if [[ -z "$ACCESS_TOKEN" ]]; then
    echo -e "  ${RED}✗ Token OIDC non obtenu${RST}"
    echo -e "  ${DIM}Réponse Keycloak: ${TOKEN_RESP:0:300}${RST}"
    return 1
  fi

  echo "$ACCESS_TOKEN" > "$DEMO_DIR/oidc_token"

  local PAYLOAD
  PAYLOAD=$(echo "$ACCESS_TOKEN" | cut -d. -f2 | tr '_-' '/+' | \
    python3 -c "import sys,base64,json; raw=sys.stdin.read().strip(); \
    padded=raw+'='*(-len(raw)%4); d=json.loads(base64.b64decode(padded)); \
    print(json.dumps(d,indent=2))" 2>/dev/null || echo "{}")

  local USERNAME GROUPS EXP_TS EXP_HR
  USERNAME=$(echo "$PAYLOAD" | python3 -c "import sys,json; print(json.load(sys.stdin).get('preferred_username','?'))" 2>/dev/null || echo "?")
  GROUPS=$(echo "$PAYLOAD" | python3 -c "import sys,json; g=json.load(sys.stdin).get('groups',[]); print(', '.join(g) if g else '—')" 2>/dev/null || echo "?")
  EXP_TS=$(echo "$PAYLOAD" | python3 -c "import sys,json; print(json.load(sys.stdin).get('exp',0))" 2>/dev/null || echo "0")
  EXP_HR=$(python3 -c "import datetime; print(datetime.datetime.fromtimestamp(${EXP_TS}).strftime('%H:%M:%S'))" 2>/dev/null || echo "?")

  echo -e "  ${GREEN}${BOLD}✓ Token OIDC obtenu${RST} (${#ACCESS_TOKEN} caractères)"
  echo ""
  echo -e "  ${BOLD}  Claims JWT :${RST}"
  echo -e "    Utilisateur : ${BOLD}${USERNAME}${RST}"
  echo -e "    Groupes     : ${BOLD}${GROUPS}${RST}"
  echo -e "    Expire à    : ${EXP_HR}"
  echo -e "    Token       : ${DIM}${ACCESS_TOKEN:0:60}...${RST}"
}

client_step_3() {
  load_scenario
  clear

  local TOKEN
  TOKEN=$(cat "$DEMO_DIR/oidc_token" 2>/dev/null || true)
  [[ -z "$TOKEN" ]] && { echo -e "  ${RED}✗ Token OIDC manquant — exécutez l'étape 2${RST}"; return 1; }

  if [[ "$RES_TYPE" == "ssh" ]]; then
    client_step_3_ssh_cert "$TOKEN"
  else
    client_step_3_device_cert "$TOKEN"
  fi
}

client_step_3_ssh_cert() {
  local TOKEN="$1"
  client_header "Étape 3 — Émission Certificat SSH (RÉEL)"
  echo ""
  echo -e "  ${BOLD}\$ ztna cert --type ssh${RST}"
  echo -e "  ${DIM}Génération clé Ed25519 + demande de certificat SSH au CP${RST}"
  echo ""

  local KEY_FILE="$DEMO_DIR/id_ztna_alice"
  local CERT_FILE="${KEY_FILE}-cert.pub"

  echo -e "  ${DIM}→ ssh-keygen -t ed25519 -f ${KEY_FILE}${RST}"
  rm -f "$KEY_FILE" "$KEY_FILE.pub" "$CERT_FILE"
  ssh-keygen -t ed25519 -f "$KEY_FILE" -N "" -C "ztna-${ZTNA_USER}" -q
  echo -e "  ${GREEN}✓${RST} Clé Ed25519 générée"

  local PUB_KEY
  PUB_KEY=$(cat "${KEY_FILE}.pub")

  echo -e "  ${DIM}→ POST ${CP_API}/api/v1/credentials/ssh-cert${RST}"
  echo -e "  ${DIM}  principals: [ztna, ${ZTNA_USER}]${RST}"
  echo ""

  local CERT_RESP
  CERT_RESP=$(curl -sk \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"public_key\": \"${PUB_KEY}\", \"principals\": [\"ztna\", \"${ZTNA_USER}\"]}" \
    "${CP_API}/api/v1/credentials/ssh-cert" 2>&1)

  local CERT
  CERT=$(echo "$CERT_RESP" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('certificate',''))" 2>/dev/null || true)

  if [[ -z "$CERT" ]]; then
    echo -e "  ${RED}✗ Certificat SSH non obtenu${RST}"
    echo -e "  ${DIM}Réponse CP: ${CERT_RESP:0:300}${RST}"
    return 1
  fi

  echo "$CERT" > "$CERT_FILE"
  chmod 600 "$CERT_FILE"

  echo -e "  ${GREEN}${BOLD}✓ Certificat SSH obtenu${RST}"
  echo ""
  echo -e "  ${BOLD}  Détails du certificat :${RST}"
  ssh-keygen -L -f "$CERT_FILE" 2>/dev/null | grep -E '^\s+(Type|Key ID|Valid|Principals|Serial)' | \
    while IFS= read -r line; do echo -e "    ${line}"; done
  echo ""
  echo -e "  ${DIM}  Fichiers : ${KEY_FILE}  +  ${CERT_FILE}${RST}"
}

client_step_3_device_cert() {
  local TOKEN="$1"
  client_header "Étape 3 — Émission Certificat Device X.509 (RÉEL)"
  echo ""
  echo -e "  ${BOLD}\$ ztna cert --type device${RST}"
  echo -e "  ${DIM}Génération bi-clé ECDSA P-256 + CSR + signature par le CP${RST}"
  echo ""

  local DEVICE_KEY="$DEMO_DIR/device.key"
  local DEVICE_CSR="$DEMO_DIR/device.csr"
  local DEVICE_CRT="$DEMO_DIR/device.crt"

  echo -e "  ${DIM}→ openssl ecparam -name prime256v1 -genkey${RST}"
  openssl ecparam -name prime256v1 -genkey -noout -out "$DEVICE_KEY" 2>/dev/null
  echo -e "  ${GREEN}✓${RST} Clé ECDSA P-256 générée"

  echo -e "  ${DIM}→ openssl req -new -subj '/CN=${ZTNA_USER}/O=ztna-admins'${RST}"
  openssl req -new -key "$DEVICE_KEY" \
    -subj "/CN=${ZTNA_USER}/O=ztna-admins" \
    -out "$DEVICE_CSR" 2>/dev/null
  echo -e "  ${GREEN}✓${RST} CSR généré"

  echo ""
  echo -e "  ${DIM}→ POST ${CP_API}/api/v1/credentials/device-cert${RST}"

  local CSR_JSON
  CSR_JSON=$(python3 -c "import json; print(json.dumps(open('${DEVICE_CSR}').read()))" 2>/dev/null)

  local CERT_RESP
  CERT_RESP=$(curl -sk \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "{\"csr_pem\": ${CSR_JSON}}" \
    "${CP_API}/api/v1/credentials/device-cert" 2>&1)

  local CERT_PEM
  CERT_PEM=$(echo "$CERT_RESP" | python3 -c \
    "import sys,json; d=json.load(sys.stdin); print(d.get('certificate_pem',''))" 2>/dev/null || true)

  if [[ -z "$CERT_PEM" ]]; then
    echo -e "  ${RED}✗ Certificat device non obtenu${RST}"
    echo -e "  ${DIM}Réponse CP: ${CERT_RESP:0:300}${RST}"
    return 1
  fi

  echo "$CERT_PEM" > "$DEVICE_CRT"
  chmod 600 "$DEVICE_CRT"

  local SUBJECT SERIAL EXPIRY ISSUER
  SUBJECT=$(openssl x509 -noout -subject -in "$DEVICE_CRT" 2>/dev/null | sed 's/subject= *//')
  SERIAL=$(openssl x509 -noout -serial -in "$DEVICE_CRT" 2>/dev/null | sed 's/serial=//')
  EXPIRY=$(openssl x509 -noout -enddate -in "$DEVICE_CRT" 2>/dev/null | sed 's/notAfter=//')
  ISSUER=$(openssl x509 -noout -issuer -in "$DEVICE_CRT" 2>/dev/null | sed 's/issuer= *//')

  echo ""
  echo -e "  ${GREEN}${BOLD}✓ Certificat Device X.509 obtenu${RST}"
  echo ""
  echo -e "  ${BOLD}  Détails du certificat :${RST}"
  echo -e "    Subject : ${SUBJECT}"
  echo -e "    Serial  : ${SERIAL}"
  echo -e "    Issuer  : ${ISSUER}"
  echo -e "    Expire  : ${EXPIRY}"
  echo ""
  echo -e "  ${DIM}  Fichiers : ${DEVICE_KEY}  +  ${DEVICE_CRT}${RST}"
}

client_step_4() {
  load_scenario
  clear
  client_header "Étape 4 — Découverte des Ressources Publiées (RÉEL)"
  echo ""
  echo -e "  ${BOLD}\$ ztna resources${RST}"
  echo -e "  ${DIM}Récupération des ressources publiées depuis le CP${RST}"
  echo ""

  local TOKEN
  TOKEN=$(cat "$DEMO_DIR/oidc_token" 2>/dev/null || true)
  [[ -z "$TOKEN" ]] && { echo -e "  ${RED}✗ Token OIDC manquant${RST}"; return 1; }

  echo -e "  ${DIM}→ GET ${CP_API}/api/v1/resources${RST}"
  echo ""

  local RESOURCES_RESP
  RESOURCES_RESP=$(curl -sk \
    -H "Authorization: Bearer ${TOKEN}" \
    "${CP_API}/api/v1/resources" 2>&1)

  local PARSED
  PARSED=$(echo "$RESOURCES_RESP" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    resources = data if isinstance(data, list) else data.get('resources', data.get('items', [data]))
    if not isinstance(resources, list):
        resources = [resources]
    print(f'  Nombre de ressources : {len(resources)}')
    print()
    fmt = '  {:<22} {:<8} {:<14} {:<28} {}'
    print(fmt.format('NOM', 'TYPE', 'ACCESS', 'BACKEND', 'DESCRIPTION'))
    print(fmt.format('───', '────', '──────', '───────', '───────────'))
    for r in resources:
        name = r.get('name', '?')
        rtype = r.get('type', '?')
        access = r.get('access_mode', '?')
        backend = r.get('backend', '?')
        desc = r.get('description', r.get('display_name', ''))[:40]
        print(fmt.format(name, rtype, access, backend, desc))
except Exception as e:
    print(f'  (Parsing error: {e})')
" 2>/dev/null)

  if [[ -n "$PARSED" ]]; then
    echo "$PARSED"
  else
    echo -e "  ${DIM}Réponse brute :${RST}"
    echo "$RESOURCES_RESP" | python3 -m json.tool 2>/dev/null | head -30 || echo "$RESOURCES_RESP" | head -10
  fi

  echo ""
  echo -e "  ${GREEN}${BOLD}✓ Ressources récupérées depuis le Control Plane${RST}"
}

client_step_5() {
  load_scenario
  clear

  case "$RES_TYPE" in
    ssh)  client_step_5_ssh ;;
    http) client_step_5_http ;;
    db)   client_step_5_db ;;
  esac
}

client_step_5_ssh() {
  client_header "Étape 5 — Connexion SSH via ZTNA (RÉEL)"
  echo ""
  echo -e "  ${BOLD}\$ ztna access ${RES_NAME}${RST}"
  echo ""

  local KEY_FILE="$DEMO_DIR/id_ztna_alice"
  local CERT_FILE="${KEY_FILE}-cert.pub"

  if [[ ! -f "$KEY_FILE" || ! -f "$CERT_FILE" ]]; then
    echo -e "  ${RED}✗ Certificat SSH manquant — exécutez l'étape 3${RST}"
    echo "done" > "$DEMO_DIR/client-done"
    return 1
  fi

  echo -e "  ${BOLD}Commande SSH réelle :${RST}"
  echo -e "  ${DIM}ssh -i ${KEY_FILE} -i ${CERT_FILE} \\${RST}"
  echo -e "  ${DIM}    -J ztna@${GW_IP} ztna@${APP_IP}${RST}"
  echo ""
  echo -e "  ${YELLOW}→ Session SSH interactive — tapez 'exit' pour terminer${RST}"
  echo -e "  ${DIM}───────────────────────────────────────────────────────${RST}"
  echo ""

  ssh ${SSH_OPTS} \
    -i "$KEY_FILE" \
    -i "$CERT_FILE" \
    -J "ztna@${GW_IP}" \
    "ztna@${APP_IP}" || true

  echo ""
  echo -e "  ${DIM}───────────────────────────────────────────────────────${RST}"
  echo -e "  ${GREEN}${BOLD}✓ Session SSH terminée${RST}"
  echo "done" > "$DEMO_DIR/client-done"
}

client_step_5_http() {
  client_header "Étape 5 — Accès HTTP via mTLS ZTNA (RÉEL)"
  echo ""
  echo -e "  ${BOLD}\$ ztna access ${RES_NAME}${RST}"
  echo ""

  local DEVICE_CRT="$DEMO_DIR/device.crt"
  local DEVICE_KEY="$DEMO_DIR/device.key"

  if [[ ! -f "$DEVICE_CRT" || ! -f "$DEVICE_KEY" ]]; then
    echo -e "  ${RED}✗ Certificat device manquant — exécutez l'étape 3${RST}"
    echo "done" > "$DEMO_DIR/client-done"
    return 1
  fi

  echo -e "  ${DIM}Tunnel mTLS vers ${GW_IP}:${GW_PORT} avec certificat device${RST}"
  echo ""

  python3 - "${GW_IP}" "${GW_PORT}" "${DEVICE_CRT}" "${DEVICE_KEY}" << 'PYEOF'
import sys, ssl, socket, json

gw_host, gw_port, cert, key = sys.argv[1:]
gw_port = int(gw_port)

ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=cert, keyfile=key)

print(f"  [mTLS] Connexion vers {gw_host}:{gw_port} ...")
raw = socket.create_connection((gw_host, gw_port), timeout=15)
tls = ctx.wrap_socket(raw, server_hostname=gw_host)
print(f"  [mTLS] Handshake OK — protocole: {tls.version()}")
print(f"  [mTLS] Cipher: {tls.cipher()[0]}")

connect_req = json.dumps({
    "resource_type": "http",
    "resource_match": "http:lan-app:80",
    "action": "connect"
})
tls.sendall((connect_req + "\n").encode())
print(f"  [mTLS] ConnectRequest envoyé: {connect_req}")

buf = b""
while b"\n" not in buf:
    chunk = tls.recv(4096)
    if not chunk:
        break
    buf += chunk

resp = json.loads(buf.split(b"\n")[0])
print()
print("  ┌─ ConnectResponse (réel) ────────────────────────────────")
print(f"  │  allowed     : {resp.get('allowed')}")
print(f"  │  decision_id : {resp.get('decision_id', '—')}")
print(f"  │  reason      : {resp.get('reason', '—')}")
print("  └─────────────────────────────────────────────────────────")

if not resp.get("allowed"):
    print(f"\n  [REFUSÉ] Accès non autorisé par le PEP")
    sys.exit(1)

print(f"\n  [AUTORISÉ] Tunnel TCP ouvert → lan-app:80")

http_req = "GET / HTTP/1.0\r\nHost: lan-app\r\n\r\n"
tls.sendall(http_req.encode())
print(f"  [HTTP] GET / HTTP/1.0 envoyé")

tls.settimeout(10.0)
response = b""
try:
    while True:
        data = tls.recv(4096)
        if not data:
            break
        response += data
except:
    pass

if response:
    decoded = response.decode(errors="replace")
    print()
    print("  ┌─ Réponse HTTP depuis lan-app (réelle) ──────────────────")
    for line in decoded.split("\n")[:20]:
        print(f"  │ {line}")
    print("  └─────────────────────────────────────────────────────────")
else:
    print("\n  [ERREUR] Aucune réponse HTTP reçue")

tls.close()
print("\n  ✓ Test mTLS terminé avec succès")
PYEOF

  echo ""
  echo -e "  ${GREEN}${BOLD}✓ Accès HTTP via mTLS ZTNA terminé${RST}"
  echo "done" > "$DEMO_DIR/client-done"
}

client_step_5_db() {
  client_header "Étape 5 — Accès PostgreSQL via mTLS ZTNA (RÉEL)"
  echo ""
  echo -e "  ${BOLD}\$ ztna access ${RES_NAME}${RST}"
  echo ""

  local DEVICE_CRT="$DEMO_DIR/device.crt"
  local DEVICE_KEY="$DEMO_DIR/device.key"

  if [[ ! -f "$DEVICE_CRT" || ! -f "$DEVICE_KEY" ]]; then
    echo -e "  ${RED}✗ Certificat device manquant — exécutez l'étape 3${RST}"
    echo "done" > "$DEMO_DIR/client-done"
    return 1
  fi

  local USE_MTLS_TUNNEL=false

  if command -v psql >/dev/null 2>&1; then
    USE_MTLS_TUNNEL=true
    echo -e "  ${GREEN}✓${RST} psql disponible localement → tunnel mTLS TCP"
  else
    echo -e "  ${DIM}psql non disponible localement → tunnel SSH ZTNA${RST}"

    # Obtenir un cert SSH si nécessaire
    local KEY_FILE="$DEMO_DIR/id_ztna_alice"
    local CERT_FILE="${KEY_FILE}-cert.pub"
    if [[ ! -f "$KEY_FILE" || ! -f "$CERT_FILE" ]]; then
      echo -e "  ${DIM}Demande automatique d'un certificat SSH pour le tunnel...${RST}"
      local TOKEN
      TOKEN=$(cat "$DEMO_DIR/oidc_token" 2>/dev/null || true)
      if [[ -n "$TOKEN" ]]; then
        rm -f "$KEY_FILE" "$KEY_FILE.pub" "$CERT_FILE"
        ssh-keygen -t ed25519 -f "$KEY_FILE" -N "" -C "ztna-${ZTNA_USER}" -q
        local PUB_KEY
        PUB_KEY=$(cat "${KEY_FILE}.pub")
        local CERT_RESP
        CERT_RESP=$(curl -sk \
          -H "Authorization: Bearer ${TOKEN}" \
          -H "Content-Type: application/json" \
          -d "{\"public_key\": \"${PUB_KEY}\", \"principals\": [\"ztna\", \"${ZTNA_USER}\"]}" \
          "${CP_API}/api/v1/credentials/ssh-cert" 2>&1)
        local CERT
        CERT=$(echo "$CERT_RESP" | python3 -c \
          "import sys,json; d=json.load(sys.stdin); print(d.get('certificate',''))" 2>/dev/null || true)
        if [[ -n "$CERT" ]]; then
          echo "$CERT" > "$CERT_FILE"
          chmod 600 "$CERT_FILE"
          echo -e "  ${GREEN}✓${RST} Certificat SSH obtenu pour le tunnel"
        fi
      fi
    fi
  fi

  echo ""

  if $USE_MTLS_TUNNEL; then
    # ── mTLS TCP tunnel via ztna-tcp-tunnel.py ──
    echo -e "  ${BOLD}Tunnel mTLS TCP vers PostgreSQL :${RST}"
    echo -e "  ${DIM}  localhost:${LOCAL_PORT} → mTLS ${GW_IP}:${GW_PORT} → pg lan-app:5432${RST}"
    echo ""

    local READY_FILE="$DEMO_DIR/tunnel-ready"
    rm -f "$READY_FILE"

    TUNNEL_READY_FILE="$READY_FILE" python3 "${SCRIPT_DIR}/ztna-tcp-tunnel.py" \
      --listen "${LOCAL_PORT}" \
      --gateway "${GW_IP}:${GW_PORT}" \
      --cert "$DEVICE_CRT" \
      --key "$DEVICE_KEY" \
      --resource "db:pg-staging" &
    local TUNNEL_PID=$!

    local wait_count=0
    while [[ ! -f "$READY_FILE" && $wait_count -lt 30 ]]; do
      sleep 0.3
      wait_count=$((wait_count + 1))
      if ! kill -0 "$TUNNEL_PID" 2>/dev/null; then
        echo -e "  ${RED}✗ Tunnel mTLS échoué${RST}"
        echo -e "  ${DIM}Vérifiez la route db:pg-staging sur le gateway (make deploy-db)${RST}"
        echo "done" > "$DEMO_DIR/client-done"
        return 1
      fi
    done

    echo -e "  ${GREEN}✓${RST} Tunnel mTLS actif (PID: $TUNNEL_PID)"
    sleep 0.5
    echo ""
    echo -e "  ${BOLD}\$ psql -h localhost -p ${LOCAL_PORT} -U alice appdb${RST}"
    echo -e "  ${YELLOW}→ Session PostgreSQL interactive — tapez '\\q' pour terminer${RST}"
    echo -e "  ${DIM}───────────────────────────────────────────────────────${RST}"
    echo ""

    PGPASSWORD=ztna2026 psql -h localhost -p "${LOCAL_PORT}" -U alice appdb 2>&1 || true

    echo ""
    echo -e "  ${DIM}───────────────────────────────────────────────────────${RST}"
    kill "$TUNNEL_PID" 2>/dev/null; wait "$TUNNEL_PID" 2>/dev/null || true
    echo -e "  ${GREEN}✓${RST} Tunnel mTLS fermé"

  else
    # ── SSH tunnel fallback ──
    local KEY_FILE="$DEMO_DIR/id_ztna_alice"
    local CERT_FILE="${KEY_FILE}-cert.pub"

    echo -e "  ${BOLD}Tunnel SSH ZTNA vers PostgreSQL :${RST}"
    echo -e "  ${DIM}  ssh -J ztna@${GW_IP} ztna@${APP_IP} → psql${RST}"
    echo ""
    echo -e "  ${YELLOW}→ Session PostgreSQL interactive via SSH ZTNA — tapez '\\q' pour terminer${RST}"
    echo -e "  ${DIM}───────────────────────────────────────────────────────${RST}"
    echo ""

    ssh ${SSH_OPTS} \
      -i "$KEY_FILE" \
      -i "$CERT_FILE" \
      -J "ztna@${GW_IP}" \
      "ztna@${APP_IP}" \
      -t "PGPASSWORD=ztna2026 psql -U alice appdb" 2>/dev/null || true

    echo ""
    echo -e "  ${DIM}───────────────────────────────────────────────────────${RST}"
  fi

  echo -e "  ${GREEN}${BOLD}✓ Session PostgreSQL terminée${RST}"
  echo "done" > "$DEMO_DIR/client-done"
}

client_step_6() {
  load_scenario
  clear
  client_header "Résumé de la session"
  echo ""
  echo -e "  ${GREEN}${BOLD}✓ Opérations ZTNA réelles complétées :${RST}"
  echo ""
  echo -e "    ${GREEN}✓${RST}  Authentification OIDC (Keycloak ${CP_IP}:8443)"
  if [[ "$RES_TYPE" == "ssh" ]]; then
    echo -e "    ${GREEN}✓${RST}  Certificat SSH signé par le CP (ZTNA SSH CA)"
  else
    echo -e "    ${GREEN}✓${RST}  Certificat Device X.509 signé par le CP (Device CA)"
  fi
  echo -e "    ${GREEN}✓${RST}  Découverte des ressources publiées (CP API)"
  echo -e "    ${GREEN}✓${RST}  Accès réel à ${BOLD}${RES_NAME}${RST} (${RES_TYPE})"
  echo ""
  echo -e "  ${DIM}  Logs réels visibles dans les fenêtres GATEWAY et CONTROL PLANE${RST}"
  echo -e "  ${DIM}  Aucune donnée simulée — tout a été exécuté sur l'infrastructure réelle${RST}"
}

# ============================================================================
# RUN CLIENT — Watches for step changes, executes real commands
# ============================================================================

run_client() {
  printf '\033]0;ZTNA — CLIENT\007'
  local last_step="-1"

  while [ ! -d "$DEMO_DIR" ]; do sleep 0.2; done

  clear
  echo -e "${BOLD}${CYAN}"
  echo "    ╔══════════════════════════════════════════════╗"
  echo "    ║  ZTNA — Client Terminal (Real Operations)   ║"
  echo "    ╚══════════════════════════════════════════════╝"
  echo -e "${RST}"
  echo -e "  ${DIM}En attente du CONTROLLER...${RST}"

  while true; do
    local current
    current=$(cat "$DEMO_DIR/client-step" 2>/dev/null || echo "")
    if [[ "$current" != "$last_step" && -n "$current" ]]; then
      last_step="$current"
      load_scenario
      "client_step_${current}" 2>&1 || true
    fi
    sleep 0.3
  done
}

# ============================================================================
# LIVE LOGS — SSH journalctl in CP/GW windows
# ============================================================================

run_live_logs() {
  local component="$1"
  local ip="" label="" service=""

  case "$component" in
    cp) ip="$CP_IP"; label="CONTROL PLANE"; service="ztna-cp" ;;
    gw) ip="$GW_IP"; label="GATEWAY"; service="ztna-gateway" ;;
    *)  echo "Usage: $0 --live-logs <cp|gw>"; exit 1 ;;
  esac

  printf '\033]0;ZTNA — %s\007' "$label"
  clear
  echo -e "${BOLD}${CYAN}  ━━━ ${label} — LIVE LOGS ━━━${RST}"
  echo -e "  ${DIM}${ip} — journalctl -u ${service} -f${RST}"
  echo -e "  ${DIM}Les logs réels apparaissent en temps réel${RST}"
  echo ""

  while true; do
    ${SSH_CMD} ztna@"${ip}" \
      "sudo journalctl -u ${service} -f --no-pager --output=short-iso 2>&1" \
      2>/dev/null || true
    echo -e "\n  ${YELLOW}[!]${RST} Connexion perdue — reconnexion dans 3s..."
    sleep 3
    echo -e "  ${DIM}Reconnexion à ${ip}...${RST}"
  done
}

# ============================================================================
# DISPLAY LOOP — File-based display for FLOW window
# ============================================================================

run_display() {
  local pane="$1"
  local last_epoch=""

  printf '\033]0;ZTNA — FLUX\007'

  while [ ! -d "$DEMO_DIR" ]; do sleep 0.2; done

  while true; do
    local epoch
    epoch=$(cat "$DEMO_DIR/epoch-${pane}" 2>/dev/null || echo "")
    if [[ "$epoch" != "$last_epoch" && -n "$epoch" ]]; then
      last_epoch="$epoch"
      printf '\033[H'
      cat "$DEMO_DIR/pane-${pane}" 2>/dev/null || true
      printf '\033[J'
    fi
    sleep 0.12
  done
}

# ============================================================================
# CONTROLLER — Interactive Navigation
# ============================================================================

run_controller() {
  while [ ! -d "$DEMO_DIR" ]; do sleep 0.1; done
  mkdir -p "$DEMO_DIR"

  local step=0
  local max_step=6

  local step_names=(
    "Pre-flight Checks"
    "Sélection du Scénario"
    "Authentification OIDC"
    "Émission de Certificat"
    "Découverte des Ressources"
    "Accès à la Ressource"
    "Résumé"
  )

  while true; do
    clear
    printf '%b\n' \
      "" \
      "  ${BOLD}${CYAN}╔══════════════════════════════════════════════════════════╗${RST}" \
      "  ${BOLD}${CYAN}║     ZTNA — Démonstration Interactive (100% Réelle)      ║${RST}" \
      "  ${BOLD}${CYAN}╚══════════════════════════════════════════════════════════╝${RST}" \
      ""

    if [[ $step -eq 0 ]]; then
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  ${DIM}  Appuyez sur ${RST}${BOLD}ENTER${RST}${DIM} pour lancer les vérifications${RST}" \
        "" \
        "  ${DIM}  Navigation :${RST}" \
        "  ${DIM}    ENTER  → Étape suivante${RST}" \
        "  ${DIM}    b      → Étape précédente${RST}" \
        "  ${DIM}    r      → Rejouer l'étape${RST}" \
        "  ${DIM}    q      → Quitter${RST}" \
        "" \
        "  ${YELLOW}★ Toutes les opérations sont exécutées sur l'infrastructure réelle ★${RST}" \
        "  ${DIM}  CP logs et GW logs sont en temps réel (journalctl -f)${RST}" \
        "  ${DIM}  Fenêtre CLIENT exécute de vrais curl, openssl, ssh, psql${RST}"

    elif [[ $step -eq 1 ]]; then
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  Choisissez le type d'accès à démontrer :" \
        "" \
        "  ${BOLD}${GREEN}  [1]${RST}  Accès SSH        — Flux 1 (SSH cert) → lan-app:22" \
        "  ${BOLD}${BLUE}  [2]${RST}  Accès HTTP       — Flux 2 (mTLS) → nginx lan-app:80" \
        "  ${BOLD}${YELLOW}  [3]${RST}  Accès PostgreSQL — Flux 2 (mTLS TCP) → pg lan-app:5432" \
        "" \
        "  ${DIM}  Appuyez sur 1, 2 ou 3${RST}"

    elif [[ $step -eq $max_step ]]; then
      load_scenario
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}  ${DIM}[${RES_DESC}]${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  ${GREEN}${BOLD}  ✓ Démonstration complète ! (100% réelle)${RST}" \
        "" \
        "  ${DIM}  [b] Retour   [1] Autre scénario   [q] Quitter${RST}"

    elif [[ $step -eq 5 ]]; then
      load_scenario
      local access_hint=""
      case "$RES_TYPE" in
        ssh)  access_hint="Session SSH interactive — tapez 'exit' dans CLIENT pour terminer" ;;
        http) access_hint="Requête HTTP automatique — résultat affiché dans CLIENT" ;;
        db)   access_hint="Session psql interactive — tapez '\\q' dans CLIENT pour terminer" ;;
      esac
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}  ${DIM}[${RES_DESC}]${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  ${YELLOW}★ ${access_hint} ★${RST}" \
        "" \
        "  ${DIM}  [ENTER] Lancer l'accès   [b] Retour   [q] Quitter${RST}"

    else
      load_scenario
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}  ${DIM}[${RES_DESC}]${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  ${DIM}  [ENTER] Suivant   [b] Retour   [r] Rejouer   [q] Quitter${RST}"
    fi

    # Update flow diagram
    "flow_step_${step}" 2>/dev/null &
    local flow_pid=$!

    # Signal client for this step
    signal_client "$step"

    # Wait for user input
    if [[ $step -eq 1 ]]; then
      while true; do
        read -rsn1 key
        case "$key" in
          1) SCENARIO=1; RES_TYPE="ssh"; RES_NAME="ssh-dev-01"
             RES_DESC="Serveur SSH Backend (Flux 1)"
             RES_BACKEND="${APP_IP}:22"; LOCAL_PORT=2222
             save_scenario; step=2; break ;;
          2) SCENARIO=2; RES_TYPE="http"; RES_NAME="grafana-internal"
             RES_DESC="HTTP via mTLS (Flux 2)"
             RES_BACKEND="${APP_IP}:80"; LOCAL_PORT=8888
             save_scenario; step=2; break ;;
          3) SCENARIO=3; RES_TYPE="db"; RES_NAME="pg-staging"
             RES_DESC="PostgreSQL via mTLS (Flux 2)"
             RES_BACKEND="${APP_IP}:5432"; LOCAL_PORT=15432
             save_scenario; step=2; break ;;
          b|B) step=0; break ;;
          q|Q) cleanup_all; exit 0 ;;
        esac
      done
    elif [[ $step -eq 5 ]]; then
      # Resource access step — wait for interactive session to finish
      while true; do
        read -rsn1 key
        case "$key" in
          "")
            rm -f "$DEMO_DIR/client-done"
            signal_client "5"
            echo ""
            echo -e "  ${DIM}  En attente de la fin de la session dans CLIENT...${RST}"
            echo -e "  ${DIM}  (SSH: exit | HTTP: auto | psql: \\q)${RST}"
            local wait_count=0
            while [[ ! -f "$DEMO_DIR/client-done" && $wait_count -lt 1200 ]]; do
              sleep 0.5
              wait_count=$((wait_count + 1))
            done
            step=6; break ;;
          b|B) step=4; break ;;
          q|Q) cleanup_all; exit 0 ;;
        esac
      done
    else
      while true; do
        read -rsn1 key
        case "$key" in
          "") if [[ $step -lt $max_step ]]; then step=$((step + 1)); fi
              break ;;
          b|B) if [[ $step -gt 0 ]]; then step=$((step - 1)); fi
               break ;;
          r|R) break ;;
          q|Q) cleanup_all; exit 0 ;;
          1)   step=1; break ;;
          0)   step=0; break ;;
        esac
      done
    fi

    wait $flow_pid 2>/dev/null || true
  done
}

cleanup_all() {
  printf '\n%b\n\n' "  ${DIM}Fermeture des fenêtres...${RST}"
  sleep 0.3
  pkill -f "ztna-tcp-tunnel.py" 2>/dev/null || true
  if [[ -f "$PID_FILE" ]]; then
    while read -r pid; do kill "$pid" 2>/dev/null; done < "$PID_FILE"
  fi
  rm -rf "$DEMO_DIR"
}

# ============================================================================
# TERMINAL DETECTION & WINDOW MANAGEMENT
# ============================================================================

detect_terminal() {
  for t in konsole gnome-terminal xfce4-terminal mate-terminal lxterminal xterm; do
    if command -v "$t" >/dev/null 2>&1; then
      echo "$t"
      return 0
    fi
  done
  return 1
}

ensure_xdotool() {
  local needs_install=false
  if ! command -v xdotool >/dev/null 2>&1; then needs_install=true; fi
  if ! command -v wmctrl >/dev/null 2>&1; then needs_install=true; fi

  if [[ "$needs_install" == true ]]; then
    echo -e "${CYAN}[ZTNA Demo]${RST} Installation des outils de gestion de fenêtres..."
    if command -v apt-get >/dev/null 2>&1; then
      sudo apt-get install -y -qq xdotool wmctrl >/dev/null 2>&1
    elif command -v dnf >/dev/null 2>&1; then
      sudo dnf install -y -q xdotool wmctrl >/dev/null 2>&1
    elif command -v pacman >/dev/null 2>&1; then
      sudo pacman -S --noconfirm xdotool wmctrl >/dev/null 2>&1
    fi
  fi

  command -v xdotool >/dev/null 2>&1 && command -v wmctrl >/dev/null 2>&1
}

position_window_by_title() {
  local title="$1" x="$2" y="$3" w="$4" h="$5"

  if ! command -v xdotool >/dev/null 2>&1; then return 1; fi

  local wid="" attempts=0
  while [[ -z "$wid" && $attempts -lt 30 ]]; do
    wid=$(xdotool search --name "$title" 2>/dev/null | head -1)
    [[ -n "$wid" ]] && break
    sleep 0.1
    attempts=$((attempts + 1))
  done

  if [[ -n "$wid" ]]; then
    wmctrl -ir "$wid" -b remove,maximized_vert,maximized_horz 2>/dev/null || true
    xdotool windowstate --remove MAXIMIZED_VERT MAXIMIZED_HORZ "$wid" 2>/dev/null || true
    sleep 0.05
    xdotool windowsize --sync "$wid" "$w" "$h" 2>/dev/null
    sleep 0.05
    xdotool windowmove --sync "$wid" "$x" "$y" 2>/dev/null
    xdotool windowraise "$wid" 2>/dev/null
    xdotool windowfocus "$wid" 2>/dev/null
    sleep 0.05
    xdotool windowmove --sync "$wid" "$x" "$y" 2>/dev/null
    return 0
  fi
  return 1
}

# ============================================================================
# MAIN LAUNCHER — Opens 5 windows on the desktop
# ============================================================================

launch_windows() {
  local term
  term=$(detect_terminal) || {
    echo "ERREUR: Aucun émulateur de terminal trouvé (konsole, gnome-terminal, xfce4-terminal, xterm)"
    exit 1
  }
  echo -e "${CYAN}[ZTNA Demo]${RST} Terminal détecté : $term"

  local has_xdotool=false
  if ensure_xdotool; then
    has_xdotool=true
    echo -e "${CYAN}[ZTNA Demo]${RST} xdotool disponible ✓"
  else
    echo -e "${YELLOW}[ZTNA Demo]${RST} xdotool non disponible — pas de positionnement automatique"
  fi

  if [[ -f "$PID_FILE" ]]; then
    while read -r pid; do kill "$pid" 2>/dev/null; done < "$PID_FILE"
  fi
  rm -rf "$DEMO_DIR"
  mkdir -p "$DEMO_DIR"

  echo "" > "$DEMO_DIR/pane-flow"
  echo "0" > "$DEMO_DIR/epoch-flow"
  echo "" > "$DEMO_DIR/client-step"

  save_scenario

  local gap=10
  local margin_top=10
  local margin_bottom=90
  local sw=1920
  local row1_h=340
  local row2_h=500
  local half_w=$(( (sw - gap * 3) / 2 ))
  local third_w=$(( (sw - gap * 4) / 3 ))
  local row2_y=$(( margin_top + row1_h + gap * 2 ))

  echo -e "${CYAN}[ZTNA Demo]${RST} Ouverture de 5 fenêtres séparées..."

  local PIDS=()

  launch_terminal_window() {
    local title="$1" cmd="$2"
    case "$term" in
      konsole)
        konsole --separate --hide-menubar --hide-tabbar --notransparency \
          -p tabtitle="$title" -e bash -c "$cmd" &
        ;;
      gnome-terminal)
        gnome-terminal --window --title="$title" -- bash -c "$cmd" &
        ;;
      xfce4-terminal)
        xfce4-terminal --title="$title" -e "bash -c '$cmd'" &
        ;;
      mate-terminal)
        mate-terminal --window --title="$title" -e "bash -c '$cmd'" &
        ;;
      xterm)
        xterm -T "$title" -fa "Monospace" -fs 10 -e bash -c "$cmd" &
        ;;
    esac
    echo $!
  }

  echo -e "${CYAN}[ZTNA Demo]${RST}   → CONTROLLER..."
  launch_terminal_window "ZTNA — CONTROLLER" "bash '${SCRIPT_PATH}' --controller"
  PIDS+=($!)
  sleep 0.3

  echo -e "${CYAN}[ZTNA Demo]${RST}   → FLUX (diagrams)..."
  launch_terminal_window "ZTNA — FLUX" "bash '${SCRIPT_PATH}' --display flow"
  PIDS+=($!)
  sleep 0.3

  echo -e "${CYAN}[ZTNA Demo]${RST}   → CLIENT (real ops)..."
  launch_terminal_window "ZTNA — CLIENT" "bash '${SCRIPT_PATH}' --client"
  PIDS+=($!)
  sleep 0.3

  echo -e "${CYAN}[ZTNA Demo]${RST}   → GATEWAY (live logs)..."
  launch_terminal_window "ZTNA — GATEWAY" "bash '${SCRIPT_PATH}' --live-logs gw"
  PIDS+=($!)
  sleep 0.3

  echo -e "${CYAN}[ZTNA Demo]${RST}   → CONTROL PLANE (live logs)..."
  launch_terminal_window "ZTNA — CONTROL PLANE" "bash '${SCRIPT_PATH}' --live-logs cp"
  PIDS+=($!)
  sleep 0.2

  printf '%s\n' "${PIDS[@]}" > "$PID_FILE"
  echo -e "${CYAN}[ZTNA Demo]${RST}   → 5 fenêtres créées ✓"

  if [[ "$has_xdotool" == true ]]; then
    sleep 0.8
    echo -e "${CYAN}[ZTNA Demo]${RST} Positionnement automatique..."

    position_window_by_title "ZTNA — CONTROLLER" "$gap" "$margin_top" "$half_w" "$row1_h" && \
      echo -e "${CYAN}[ZTNA Demo]${RST}   ✓ CONTROLLER"
    position_window_by_title "ZTNA — FLUX" "$((half_w + gap * 2))" "$margin_top" "$half_w" "$row1_h" && \
      echo -e "${CYAN}[ZTNA Demo]${RST}   ✓ FLUX"
    position_window_by_title "ZTNA — CLIENT" "$gap" "$row2_y" "$third_w" "$row2_h" && \
      echo -e "${CYAN}[ZTNA Demo]${RST}   ✓ CLIENT"
    position_window_by_title "ZTNA — GATEWAY" "$((third_w + gap * 2))" "$row2_y" "$third_w" "$row2_h" && \
      echo -e "${CYAN}[ZTNA Demo]${RST}   ✓ GATEWAY"
    position_window_by_title "ZTNA — CONTROL PLANE" "$((third_w * 2 + gap * 3))" "$row2_y" "$third_w" "$row2_h" && \
      echo -e "${CYAN}[ZTNA Demo]${RST}   ✓ CONTROL PLANE"

    echo -e "${CYAN}[ZTNA Demo]${RST} Layout terminé ✓"
  fi

  echo ""
  echo -e "${CYAN}[ZTNA Demo]${RST} ┌─────────────────────────────────────────────────────────┐"
  echo -e "${CYAN}[ZTNA Demo]${RST} │  5 fenêtres ouvertes — 100% opérations réelles          │"
  echo -e "${CYAN}[ZTNA Demo]${RST} │                                                         │"
  echo -e "${CYAN}[ZTNA Demo]${RST} │  ► CONTROLLER : naviguer la démo (ENTER / b / q)        │"
  echo -e "${CYAN}[ZTNA Demo]${RST} │  ► CLIENT     : commandes réelles (curl, ssh, psql)     │"
  echo -e "${CYAN}[ZTNA Demo]${RST} │  ► GATEWAY    : vrais logs journalctl en temps réel      │"
  echo -e "${CYAN}[ZTNA Demo]${RST} │  ► CTRL PLANE : vrais logs journalctl en temps réel      │"
  echo -e "${CYAN}[ZTNA Demo]${RST} │  ► FLUX       : diagrammes d'architecture                │"
  echo -e "${CYAN}[ZTNA Demo]${RST} │                                                         │"
  echo -e "${CYAN}[ZTNA Demo]${RST} │  Ctrl+C ici pour tout fermer                            │"
  echo -e "${CYAN}[ZTNA Demo]${RST} └─────────────────────────────────────────────────────────┘"
  echo ""

  trap 'echo ""; echo -e "${CYAN}[ZTNA Demo]${RST} Arrêt..."; pkill -f "ztna-tcp-tunnel.py" 2>/dev/null; for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null; done; rm -rf "$DEMO_DIR"; exit 0' INT TERM

  local ctrl_pid="${PIDS[0]}"
  while kill -0 "$ctrl_pid" 2>/dev/null; do
    sleep 1
  done

  pkill -f "ztna-tcp-tunnel.py" 2>/dev/null || true
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null
  done
  rm -rf "$DEMO_DIR"
  echo -e "${CYAN}[ZTNA Demo]${RST} Terminé."
}

# ============================================================================
# ENTRY POINT
# ============================================================================

case "${1:-}" in
  --display)
    run_display "${2:?Usage: $0 --display flow}"
    ;;
  --controller)
    run_controller
    ;;
  --client)
    run_client
    ;;
  --live-logs)
    run_live_logs "${2:?Usage: $0 --live-logs <cp|gw>}"
    ;;
  *)
    launch_windows
    ;;
esac
