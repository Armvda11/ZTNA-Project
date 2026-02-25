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

cleanup_orphan_cloudinit() {
  # Les volumes cloud-init ISO peuvent rester orphelins si le state Terraform
  # a été recréé. On les supprime s'ils sont présents dans libvirt mais pas
  # dans le state Terraform — Terraform les recréera à l'apply.
  local pool_name="ztna-lab"
  local -a ci_vols=(
    "libvirt_cloudinit_disk.wan_client_ci:wan-client-ci.iso"
    "libvirt_cloudinit_disk.ztna_gw_ci:ztna-gw-ci.iso"
    "libvirt_cloudinit_disk.ztna_cp_ci:ztna-cp-ci.iso"
    "libvirt_cloudinit_disk.lan_app_ci:lan-app-ci.iso"
    "libvirt_cloudinit_disk.lan_admin_ci:lan-admin-ci.iso"
  )

  info "Nettoyage des volumes cloud-init orphelins"

  for entry in "${ci_vols[@]}"; do
    local tf_resource="${entry%%:*}"
    local vol_name="${entry#*:}"

    # Si déjà suivi par Terraform → rien à faire
    if tf state show "${tf_resource}" >/dev/null 2>&1; then
      continue
    fi

    # Si le volume existe dans libvirt → le supprimer (sera recréé par Terraform)
    if virsh_lab vol-info --pool "${pool_name}" "${vol_name}" >/dev/null 2>&1; then
      info "Suppression du volume orphelin : ${vol_name}"
      virsh_lab vol-delete --pool "${pool_name}" "${vol_name}" >/dev/null 2>&1 \
        && ok "Volume orphelin supprimé : ${vol_name}" \
        || fail "Impossible de supprimer ${vol_name}"
    fi
  done
}

reconcile_pool_state() {
  local pool_name="ztna-lab"
  local tf_resource="libvirt_pool.ztna"

  info "Réconciliation Terraform state <-> libvirt (pool)"

  if tf state show "${tf_resource}" >/dev/null 2>&1; then
    ok "Pool '${pool_name}' déjà suivi par Terraform"
    return
  fi

  if ! virsh_lab pool-info "${pool_name}" >/dev/null 2>&1; then
    info "Pool '${pool_name}' absent dans libvirt, sera créé par Terraform"
    return
  fi

  # Le provider libvirt v0.7.x attend un UUID, pas un nom
  local pool_uuid
  pool_uuid="$(virsh_lab pool-dumpxml "${pool_name}" 2>/dev/null \
    | grep '<uuid>' | sed 's|.*<uuid>\(.*\)</uuid>.*|\1|' | tr -d '[:space:]')"

  if [ -z "${pool_uuid}" ]; then
    fail "UUID introuvable pour le pool '${pool_name}', import impossible"
    exit 1
  fi

  info "Import pool '${pool_name}' (${pool_uuid}) dans ${tf_resource}"
  if tf import "${tf_resource}" "${pool_uuid}" >/dev/null 2>&1; then
    ok "Pool '${pool_name}' importé dans le state Terraform"
  else
    fail "Échec import Terraform pour le pool '${pool_name}'"
    exit 1
  fi
}

reconcile_network_state() {
  local entry tf_resource net_name net_uuid
  local -a networks=(
    "libvirt_network.wan:wan-net"
    "libvirt_network.dmz:dmz-net"
    "libvirt_network.lan:lan-net"
  )

  info "Réconciliation Terraform state <-> libvirt (réseaux)"

  for entry in "${networks[@]}"; do
    tf_resource="${entry%%:*}"
    net_name="${entry#*:}"

    if tf state show "${tf_resource}" >/dev/null 2>&1; then
      ok "Réseau '${net_name}' déjà suivi par Terraform"
      continue
    fi

    if ! virsh_lab net-info "${net_name}" >/dev/null 2>&1; then
      info "Réseau '${net_name}' absent dans libvirt, sera créé par Terraform"
      continue
    fi

    net_uuid="$(virsh_lab net-uuid "${net_name}" 2>/dev/null | tr -d '[:space:]')"
    if [ -z "${net_uuid}" ]; then
      fail "UUID introuvable pour le réseau '${net_name}', import impossible"
      exit 1
    fi

    info "Import réseau '${net_name}' (${net_uuid}) dans ${tf_resource}"
    if tf import "${tf_resource}" "${net_uuid}" >/dev/null 2>&1; then
      ok "Réseau '${net_name}' importé dans le state Terraform"
    else
      fail "Échec import Terraform pour le réseau '${net_name}'"
      exit 1
    fi
  done
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
    if tf import "${tf_resource}" "${vm_uuid}" >/dev/null 2>&1; then
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

  reconcile_pool_state
  reconcile_network_state
  reconcile_domain_state
  cleanup_orphan_cloudinit

  info "Terraform apply"
  tf apply -var-file=terraform.tfvars -auto-approve

  ok "Infrastructure prête"
  info "Étapes suivantes:"
  info "  make deploy"
  info "  make deploy-gw"
  info "  make check"
}

main "$@"
