#!/usr/bin/env bash
# ============================================================================
# ZTNA Interactive Demo — Multi-Window Orchestrated Presentation
# ============================================================================
#
# Ouvre 5 fenêtres séparées sur le bureau, chacune dédiée à un composant :
#
#   ┌───────────────────────┐  ┌───────────────────────┐
#   │  🖥️  CONTROLLER       │  │  📊 FLUX / DIAGRAMME  │
#   │  (navigation)         │  │  (architecture)       │
#   └───────────────────────┘  └───────────────────────┘
#   ┌───────────────┐ ┌───────────────┐ ┌───────────────┐
#   │ 👤 CLIENT     │ │ 🌐 GATEWAY    │ │ 🔒 CTRL PLANE │
#   │ 10.10.10.10   │ │ 10.10.10.20   │ │ 10.10.20.30   │
#   └───────────────┘ └───────────────┘ └───────────────┘
#
# Usage:
#   bash scripts/demo-interactive.sh
#
# Modes internes (lancés automatiquement) :
#   --display <name>    Boucle d'affichage pour une fenêtre satellite
#   --controller        Fenêtre de contrôle interactive
#
# Fonctionne avec : konsole, gnome-terminal, xfce4-terminal, xterm
# ============================================================================

set -uo pipefail

# ============================================================================
# CONSTANTS
# ============================================================================

SCRIPT_PATH="$(readlink -f "$0")"
DEMO_DIR="/tmp/ztna-demo"
PID_FILE="$DEMO_DIR/pids"

# IPs
CP_IP="10.10.20.30"
GW_IP="10.10.10.20"
CLIENT_IP="10.10.10.10"
APP_IP="10.10.30.10"
BACKEND_IP="10.10.30.15"

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
RES_PORT=22
RES_HOST="lan-app"
RES_BACKEND="${BACKEND_IP}:22"
RES_DESC="Serveur SSH Backend"
RES_PROTO="SSH"
RES_NAME="ssh-dev-01"
LOCAL_PORT=2222

# Consistent IDs
CERT_SERIAL="3A:2B:1C:0D:4E:5F:6A:7B"
SESSION_UUID="f47ac10b-58cc-4372-a567-0e02b2c3d479"
DECISION_ID="dec-7f3a21b9-4c5e-48d1-9f2a-e08314a82bc0"
JWT_SUB="auth0|alice-0x29f3"

# Timestamps
TSEC=0
ts() {
  TSEC=$((TSEC + RANDOM % 3 + 1))
  local m=$((30 + TSEC / 60)) s=$((TSEC % 60)) ms=$((RANDOM % 1000))
  printf '2026-03-09T15:%02d:%02d.%03d+01:00' "$m" "$s" "$ms"
}

# ============================================================================
# UTILITY FUNCTIONS
# ============================================================================

trigger() { echo "$RANDOM$RANDOM" > "$DEMO_DIR/epoch-$1"; }

set_pane() {
  local p="$1"; shift
  printf '%b\n' "$@" > "$DEMO_DIR/pane-$p"
  trigger "$p"
}

anim_pane() {
  local p="$1" delay="$2"; shift 2
  > "$DEMO_DIR/pane-$p"
  trigger "$p"
  for line in "$@"; do
    printf '%b\n' "$line" >> "$DEMO_DIR/pane-$p"
    trigger "$p"
    sleep "$delay"
  done
}

hdr_active() {
  local name="$1" ip="$2"
  printf "${BG_GREEN}${FG_BLACK}${BOLD} ● %-18s %-16s %14s ${RST}" "$name" "$ip" "ACTIF"
}

hdr_idle() {
  local name="$1" ip="$2"
  printf "${BG_GRAY}${WHITE} ○ %-18s %-16s %14s ${RST}" "$name" "$ip" "en attente"
}

hdr_done() {
  local name="$1" ip="$2"
  printf "${BG_BLUE}${WHITE}${BOLD} ✓ %-18s %-16s %14s ${RST}" "$name" "$ip" "terminé"
}

progress_bar() {
  local step=$1 total=${2:-10}
  local pct=$((step * 100 / total))
  local filled=$((step * 30 / total))
  local empty=$((30 - filled))
  local bar=""
  for ((i=0; i<filled; i++)); do bar+="▓"; done
  for ((i=0; i<empty; i++)); do bar+="░"; done
  printf '%s %d%%' "$bar" "$pct"
}

