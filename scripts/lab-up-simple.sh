#!/usr/bin/env bash
# Création/maj de l'infrastructure lab (Terraform + libvirt)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF_WRAPPER="${ROOT_DIR}/scripts/tf-lab"
VIRSH_WRAPPER="${ROOT_DIR}/scripts/virsh-lab"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()   { echo -e "${GREEN}[✓]${NC} $*"; }
fail() { echo -e "${RED}[✗]${NC} $*"; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { fail "Commande requise introuvable: $1"; exit 1; }
}

tf() {
  bash "${TF_WRAPPER}" "$@"
}

virsh_lab() {
  bash "${VIRSH_WRAPPER}" "$@"
}

reconcile_domain_state() {
  local entry tf_resource vm_name vm_uuid
  local -a domains=(
    "libvirt_domain.wan_client:wan-client"
    "libvirt_domain.ztna_gw:ztna-gw"
    "libvirt_domain.ztna_cp:ztna-cp"
    "libvirt_domain.lan_app:lan-app"
    "libvirt_domain.lan_admin:lan-admin"
  )

  info "Réconciliation Terraform state <-> libvirt (domains)"

  for entry in "${domains[@]}"; do
    tf_resource="${entry%%:*}"
    vm_name="${entry#*:}"

    if tf state show "${tf_resource}" >/dev/null 2>&1; then
      ok "${vm_name} déjà suivi par Terraform"
      continue
    fi

    if ! virsh_lab dominfo "${vm_name}" >/dev/null 2>&1; then
      info "${vm_name} absent dans libvirt, création par Terraform"
      continue
    fi

    vm_uuid="$(virsh_lab domuuid "${vm_name}" 2>/dev/null | tr -d '[:space:]')"
    if [ -z "${vm_uuid}" ]; then
      fail "UUID introuvable pour ${vm_name}, import impossible"
      exit 1
    fi

    info "Import ${vm_name} (${vm_uuid}) dans ${tf_resource}"
    if tf import "${tf_resource}" "${vm_uuid}" >/dev/null; then
      ok "${vm_name} importée dans le state Terraform"
    else
      fail "Échec import Terraform pour ${vm_name}"
      exit 1
    fi
  done
}

main() {
  info "ZTNA Lab UP (infra)"

  require_cmd terraform
  require_cmd sg
  require_cmd virsh

  if [ ! -x "${TF_WRAPPER}" ]; then
    fail "Wrapper Terraform manquant ou non exécutable: ${TF_WRAPPER}"
    exit 1
  fi

  if [ ! -x "${VIRSH_WRAPPER}" ]; then
    fail "Wrapper virsh manquant ou non exécutable: ${VIRSH_WRAPPER}"
    exit 1
  fi

  info "Terraform init"
  tf init -upgrade

  reconcile_domain_state

  info "Terraform apply"
  tf apply -var-file=terraform.tfvars -auto-approve

  ok "Infrastructure prête"
  info "Étapes suivantes:"
  info "  make deploy"
  info "  make deploy-gw"
  info "  make check"
}

main "$@"
