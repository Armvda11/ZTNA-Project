#!/usr/bin/env bash
# =============================================================================
# ztna-doctor.sh — Diagnostic & Réparation automatique du lab ZTNA
#
# Ce script détecte et corrige automatiquement les causes les plus fréquentes
# d'échec au lancement des VMs (KVM, libvirt, groupes, AppArmor, Terraform,
# SSH, provider, image cloud, pool, socket).
#
# Usage :
#   bash scripts/ztna-doctor.sh         # diagnostic + réparation
#   bash scripts/ztna-doctor.sh --dry   # diagnostic seulement (pas de fix)
#   bash scripts/ztna-doctor.sh --full  # tout réparer + relancer les VMs
#
# Prérequis : Ubuntu 22.04 / 24.04, droits sudo
# =============================================================================

set -uo pipefail

# ---------------------------------------------------------------------------
# Couleurs et helpers
# ---------------------------------------------------------------------------
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

OK=0; WARN=0; FIXED=0; ERR=0
DRY_RUN=false
FULL_MODE=false

for arg in "$@"; do
  case "$arg" in
    --dry)  DRY_RUN=true ;;
    --full) FULL_MODE=true ;;
  esac
done

hdr()  { echo -e "\n${BLUE}${BOLD}━━━ $* ━━━${NC}"; }
ok()   { echo -e "  ${GREEN}[✔]${NC} $*"; OK=$((OK+1)); }
warn() { echo -e "  ${YELLOW}[⚠]${NC} $*"; WARN=$((WARN+1)); }
err()  { echo -e "  ${RED}[✘]${NC} $*"; ERR=$((ERR+1)); }
fix()  { echo -e "  ${CYAN}[FIX]${NC} $*"; FIXED=$((FIXED+1)); }
info() { echo -e "  ${BLUE}[→]${NC} $*"; }

run_fix() {
  # run_fix "description" "commande"
  local desc="$1"; shift
  if $DRY_RUN; then
    fix "${desc} (--dry: ignoré)"
  else
    fix "${desc}"
    eval "$*"
  fi
}

need_relogin=false

# ---------------------------------------------------------------------------
banner() {
  echo -e "${BLUE}${BOLD}"
  echo "╔═══════════════════════════════════════════════════════════════╗"
  echo "║      ZTNA Lab — Doctor (Diagnostic & Réparation)             ║"
  if $DRY_RUN; then
  echo "║      MODE : DRY-RUN (lecture seule)                          ║"
  elif $FULL_MODE; then
  echo "║      MODE : FULL (réparation + tentative de lancement VMs)   ║"
  else
  echo "║      MODE : AUTO-REPAIR                                      ║"
  fi
  echo "╚═══════════════════════════════════════════════════════════════╝"
  echo -e "${NC}"
}

# =============================================================================
# 1. Système & matériel
# =============================================================================
check_system() {
  hdr "1. Système & Matériel"

  # OS
  if grep -qiE 'ubuntu|debian' /etc/os-release 2>/dev/null; then
    local os; os=$(grep PRETTY_NAME /etc/os-release | cut -d'"' -f2)
    ok "OS : ${os}"
  else
    warn "OS non testé (Ubuntu/Debian recommandé)"
  fi

  # Virtualisation CPU
  if grep -qE 'vmx|svm' /proc/cpuinfo; then
    ok "VT-x / AMD-V détecté dans /proc/cpuinfo"
  else
    warn "Virtualisation CPU absente — on continue quand même (les VMs peuvent échouer)"
  fi

  # RAM
  local ram; ram=$(free -g | awk 'NR==2 {print $2}')
  if [[ "${ram:-0}" -ge 16 ]]; then
    ok "RAM : ${ram} GB"
  elif [[ "${ram:-0}" -ge 8 ]]; then
    warn "RAM : ${ram} GB (16 GB recommandés, lab peut être lent)"
  else
    warn "RAM : ${ram} GB — faible, on continue quand même"
  fi

  # Disque
  local disk; disk=$(df / | awk 'NR==2 {printf "%d", $4/1024/1024}')
  if [[ "${disk:-0}" -ge 100 ]]; then
    ok "Espace libre : ${disk} GB"
  elif [[ "${disk:-0}" -ge 40 ]]; then
    warn "Espace libre : ${disk} GB (100 GB recommandés)"
  else
    warn "Espace libre : ${disk} GB — faible, on continue quand même"
  fi
}