jlog() {
  local level="$1" msg="$2"; shift 2
  local t; t=$(ts)
  local extra=""
  while [ $# -gt 0 ]; do
    extra+=",\"$1\":\"$2\""
    shift 2
  done
  printf '{"time":"%s","level":"%s","msg":"%s"%s}' "$t" "$level" "$msg" "$extra"
}

load_scenario() {
  if [[ -f "$DEMO_DIR/scenario.env" ]]; then
    source "$DEMO_DIR/scenario.env"
  fi
}

save_scenario() {
  cat > "$DEMO_DIR/scenario.env" <<EOF
SCENARIO=$SCENARIO
RES_TYPE=$RES_TYPE
RES_PORT=$RES_PORT
RES_HOST=$RES_HOST
RES_BACKEND=$RES_BACKEND
RES_DESC="$RES_DESC"
RES_PROTO=$RES_PROTO
RES_NAME=$RES_NAME
LOCAL_PORT=$LOCAL_PORT
EOF
}

# ============================================================================
# DISPLAY LOOP — runs in each satellite window
# ============================================================================

run_display() {
  local pane="$1"
  local last_epoch=""

  # Set terminal window title
  printf '\033]0;ZTNA — %s\007' "$pane"

  while [ ! -d "$DEMO_DIR" ]; do sleep 0.2; done

  while true; do
    local epoch
    epoch=$(cat "$DEMO_DIR/epoch-$pane" 2>/dev/null || echo "")
    if [[ "$epoch" != "$last_epoch" && -n "$epoch" ]]; then
      last_epoch="$epoch"
      printf '\033[H'
      cat "$DEMO_DIR/pane-$pane" 2>/dev/null || true
      printf '\033[J'
    fi
    sleep 0.12
  done
}

# ============================================================================
# STEP FUNCTIONS (0-10)
# ============================================================================

step_0() {
  set_pane client \
    "$(hdr_idle "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  ${DIM}ZTNA Client CLI v1.0${RST}" \
    "  ${DIM}Prêt pour la démonstration${RST}" \
    "" \
    "  ${DIM}Commandes disponibles :${RST}" \
    "  ${DIM}  ztna login      → Auth OIDC${RST}" \
    "  ${DIM}  ztna cert       → Certificat device${RST}" \
    "  ${DIM}  ztna resources  → Lister ressources publiées${RST}" \
    "  ${DIM}  ztna access     → Accéder à une ressource${RST}"

  set_pane gateway \
    "$(hdr_idle "GATEWAY PEP" "$GW_IP")" \
    "" \
    "  ${DIM}ZTNA Gateway v1.0${RST}" \
    "  ${DIM}mTLS Listener :8443${RST}" \
    "" \
    "  ${DIM}Composants :${RST}" \
    "  ${DIM}  - CRL Auto-Refresh${RST}" \
    "  ${DIM}  - Session Manager${RST}" \
    "  ${DIM}  - Decision Cache${RST}" \
    "  ${DIM}  - SSRF Protection${RST}"

  set_pane cp \
    "$(hdr_idle "CONTROL PLANE" "$CP_IP")" \
    "" \
    "  ${DIM}ZTNA Control Plane v1.0${RST}" \
    "  ${DIM}API :8080 / PEP :8443${RST}" \
    "" \
    "  ${DIM}Services :${RST}" \
    "  ${DIM}  - PKI / CA${RST}" \
    "  ${DIM}  - Policy Engine ABAC${RST}" \
    "  ${DIM}  - Session Store${RST}" \
    "  ${DIM}  - OIDC Validation${RST}"

  set_pane flow \
    "" \
    "  ${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════════════════════╗${RST}" \
    "  ${BOLD}${CYAN}║                     ARCHITECTURE ZTNA — ZERO TRUST                         ║${RST}" \
    "  ${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════════════════════╝${RST}" \
    "" \
    "  ${GREEN}  CLIENT${RST}              ${YELLOW}  GATEWAY${RST}                  ${CYAN}  CONTROL PLANE${RST}        ${MAGENTA}  RESOURCE${RST}" \
    "  ${GREEN}  ┌──────┐${RST}            ${YELLOW}  ┌──────────┐${RST}              ${CYAN}  ┌────────────┐${RST}      ${MAGENTA}  ┌────────┐${RST}" \
    "  ${GREEN}  │ CLI  │${RST}            ${YELLOW}  │  PEP     │${RST}              ${CYAN}  │  PDP       │${RST}      ${MAGENTA}  │ SSH/   │${RST}" \
    "  ${GREEN}  │      │${RST}   mTLS     ${YELLOW}  │  Proxy   │${RST}   authz      ${CYAN}  │  PKI       │${RST}      ${MAGENTA}  │ HTTP   │${RST}" \
    "  ${GREEN}  │ OIDC │${RST}  ════════  ${YELLOW}  │  CRL     │${RST}  ═════════   ${CYAN}  │  Policy    │${RST}      ${MAGENTA}  │        │${RST}" \
    "  ${GREEN}  │ Cert │${RST}   TLS1.3   ${YELLOW}  │  SSRF    │${RST}   PEP auth   ${CYAN}  │  OIDC      │${RST}      ${MAGENTA}  │  :22   │${RST}" \
    "  ${GREEN}  └──────┘${RST}            ${YELLOW}  └──────────┘${RST}              ${CYAN}  └────────────┘${RST}      ${MAGENTA}  │  :80   │${RST}" \
    "  ${DIM}  10.10.10.10${RST}          ${DIM}  10.10.10.20${RST}                ${DIM}  10.10.20.30${RST}        ${MAGENTA}  └────────┘${RST}" \
    "  ${DIM}  (WAN)${RST}                ${DIM}  (WAN/DMZ/LAN)${RST}              ${DIM}  (DMZ)${RST}              ${DIM}  10.10.30.10${RST}"
}

step_1() {
  set_pane flow \
    "" \
    "  ${BOLD}Scénarios de démonstration disponibles :${RST}" \
    "" \
    "  ${BOLD}${GREEN}  [1]${RST}  ${BOLD}Accès SSH${RST} — ztna access ssh-dev-01" \
    "       Ressource publiée : ssh-dev-01 (ssh)  →  Backend : ${BACKEND_IP}:22" \
    "       Politique : allow ztna-admins sur ssh:*" \
    "" \
    "  ${BOLD}${BLUE}  [2]${RST}  ${BOLD}Accès Web${RST} — ztna access grafana-internal" \
    "       Ressource publiée : grafana-internal (web)  →  Backend : ${BACKEND_IP}:3000" \
    "       Politique : allow ztna-admins sur web:*" \
    "" \
    "  ${BOLD}${YELLOW}  [3]${RST}  ${BOLD}Accès PostgreSQL${RST} — ztna access pg-staging" \
    "       Ressource publiée : pg-staging (db)  →  Backend : ${BACKEND_IP}:5432" \
    "       Politique : allow ztna-dba sur db:*" \
    "" \
    "  ${DIM}  Appuyez sur 1, 2 ou 3 dans la fenêtre CONTROLLER${RST}"
}

step_2() {
  load_scenario
  TSEC=0
  set_pane gateway "$(hdr_idle "GATEWAY PEP" "$GW_IP")" "" "  ${DIM}  En attente de connexion client...${RST}"

  anim_pane client 0.18 \
    "$(hdr_active "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  \$ ${BOLD}ztna login --provider keycloak${RST}" \
    "" \
    "  $(jlog INFO "début authentification OIDC" "provider" "keycloak" "realm" "ztna")" \
    "  $(jlog INFO "redirection vers IdP" "url" "https://${CP_IP}:8080/auth/realms/ztna/protocol/openid-connect/auth")" \
    "  ${DIM}  → Ouverture du navigateur... authentification en cours${RST}" \
    "  $(jlog INFO "callback OIDC reçu" "code" "SplxlOBe...XQFe" "state" "xyz")" \
    "  $(jlog INFO "échange code → tokens" "token_endpoint" "https://${CP_IP}:8080/auth/realms/ztna/token")" \
    "  $(jlog INFO "tokens obtenus" "access_token" "eyJhbG...truncated" "expires_in" "300")" \
    "" \
    "  ${GREEN}${BOLD}  ✓ Authentification OIDC réussie${RST}" \
    "  ${DIM}    Utilisateur : alice | Groupes : ztna-admins, ztna-dba${RST}" \
    "  ${DIM}    JWT sub : ${JWT_SUB}${RST}" &
  local pid1=$!

  sleep 0.5
  anim_pane cp 0.2 \
    "$(hdr_active "CONTROL PLANE" "$CP_IP")" \
    "" \
    "  $(jlog INFO "requête OIDC authorize reçue" "client_id" "ztna-cli" "redirect_uri" "http://localhost:9876/callback")" \
    "  $(jlog INFO "authentification utilisateur" "username" "alice" "realm" "ztna")" \
    "  $(jlog INFO "émission access_token" "sub" "${JWT_SUB}" "groups" "[ztna-admins,ztna-dba]" "expires_in" "300")" \
    "  $(jlog INFO "émission refresh_token" "sub" "${JWT_SUB}" "expires_in" "1800")" &
  local pid2=$!

  set_pane flow \
    "" \
    "  ${BOLD}Flux OIDC — Authorization Code Flow${RST}" \
    "" \
    "    ${GREEN}CLIENT${RST}                                              ${CYAN}KEYCLOAK (IdP)${RST}" \
    "    ┌──────┐                                            ┌────────────┐" \
    "    │      │ ─── 1. /authorize (client_id, scope) ────► │            │" \
    "    │      │                                             │  Realm:    │" \
    "    │ CLI  │ ◄── 2. Login page (user/password) ──────── │  ztna      │" \
    "    │      │                                             │            │" \
    "    │      │ ─── 3. credentials (alice / ****) ────────► │  Verify    │" \
    "    │      │                                             │  ✓         │" \
    "    │      │ ◄── 4. Redirect + auth code ─────────────── │            │" \
    "    │      │                                             │            │" \
    "    │      │ ─── 5. /token (code) ────────────────────► │  Issue     │" \
    "    │      │                                             │  JWT       │" \
    "    │      │ ◄── 6. access_token + refresh_token ────── │            │" \
    "    └──────┘                                            └────────────┘" \
    "" \
    "    ${GREEN}JWT Claims :${RST} sub=${JWT_SUB}, groups=[ztna-admins, ztna-dba]"

  wait $pid1 $pid2 2>/dev/null || true
}

step_3() {
  load_scenario
  TSEC=10

  anim_pane client 0.18 \
    "$(hdr_active "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  \$ ${BOLD}ztna cert --request${RST}" \
    "" \
    "  $(jlog INFO "génération bi-clé RSA 2048" "algorithm" "RSA" "bits" "2048")" \
    "  $(jlog INFO "création CSR" "cn" "alice" "org" "ztna-admins,ztna-dba")" \
    "  $(jlog INFO "envoi CSR au Control Plane" "url" "https://${CP_IP}:8080/api/v1/credentials/device-cert" "jwt" "Bearer eyJh...")" \
    "  ${DIM}  → Attente signature CA...${RST}" \
    "  $(jlog INFO "certificat device reçu" "serial" "${CERT_SERIAL}" "cn" "alice" "valid_hours" "24")" \
    "  $(jlog INFO "CA certificate sauvegardé" "file" "client-ca.crt")" \
    "" \
    "  ${GREEN}${BOLD}  ✓ Certificat X.509 obtenu + CA sauvegardée${RST}" \
    "  ${DIM}    Serial : ${CERT_SERIAL}${RST}" \
    "  ${DIM}    CN=alice, O=ztna-admins,ztna-dba${RST}" \
    "  ${DIM}    Validité : 24h | Émetteur : ZTNA Device CA${RST}" &
  local pid1=$!

  sleep 0.6
  anim_pane cp 0.2 \
    "$(hdr_active "CONTROL PLANE" "$CP_IP")" \
    "" \
    "  $(jlog INFO "requête /credentials/device-cert reçue" "method" "POST" "remote" "${CLIENT_IP}")" \
    "  $(jlog INFO "validation JWT" "sub" "${JWT_SUB}" "issuer" "keycloak" "valid" "true")" \
    "  $(jlog INFO "extraction groupes depuis JWT" "groups" "[ztna-admins,ztna-dba]")" \
    "  $(jlog INFO "signature CSR par Device CA" "serial" "${CERT_SERIAL}" "cn" "alice" "org" "[ztna-admins,ztna-dba]")" \
    "  $(jlog INFO "certificat émis" "serial" "${CERT_SERIAL}" "expires" "2026-03-10T15:30:00Z" "key_type" "RSA")" &
  local pid2=$!

  set_pane gateway "$(hdr_idle "GATEWAY PEP" "$GW_IP")" "" \
    "  ${DIM}  En attente de connexion client...${RST}" "" \
    "  $(jlog INFO "CRL auto-refresh" "serials_count" "0" "last_fetch" "2026-03-09T15:29:00Z")" \
    "  $(jlog INFO "heartbeat envoyé" "pep_id" "ztna-gw-01" "status" "ok")"

  set_pane flow \
    "" \
    "  ${BOLD}Émission de certificat device — PKI Zero Trust${RST}" \
    "" \
    "    ${GREEN}CLIENT${RST}                                              ${CYAN}CONTROL PLANE (PKI)${RST}" \
    "    ┌──────┐                                            ┌────────────┐" \
    "    │      │ ─── 1. Generate RSA 2048 keypair           │            │" \
    "    │      │                                             │            │" \
    "    │ CSR  │ ─── 2. POST /credentials/device-cert ────► │  Validate  │" \
    "    │      │        (CSR + JWT Bearer)                   │  JWT       │" \
    "    │      │                                             │  ✓         │" \
    "    │      │                                             │            │" \
    "    │      │                                             │  Sign CSR  │" \
    "    │      │                                             │  Device CA │" \
    "    │      │ ◄── 3. X.509 Cert + CA Cert ──────────────  │  Serial:   │" \
    "    │      │        CN=alice O=ztna-admins               │  ${CERT_SERIAL}│" \
    "    └──────┘                                            └────────────┘" \
    "" \
    "    ${YELLOW}Groupes OIDC → Organization (X.509)${RST} : les groupes Keycloak sont" \
    "    encodés dans le champ Organization du certificat pour autorisation ABAC."

  wait $pid1 $pid2 2>/dev/null || true
}

step_4() {
  load_scenario
  TSEC=15

  anim_pane client 0.18 \
    "$(hdr_active "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  \$ ${BOLD}ztna resources${RST}" \
    "" \
    "  $(jlog INFO "récupération des ressources publiées" "endpoint" "https://${CP_IP}:8080/api/v1/resources")" \
    "" \
    "  NOM                  TYPE     ACCÈS            DESCRIPTION" \
    "  ---                  ----     -----            -----------" \
    "  grafana-internal     web      http-proxy       Grafana interne" \
    "  ssh-dev-01           ssh      ssh-cert         Serveur SSH dev" \
    "  pg-staging           db       tcp-tunnel       PostgreSQL staging" \
    "" \
    "  ${GREEN}${BOLD}  ✓ 3 ressources disponibles${RST}" &
  local pid1=$!

  sleep 0.5
  anim_pane cp 0.2 \
    "$(hdr_active "CONTROL PLANE" "$CP_IP")" \
    "" \
    "  $(jlog INFO "GET /api/v1/resources" "sub" "alice" "groups" "[ztna-admins,ztna-dba]")" \
    "  $(jlog INFO "ressources filtrées par groupes" "count" "3")" &
  local pid2=$!

  set_pane gateway "$(hdr_idle "GATEWAY PEP" "$GW_IP")" "" \
    "  ${DIM}  En attente de connexion client...${RST}"

  set_pane flow \
    "" \
    "  ${BOLD}Découverte des Ressources Publiées${RST}" \
    "" \
    "    ${GREEN}CLIENT${RST}                                              ${CYAN}CONTROL PLANE${RST}" \
    "    ┌──────┐                                            ┌────────────────┐" \
    "    │      │ ── GET /api/v1/resources (Bearer JWT) ───► │                │" \
    "    │      │                                            │  Filter by     │" \
    "    │ list │                                            │  user groups   │" \
    "    │      │ ◄── [{name, type, access_mode}] ────────── │                │" \
    "    └──────┘                                            └────────────────┘" \
    "" \
    "    ${BOLD}Ressources publiées (filtrage par groupes OIDC) :${RST}" \
    "    ┌──────────────────┬──────┬──────────────────────────────┐" \
    "    │ grafana-internal  │ web  │ http-proxy → ${BACKEND_IP}:3000 │" \
    "    │ ssh-dev-01        │ ssh  │ ssh-cert → ${BACKEND_IP}:22     │" \
    "    │ pg-staging        │ db   │ tcp-tunnel → ${BACKEND_IP}:5432 │" \
    "    └──────────────────┴──────┴──────────────────────────────┘"

  wait $pid1 $pid2 2>/dev/null || true
}

step_5() {
  load_scenario
  TSEC=20
  set_pane cp "$(hdr_idle "CONTROL PLANE" "$CP_IP")" "" \
    "  ${DIM}  PKI idle — CRL disponible à /pki/device-ca/crl${RST}"

  anim_pane client 0.18 \
    "$(hdr_active "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  \$ ${BOLD}ztna access ${RES_NAME}${RST}" \
    "" \
    "  $(jlog INFO "écoute locale ouverte" "addr" "127.0.0.1:${LOCAL_PORT}" "resource" "${RES_NAME}")" \
    "  ${GREEN}  → Ressource '${RES_NAME}' disponible sur 127.0.0.1:${LOCAL_PORT}${RST}" \
    "  ${DIM}    Ctrl+C pour fermer le tunnel.${RST}" \
    "" \
    "  $(jlog INFO "connexion mTLS vers la Gateway" "addr" "${GW_IP}:8443" "tls" "1.3")" \
    "  $(jlog INFO "envoi ClientHello" "cipher_suites" "[TLS_AES_256_GCM_SHA384,TLS_CHACHA20_POLY1305]" "curves" "X25519")" \
    "  $(jlog INFO "présentation certificat client" "serial" "${CERT_SERIAL}" "cn" "alice")" \
    "  $(jlog INFO "handshake mTLS terminé" "negotiated" "TLS_AES_256_GCM_SHA384" "version" "TLS1.3")" \
    "" \
    "  ${GREEN}${BOLD}  ✓ Tunnel mTLS établi avec la Gateway${RST}" &
  local pid1=$!

  sleep 0.4
  anim_pane gateway 0.18 \
    "$(hdr_active "GATEWAY PEP" "$GW_IP")" \
    "" \
    "  $(jlog INFO "connexion entrante" "remote" "${CLIENT_IP}:49832" "local" "${GW_IP}:8443")" \
    "  $(jlog INFO "handshake TLS" "version" "TLS1.3" "cipher" "TLS_AES_256_GCM_SHA384")" \
    "  $(jlog INFO "certificat client vérifié" "cn" "alice" "serial" "${CERT_SERIAL}" "issuer" "ZTNA Device CA")" \
    "  $(jlog INFO "vérification CRL" "serial" "${CERT_SERIAL}" "revoked" "false" "crl_size" "0")" \
    "  $(jlog INFO "extraction identité" "sub" "alice" "groups" "[ztna-admins,ztna-dba]" "source_ip" "${CLIENT_IP}")" \
    "" \
    "  ${GREEN}${BOLD}  ✓ Client authentifié — certificat non révoqué${RST}" &
  local pid2=$!

  set_pane flow \
    "" \
    "  ${BOLD}Port Forward Local + Tunnel mTLS + Vérification CRL${RST}" \
    "" \
    "    ${GREEN}CLIENT${RST}              ${DIM}TLS 1.3 mTLS${RST}            ${YELLOW}GATEWAY${RST}" \
    "    ┌──────┐                                    ┌──────────┐        ┌─────────┐" \
    "    │listen│ 127.0.0.1:${LOCAL_PORT}                    │          │        │  CRL    │" \
    "    │      │ ══ 1. ClientHello ═══════════════►  │          │        │  Store  │" \
    "    │cert  │                                     │  TLS     │        │ (0 rev) │" \
    "    │key   │ ◄═ 2. ServerHello + CertRequest ══  │  Accept  │        └────┬────┘" \
    "    │      │                                     │          │             │" \
    "    │      │ ══ 3. Client Certificate ═════════► │  Verify  │ ◄── check ─┘" \
    "    │      │    (CN=alice, Serial=${CERT_SERIAL})│  Chain ✓ │    not revoked ✓" \
    "    │      │                                     │          │" \
    "    │      │ ◄═ 4. Finished ═════════════════ ═  │  Ready   │" \
    "    └──────┘                                     └──────────┘" \
    "" \
    "    ${GREEN}✓${RST} Port forward local 127.0.0.1:${LOCAL_PORT}    ${GREEN}✓${RST} mTLS complet    ${GREEN}✓${RST} CRL OK    ${GREEN}✓${RST} TLS 1.3"

  wait $pid1 $pid2 2>/dev/null || true
}

step_6() {
  load_scenario
  TSEC=30
  set_pane cp "$(hdr_idle "CONTROL PLANE" "$CP_IP")" "" "  ${DIM}  En attente de requête d'autorisation...${RST}"

  anim_pane client 0.2 \
    "$(hdr_active "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  $(jlog INFO "envoi requête CONNECT" "action" "connect" "name" "${RES_NAME}")" \
    "  ${DIM}  → Requête (length-prefixed JSON, 4 bytes big-endian + payload) :${RST}" \
    "  ${DIM}    {\"action\":\"connect\",\"resource\":{\"name\":\"${RES_NAME}\"}}${RST}" \
    "  ${DIM}  → En attente de la décision...${RST}" &
  local pid1=$!

  sleep 0.3
  anim_pane gateway 0.18 \
    "$(hdr_active "GATEWAY PEP" "$GW_IP")" \
    "" \
    "  $(jlog INFO "requête CONNECT reçue" "action" "connect" "name" "${RES_NAME}")" \
    "  $(jlog INFO "résolution ressource via CP" "name" "${RES_NAME}" "endpoint" "/api/v1/pep/resources/${RES_NAME}")" \
    "  $(jlog INFO "ressource résolue" "name" "${RES_NAME}" "type" "${RES_TYPE}" "backend" "${RES_BACKEND}" "access_mode" "$([ "$RES_TYPE" = web ] && echo http-proxy || ([ "$RES_TYPE" = ssh ] && echo ssh-cert || echo tcp-tunnel))")" \
    "  $(jlog INFO "consultation decision cache" "key" "alice|connect|${RES_TYPE}:${RES_NAME}" "hit" "false")" \
    "  $(jlog INFO "cache miss — appel au Control Plane nécessaire")" &
  local pid2=$!

  set_pane flow \
    "" \
    "  ${BOLD}Requête CONNECT — Résolution par Nom${RST}" \
    "" \
    "    ${GREEN}CLIENT${RST}                               ${YELLOW}GATEWAY${RST}                       ${CYAN}CONTROL PLANE${RST}" \
    "    ┌──────┐                             ┌──────────────┐               ┌────────────────┐" \
    "    │      │ ── CONNECT Request ───────► │              │               │                │" \
    "    │      │    ${DIM}4-byte length + JSON${RST}      │  1. Parse    │               │                │" \
    "    │      │                              │  2. Resolve  │ GET /pep/     │  ResourceRepo  │" \
    "    │ wait │    ${DIM}{${RST}                          │   name via   │ resources/     │    ┌────────┐ │" \
    "    │  ..  │    ${DIM}  action: connect${RST}         │   CP API ────│─────────────►  │    │ ${RES_NAME} │ │" \
    "    │      │    ${DIM}  name: ${RES_NAME}${RST}       │              │               │    │ → ${RES_BACKEND}│ │" \
    "    │      │    ${DIM}}${RST}                          │  3. Cache?   │ ◄──── backend  │    └────────┘ │" \
    "    │      │                              │     miss →   │               │                │" \
    "    │      │                              │     CP call  │               │                │" \
    "    └──────┘                             └──────────────┘               └────────────────┘"

  wait $pid1 $pid2 2>/dev/null || true
}

step_7() {
  load_scenario
  TSEC=35

  set_pane client "$(hdr_idle "CLIENT CLI" "$CLIENT_IP")" "" \
    "  ${DIM}  En attente de la décision d'autorisation...${RST}" "" \
    "  ${DIM}  [tunnel mTLS actif — handshake complété]${RST}"

  anim_pane gateway 0.2 \
    "$(hdr_active "GATEWAY PEP" "$GW_IP")" \
    "" \
    "  $(jlog INFO "envoi requête authorize au CP" \
        "url" "https://${CP_IP}:8443/api/v1/pep/authorize" \
        "pep_id" "ztna-gw-01")" \
    "  ${DIM}  → POST avec headers X-PEP-ID + X-PEP-TOKEN${RST}" \
    "  ${DIM}  → Body: {sub, groups, action, resource:{name, type}, source_ip}${RST}" &
  local pid1=$!

  sleep 0.5
  anim_pane cp 0.18 \
    "$(hdr_active "CONTROL PLANE" "$CP_IP")" \
    "" \
    "  $(jlog INFO "requête authorize reçue" \
        "pep_id" "ztna-gw-01" "sub" "alice" "action" "connect" \
        "resource_name" "${RES_NAME}" "resource_type" "${RES_TYPE}")" \
    "  $(jlog INFO "authentification PEP" "pep_id" "ztna-gw-01" "token" "valid" "method" "constant-time compare")" \
    "  $(jlog INFO "chargement snapshot politiques" "rules_count" "5")" \
    "  $(jlog INFO "évaluation politique ABAC" \
        "rule" "allow-admins-${RES_TYPE}" "subject_match" "group:ztna-admins" \
        "action_match" "connect" "resource_match" "${RES_TYPE}:${RES_NAME}")" \
    "  $(jlog INFO "vérification conditions contextuelles" "allowed_hours" "08:00-22:00" "check" "pass")" \
    "  $(jlog INFO "vérification device trust" "required" "medium" "provided" "high" "check" "pass")" \
    "  $(jlog INFO "règle matchée" "rule_id" "1" "effect" "allow" "groups" "[ztna-admins]")" \
    "  $(jlog INFO "décision émise" \
        "decision_id" "${DECISION_ID}" "decision" "allow" "ttl" "3600" \
        "reason" "rule:allow-admins-${RES_TYPE}")" \
    "" \
    "  ${GREEN}${BOLD}  ✓ ALLOW — TTL 3600s — rule:allow-admins-${RES_TYPE}${RST}" &
  local pid2=$!

  sleep 1.2
  anim_pane gateway 0.18 \
    "" \
    "  $(jlog INFO "décision CP reçue" "decision" "allow" "ttl_seconds" "3600" "decision_id" "${DECISION_ID}")" \
    "  $(jlog INFO "décision mise en cache" \
        "key" "alice|connect|${RES_TYPE}:${RES_NAME}" "ttl" "60s")" \
    "" \
    "  ${GREEN}${BOLD}  ✓ Accès autorisé par le Control Plane${RST}"

  set_pane flow \
    "" \
    "  ${BOLD}Autorisation — Policy Decision Point (ABAC + Context)${RST}" \
    "" \
    "    ${YELLOW}GATEWAY (PEP)${RST}         ${DIM}PEP Auth${RST}           ${CYAN}CONTROL PLANE (PDP)${RST}" \
    "    ┌──────────┐                            ┌──────────────────────┐" \
    "    │          │ ── POST /pep/authorize ──►  │  1. Auth PEP token   │" \
    "    │  Cache   │    {sub: alice,             │     constant-time ✓  │" \
    "    │  miss    │     groups: [ztna-admins],  │                      │" \
    "    │          │     action: connect,        │  2. Load policies    │" \
    "    │          │     resource: {             │     5 rules loaded   │" \
    "    │          │       name: ${RES_NAME},    │                      │" \
    "    │          │       type: ${RES_TYPE}},   │  3. ABAC evaluation  │" \
    "    │          │     context: {              │     ${GREEN}match: rule #1${RST}    │" \
    "    │          │       device_trust: high,   │     ${GREEN}allow-admins-${RES_TYPE}${RST} │" \
    "    │          │       src_ip: ${CLIENT_IP}}}│                      │" \
    "    │          │                             │  4. Context checks   │" \
    "    │          │                             │     hours: 08-22 ✓   │" \
    "    │          │                             │     trust: high ✓    │" \
    "    │          │ ◄─ {allow, ttl: 3600} ────  │                      │" \
    "    │  Cache   │                             │  5. Decision logged  │" \
    "    │  store ✓ │                             │     id: ${DECISION_ID:0:12}..│" \
    "    └──────────┘                            └──────────────────────┘"

  wait $pid1 $pid2 2>/dev/null || true
}

step_8() {
  load_scenario
  TSEC=42

  set_pane client "$(hdr_idle "CLIENT CLI" "$CLIENT_IP")" "" \
    "  ${DIM}  En attente — autorisation en cours de traitement...${RST}"

  anim_pane gateway 0.18 \
    "$(hdr_active "GATEWAY PEP" "$GW_IP")" \
    "" \
    "  $(jlog INFO "résolution de ressource publiée" \
        "name" "${RES_NAME}" "backend" "${RES_BACKEND}" "type" "${RES_TYPE}")" \
    "  $(jlog INFO "validation cible résolue" "backend" "${RES_BACKEND}" "ssrf_check" "pass")" \
    "  $(jlog INFO "création session" \
        "session_id" "${SESSION_UUID}" "sub" "alice" \
        "resource_name" "${RES_NAME}" "backend" "${RES_BACKEND}" "cert_serial" "${CERT_SERIAL}")" \
    "  $(jlog INFO "TTL session appliqué" "ttl_seconds" "3600" "expires_at" "2026-03-09T16:30:42Z")" \
    "  $(jlog INFO "vérification limite par sujet" "sub" "alice" "current" "0" "max" "10")" \
    "  $(jlog INFO "session enregistrée" \
        "session_id" "${SESSION_UUID}" "decision_id" "${DECISION_ID}" \
        "cert_serial" "${CERT_SERIAL}" "active_count" "1")" \
    "  $(jlog INFO "envoi télémétrie session.start au CP" \
        "session_id" "${SESSION_UUID}" "endpoint" "/api/v1/pep/sessions/start")" &
  local pid1=$!

  sleep 0.6
  anim_pane cp 0.2 \
    "$(hdr_active "CONTROL PLANE" "$CP_IP")" \
    "" \
    "  $(jlog INFO "session.start reçu" \
        "session_id" "${SESSION_UUID}" "pep_id" "ztna-gw-01" \
        "sub" "alice" "resource_name" "${RES_NAME}" "backend" "${RES_BACKEND}")" \
    "  $(jlog INFO "session enregistrée dans le store" "active_sessions" "1")" &
  local pid2=$!

  set_pane flow \
    "" \
    "  ${BOLD}Résolution Ressource + Session + Télémétrie${RST}" \
    "" \
    "    ${YELLOW}GATEWAY${RST}                                         ${CYAN}CONTROL PLANE${RST}" \
    "    ┌─────────────────────────┐                     ┌────────────────┐" \
    "    │  1. Resource Resolution  │                     │                │" \
    "    │  ${RES_NAME} (${RES_TYPE})        │                     │                │" \
    "    │    → ${RES_BACKEND} (backend) │                     │  Session Store │" \
    "    │  2. SSRF Check ✓        │                     │                │" \
    "    │  ┌───────────────────┐  │                     │                │" \
    "    │  │ ID: ${SESSION_UUID:0:16}..│  │  session.start      │                │" \
    "    │  │ Sub: alice        │  │ ──────────────────► │  ✓ stored      │" \
    "    │  │ Cert: ${CERT_SERIAL:0:11}..│  │                     │  active: 1     │" \
    "    │  │ Backend: resolved │  │                     │                │" \
    "    │  │ TTL: 3600s        │  │  heartbeat (30s)    │                │" \
    "    │  │ GC: every 30s     │  │ ─ ─ ─ ─ ─ ─ ─ ─ ► │  last_seen ✓   │" \
    "    │  └───────────────────┘  │                     │                │" \
    "    └─────────────────────────┘                     └────────────────┘"

  wait $pid1 $pid2 2>/dev/null || true
}

step_9() {
  load_scenario
  TSEC=48

  anim_pane client 0.2 \
    "$(hdr_active "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  $(jlog INFO "réponse Gateway reçue" "status" "allow" "session_id" "${SESSION_UUID}")" \
    "" \
    "  ${GREEN}${BOLD}  ✓ Accès autorisé — tunnel proxy actif${RST}" \
    ""

  if [[ "$RES_TYPE" == "ssh" ]]; then
    anim_pane client 0.3 \
      "  \$ ${BOLD}ssh -p ${LOCAL_PORT} alice@localhost${RST}" \
      "  ${DIM}  Last login: Mon Mar  9 14:22:01 2026 from 10.10.30.20${RST}" \
      "  ${GREEN}  alice@lan-app:~\$${RST} ${BOLD}hostname && whoami${RST}" \
      "  lan-app" \
      "  alice" \
      "  ${GREEN}  alice@lan-app:~\$${RST} ${BOLD}cat /etc/os-release | head -2${RST}" \
      "  PRETTY_NAME=\"Ubuntu 22.04.3 LTS\"" \
      "  NAME=\"Ubuntu\"" \
      "  ${GREEN}  alice@lan-app:~\$${RST} █"
  elif [[ "$RES_TYPE" == "web" ]]; then
    anim_pane client 0.25 \
      "  \$ ${BOLD}curl http://localhost:${LOCAL_PORT}/api/health${RST}" \
      "  {\"status\":\"healthy\",\"uptime\":\"48h32m\",\"version\":\"2.1.0\"}" \
      "" \
      "  \$ ${BOLD}curl http://localhost:${LOCAL_PORT}/api/data${RST}" \
      "  {\"items\":[{\"id\":1,\"name\":\"dataset-alpha\"},{\"id\":2,\"name\":\"dataset-beta\"}]}" \
      "" \
      "  ${GREEN}${BOLD}  ✓ Réponses HTTP reçues via tunnel ZTNA${RST}"
  else
    anim_pane client 0.25 \
      "  \$ ${BOLD}psql -h localhost -p ${LOCAL_PORT} -U alice -d appdb${RST}" \
      "  ${DIM}  Password for user alice: ****${RST}" \
      "  psql (15.4)" \
      "  Type \"help\" for help." \
      "" \
      "  appdb=> ${BOLD}SELECT count(*) FROM users;${RST}" \
      "   count" \
      "  -------" \
      "     1247" \
      "  (1 row)" \
      "" \
      "  appdb=> █"
  fi &
  local pidc=$!

  sleep 0.3
  anim_pane gateway 0.2 \
    "$(hdr_active "GATEWAY PEP" "$GW_IP")" \
    "" \
    "  $(jlog INFO "réponse ALLOW envoyée au client" "session_id" "${SESSION_UUID}")" \
    "  $(jlog INFO "ouverture connexion vers backend résolu" "backend" "${RES_BACKEND}" "resource" "${RES_NAME}")" \
    "  $(jlog INFO "connexion backend établie" "target" "${RES_BACKEND}" "local_addr" "10.10.30.20:52491")" \
    "  $(jlog INFO "proxy TCP bidirectionnel actif" "client" "${CLIENT_IP}:49832" "backend" "${RES_BACKEND}")" \
    "  $(jlog INFO "relay en cours" "bytes_in" "1482" "bytes_out" "3891" "duration" "2.1s")" \
    "  $(jlog INFO "relay en cours" "bytes_in" "2947" "bytes_out" "8234" "duration" "4.8s")" &
  local pidg=$!

  set_pane cp "$(hdr_idle "CONTROL PLANE" "$CP_IP")" "" \
    "  $(jlog INFO "heartbeat reçu" "pep_id" "ztna-gw-01" "version" "1.0.0")" "" \
    "  ${DIM}  Session active : ${SESSION_UUID:0:16}...${RST}" \
    "  ${DIM}  Monitoring en cours...${RST}"

  set_pane flow \
    "" \
    "  ${BOLD}Proxy TCP Bidirectionnel — Ressource Résolue → Backend${RST}" \
    "" \
    "    ${GREEN}APP${RST}       ${GREEN}CLIENT${RST}             ${DIM}mTLS tunnel${RST}        ${YELLOW}GATEWAY${RST}            ${DIM}TCP relay${RST}       ${MAGENTA}BACKEND${RST}" \
    "    ┌─────┐  ┌──────┐                            ┌──────────┐                      ┌────────┐" \
    "    │curl │  │      │ ═══════════════════════════►│          │═════════════════════►│        │" \
    "    │ssh  │→ │ ${RES_PROTO}  │     encrypted traffic      │  PROXY   │    cleartext relay     │ ${RES_BACKEND} │" \
    "    │psql │  │      │◄═══════════════════════════ │  io.Copy │◄═════════════════════│        │" \
    "    └─────┘  └──────┘                            └──────────┘                      └────────┘" \
    "    localhost ${CLIENT_IP}                            ${GW_IP}                          ${BACKEND_IP}" \
    "    :${LOCAL_PORT}" \
    "" \
    "    ${YELLOW}Resource Resolution :${RST} ${RES_NAME} (${RES_TYPE})  →  ${RES_BACKEND} (backend résolu via CP)" \
    "    ${DIM}Port forward local : 127.0.0.1:${LOCAL_PORT} → tunnel mTLS → backend${RST}" \
    "    ${DIM}Bytes client→cible : 2947        Bytes cible→client : 8234${RST}" \
    "    ${DIM}Session TTL : 3600s              Cert serial tracké pour révocation${RST}"

  wait $pidc $pidg 2>/dev/null || true
}

step_10() {
  load_scenario
  TSEC=55

  anim_pane client 0.2 \
    "$(hdr_done "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  $(jlog INFO "déconnexion du tunnel" "session_id" "${SESSION_UUID}")" \
    "  $(jlog INFO "session terminée" "duration" "12.4s" "bytes_sent" "2947" "bytes_received" "8234")" \
    "" \
    "  ${GREEN}${BOLD}  ✓ Session terminée proprement${RST}" &
  local pidc=$!

  sleep 0.3
  anim_pane gateway 0.2 \
    "$(hdr_active "GATEWAY PEP" "$GW_IP")" \
    "" \
    "  $(jlog INFO "fin de connexion client" "remote" "${CLIENT_IP}:49832")" \
    "  $(jlog INFO "fermeture proxy TCP" "resource" "${RES_NAME}" "backend" "${RES_BACKEND}" "duration_ms" "12400")" \
    "  $(jlog INFO "métriques session finales" \
        "session_id" "${SESSION_UUID}" "bytes_in" "2947" "bytes_out" "8234" \
        "duration_ms" "12400" "end_reason" "client_close")" \
    "  $(jlog INFO "session désenregistrée" "session_id" "${SESSION_UUID}" "active_count" "0")" \
    "  $(jlog INFO "envoi télémétrie session.end" \
        "session_id" "${SESSION_UUID}" "endpoint" "/api/v1/pep/sessions/end")" &
  local pidg=$!

  sleep 0.8
  anim_pane cp 0.2 \
    "$(hdr_active "CONTROL PLANE" "$CP_IP")" \
    "" \
    "  $(jlog INFO "session.end reçu" \
        "session_id" "${SESSION_UUID}" "pep_id" "ztna-gw-01" \
        "duration_ms" "12400" "bytes_in" "2947" "bytes_out" "8234" \
        "end_reason" "client_close")" \
    "  $(jlog INFO "session archivée" "active_sessions" "0")" &
  local pidcp=$!

  set_pane flow \
    "" \
    "  ${BOLD}Fin de Session — Métriques & Télémétrie${RST}" \
    "" \
    "    ${GREEN}CLIENT${RST}               ${YELLOW}GATEWAY${RST}                         ${CYAN}CONTROL PLANE${RST}" \
    "    ┌──────┐           ┌──────────────────┐               ┌────────────────┐" \
    "    │      │ ─ close ─►│                  │               │                │" \
    "    │ done │           │ 1. Close proxy   │               │                │" \
    "    │      │           │ 2. Set end stats │               │                │" \
    "    └──────┘           │    bytes: 2947↑  │               │                │" \
    "                       │           8234↓  │  session.end  │                │" \
    "                       │    duration: 12s │ ────────────► │ Archive        │" \
    "                       │    reason: close │               │ session        │" \
    "                       │ 3. Unregister    │               │ active: 0      │" \
    "                       │    active: 0     │               │                │" \
    "                       └──────────────────┘               └────────────────┘" \
    "" \
    "    ${BOLD}Métriques collectées :${RST}" \
    "    Durée: 12.4s | Bytes↑ 2947 | Bytes↓ 8234 | Raison: client_close"

  wait $pidc $pidg $pidcp 2>/dev/null || true
}

step_11() {
  load_scenario
  set_pane client \
    "$(hdr_done "CLIENT CLI" "$CLIENT_IP")" \
    "" \
    "  ${GREEN}  ✓ Authentification OIDC${RST}" \
    "  ${GREEN}  ✓ Certificat X.509 device + CA${RST}" \
    "  ${GREEN}  ✓ Découverte ressources publiées (ztna resources)${RST}" \
    "  ${GREEN}  ✓ Port forward local 127.0.0.1:${LOCAL_PORT}${RST}" \
    "  ${GREEN}  ✓ Tunnel mTLS TLS 1.3${RST}" \
    "  ${GREEN}  ✓ Accès ${RES_NAME} (${RES_TYPE}) → ${RES_BACKEND}${RST}" \
    "  ${GREEN}  ✓ Session terminée proprement${RST}"

  set_pane gateway \
    "$(hdr_done "GATEWAY PEP" "$GW_IP")" \
    "" \
    "  ${GREEN}  ✓ CRL vérification + révocation active${RST}" \
    "  ${GREEN}  ✓ Résolution ressource via CP (name → backend)${RST}" \
    "  ${GREEN}  ✓ Protection SSRF sur backend résolu${RST}" \
    "  ${GREEN}  ✓ Autorisation via CP + cache${RST}" \
    "  ${GREEN}  ✓ Session TTL + cert_serial tracké${RST}" \
    "  ${GREEN}  ✓ Proxy TCP bidirectionnel${RST}" \
    "  ${GREEN}  ✓ Télémétrie start/end + end_reason${RST}" \
    "  ${GREEN}  ✓ Métriques bytes_in/bytes_out${RST}"

  set_pane cp \
    "$(hdr_done "CONTROL PLANE" "$CP_IP")" \
    "" \
    "  ${GREEN}  ✓ OIDC/Keycloak validation${RST}" \
    "  ${GREEN}  ✓ PKI — Device CA + CA cert${RST}" \
    "  ${GREEN}  ✓ PEP authentication${RST}" \
    "  ${GREEN}  ✓ Ressources publiées (web/ssh/db)${RST}" \
    "  ${GREEN}  ✓ Policy Engine ABAC + Context${RST}" \
    "  ${GREEN}  ✓ Session monitoring + audit${RST}" \
    "  ${GREEN}  ✓ Heartbeat tracking${RST}" \
    "  ${GREEN}  ✓ CRL management + push revoke${RST}"

  set_pane flow \
    "" \
    "  ${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════════════════════╗${RST}" \
    "  ${BOLD}${CYAN}║                RÉSUMÉ — Plateforme de Ressources Publiées                  ║${RST}" \
    "  ${BOLD}${CYAN}╠══════════════════════════════════════════════════════════════════════════════╣${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Ressources publiées  ${GREEN}✓${RST} CRL Auto-Refresh       ${GREEN}✓${RST} Protection SSRF         ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Policy ABAC+Context ${GREEN}✓${RST} Decision Cache + TTL   ${GREEN}✓${RST} CP-Down Resilience      ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Name → Backend (CP) ${GREEN}✓${RST} Active Revocation      ${GREEN}✓${RST} Admin Session Kill      ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Session TTL/GC      ${GREEN}✓${RST} Télémétrie + EndReason ${GREEN}✓${RST} MaxBytesReader          ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${GREEN}✓${RST} Device Cert + CA    ${GREEN}✓${RST} Graceful Shutdown       ${GREEN}✓${RST} Architecture Hexagonale ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}╠══════════════════════════════════════════════════════════════════════════════╣${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${BOLD}Resource:${RST} ${RES_NAME} (${RES_TYPE}) → ${RES_BACKEND} (résolution centralisée CP)     ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${BOLD}Policy:${RST} ABAC + AllowedHours + DeviceTrust (context-aware)              ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}║${RST}  ${BOLD}Revoke:${RST} CRL diff → KillBySerial → sessions actives terminées           ${CYAN}║${RST}" \
    "  ${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════════════════════╝${RST}" \
    "" \
    "  ${DIM}  Scénario testé : ${RES_DESC} (${RES_NAME} → ${RES_BACKEND})${RST}" \
    "  ${DIM}  Flux : OIDC → DeviceCert → Resources → PortForward → mTLS → CRL → CONNECT → Resolve(CP) → AuthZ → Session → Proxy → End${RST}"
}

# ============================================================================
# CONTROLLER — Interactive Navigation (runs in its own window)
# ============================================================================

run_controller() {
  while [ ! -d "$DEMO_DIR" ]; do sleep 0.1; done
  mkdir -p "$DEMO_DIR"

  set -m  # Enable job control for process group kills

  local step=0
  local max_step=11
  local anim_pid=0

  kill_step() {
    [[ $anim_pid -gt 0 ]] || return 0
    kill -- -"$anim_pid" 2>/dev/null
    kill "$anim_pid" 2>/dev/null
    wait "$anim_pid" 2>/dev/null
    anim_pid=0
  }

  local step_names=(
    "Bienvenue"
    "Sélection du Scénario"
    "Authentification OIDC"
    "Émission de Certificat"
    "Découverte des Ressources"
    "Port Forward + mTLS + CRL"
    "Requête CONNECT + Résolution"
    "Autorisation ABAC"
    "Session + Télémétrie"
    "Proxy TCP — Accès Ressource"
    "Fin de Session — Métriques"
    "Résumé"
  )

  while true; do
    clear
    printf '%b\n' \
      "" \
      "  ${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════════╗${RST}" \
      "  ${BOLD}${CYAN}║          ZTNA — Démonstration Interactive Orchestrée            ║${RST}" \
      "  ${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════════╝${RST}" \
      ""

    if [[ $step -eq 0 ]]; then
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  ${DIM}  Appuyez sur ${RST}${BOLD}ENTER${RST}${DIM} pour commencer la démonstration${RST}" \
        "" \
        "  ${DIM}  Navigation :${RST}" \
        "  ${DIM}    ENTER  → Étape suivante${RST}" \
        "  ${DIM}    b      → Étape précédente${RST}" \
        "  ${DIM}    r      → Rejouer l'étape${RST}" \
        "  ${DIM}    q      → Quitter${RST}"
    elif [[ $step -eq 1 ]]; then
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  Choisissez le type d'accès à démontrer :" \
        "" \
        "  ${BOLD}${GREEN}  [1]${RST}  Accès SSH       (ssh-dev-01 → ${BACKEND_IP}:22)" \
        "  ${BOLD}${BLUE}  [2]${RST}  Accès Web       (grafana-internal → ${BACKEND_IP}:3000)" \
        "  ${BOLD}${YELLOW}  [3]${RST}  Accès PostgreSQL (pg-staging → ${BACKEND_IP}:5432)" \
        "" \
        "  ${DIM}  Appuyez sur 1, 2 ou 3${RST}"
    elif [[ $step -eq $max_step ]]; then
      load_scenario
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}  ${DIM}[${RES_DESC}]${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  ${GREEN}${BOLD}  ✓ Démonstration complète !${RST}" \
        "" \
        "  ${DIM}  [b] Retour   [r] Rejouer   [1] Autre scénario   [q] Quitter${RST}"
    else
      load_scenario
      printf '%b\n' \
        "  ${BOLD}Étape ${step}/${max_step}${RST} — ${BOLD}${step_names[$step]}${RST}  ${DIM}[${RES_DESC}]${RST}" \
        "  $(progress_bar $step $max_step)" \
        "" \
        "  ${DIM}  [ENTER] Suivant   [b] Retour   [r] Rejouer   [q] Quitter${RST}"
    fi

    # Run step animation in background
    "step_${step}" 2>/dev/null &
    anim_pid=$!

    # Wait for user input
    if [[ $step -eq 1 ]]; then
      while true; do
        read -rsn1 key
        case "$key" in
          1) SCENARIO=1; RES_TYPE="ssh"; RES_PORT=22; RES_HOST="lan-app"
             RES_BACKEND="${BACKEND_IP}:22"
             RES_DESC="Serveur SSH Backend"; RES_PROTO="SSH"; RES_NAME="ssh-dev-01"; LOCAL_PORT=2222
             save_scenario; kill_step; step=2; break ;;
          2) SCENARIO=2; RES_TYPE="web"; RES_PORT=3000; RES_HOST="lan-app"
             RES_BACKEND="${BACKEND_IP}:3000"
             RES_DESC="Grafana Interne (Web)"; RES_PROTO="HTTP"; RES_NAME="grafana-internal"; LOCAL_PORT=8888
             save_scenario; kill_step; step=2; break ;;
          3) SCENARIO=3; RES_TYPE="db"; RES_PORT=5432; RES_HOST="lan-app"
             RES_BACKEND="${BACKEND_IP}:5432"
             RES_DESC="Base de données PostgreSQL"; RES_PROTO="PSQL"; RES_NAME="pg-staging"; LOCAL_PORT=15432
             save_scenario; kill_step; step=2; break ;;
          b|B) kill_step; step=0; break ;;
          q|Q) kill_step; cleanup_all; exit 0 ;;
        esac
      done
    else
      while true; do
        read -rsn1 key
        case "$key" in
          "") kill_step
              if [[ $step -lt $max_step ]]; then step=$((step + 1)); fi
              break ;;
          b|B) kill_step
               if [[ $step -gt 0 ]]; then step=$((step - 1)); fi
               break ;;
          r|R) kill_step; break ;;
          q|Q) kill_step; cleanup_all; exit 0 ;;
          1)   kill_step; step=1; break ;;
          0)   kill_step; step=0; break ;;
        esac
      done
    fi
  done
}

