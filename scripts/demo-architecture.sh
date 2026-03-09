#!/usr/bin/env bash
# ============================================================================
# ZTNA Architecture Visualization — Terminal ASCII Art
# ============================================================================
# Affiche une vue d'architecture complète du projet ZTNA pour présentations.
# Usage: bash scripts/demo-architecture.sh
# ============================================================================

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

clear

echo -e "${CYAN}${BOLD}"
cat << 'BANNER'
  ╔══════════════════════════════════════════════════════════════════════════╗
  ║              ZTNA — Zero Trust Network Access Architecture             ║
  ║                    Soutenance Technique — 2026                         ║
  ╚══════════════════════════════════════════════════════════════════════════╝
BANNER
echo -e "${NC}"

echo -e "${BOLD}  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Network topology
echo -e "  ${BOLD}Architecture réseau ZTNA (3 zones, 5 VMs)${NC}"
echo ""
echo -e "  ${BLUE}┌─────────────── WAN (10.10.10.0/24) ───────────────────────────┐${NC}"
echo -e "  ${BLUE}│${NC}                                                               ${BLUE}│${NC}"
echo -e "  ${BLUE}│${NC}  ${GREEN}┌──────────────────┐${NC}          ${YELLOW}┌──────────────────────┐${NC}     ${BLUE}│${NC}"
echo -e "  ${BLUE}│${NC}  ${GREEN}│  wan-client       │${NC}          ${YELLOW}│  ztna-gw             │${NC}     ${BLUE}│${NC}"
echo -e "  ${BLUE}│${NC}  ${GREEN}│  10.10.10.10      │${NC}  mTLS   ${YELLOW}│  10.10.10.20 (WAN)   │${NC}     ${BLUE}│${NC}"
echo -e "  ${BLUE}│${NC}  ${GREEN}│                   │${NC} ════════${YELLOW}│  10.10.20.20 (DMZ)   │${NC}     ${BLUE}│${NC}"
echo -e "  ${BLUE}│${NC}  ${GREEN}│  ZTNA CLI Client  │${NC}  TLS1.3 ${YELLOW}│  10.10.30.20 (LAN)   │${NC}     ${BLUE}│${NC}"
echo -e "  ${BLUE}│${NC}  ${GREEN}│  + Device Cert    │${NC}         ${YELLOW}│                      │${NC}     ${BLUE}│${NC}"
echo -e "  ${BLUE}│${NC}  ${GREEN}└──────────────────┘${NC}          ${YELLOW}│  Gateway ZTNA        │${NC}     ${BLUE}│${NC}"
echo -e "  ${BLUE}│${NC}                                ${YELLOW}│  - mTLS Listener     │${NC}     ${BLUE}│${NC}"
echo -e "  ${BLUE}└────────────────────────────────${YELLOW}│  - CRL Auto-Refresh  │${NC}─────${BLUE}┘${NC}"
echo -e "                                   ${YELLOW}│  - SSRF Protection   │${NC}"
echo -e "  ${MAGENTA}┌─────────── DMZ (10.10.20.0/24) ${YELLOW}│  - Session Manager   │${NC}─────${MAGENTA}┐${NC}"
echo -e "  ${MAGENTA}│${NC}                                ${YELLOW}│  - Decision Cache    │${NC}     ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}┌──────────────────┐${NC}        ${YELLOW}│  - Heartbeat         │${NC}     ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│  ztna-cp          │${NC}  PEP   ${YELLOW}│  - Telemetry         │${NC}     ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│  10.10.20.30      │${NC} ◄═════${YELLOW}└──────────────────────┘${NC}     ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│                   │${NC}  authz                           ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│  Control Plane    │${NC}                                   ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│  - Policy Engine  │${NC}                                   ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│  - PKI/CA         │${NC}                                   ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│  - Session Store  │${NC}                                   ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│  - OIDC/Keycloak  │${NC}                                   ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}│  - CRL Manager    │${NC}                                   ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}│${NC}  ${CYAN}└──────────────────┘${NC}                                   ${MAGENTA}│${NC}"
echo -e "  ${MAGENTA}└───────────────────────────────────────────────────────────────┘${NC}"
echo ""
echo -e "  ${RED}┌─────────── LAN (10.10.30.0/24) ───────────────────────────────┐${NC}"
echo -e "  ${RED}│${NC}                                                               ${RED}│${NC}"
echo -e "  ${RED}│${NC}  ┌──────────────────┐          ┌──────────────────┐          ${RED}│${NC}"
echo -e "  ${RED}│${NC}  │  lan-app          │          │  lan-admin       │          ${RED}│${NC}"
echo -e "  ${RED}│${NC}  │  10.10.30.10      │          │  10.10.30.11     │          ${RED}│${NC}"
echo -e "  ${RED}│${NC}  │  HTTP/SSH/TCP     │          │  Admin tools     │          ${RED}│${NC}"
echo -e "  ${RED}│${NC}  │  (protected)      │          │  (admin only)    │          ${RED}│${NC}"
echo -e "  ${RED}│${NC}  └──────────────────┘          └──────────────────┘          ${RED}│${NC}"
echo -e "  ${RED}└───────────────────────────────────────────────────────────────┘${NC}"
echo ""