# =============================================================================
# 2. Paquets requis
# =============================================================================
check_packages() {
  hdr "2. Paquets Système"

  # Mapping : nom_affiché → paquet_apt_réel
  # Certains méta-paquets (qemu-kvm, ssh, scp) n'existent pas sous Ubuntu 25+
  # On vérifie d'abord si le binaire/service est disponible avant de déclarer manquant.
  declare -A PKG_MAP=(
    [qemu-kvm]="qemu-system-x86"          # méta-paquet → alias
    [qemu-system-x86]="qemu-system-x86"
    [qemu-utils]="qemu-utils"
    [libvirt-daemon-system]="libvirt-daemon-system"
    [libvirt-clients]="libvirt-clients"
    [libvirt-daemon]="libvirt-daemon"
    [virtinst]="virtinst"
    [cpu-checker]="cpu-checker"
    [cloud-utils]="cloud-image-utils"     # renommé sur Ubuntu 22+
    [apparmor]="apparmor"
    [apparmor-utils]="apparmor-utils"
    [curl]="curl"
    [wget]="wget"
    [git]="git"
    [make]="make"
    [ssh]="openssh-client"               # pas de paquet nommé 'ssh'
    [scp]="openssh-client"               # scp est inclus dans openssh-client
  )

  # Binaires correspondant à chaque label (pour détecter les providers alternatifs)
  declare -A BIN_MAP=(
    [qemu-kvm]="qemu-system-x86_64"
    [qemu-system-x86]="qemu-system-x86_64"
    [qemu-utils]="qemu-img"
    [libvirt-daemon-system]=""
    [libvirt-clients]="virsh"
    [libvirt-daemon]=""
    [virtinst]="virt-install"
    [cpu-checker]="kvm-ok"
    [cloud-utils]="cloud-localds"
    [apparmor]="apparmor_parser"
    [apparmor-utils]="aa-status"
    [curl]="curl"
    [wget]="wget"
    [git]="git"
    [make]="make"
    [ssh]="ssh"
    [scp]="scp"
  )

  local labels=(qemu-kvm qemu-system-x86 qemu-utils
    libvirt-daemon-system libvirt-clients libvirt-daemon
    virtinst cpu-checker cloud-utils
    apparmor apparmor-utils
    curl wget git make ssh scp)

  local missing_pkgs=()
  local last_pkg=""

  for label in "${labels[@]}"; do
    local apt_pkg="${PKG_MAP[${label}]}"
    local bin="${BIN_MAP[${label}]}"

    # Si le binaire est disponible, on considère satisfait même si le nom de paquet diffère
    if [[ -n "${bin}" ]] && command -v "${bin}" &>/dev/null; then
      ok "${label} (binaire '${bin}' présent)"
      continue
    fi

    # Sinon vérifier le paquet apt (avec le nom réel)
    if dpkg -s "${apt_pkg}" &>/dev/null 2>&1; then
      ok "Paquet : ${apt_pkg}"
    else
      err "Manquant : ${label} (paquet apt : ${apt_pkg})"
      # Éviter les doublons (ssh et scp → même paquet openssh-client)
      if [[ "${apt_pkg}" != "${last_pkg}" ]]; then
        missing_pkgs+=("${apt_pkg}")
        last_pkg="${apt_pkg}"
      fi
    fi
  done

  if [[ ${#missing_pkgs[@]} -gt 0 ]]; then
    # Dédupliquer
    local -a dedup
    declare -A seen
    for p in "${missing_pkgs[@]}"; do
      if [[ -z "${seen[${p}]+x}" ]]; then
        dedup+=("${p}")
        seen["${p}"]=1
      fi
    done
    if $DRY_RUN; then
      fix "Installation des paquets manquants : ${dedup[*]} (--dry: ignoré)"
    else
      fix "Installation des paquets manquants : ${dedup[*]}"
      # Installer un par un pour ne pas bloquer sur un paquet inexistant
      for p in "${dedup[@]}"; do
        if apt-cache show "${p}" &>/dev/null 2>&1; then
          sudo apt-get install -y "${p}" || warn "Impossible d'installer ${p}"
        else
          warn "Paquet '${p}' introuvable dans les dépôts — ignoré"
        fi
      done
    fi
  fi

  # Binaires essentiels (post-install)
  for cmd in virsh terraform ssh scp curl; do
    if command -v "${cmd}" &>/dev/null; then
      local ver; ver=$(${cmd} --version 2>/dev/null | head -1 || true)
      ok "${cmd} : ${ver:-disponible}"
    else
      err "${cmd} introuvable dans PATH"
    fi
  done

  # Terraform spécifiquement
  if ! command -v terraform &>/dev/null; then
    _install_terraform
  fi
}

_install_terraform() {
  local TF_VERSION="1.14.3"
  info "Installation de Terraform ${TF_VERSION}..."
  if $DRY_RUN; then
    fix "Terraform serait installé (--dry)"
    return
  fi
  local TMP; TMP=$(mktemp -d)
  local URL="https://releases.hashicorp.com/terraform/${TF_VERSION}/terraform_${TF_VERSION}_linux_amd64.zip"
  fix "Téléchargement de Terraform ${TF_VERSION}"
  wget -q "${URL}" -O "${TMP}/terraform.zip"
  unzip -q "${TMP}/terraform.zip" -d "${TMP}"
  sudo mv "${TMP}/terraform" /usr/local/bin/terraform
  sudo chmod +x /usr/local/bin/terraform
  rm -rf "${TMP}"
  ok "Terraform ${TF_VERSION} installé dans /usr/local/bin"
  FIXED=$((FIXED+1))
}

# =============================================================================
# 3. Modules KVM
# =============================================================================
check_kvm_modules() {
  hdr "3. Modules KVM"

  # /dev/kvm est le test canonique : si présent, KVM fonctionne
  # (les modules peuvent être built-in dans le kernel → invisibles dans lsmod)
  if [[ -c /dev/kvm ]]; then
    ok "/dev/kvm présent — KVM opérationnel"
    # Vérifier aussi lsmod par curiosité, sans échouer si absent
    if lsmod | grep -q '^kvm'; then
      ok "Module kvm visible dans lsmod (chargé dynamiquement)"
    else
      info "Modules KVM non visibles dans lsmod (built-in kernel — normal sur noyaux récents)"
    fi
    return 0
  fi

  # /dev/kvm absent → essayer de charger les modules
  err "/dev/kvm absent — tentative de chargement des modules KVM"

  if grep -q "vmx" /proc/cpuinfo; then
    run_fix "Chargement kvm + kvm_intel" \
      "sudo modprobe kvm && sudo modprobe kvm_intel"
  elif grep -q "svm" /proc/cpuinfo; then
    run_fix "Chargement kvm + kvm_amd" \
      "sudo modprobe kvm && sudo modprobe kvm_amd"
  fi

  # Re-vérifier
  if [[ -c /dev/kvm ]]; then
    ok "/dev/kvm présent après chargement des modules"
  else
    err "/dev/kvm toujours absent — vérifiez le BIOS (VT-x/AMD-V) ou le kernel"
  fi

  # Rendre les modules persistants si nécessaire
  if [[ ! -f /etc/modules-load.d/kvm.conf ]] && ! $DRY_RUN; then
    if grep -q "vmx" /proc/cpuinfo; then
      printf 'kvm\nkvm_intel\n' | sudo tee /etc/modules-load.d/kvm.conf >/dev/null
    else
      printf 'kvm\nkvm_amd\n' | sudo tee /etc/modules-load.d/kvm.conf >/dev/null
    fi
    fix "Modules KVM rendus persistants dans /etc/modules-load.d/kvm.conf"
  fi
}

# =============================================================================
# 4. Service libvirtd
# =============================================================================
check_libvirtd() {
  hdr "4. Service libvirtd"

  if systemctl is-active --quiet libvirtd 2>/dev/null; then
    ok "libvirtd actif"
  else
    err "libvirtd inactif"
    run_fix "Activation et démarrage de libvirtd" \
      "sudo systemctl enable libvirtd && sudo systemctl start libvirtd"
  fi

  # Socket libvirt
  local sock="/var/run/libvirt/libvirt-sock"
  if [[ -S "${sock}" ]]; then
    ok "Socket libvirt présent : ${sock}"
    local perms; perms=$(stat -c '%a' "${sock}" 2>/dev/null || echo "???")
    if [[ "${perms}" == "666" ]] || [[ "${perms}" == "660" ]]; then
      ok "Permissions socket : ${perms}"
    else
      warn "Permissions socket : ${perms} (attendu 666 ou 660)"
      run_fix "Fix permissions socket libvirt" \
        "sudo chmod 666 ${sock}"
    fi
  else
    err "Socket libvirt absent (libvirtd démarré ?)"
  fi
}

# =============================================================================
# 5. Groupes utilisateur
# =============================================================================
check_groups() {
  hdr "5. Groupes Utilisateur (${USER})"

  local cur_groups; cur_groups=$(id -Gn 2>/dev/null || groups)

  for grp in libvirt kvm; do
    if getent group "${grp}" &>/dev/null; then
      if echo "${cur_groups}" | grep -qw "${grp}"; then
        ok "Membre du groupe ${grp} (session courante)"
      else
        # Vérifier si l'utilisateur est dans /etc/group même pas en session
        if getent group "${grp}" | grep -qw "${USER}"; then
          warn "Dans le groupe ${grp} mais pas rechargé — relancez : newgrp ${grp}"
          need_relogin=true
        else
          err "Hors du groupe ${grp}"
          run_fix "Ajout de ${USER} au groupe ${grp}" \
            "sudo usermod -aG ${grp} ${USER}"
          need_relogin=true
        fi
      fi
    else
      warn "Groupe ${grp} inexistant (sera créé à l'install de libvirt)"
    fi
  done

  if $need_relogin && ! $DRY_RUN; then
    echo ""
    warn "Les groupes ont été modifiés."
    warn "Pour cette session uniquement, on applique sg libvirt automatiquement"
    warn "lors des commandes virsh/terraform (via les wrappers scripts/virsh-lab"
    warn "et scripts/tf-lab). Pour une session complète, relancez: newgrp libvirt"
  fi
}

# =============================================================================
# 6. AppArmor
# =============================================================================
check_apparmor() {
  hdr "6. AppArmor"

  # aa-status peut être dans apparmor-utils (pas toujours installé)
  local aa_cmd
  if command -v aa-status &>/dev/null; then
    aa_cmd="aa-status"
  elif command -v apparmor_status &>/dev/null; then
    aa_cmd="apparmor_status"
  else
    warn "aa-status introuvable — AppArmor supposé actif, vérification partielle"
    aa_cmd=""
  fi

  if [[ -n "${aa_cmd}" ]]; then
    if sudo ${aa_cmd} --enabled 2>/dev/null; then
      ok "AppArmor actif"
    else
      warn "AppArmor inactif — aucune règle à corriger"
      return
    fi
  fi

  # Règle pour le pool d'images ztna-lab
  local aa_local="/etc/apparmor.d/local/usr.sbin.libvirtd"
  local aa_rule='  /var/lib/libvirt/images/ztna-lab/** rwk,'

  if [[ -f "${aa_local}" ]] && grep -q 'ztna-lab' "${aa_local}" 2>/dev/null; then
    ok "Règle AppArmor ztna-lab (images) déjà présente"
  else
    err "Règle AppArmor manquante pour /var/lib/libvirt/images/ztna-lab"
    if ! $DRY_RUN; then
      sudo mkdir -p "$(dirname "${aa_local}")"
      echo "${aa_rule}" | sudo tee -a "${aa_local}" >/dev/null
      sudo systemctl reload apparmor 2>/dev/null || true
      sudo systemctl restart libvirtd 2>/dev/null || true
      fix "Règle AppArmor ajoutée et libvirtd redémarré"
    else
      fix "Règle AppArmor serait ajoutée (--dry)"
    fi
  fi

  # Les blocages AppArmor fusermount3/VSCode ptrace sont inoffensifs pour libvirt
  # On les signale juste en info pour ne pas noyer les vraies erreurs
  local aa_noise
  aa_noise=$(sudo dmesg 2>/dev/null \
    | grep 'apparmor="DENIED"' \
    | grep -E 'vscode|fusermount' | wc -l 2>/dev/null || echo 0)
  if [[ "${aa_noise}" -gt 0 ]]; then
    info "${aa_noise} refus AppArmor VSCode/fusermount détectés (inoffensifs, ignorés)"
  fi
}

# =============================================================================
# 7. Clé SSH
# =============================================================================
check_ssh_key() {
  hdr "7. Clé SSH"

  local key="${SSH_KEY:-${HOME}/.ssh/id_ed25519}"

  if [[ -f "${key}" ]] && [[ -f "${key}.pub" ]]; then
    ok "Clé SSH présente : ${key}"
    # Vérifier la synchronisation avec terraform.tfvars
    local tf_key; tf_key="${ROOT_DIR}/lab/terraform/terraform.tfvars"
    if [[ -f "${tf_key}" ]]; then
      local pub; pub=$(cat "${key}.pub")
      if grep -qF "${pub}" "${tf_key}" 2>/dev/null; then
        ok "Clé SSH synchronisée dans terraform.tfvars"
      else
        err "Clé SSH dans terraform.tfvars NON synchronisée avec ${key}.pub"
        if ! $DRY_RUN; then
          # Remplacer la ligne ssh_public_key
          local escaped; escaped=$(echo "${pub}" | sed 's/[\/&]/\\&/g')
          sed -i "s|^ssh_public_key.*|ssh_public_key = \"${pub}\"|" "${tf_key}"
          fix "terraform.tfvars mis à jour avec la clé courante"
        else
          fix "terraform.tfvars serait mis à jour (--dry)"
        fi
      fi
    fi
  else
    err "Clé SSH absente : ${key}"
    if ! $DRY_RUN; then
      mkdir -p "${HOME}/.ssh"
      chmod 700 "${HOME}/.ssh"
      ssh-keygen -t ed25519 -f "${key}" -N "" -C "${USER}@$(hostname)"
      fix "Clé SSH générée : ${key}"
      # Synchroniser dans terraform.tfvars
      local tf_key="${ROOT_DIR}/lab/terraform/terraform.tfvars"
      local pub; pub=$(cat "${key}.pub")
      if [[ -f "${tf_key}" ]]; then
        sed -i "s|^ssh_public_key.*|ssh_public_key = \"${pub}\"|" "${tf_key}"
        fix "terraform.tfvars mis à jour"
      fi
      # Copier aussi dans le répertoire terraform
      cp "${key}.pub" "${ROOT_DIR}/lab/terraform/ssh_public_key.pub" 2>/dev/null || true
    else
      fix "Clé SSH serait générée (--dry)"
    fi
  fi
}

# =============================================================================
# 8. Pool et répertoire libvirt
# =============================================================================
check_libvirt_pool() {
  hdr "8. Pool Libvirt & Répertoires"

  local pool_dir="/var/lib/libvirt/images/ztna-lab"

  # Créer le répertoire du pool si absent
  if [[ -d "${pool_dir}" ]]; then
    ok "Répertoire pool : ${pool_dir}"
  else
    err "Répertoire pool absent : ${pool_dir}"
    run_fix "Création de ${pool_dir}" \
      "sudo mkdir -p '${pool_dir}' && sudo chown root:libvirt '${pool_dir}' && sudo chmod 775 '${pool_dir}'"
  fi

  # Vérifier le pool dans libvirt
  local pool_info
  pool_info=$(sg libvirt -c "virsh --connect qemu:///system pool-info ztna-lab" 2>/dev/null || true)

  if [[ -z "${pool_info}" ]]; then
    warn "Pool 'ztna-lab' absent dans libvirt (sera créé par Terraform lors de 'make up')"
  else
    # pool-info peut répondre en français ("en cours d'exécution" / "actif")
    # ou en anglais ("running" / "active") selon la locale
    if echo "${pool_info}" | grep -qiE 'running|actif|active|éxécution|execution|en cours'; then
      ok "Pool libvirt 'ztna-lab' actif"
    else
      local pool_state
      pool_state=$(echo "${pool_info}" | awk -F: 'NR==2{gsub(/^[[:space:]]+/,"",$2); print $2}' | head -1)
      err "Pool libvirt 'ztna-lab' pas actif (état: ${pool_state:-inconnu})"
      run_fix "Démarrage du pool ztna-lab" \
        "sg libvirt -c 'virsh --connect qemu:///system pool-start ztna-lab' || true"
    fi
  fi
}

# =============================================================================
# 9. Provider Terraform libvirt
# =============================================================================
check_terraform_init() {
  hdr "9. Terraform (provider libvirt)"

  local tf_dir="${ROOT_DIR}/lab/terraform"
  local provider_dir="${tf_dir}/.terraform/providers"

  if [[ -d "${provider_dir}" ]]; then
    ok "Providers Terraform déjà téléchargés"
  else
    err "Providers Terraform manquants (.terraform/ absent)"
    if ! $DRY_RUN; then
      fix "Exécution de terraform init -upgrade"
      sg libvirt -c "cd '${tf_dir}' && terraform init -upgrade" \
        || { err "terraform init a échoué — vérifiez la connexion internet"; return 1; }
      ok "terraform init réussi"
    else
      fix "terraform init serait exécuté (--dry)"
    fi
  fi

  # Vérifier le .terraform.lock.hcl
  if [[ -f "${tf_dir}/.terraform.lock.hcl" ]]; then
    ok "Lock file Terraform présent"
  else
    warn "Lock file Terraform absent (init probablement non exécuté)"
  fi

  # Vérifier terraform.tfvars
  if [[ -f "${tf_dir}/terraform.tfvars" ]]; then
    ok "terraform.tfvars présent"
  else
    err "terraform.tfvars absent"
    if ! $DRY_RUN; then
      local pub=""
      [[ -f "${HOME}/.ssh/id_ed25519.pub" ]] && pub=$(cat "${HOME}/.ssh/id_ed25519.pub")
      cat > "${tf_dir}/terraform.tfvars" <<EOF
ssh_public_key = "${pub}"
EOF
      fix "terraform.tfvars créé"
    else
      fix "terraform.tfvars serait créé (--dry)"
    fi
  fi
}

# =============================================================================
# 10. Connexion virsh
# =============================================================================
check_virsh_connection() {
  hdr "10. Connexion à libvirt (virsh)"

  if sg libvirt -c "virsh --connect qemu:///system list --all" &>/dev/null; then
    ok "virsh --connect qemu:///system → OK"
    local count; count=$(sg libvirt -c "virsh --connect qemu:///system list --all" 2>/dev/null | grep -c '^\s*[0-9-]' || echo 0)
    ok "VMs connues dans libvirt : ${count}"
  else
    err "Connexion à qemu:///system en échec"
    err "Tentative de correction : redémarrage libvirtd + fix socket"
    if ! $DRY_RUN; then
      sudo systemctl restart libvirtd
      sudo chmod 666 /var/run/libvirt/libvirt-sock 2>/dev/null || true
      sleep 2
      if sg libvirt -c "virsh --connect qemu:///system list --all" &>/dev/null; then
        fix "Connexion virsh rétablie après redémarrage"
      else
        err "Toujours impossible — vérifiez sudo journalctl -u libvirtd"
      fi
    fi
  fi
}

# =============================================================================
# 11. État actuel des VMs
# =============================================================================
check_vms_state() {
  hdr "11. État des VMs"

  local vms=(wan-client ztna-gw ztna-cp lan-app lan-admin)

  if ! sg libvirt -c "virsh --connect qemu:///system list --all" &>/dev/null; then
    warn "Impossible d'interroger libvirt — check précédent en échec"
    return
  fi

  for vm in "${vms[@]}"; do
    local state
    state=$(sg libvirt -c "virsh --connect qemu:///system domstate '${vm}'" 2>/dev/null | tr -d '[:space:]' || echo "absent")
    case "${state}" in
      running)  ok "${vm} : running" ;;
      "shut off"|paused)
        warn "${vm} : ${state}"
        if $FULL_MODE && ! $DRY_RUN; then
          sg libvirt -c "virsh --connect qemu:///system start '${vm}'" >/dev/null 2>&1 \
            && fix "${vm} démarré" || warn "Impossible de démarrer ${vm}"
        fi
        ;;
      absent)   warn "${vm} : absent (non créé — lancez 'make up')" ;;
      *)        warn "${vm} : ${state}" ;;
    esac
  done
}

