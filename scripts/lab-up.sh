#!/usr/bin/env bash
# Wrapper de compatibilité: conserve l'entrée historique scripts/lab-up.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()   { echo -e "${GREEN}[✓]${NC} $*"; }

main() {
  info "Lab UP (compat): redirection vers scripts/lab-up-simple.sh"
  bash "${ROOT_DIR}/scripts/lab-up-simple.sh"
  ok "Terminé"
}

main "$@"
