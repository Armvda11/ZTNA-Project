#!/usr/bin/env bash
# =============================================================================
# bootstrap.sh — Installation & lancement complet du lab ZTNA (plug & play)
#
# Une seule commande, tout est fait automatiquement :
#   bash scripts/bootstrap.sh
#   ou : make bootstrap
#
# Ce script :
#   1. Vérifie les prérequis matériels (CPU virt, RAM, disque)
#   2. Installe tous les paquets système nécessaires
#   3. Configure KVM / libvirt / AppArmor
#   4. Ajoute l'utilisateur aux groupes libvirt + kvm
#   5. Génère la clé SSH si absente + synchronise terraform.tfvars
#   6. Installe Terraform si absent
#   7. Initialise Terraform (providers)
#   8. Crée les VMs via Terraform (make up)
#   9. Vérifie la connectivité SSH
#
# Compatible : Ubuntu 22.04 / 24.04 / 25.x  —  non-root requis (sudo disponible)
# =============================================================================

set -uo pipefail

# ---------------------------------------------------------------------------
# Couleurs
# ---------------------------------------------------------------------------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_FILE="/tmp/ztna-bootstrap-$(date +%Y%m%d-%H%M%S).log"
SKIP_VM=false
[[ "${1:-}" == "--skip-vm" ]] && SKIP_VM=true

hdr()  { echo -e "\n${BLUE}${BOLD}━━━ $* ━━━${NC}"; }
ok()   { echo -e "  ${GREEN}[✔]${NC} $*"; }
warn() { echo -e "  ${YELLOW}[⚠]${NC} $*"; }
err()  { echo -e "  ${RED}[✘]${NC} $*"; }
info() { echo -e "  ${BLUE}[→]${NC} $*"; }
step() { echo -e "\n${CYAN}${BOLD}▶ $*${NC}"; }
die()  { echo -e "\n${RED}${BOLD}FATAL: $*${NC}"; echo "Logs: ${LOG_FILE}"; exit 1; }

# Redirige aussi vers le log
exec > >(tee -a "${LOG_FILE}") 2>&1

# ---------------------------------------------------------------------------
banner() {
  clear
  echo -e "${BLUE}${BOLD}"
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║    ZTNA Lab — Bootstrap (Installation automatique)          ║"
  echo "║    Zero Trust Network Access — Plug & Play                  ║"
  echo "╚══════════════════════════════════════════════════════════════╝"
  echo -e "${NC}"
  echo -e "  ${BLUE}Logs complets :${NC} ${LOG_FILE}"
  echo -e "  ${BLUE}Durée estimée :${NC} 5-15 minutes (téléchargement image Ubuntu)"
  echo ""
}

# =============================================================================
# ÉTAPE 0 — Vérifications préalables
# =============================================================================
step_preflight() {
  hdr "0. Vérifications préalables"

  # Non-root
  if [[ "${EUID}" -eq 0 ]]; then
    die "Ne pas exécuter ce script en tant que root. Utilisez votre compte normal avec sudo."
  fi
  ok "Utilisateur non-root : ${USER}"

  # sudo disponible
  if ! sudo -n true 2>/dev/null && ! sudo -v 2>/dev/null; then
    die "Droits sudo introuvables. Exécutez : sudo usermod -aG sudo ${USER}"
  fi
  ok "Droits sudo disponibles"

  # OS
  if ! grep -qiE 'ubuntu|debian' /etc/os-release 2>/dev/null; then
    warn "OS non Ubuntu/Debian — ce script est optimisé pour Ubuntu"
  else
    local os; os=$(grep PRETTY_NAME /etc/os-release | cut -d'"' -f2)
    ok "OS : ${os}"
  fi

  # Virtualisation
  if ! grep -qE 'vmx|svm' /proc/cpuinfo; then
    die "VT-x / AMD-V non détecté. Activez la virtualisation dans le BIOS et relancez."
  fi
  ok "Virtualisation CPU : OK"

  # RAM
  local ram; ram=$(free -g | awk 'NR==2 {print $2}')
  if [[ "${ram:-0}" -lt 8 ]]; then
    die "RAM insuffisante : ${ram} GB (8 GB minimum)"
  fi
  [[ "${ram:-0}" -ge 16 ]] && ok "RAM : ${ram} GB" || warn "RAM : ${ram} GB (16 GB recommandés)"

  # Disque
  local disk; disk=$(df / | awk 'NR==2 {printf "%d", $4/1024/1024}')
  if [[ "${disk:-0}" -lt 40 ]]; then
    die "Espace disque insuffisant : ${disk} GB (40 GB minimum)"
  fi
  [[ "${disk:-0}" -ge 100 ]] && ok "Espace libre : ${disk} GB" \
    || warn "Espace libre : ${disk} GB (100 GB recommandés)"
}