# =============================================================================
# 12. Réseaux libvirt
# =============================================================================
check_networks() {
  hdr "12. Réseaux libvirt"

  local nets=(wan-net dmz-net lan-net)

  if ! sg libvirt -c "virsh --connect qemu:///system net-list --all" &>/dev/null; then
    warn "Impossible d'interroger les réseaux libvirt"
    return
  fi

  for net in "${nets[@]}"; do
    local state
    state=$(sg libvirt -c "virsh --connect qemu:///system net-info '${net}'" 2>/dev/null \
      | awk '/Active/{print $2}' || echo "absent")
    if [[ "${state}" == "yes" ]]; then
      ok "Réseau ${net} : actif"
    elif [[ "${state}" == "no" ]]; then
      err "Réseau ${net} : inactif"
      run_fix "Démarrage du réseau ${net}" \
        "sg libvirt -c \"virsh --connect qemu:///system net-start '${net}'\""
    else
      warn "Réseau ${net} absent (sera créé par Terraform)"
    fi
  done
}

# =============================================================================
# 13. Logs KVM récents
# =============================================================================
check_recent_errors() {
  hdr "13. Erreurs récentes KVM/libvirt"

  local errors
  errors=$(sudo journalctl -u libvirtd --since "10 minutes ago" --no-pager -q 2>/dev/null \
    | grep -iE 'error|fail|denied|cannot' | head -10 || true)

  if [[ -z "${errors}" ]]; then
    ok "Aucune erreur libvirtd dans les 10 dernières minutes"
  else
    warn "Erreurs libvirtd récentes :"
    echo "${errors}" | while read -r line; do
      echo "    ${line}"
    done
  fi

  # AppArmor — on ignore les blocages ptrace liés à VSCode/Electron (inoffensifs)
  # et les blocages fusermount3/FUSE (snap/docker) qui n'impactent pas libvirt
  # On filtre sur apparmor="DENIED" (et non denied_mask qui apparaît aussi dans les ALLOWED)
  local aa_denied
  aa_denied=$(sudo dmesg 2>/dev/null \
    | grep 'apparmor="DENIED"' \
    | grep -v 'vscode\|code-server\|electron\|peer="vscode"\|fusermount\|profile="Xorg"' \
    | tail -5 || true)
  if [[ -n "${aa_denied}" ]]; then
    err "Blocages AppArmor critiques détectés :"
    echo "${aa_denied}" | while read -r line; do
      echo "    ${line}"
    done
  else
    ok "Aucun blocage AppArmor critique (refus ptrace VSCode et fusermount ignorés — inoffensifs)"
  fi
}

