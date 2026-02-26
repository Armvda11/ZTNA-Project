#!/usr/bin/env bash
# =============================================================================
# demo/steps/00_welcome.sh — Bienvenue & présentation de l'architecture
# =============================================================================
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_ztna_banner
print_step_banner "0" "INTRODUCTION" "Architecture Zero Trust Network Access"

echo -e "${WHITE}${BOLD}Présentation du projet${NC}"
echo -e ""
print_kv "Architecture"   "Zero Trust — default-deny, verify-explicitly"
print_kv "Composants"     "Client • Gateway (PEP) • Control Plane (PDP)"
print_kv "Auth"           "OIDC / Keycloak (RS256 JWT)"
print_kv "Transport"      "mTLS TLS 1.3 — certificats X.509 courts (7j)"
print_kv "Politique"      "RBAC séquentiel — 1er match gagne"
print_kv "Flux 1"         "SSH via certificat signé par SSH CA"
print_kv "Flux 2"         "TCP via mTLS device-cert + proxy CP-autorisé"
echo -e ""

print_architecture

print_separator
echo -e "${DIM}Appuyez sur Entrée pour vérifier l'état des services…${NC}"
