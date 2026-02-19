#!/usr/bin/env bash
# Lancement complet du lab ZTNA (architecture simplifiée)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TF_DIR="${ROOT_DIR}/lab/terraform"

log_info() { echo "[INFO] $*"; }
log_ok() { echo "[✓] $*"; }

main() {
  log_info "=== ZTNA Lab UP (Architecture simplifiée) ==="
  
  cd "${TF_DIR}"
  
  log_info "Application Terraform..."
  terraform init -upgrade >/dev/null 2>&1
  terraform apply -auto-approve
  
  log_ok "Infrastructure créée (5 VMs, 3 réseaux)"
  
  log_info ""
  log_info "Attente boot VMs (60s pour cloud-init Docker)..."
  sleep 60
  
  log_info ""
  log_info "=== NEXT STEPS ==="
  log_info ""
  log_info "1. Déployer le control-plane:"
  log_info "   bash scripts/deploy-control-plane.sh"
  log_info ""
  log_info "2. Accès SSH direct (tous en NAT, accessibles depuis ton PC):"
  log_info "   ssh ztna@10.10.10.10  # wan-client"
  log_info "   ssh ztna@10.10.20.30  # ztna-cp (DMZ)"
  log_info "   ssh ztna@10.10.10.20  # ztna-gw"
  log_info ""
  log_info "3. Test depuis wan-client:"
  log_info "   ssh ztna@10.10.10.10"
  log_info "   curl http://10.10.20.30:8081/realms/ztna"
  log_info ""
  log_ok "Lab prêt !"
}

main "$@"