# =============================================================================
# Résumé final
# =============================================================================
print_summary() {
  echo -e "\n${BOLD}${BLUE}━━━━━━━━━━━━━━━━━ RÉSUMÉ ━━━━━━━━━━━━━━━━━${NC}"
  echo -e "  ${GREEN}OK    :${NC} ${OK}"
  echo -e "  ${YELLOW}AVERT :${NC} ${WARN}"
  echo -e "  ${CYAN}FIXES :${NC} ${FIXED}"
  echo -e "  ${RED}ERREURS:${NC} ${ERR}"
  echo -e "${BOLD}${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

  if [[ "${ERR}" -eq 0 ]] && [[ "${WARN}" -eq 0 ]]; then
    echo -e "\n  ${GREEN}${BOLD}✔ Tout est en ordre — lancez : make up${NC}"
  elif [[ "${ERR}" -eq 0 ]]; then
    echo -e "\n  ${YELLOW}${BOLD}⚠ Quelques avertissements — lab peut fonctionner${NC}"
    echo -e "  Lancez : make up"
  else
    echo -e "\n  ${RED}${BOLD}✘ Des erreurs persistent${NC}"
    if $DRY_RUN; then
      echo -e "  Relancez sans --dry pour corriger automatiquement :"
      echo -e "    ${BOLD}bash scripts/ztna-doctor.sh${NC}"
    else
      echo -e "  Vérifiez les points marqués [✘] ci-dessus."
      echo -e "  Consultez : TROUBLESHOOTING.md"
    fi
  fi

  if $need_relogin && ! $DRY_RUN; then
    echo ""
    echo -e "  ${YELLOW}${BOLD}⚠ Groupes modifiés — pour cette session exécutez :${NC}"
    echo -e "     ${BOLD}newgrp libvirt${NC}"
    echo -e "  (ou déconnectez-vous / reconnectez-vous pour que ce soit permanent)"
  fi

  echo ""
  echo -e "  ${BLUE}Commandes essentielles post-repair :${NC}"
  echo -e "    make prereq    # re-check prérequis"
  echo -e "    make up        # créer/mettre à jour les VMs (Terraform)"
  echo -e "    make lab-start # démarrer les VMs existantes"
  echo -e "    make check     # vérification globale"
  echo ""
}

# =============================================================================
# MAIN
# =============================================================================
main() {
  banner

  check_system
  check_packages
  check_kvm_modules
  check_libvirtd
  check_groups
  check_apparmor
  check_ssh_key
  check_libvirt_pool
  check_terraform_init
  check_virsh_connection
  check_vms_state
  check_networks
  check_recent_errors

  print_summary
}

main "$@"
