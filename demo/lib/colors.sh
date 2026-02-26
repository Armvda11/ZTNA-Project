#!/usr/bin/env bash
# =============================================================================
# demo/lib/colors.sh — Codes couleur ANSI et fonctions d'affichage formaté
# =============================================================================

# Couleurs de base
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
WHITE='\033[1;37m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'   # No Color / Reset

# Couleurs sémantiques ZTNA
COLOR_ALLOW="${GREEN}"
COLOR_DENY="${RED}"
COLOR_WAIT="${YELLOW}"
COLOR_INFO="${CYAN}"
COLOR_STEP="${MAGENTA}"
COLOR_OK="${GREEN}"

# ─── Fonctions d'affichage ───────────────────────────────────────────────────

print_ok() {
    echo -e "${GREEN}[✓]${NC} $*"
}

print_err() {
    echo -e "${RED}[✗]${NC} $*"
}

print_info() {
    echo -e "${CYAN}[i]${NC} $*"
}

print_warn() {
    echo -e "${YELLOW}[!]${NC} $*"
}

print_step() {
    local title="$*"
    echo -e ""
    echo -e "${MAGENTA}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${MAGENTA}${BOLD}  $title${NC}"
    echo -e "${MAGENTA}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e ""
}

print_allow() {
    echo -e "${GREEN}${BOLD}┌─────────────────────────────────┐${NC}"
    echo -e "${GREEN}${BOLD}│   ✅  ACCÈS AUTORISÉ — ALLOW    │${NC}"
    echo -e "${GREEN}${BOLD}└─────────────────────────────────┘${NC}"
}

print_deny() {
    echo -e "${RED}${BOLD}┌─────────────────────────────────┐${NC}"
    echo -e "${RED}${BOLD}│   🚫  ACCÈS REFUSÉ  — DENY     │${NC}"
    echo -e "${RED}${BOLD}└─────────────────────────────────┘${NC}"
}

print_separator() {
    echo -e "${DIM}─────────────────────────────────────────────────────────────${NC}"
}

# Affiche une paire clé/valeur joliment
print_kv() {
    local key="$1"
    local val="$2"
    printf "${CYAN}%-22s${NC} %s\n" "$key" "$val"
}