# =============================================================================
# ÉTAPE 1 — Paquets système
# =============================================================================
step_packages() {
  hdr "1. Installation des paquets système"

  info "Mise à jour des dépôts APT..."
  sudo apt-get update -qq || warn "apt update a rencontré des erreurs (non bloquant)"

  # Liste des paquets à installer (noms réels Ubuntu 22+)
  local pkgs=(
    qemu-system-x86
    qemu-utils
    libvirt-daemon-system
    libvirt-clients
    libvirt-daemon
    virtinst
    cpu-checker
    cloud-image-utils
    apparmor
    apparmor-utils
    openssh-client
    openssh-server
    curl
    wget
    git
    make
    build-essential
    jq
    unzip
  )

  local to_install=()
  for p in "${pkgs[@]}"; do
    if dpkg -s "${p}" &>/dev/null 2>&1; then
      ok "Déjà installé : ${p}"
    else
      to_install+=("${p}")
    fi
  done

  if [[ ${#to_install[@]} -gt 0 ]]; then
    info "Installation : ${to_install[*]}"
    for p in "${to_install[@]}"; do
      if apt-cache show "${p}" &>/dev/null 2>&1; then
        sudo apt-get install -y "${p}" && ok "Installé : ${p}" \
          || warn "Impossible d'installer ${p} (non bloquant)"
      else
        warn "Paquet '${p}' introuvable dans les dépôts — ignoré"
      fi
    done
  else
    ok "Tous les paquets déjà présents"
  fi
}

# =============================================================================
# ÉTAPE 2 — Modules KVM
# =============================================================================
step_kvm() {
  hdr "2. Configuration KVM"

  if [[ -c /dev/kvm ]]; then
    ok "/dev/kvm présent — KVM déjà opérationnel"
    return
  fi

  info "Chargement des modules KVM..."
  sudo modprobe kvm 2>/dev/null || true
  if grep -q "vmx" /proc/cpuinfo; then
    sudo modprobe kvm_intel 2>/dev/null || true
  elif grep -q "svm" /proc/cpuinfo; then
    sudo modprobe kvm_amd 2>/dev/null || true
  fi

  if [[ -c /dev/kvm ]]; then
    ok "/dev/kvm disponible après chargement"
  else
    warn "/dev/kvm toujours absent — vérifiez le BIOS"
  fi

  # Persister
  if [[ ! -f /etc/modules-load.d/kvm.conf ]]; then
    if grep -q "vmx" /proc/cpuinfo; then
      printf 'kvm\nkvm_intel\n' | sudo tee /etc/modules-load.d/kvm.conf >/dev/null
    else
      printf 'kvm\nkvm_amd\n' | sudo tee /etc/modules-load.d/kvm.conf >/dev/null
    fi
    ok "Modules KVM rendus persistants"
  fi
}

# =============================================================================
# ÉTAPE 3 — libvirtd
# =============================================================================
step_libvirt() {
  hdr "3. Service libvirtd"

  sudo systemctl enable libvirtd 2>/dev/null || true
  sudo systemctl start libvirtd 2>/dev/null || true
  sleep 1

  if systemctl is-active --quiet libvirtd; then
    ok "libvirtd actif"
  else
    warn "libvirtd n'a pas démarré — vérifiez : sudo journalctl -u libvirtd"
  fi

  # Fix socket
  local sock="/var/run/libvirt/libvirt-sock"
  if [[ -S "${sock}" ]]; then
    sudo chmod 660 "${sock}" 2>/dev/null || true
    ok "Socket libvirt : ${sock}"
  fi
}

# =============================================================================
# ÉTAPE 4 — Groupes utilisateur
# =============================================================================
step_groups() {
  hdr "4. Groupes utilisateur (${USER})"

  local changed=false
  for grp in libvirt kvm; do
    if getent group "${grp}" &>/dev/null; then
      if id -Gn | grep -qw "${grp}"; then
        ok "Déjà dans le groupe ${grp}"
      else
        sudo usermod -aG "${grp}" "${USER}"
        ok "Ajouté au groupe ${grp}"
        changed=true
      fi
    fi
  done

  if $changed; then
    warn "Groupes modifiés — on utilise 'sg libvirt' automatiquement pour cette session"
    warn "Pour une session persistante : déconnectez-vous et reconnectez-vous"
  fi
}

# =============================================================================
# ÉTAPE 5 — AppArmor
# =============================================================================
step_apparmor() {
  hdr "5. AppArmor"

  local aa_local="/etc/apparmor.d/local/usr.sbin.libvirtd"
  local aa_rule='  /var/lib/libvirt/images/ztna-lab/** rwk,'

  sudo mkdir -p "$(dirname "${aa_local}")"

  if [[ -f "${aa_local}" ]] && grep -q 'ztna-lab' "${aa_local}" 2>/dev/null; then
    ok "Règle AppArmor ztna-lab déjà présente"
  else
    echo "${aa_rule}" | sudo tee -a "${aa_local}" >/dev/null
    sudo systemctl reload apparmor 2>/dev/null || true
    sudo systemctl restart libvirtd 2>/dev/null || true
    ok "Règle AppArmor ajoutée pour /var/lib/libvirt/images/ztna-lab"
  fi

  # Créer le répertoire du pool
  local pool_dir="/var/lib/libvirt/images/ztna-lab"
  if [[ ! -d "${pool_dir}" ]]; then
    sudo mkdir -p "${pool_dir}"
    sudo chown root:libvirt "${pool_dir}"
    sudo chmod 775 "${pool_dir}"
    ok "Répertoire pool créé : ${pool_dir}"
  else
    ok "Répertoire pool présent : ${pool_dir}"
  fi
}

# =============================================================================
# ÉTAPE 6 — Terraform
# =============================================================================
step_terraform() {
  hdr "6. Terraform"

  if command -v terraform &>/dev/null; then
    local ver; ver=$(terraform version 2>/dev/null | head -1)
    ok "Terraform déjà installé : ${ver}"
    return
  fi

  local TF_VERSION="1.9.8"
  local TF_URL="https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_amd64.zip"
  local TMP; TMP=$(mktemp -d)

  info "Téléchargement Terraform ${TF_VERSION}..."
  if wget -q "${TF_URL}" -O "${TMP}/terraform.zip"; then
    unzip -q "${TMP}/terraform.zip" -d "${TMP}"
    sudo mv "${TMP}/terraform" /usr/local/bin/terraform
    sudo chmod +x /usr/local/bin/terraform
    rm -rf "${TMP}"
    ok "Terraform ${TF_VERSION} installé dans /usr/local/bin"
  else
    warn "Téléchargement Terraform échoué — vérifiez la connexion internet"
    warn "Installation manuelle : https://developer.hashicorp.com/terraform/downloads"
    rm -rf "${TMP}"
  fi
}

# =============================================================================
# ÉTAPE 7 — Clé SSH
# =============================================================================
step_ssh_key() {
  hdr "7. Clé SSH"

  local key="${HOME}/.ssh/id_ed25519"
  local tf_vars="${ROOT_DIR}/lab/terraform/terraform.tfvars"

  if [[ ! -f "${key}" ]]; then
    mkdir -p "${HOME}/.ssh"
    chmod 700 "${HOME}/.ssh"
    ssh-keygen -t ed25519 -f "${key}" -N "" -C "${USER}@$(hostname)"
    ok "Clé SSH générée : ${key}"
  else
    ok "Clé SSH présente : ${key}"
  fi

  # Synchroniser dans terraform.tfvars
  local pub; pub=$(cat "${key}.pub")
  if [[ -f "${tf_vars}" ]]; then
    if grep -qF "${pub}" "${tf_vars}"; then
      ok "Clé SSH synchronisée dans terraform.tfvars"
    else
      sed -i "s|^ssh_public_key.*|ssh_public_key = \"${pub}\"|" "${tf_vars}"
      ok "terraform.tfvars mis à jour avec la clé courante"
    fi
  else
    echo "ssh_public_key = \"${pub}\"" > "${tf_vars}"
    ok "terraform.tfvars créé"
  fi

  # Copie dans le dossier terraform
  cp "${key}.pub" "${ROOT_DIR}/lab/terraform/ssh_public_key.pub" 2>/dev/null || true
}

# =============================================================================
# ÉTAPE 8 — Initialisation Terraform
# =============================================================================
step_tf_init() {
  hdr "8. Initialisation Terraform (providers)"

  local tf_dir="${ROOT_DIR}/lab/terraform"
  local lock="${tf_dir}/.terraform.lock.hcl"

  if [[ -f "${lock}" ]] && [[ -d "${tf_dir}/.terraform/providers" ]]; then
    ok "Providers Terraform déjà téléchargés"
  else
    info "Exécution de terraform init -upgrade..."
    if sg libvirt -c "cd '${tf_dir}' && terraform init -upgrade"; then
      ok "Providers Terraform initialisés"
    else
      warn "terraform init a échoué — vérifiez la connexion internet"
    fi
  fi
}

# =============================================================================
# ÉTAPE 9 — Création des VMs
# =============================================================================
step_create_vms() {
  hdr "9. Création des VMs (Terraform)"

  if $SKIP_VM; then
    warn "Création des VMs ignorée (--skip-vm)"
    return
  fi

  info "Lancement de make up..."
  info "(Téléchargement image Ubuntu cloud la première fois : 5-10 min)"
  echo ""

  if make -C "${ROOT_DIR}" up; then
    ok "Infrastructure VMs créée"
  else
    err "make up a échoué"
    err "Vérifiez les erreurs ci-dessus, puis relancez : make up"
    err "Pour diagnostiquer : make doctor"
    return 1
  fi
}

# =============================================================================
# ÉTAPE 10 — Vérification finale
# =============================================================================
step_verify() {
  hdr "10. Vérification finale"

  info "Lancement de make doctor..."
  if make -C "${ROOT_DIR}" doctor-dry 2>/dev/null | grep -E '(RÉSUMÉ|ERREURS)'; then
    ok "Diagnostic terminé"
  fi

  info "Liste des VMs"
  sg libvirt -c "virsh --connect qemu:///system list --all" 2>/dev/null || true
}

# =============================================================================
# Résumé final
# =============================================================================
print_final_summary() {
  echo ""
  echo -e "${GREEN}${BOLD}"
  echo "╔══════════════════════════════════════════════════════════════╗"
  echo "║    Bootstrap terminé !                                      ║"
  echo "╚══════════════════════════════════════════════════════════════╝"
  echo -e "${NC}"
  echo -e "  ${BLUE}Logs complets :${NC} ${LOG_FILE}"
  echo ""
  echo -e "${YELLOW}${BOLD}Commandes disponibles :${NC}"
  echo -e "  ${BOLD}make check${NC}      — vérifier l'état du lab (VMs + SSH + healthz)"
  echo -e "  ${BOLD}make deploy${NC}     — déployer le control-plane + Keycloak"
  echo -e "  ${BOLD}make deploy-gw${NC}  — déployer la gateway ZTNA"
  echo -e "  ${BOLD}make doctor${NC}     — diagnostiquer et réparer les problèmes"
  echo -e "  ${BOLD}make lab-start${NC}  — redémarrer les VMs après un reboot système"
  echo -e "  ${BOLD}make ssh-cp${NC}     — SSH vers le control-plane"
  echo -e "  ${BOLD}make ssh-gw${NC}     — SSH vers la gateway"
  echo ""
  echo -e "${YELLOW}Prochaine étape recommandée :${NC}"
  echo -e "  ${BOLD}make check${NC}  (attendre 60s pour le boot cloud-init des VMs)"
  echo -e "  puis : ${BOLD}make deploy && make deploy-gw${NC}"
  echo ""
}

# =============================================================================
# MAIN
# =============================================================================
main() {
  banner

  step_preflight
  step_packages
  step_kvm
  step_libvirt
  step_groups
  step_apparmor
  step_terraform
  step_ssh_key
  step_tf_init

  if ! $SKIP_VM; then
    step_create_vms || true   # non-fatal, l'utilisateur peut relancer make up
  fi

  step_verify
  print_final_summary
}

main "$@"