echo -e "  ${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Security features
echo -e "  ${BOLD}Fonctionnalités de sécurité implémentées${NC}"
echo ""
echo -e "  ${GREEN}✓${NC} mTLS TLS 1.3 — Authentification mutuelle client/gateway"
echo -e "  ${GREEN}✓${NC} CRL Auto-Refresh — Révocation de certificats en temps réel"
echo -e "  ${GREEN}✓${NC} Protection SSRF — Blocage loopback, link-local, metadata cloud"
echo -e "  ${GREEN}✓${NC} Session Manager — TTL enforcement, GC, per-subject limits, admin kill"
echo -e "  ${GREEN}✓${NC} Decision Cache — Cache LRU des autorisations avec TTL"
echo -e "  ${GREEN}✓${NC} CP Down Mode — Décisions de repli si le Control Plane est indisponible"
echo -e "  ${GREEN}✓${NC} Heartbeat — Battement de cœur périodique Gateway→Control Plane"
echo -e "  ${GREEN}✓${NC} Télémétrie — Notification start/end de sessions au CP"
echo -e "  ${GREEN}✓${NC} MaxBytesReader — Protection contre les requêtes surdimensionnées"
echo -e "  ${GREEN}✓${NC} Sanitisation d'erreurs — Pas de fuite d'erreurs internes"
echo -e "  ${GREEN}✓${NC} Policy Engine — Évaluation des politiques ABAC (device, user, resource)"
echo -e "  ${GREEN}✓${NC} Graceful Shutdown — Drain des sessions, fermeture ordonnée"
echo -e "  ${GREEN}✓${NC} Architecture hexagonale — Ports & adapters, testabilité maximale"
echo ""

echo -e "  ${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Connection flow
echo -e "  ${BOLD}Flux de connexion ZTNA (CONNECT Protocol)${NC}"
echo ""
echo -e "  ${DIM}1.${NC} Client CLI → ${GREEN}Login OIDC${NC} → Keycloak → JWT"
echo -e "  ${DIM}2.${NC} Client CLI → ${GREEN}Issue Cert${NC} → CP → Device Certificate (X.509)"
echo -e "  ${DIM}3.${NC} Client CLI → ${GREEN}mTLS Handshake${NC} → Gateway (TLS 1.3 + client cert)"
echo -e "  ${DIM}4.${NC} Gateway    → ${YELLOW}CRL Check${NC} → Certificat non révoqué ?"
echo -e "  ${DIM}5.${NC} Gateway    → ${YELLOW}Extract Identity${NC} → CN, Org (groups) du cert client"
echo -e "  ${DIM}6.${NC} Client     → ${GREEN}CONNECT Request${NC} → {action, resource} (length-prefixed JSON)"
echo -e "  ${DIM}7.${NC} Gateway    → ${CYAN}Decision Cache${NC} → Hit ? → skip CP call"
echo -e "  ${DIM}8.${NC} Gateway    → ${CYAN}CP Authorize${NC} → POST /api/v1/pep/authorize"
echo -e "  ${DIM}9.${NC} CP         → ${MAGENTA}Policy Engine${NC} → Évaluation ABAC → allow/deny + TTL"
echo -e "  ${DIM}10.${NC} Gateway   → ${GREEN}Session Register${NC} → UUID, TTL, per-subject limit check"
echo -e "  ${DIM}11.${NC} Gateway   → ${GREEN}Telemetry Start${NC} → POST /api/v1/pep/sessions/start"
echo -e "  ${DIM}12.${NC} Gateway   → ${GREEN}TCP Proxy${NC} → Relay bidirectionnel client ↔ cible"
echo -e "  ${DIM}13.${NC} GC        → ${RED}TTL Expired?${NC} → Kill session + CancelFunc"
echo -e "  ${DIM}14.${NC} Gateway   → ${GREEN}Telemetry End${NC} → POST /api/v1/pep/sessions/end"
echo ""

echo -e "  ${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  ${DIM}Appuyez sur Entrée pour revenir...${NC}"
read -r
