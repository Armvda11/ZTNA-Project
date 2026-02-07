#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF_DIR="${ROOT_DIR}/lab/terraform"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_ok() { echo -e "${GREEN}[✓]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[⚠]${NC} $*"; }
log_err() { echo -e "${RED}[✗]${NC} $*"; }

ensure_prereqs() {
  command -v terraform >/dev/null 2>&1 || { log_err "terraform non trouvé"; exit 1; }
  command -v virsh >/dev/null 2>&1 || { log_err "virsh non trouvé"; exit 1; }
}

prepare_libvirt() {
  log_info "Préparation libvirt..."
  sudo systemctl start libvirtd || true
  sudo chmod 666 /var/run/libvirt/libvirt-sock 2>/dev/null || true
  log_ok "libvirtd prêt"
}

start_network_if_exists() {
  local net="$1"
  if virsh net-info "${net}" >/dev/null 2>&1; then
    virsh net-start "${net}" >/dev/null 2>&1 || true
    virsh net-autostart "${net}" >/dev/null 2>&1 || true
    log_ok "Réseau ${net} actif + autostart"
  else
    log_warn "Réseau ${net} absent (sera créé par Terraform si nécessaire)"
  fi
}

start_known_networks() {
  log_info "Activation des réseaux libvirt..."
  start_network_if_exists "wan-net"
  start_network_if_exists "dmz-net"
  start_network_if_exists "lan-net"
}

start_known_vms() {
  local vm
  log_info "Démarrage des VMs (idempotent)..."
  for vm in wan-client wan-attacker ztna-gw ztna-cp lan-app lan-admin; do
    virsh start "${vm}" >/dev/null 2>&1 || true
  done
  log_ok "Commande de démarrage envoyée aux VMs"
}

terraform_apply() {
  log_info "Terraform init/apply..."
  cd "${TF_DIR}"
  terraform init -upgrade
  terraform apply -var-file=terraform.tfvars -auto-approve
  log_ok "Terraform apply terminé"
}

final_check() {
  log_info "Vérification finale..."
  cd "${ROOT_DIR}"
  make check
}

main() {
  log_info "ZTNA Lab UP (tout-en-un)"
  ensure_prereqs
  prepare_libvirt
  start_known_networks
  terraform_apply
  start_known_networks
  start_known_vms
  sleep 10
  final_check
  log_ok "Lab prêt. Utilise: make ssh-client / make ssh-gw / make ssh-cp"
}

main "$@"