cleanup_all() {
  printf '\n%b\n\n' "  ${DIM}Fermeture des fenêtres...${RST}"
  sleep 0.3
  if [[ -f "$PID_FILE" ]]; then
    while read -r pid; do
      kill "$pid" 2>/dev/null
    done < "$PID_FILE"
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

# Ensure xdotool is available (needed for window positioning)
ensure_xdotool() {
  local needs_install=false
  
  if ! command -v xdotool >/dev/null 2>&1; then
    needs_install=true
  fi
  
  if ! command -v wmctrl >/dev/null 2>&1; then
    needs_install=true
  fi
  
  if [[ "$needs_install" == true ]]; then
    echo -e "\033[0;36m[ZTNA Demo]\033[0m Installation des outils de gestion de fenêtres..."
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

# Open a terminal window running a command, return its PID
# $1=title  $2=command
open_window() {
  local title="$1" cmd="$2"
  local term
  term=$(detect_terminal)

  case "$term" in
    konsole)
      # Use --separate to force a new window instance (not a tab)
      konsole --separate --hide-menubar --hide-tabbar \
        --notransparency \
        -p tabtitle="$title" \
        -e bash -c "$cmd" &
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
    *)
      echo "ERREUR: Aucun émulateur de terminal trouvé" >&2
      exit 1
      ;;
  esac
  echo $!
}

