#!/usr/bin/env bash
# Démarre les VMs déjà créées et vérifie la connectivité SSH de base.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VIRSH="bash ${ROOT_DIR}/scripts/virsh-lab"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_BASE="ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 -i ${SSH_KEY}"
WAIT_SECONDS="${WAIT_SECONDS:-45}"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()   { echo -e "${GREEN}[✓]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail() { echo -e "${RED}[✗]${NC} $*"; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { fail "Commande requise introuvable: $1"; exit 1; }
}

start_network_if_exists() {
  local net="$1"
  if ${VIRSH} net-info "${net}" >/dev/null 2>&1; then
    ${VIRSH} net-start "${net}" >/dev/null 2>&1 || true
    ${VIRSH} net-autostart "${net}" >/dev/null 2>&1 || true
    ok "Réseau ${net} actif"
  else
    warn "Réseau ${net} absent (normal si géré par Terraform)"
  fi
}

start_vm() {
  local vm="$1"
  if ${VIRSH} domstate "${vm}" 2>/dev/null | grep -qi running; then
    ok "${vm} déjà démarrée"
  else
    ${VIRSH} start "${vm}" >/dev/null 2>&1 && ok "${vm} démarrée" || warn "Impossible de démarrer ${vm}"
  fi
}

check_ssh() {
  local name="$1"
  local ip="$2"
  if timeout 8 ${SSH_BASE} ztna@"${ip}" 'hostname' >/dev/null 2>&1; then
    ok "SSH ${name} (${ip})"
    return 0
  fi
  fail "SSH ${name} (${ip})"
  return 1
}

main() {
  require_cmd ssh
  require_cmd timeout
  require_cmd virsh

  if [ ! -f "${SSH_KEY}" ]; then
    fail "Clé SSH introuvable: ${SSH_KEY}"
    fail "Générez-la avec: ssh-keygen -t ed25519 -f ${SSH_KEY} -N ''"
    exit 1
  fi

  info "Activation des réseaux libvirt"
  start_network_if_exists "wan-net"
  start_network_if_exists "dmz-net"
  start_network_if_exists "lan-net"

  info "Démarrage des VMs"
  for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do
    start_vm "${vm}"
  done

  info "Attente boot/cloud-init (${WAIT_SECONDS}s)"
  sleep "${WAIT_SECONDS}"

  info "Vérification SSH (WAN/DMZ)"
  SSH_FAILS=0
  check_ssh "wan-client" "10.10.10.10" || SSH_FAILS=$((SSH_FAILS + 1))
  check_ssh "ztna-gw" "10.10.10.20" || SSH_FAILS=$((SSH_FAILS + 1))
  check_ssh "ztna-cp" "10.10.20.30" || SSH_FAILS=$((SSH_FAILS + 1))

  echo
  info "État VMs"
  ${VIRSH} list --all || true

  if [ "${SSH_FAILS}" -gt 0 ]; then
    fail "${SSH_FAILS} vérification(s) SSH en échec. Réessayez dans 30-60s puis 'make check-ssh'."
    exit 1
  fi

  ok "Lab démarré et joignable"
  info "Suite: make deploy && make deploy-gw && make check"
}

main "$@"