# Position a window by searching for its title with xdotool
# $1=title  $2=x  $3=y  $4=width  $5=height
position_window_by_title() {
  local title="$1" x="$2" y="$3" w="$4" h="$5"

  if ! command -v xdotool >/dev/null 2>&1; then return 1; fi

  # Wait for the window to appear (retry up to 3s)
  local wid="" attempts=0
  while [[ -z "$wid" && $attempts -lt 30 ]]; do
    wid=$(xdotool search --name "$title" 2>/dev/null | head -1)
    [[ -n "$wid" ]] && break
    sleep 0.1
    attempts=$((attempts + 1))
  done

  if [[ -n "$wid" ]]; then
    # First, unmaximize if needed (different methods for different WMs)
    wmctrl -ir "$wid" -b remove,maximized_vert,maximized_horz 2>/dev/null || true
    xdotool windowstate --remove MAXIMIZED_VERT MAXIMIZED_HORZ "$wid" 2>/dev/null || true
    sleep 0.05
    
    # Set window size first, then position (more reliable)
    xdotool windowsize --sync "$wid" "$w" "$h" 2>/dev/null
    sleep 0.05
    xdotool windowmove --sync "$wid" "$x" "$y" 2>/dev/null
    
    # Raise window to ensure it's visible
    xdotool windowraise "$wid" 2>/dev/null
    xdotool windowfocus "$wid" 2>/dev/null
    sleep 0.05
    
    # Force position again (some WMs reposition after focus)
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
  echo -e "\033[0;36m[ZTNA Demo]\033[0m Terminal détecté : $term"

  # Check for xdotool
  local has_xdotool=false
  if ensure_xdotool; then
    has_xdotool=true
    echo -e "\033[0;36m[ZTNA Demo]\033[0m xdotool disponible ✓"
  else
    echo -e "\033[0;33m[ZTNA Demo]\033[0m xdotool non disponible — les fenêtres ne seront pas positionnées automatiquement"
  fi

  # Kill any existing demo
  if [[ -f "$PID_FILE" ]]; then
    while read -r pid; do kill "$pid" 2>/dev/null; done < "$PID_FILE"
  fi
  rm -rf "$DEMO_DIR"
  mkdir -p "$DEMO_DIR"

  # Initialize pane files
  for p in client gateway cp flow; do
    echo "" > "$DEMO_DIR/pane-$p"
    echo "0" > "$DEMO_DIR/epoch-$p"
  done

  # Save default scenario
  save_scenario

  # ── Screen layout ──────────────────────────────
  # 1920x1080 desktop with KDE panel + window decorations
  # 
  # Optimized to fit all 5 windows without overlap:
  #
  #  ┌─── CONTROLLER ──────┐ ┌─── FLUX ───────────┐
  #  │  (navigation 350px)  │ │ (architecture)     │  row1: 350px
  #  └──────────────────────┘ └────────────────────┘
  #  ┌─ CLIENT ─┐ ┌─ GATEWAY ─┐ ┌─ CTRL PLANE ───┐
  #  │          │ │            │ │                │  row2: 520px
  #  └──────────┘ └────────────┘ └────────────────┘
  #
  # Total height: 350 + 520 + 40 (gaps+decorations) + 60 (taskbar+margin) = 970px ✓
  #
  local gap=10
  local margin_top=10
  local margin_bottom=90  # KDE panel + safety margin
  local sw=1920
  local sh=$((1080 - margin_bottom))
  local row1_h=340
  local row2_h=500
  local half_w=$(( (sw - gap * 3) / 2 ))
  local third_w=$(( (sw - gap * 4) / 3 ))
  local row2_y=$(( margin_top + row1_h + gap * 2 ))

  echo -e "\033[0;36m[ZTNA Demo]\033[0m Ouverture de 5 fenêtres séparées..."

  local PIDS=()
  local term
  term=$(detect_terminal)

  # Helper to launch a window based on terminal type
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

  # Window 1: CONTROLLER (top-left)
  echo -e "\033[0;36m[ZTNA Demo]\033[0m   → Ouverture CONTROLLER..."
  launch_terminal_window "ZTNA — CONTROLLER" "bash '${SCRIPT_PATH}' --controller"
  PIDS+=($!)
  sleep 0.3

  # Window 2: FLUX (top-right)
  echo -e "\033[0;36m[ZTNA Demo]\033[0m   → Ouverture FLUX..."
  launch_terminal_window "ZTNA — FLUX" "bash '${SCRIPT_PATH}' --display flow"
  PIDS+=($!)
  sleep 0.3

  # Window 3: CLIENT (bottom-left)
  echo -e "\033[0;36m[ZTNA Demo]\033[0m   → Ouverture CLIENT..."
  launch_terminal_window "ZTNA — CLIENT" "bash '${SCRIPT_PATH}' --display client"
  PIDS+=($!)
  sleep 0.3

  # Window 4: GATEWAY (bottom-center)
  echo -e "\033[0;36m[ZTNA Demo]\033[0m   → Ouverture GATEWAY..."
  launch_terminal_window "ZTNA — GATEWAY" "bash '${SCRIPT_PATH}' --display gateway"
  PIDS+=($!)
  sleep 0.3

  # Window 5: CONTROL PLANE (bottom-right)
  echo -e "\033[0;36m[ZTNA Demo]\033[0m   → Ouverture CONTROL PLANE..."
  launch_terminal_window "ZTNA — CONTROL PLANE" "bash '${SCRIPT_PATH}' --display cp"
  PIDS+=($!)
  sleep 0.2

  # Save PIDs for cleanup
  printf '%s\n' "${PIDS[@]}" > "$PID_FILE"

  echo -e "\033[0;36m[ZTNA Demo]\033[0m   → 5 fenêtres créées ✓"
  echo -e "\033[0;36m[ZTNA Demo]\033[0m"

  # Position windows if xdotool is available
  if [[ "$has_xdotool" == true ]]; then
    sleep 0.8  # Let all windows fully appear with decorations

    echo -e "\033[0;36m[ZTNA Demo]\033[0m Positionnement automatique des fenêtres..."

    # Row 1: CONTROLLER (top-left) + FLUX (top-right)
    if position_window_by_title "ZTNA — CONTROLLER" "$gap" "$margin_top" "$half_w" "$row1_h"; then
      echo -e "\033[0;36m[ZTNA Demo]\033[0m   ✓ CONTROLLER positionné"
    fi
    if position_window_by_title "ZTNA — FLUX" "$((half_w + gap * 2))" "$margin_top" "$half_w" "$row1_h"; then
      echo -e "\033[0;36m[ZTNA Demo]\033[0m   ✓ FLUX positionné"
    fi
    
    # Row 2: CLIENT, GATEWAY, CONTROL PLANE (3 columns)
    if position_window_by_title "ZTNA — CLIENT" "$gap" "$row2_y" "$third_w" "$row2_h"; then
      echo -e "\033[0;36m[ZTNA Demo]\033[0m   ✓ CLIENT positionné"
    fi
    if position_window_by_title "ZTNA — GATEWAY" "$((third_w + gap * 2))" "$row2_y" "$third_w" "$row2_h"; then
      echo -e "\033[0;36m[ZTNA Demo]\033[0m   ✓ GATEWAY positionné"
    fi
    if position_window_by_title "ZTNA — CONTROL PLANE" "$((third_w * 2 + gap * 3))" "$row2_y" "$third_w" "$row2_h"; then
      echo -e "\033[0;36m[ZTNA Demo]\033[0m   ✓ CONTROL PLANE positionné"
    fi

    echo -e "\033[0;36m[ZTNA Demo]\033[0m Layout terminé — toutes les fenêtres sont visibles ✓"
  fi

  echo ""
  echo -e "\033[0;36m[ZTNA Demo]\033[0m ┌─────────────────────────────────────────────────┐"
  echo -e "\033[0;36m[ZTNA Demo]\033[0m │  5 fenêtres ouvertes sur le bureau              │"
  echo -e "\033[0;36m[ZTNA Demo]\033[0m │  ► Cliquez sur la fenêtre CONTROLLER             │"
  echo -e "\033[0;36m[ZTNA Demo]\033[0m │  ► Appuyez ENTER pour naviguer la démo           │"
  echo -e "\033[0;36m[ZTNA Demo]\033[0m │  ► 'q' pour quitter depuis le controller         │"
  echo -e "\033[0;36m[ZTNA Demo]\033[0m │  ► Ctrl+C ici pour tout fermer                   │"
  echo -e "\033[0;36m[ZTNA Demo]\033[0m └─────────────────────────────────────────────────┘"
  echo ""

  # Trap Ctrl+C to cleanup
  trap 'echo ""; echo -e "\033[0;36m[ZTNA Demo]\033[0m Arrêt..."; for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null; done; rm -rf "$DEMO_DIR"; exit 0' INT TERM

  # Wait for controller to exit
  local ctrl_pid="${PIDS[0]}"
  while kill -0 "$ctrl_pid" 2>/dev/null; do
    sleep 1
  done

  # Cleanup when controller exits
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null
  done
  rm -rf "$DEMO_DIR"
  echo -e "\033[0;36m[ZTNA Demo]\033[0m Terminé."
}

# ============================================================================
# ENTRY POINT
# ============================================================================

case "${1:-}" in
  --display)
    run_display "${2:?Usage: $0 --display <pane_name>}"
    ;;
  --controller)
    run_controller
    ;;
  *)
    launch_windows
    ;;
esac
