#!/usr/bin/env bash
# ============================================================================
# ZTNA Lab — Auto-install des prérequis manquants
# ============================================================================
# Détecte l'OS, vérifie chaque dépendance, et installe celles qui manquent.
# Supporte: Ubuntu/Debian, Fedora/RHEL, Arch Linux.
#
# Usage:
#   bash scripts/auto-install.sh              # Mode interactif
#   bash scripts/auto-install.sh --yes        # Mode non-interactif (auto-accept)
#   bash scripts/auto-install.sh --dry-run    # Afficher sans installer
# ============================================================================

set -euo pipefail

# ----- Colors ---------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

ok()   { echo -e "${GREEN}[✓]${NC} $*"; }
warn() { echo -e "${YELLOW}[!]${NC} $*"; }
err()  { echo -e "${RED}[✗]${NC} $*"; }
info() { echo -e "${BLUE}[→]${NC} $*"; }
step() { echo -e "${CYAN}${BOLD}[$1/$TOTAL_STEPS]${NC} $2"; }

AUTO_YES=false
DRY_RUN=false

for arg in "$@"; do
  case "$arg" in
    --yes|-y)     AUTO_YES=true ;;
    --dry-run|-n) DRY_RUN=true ;;
    --help|-h)
      echo "Usage: $0 [--yes|-y] [--dry-run|-n] [--help|-h]"
      echo "  --yes       Skip confirmations (non-interactive)"
      echo "  --dry-run   Show what would be installed without installing"
      exit 0
      ;;
  esac
done

# ----- OS Detection ---------------------------------------------------------
detect_os() {
  if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_ID="${ID:-unknown}"
    OS_FAMILY="${ID_LIKE:-$OS_ID}"
    OS_PRETTY="${PRETTY_NAME:-$OS_ID}"
  elif [ -f /etc/debian_version ]; then
    OS_ID="debian"
    OS_FAMILY="debian"
    OS_PRETTY="Debian $(cat /etc/debian_version)"
  else
    OS_ID="unknown"
    OS_FAMILY="unknown"
    OS_PRETTY="Unknown Linux"
  fi
}

detect_os

# Determine package manager
if command -v apt-get >/dev/null 2>&1; then
  PKG_MGR="apt"
  PKG_UPDATE="sudo apt-get update -qq"
  PKG_INSTALL="sudo apt-get install -y -qq"
elif command -v dnf >/dev/null 2>&1; then
  PKG_MGR="dnf"
  PKG_UPDATE="true"
  PKG_INSTALL="sudo dnf install -y -q"
elif command -v pacman >/dev/null 2>&1; then
  PKG_MGR="pacman"
  PKG_UPDATE="sudo pacman -Sy --noconfirm"
  PKG_INSTALL="sudo pacman -S --noconfirm --needed"
else
  PKG_MGR="unknown"
fi

echo ""
echo -e "${CYAN}${BOLD}╔═══════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}${BOLD}║   ZTNA Lab — Auto-installation des prérequis     ║${NC}"
echo -e "${CYAN}${BOLD}╚═══════════════════════════════════════════════════╝${NC}"
echo ""
info "OS détecté: ${OS_PRETTY}"
info "Package manager: ${PKG_MGR}"
echo ""

# ----- Dependency definitions -----------------------------------------------
# Format: command|package_apt|package_dnf|package_pacman|description|required
DEPS=(
  "terraform|terraform|terraform|terraform|IaC pour VMs libvirt|required"
  "virsh|libvirt-daemon-system libvirt-clients|libvirt libvirt-client|libvirt|Gestion VMs KVM|required"
  "ssh|openssh-client|openssh-clients|openssh|Client SSH|required"
  "scp|openssh-client|openssh-clients|openssh|Copie sécurisée|required"
  "curl|curl|curl|curl|Client HTTP|required"
  "go|golang|golang|go|Go compiler (build CP/GW)|required"
  "tmux|tmux|tmux|tmux|Terminal multiplexer (demo)|required"
  "jq|jq|jq|jq|JSON processor|recommended"
  "openssl|openssl|openssl|openssl|TLS debugging tools|recommended"
  "python3|python3|python3|python|Scripts utilitaires|optional"
  "qemu-img|qemu-utils|qemu-img|qemu|QEMU tools (VM images)|required"
  "make|make|make|make|Build automation|required"
  "git|git|git|git|Version control|required"
  "watch|procps|procps-ng|procps-ng|Monitoring (demo)|optional"
)

# ----- Check functions ------------------------------------------------------
TOTAL_STEPS=5
MISSING_REQUIRED=()
MISSING_RECOMMENDED=()
MISSING_OPTIONAL=()
ALREADY_OK=()
TO_INSTALL=()

confirm() {
  if [ "$AUTO_YES" = true ]; then return 0; fi
  if [ "$DRY_RUN" = true ]; then return 1; fi
  read -rp "$(echo -e "${YELLOW}Continuer? [Y/n] ${NC}")" answer
  case "$answer" in
    [nN]*) return 1 ;;
    *)     return 0 ;;
  esac
}

get_pkg_name() {
  local apt_pkg="$1"
  local dnf_pkg="$2"
  local pacman_pkg="$3"
  case "$PKG_MGR" in
    apt)    echo "$apt_pkg" ;;
    dnf)    echo "$dnf_pkg" ;;
    pacman) echo "$pacman_pkg" ;;
    *)      echo "$apt_pkg" ;;
  esac
}

# Step 1: Scan dependencies
step 1 "Scan des dépendances..."
echo ""

for dep in "${DEPS[@]}"; do
  IFS='|' read -r cmd apt_pkg dnf_pkg pacman_pkg desc level <<< "$dep"

  if command -v "$cmd" >/dev/null 2>&1; then
    version=$("$cmd" --version 2>&1 | head -1 | grep -oP '[\d]+\.[\d]+[\.\d]*' | head -1 || echo "?")
    ok "$desc ($cmd $version)"
    ALREADY_OK+=("$cmd")
  else
    pkg_name=$(get_pkg_name "$apt_pkg" "$dnf_pkg" "$pacman_pkg")
    case "$level" in
      required)
        err "$desc ($cmd) — REQUIS"
        MISSING_REQUIRED+=("$pkg_name|$cmd|$desc")
        ;;
      recommended)
        warn "$desc ($cmd) — RECOMMANDÉ"
        MISSING_RECOMMENDED+=("$pkg_name|$cmd|$desc")
        ;;
      optional)
        warn "$desc ($cmd) — optionnel"
        MISSING_OPTIONAL+=("$pkg_name|$cmd|$desc")
        ;;
    esac
  fi
done

# Step 2: Check system services
echo ""
step 2 "Vérification services système..."
echo ""

SERVICES_TO_START=()

# libvirtd
if systemctl list-unit-files libvirtd.service >/dev/null 2>&1; then
  if systemctl is-active --quiet libvirtd 2>/dev/null; then
    ok "Service libvirtd actif"
  else
    warn "Service libvirtd inactif"
    SERVICES_TO_START+=("libvirtd")
  fi
else
  if [ ${#MISSING_REQUIRED[@]} -eq 0 ] || ! printf '%s\n' "${MISSING_REQUIRED[@]}" | grep -q "virsh"; then
    warn "libvirtd non trouvé (sera installé avec libvirt)"
  fi
fi

# Check libvirt group membership
if id -Gn 2>/dev/null | grep -qw 'libvirt'; then
  ok "Utilisateur dans le groupe libvirt"
else
  warn "Utilisateur $(whoami) n'est pas dans le groupe libvirt"
fi

# Check KVM group
if id -Gn 2>/dev/null | grep -qw 'kvm'; then
  ok "Utilisateur dans le groupe kvm"
else
  warn "Utilisateur $(whoami) n'est pas dans le groupe kvm"
fi

# Step 3: Check Terraform provider
echo ""
step 3 "Vérification composants spécifiques..."
echo ""

# Go version check  
if command -v go >/dev/null 2>&1; then
  GO_VER=$(go version | grep -oP '[\d]+\.[\d]+' | head -1)
  GO_MAJOR=${GO_VER%%.*}
  GO_MINOR=${GO_VER#*.}
  if [ "${GO_MAJOR:-0}" -ge 1 ] && [ "${GO_MINOR:-0}" -ge 21 ]; then
    ok "Go version $GO_VER (>= 1.21 requis)"
  else
    warn "Go version $GO_VER détectée — 1.21+ requis"
  fi
fi

# Terraform libvirt provider
if [ -d "$HOME/.terraform.d/plugins" ] || [ -d "$HOME/.local/share/terraform/plugins" ]; then
  if find "$HOME/.terraform.d" "$HOME/.local/share/terraform" -name "*libvirt*" -type f 2>/dev/null | grep -q .; then
    ok "Terraform provider libvirt trouvé"
  else
    warn "Terraform provider libvirt non trouvé (sera téléchargé par terraform init)"
  fi
else
  info "Terraform plugins seront téléchargés lors de 'terraform init'"
fi

# SSH key
if [ -f "$HOME/.ssh/id_ed25519" ]; then
  ok "Clé SSH Ed25519 trouvée"
elif [ -f "$HOME/.ssh/id_rsa" ]; then
  warn "Clé RSA trouvée — Ed25519 recommandée (ssh-keygen -t ed25519)"
else
  warn "Aucune clé SSH trouvée — sera nécessaire pour les VMs"
fi

# Step 4: Install missing packages
echo ""
step 4 "Installation des paquets manquants..."
echo ""

ALL_MISSING=("${MISSING_REQUIRED[@]}" "${MISSING_RECOMMENDED[@]}")

if [ ${#ALL_MISSING[@]} -eq 0 ]; then
  ok "Toutes les dépendances requises et recommandées sont installées!"
else
  echo -e "${YELLOW}Paquets à installer:${NC}"
  PKGS_TO_INSTALL=()
  for entry in "${ALL_MISSING[@]}"; do
    IFS='|' read -r pkg cmd desc <<< "$entry"
    echo "  • $desc ($cmd) → $pkg"
    # Split space-separated packages
    for p in $pkg; do
      PKGS_TO_INSTALL+=("$p")
    done
  done
  echo ""

  if [ "$DRY_RUN" = true ]; then
    info "[DRY-RUN] Commande: $PKG_INSTALL ${PKGS_TO_INSTALL[*]}"
  elif [ "$PKG_MGR" = "unknown" ]; then
    err "Package manager non supporté. Installez manuellement: ${PKGS_TO_INSTALL[*]}"
  elif confirm; then
    info "Mise à jour des dépôts..."
    eval "$PKG_UPDATE" 2>/dev/null || true

    info "Installation: ${PKGS_TO_INSTALL[*]}"
    if eval "$PKG_INSTALL ${PKGS_TO_INSTALL[*]}"; then
      ok "Paquets installés avec succès"
    else
      err "Certains paquets ont échoué — vérifiez les messages ci-dessus"
    fi
  else
    warn "Installation annulée"
  fi
fi

# Terraform — special handling (HashiCorp repo)
if ! command -v terraform >/dev/null 2>&1; then
  echo ""
  info "Terraform nécessite le dépôt HashiCorp..."
  if [ "$DRY_RUN" = true ]; then
    info "[DRY-RUN] Ajout du dépôt HashiCorp + installation terraform"
  elif [ "$PKG_MGR" = "apt" ]; then
    info "Ajout du dépôt HashiCorp pour Terraform..."
    if confirm; then
      sudo apt-get update -qq && sudo apt-get install -y -qq gnupg software-properties-common
      wget -qO- https://apt.releases.hashicorp.com/gpg | \
        gpg --dearmor | sudo tee /usr/share/keyrings/hashicorp-archive-keyring.gpg >/dev/null
      echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | \
        sudo tee /etc/apt/sources.list.d/hashicorp.list >/dev/null
      sudo apt-get update -qq && sudo apt-get install -y terraform
      ok "Terraform installé"
    fi
  elif [ "$PKG_MGR" = "dnf" ]; then
    if confirm; then
      sudo dnf install -y yum-utils
      sudo yum-config-manager --add-repo https://rpm.releases.hashicorp.com/fedora/hashicorp.repo
      sudo dnf install -y terraform
      ok "Terraform installé"
    fi
  fi
fi

# Step 5: Post-install configuration
echo ""
step 5 "Configuration post-installation..."
echo ""

# Add user to libvirt/kvm groups
GROUPS_TO_ADD=()
if ! id -Gn 2>/dev/null | grep -qw 'libvirt'; then
  GROUPS_TO_ADD+=(libvirt)
fi
if ! id -Gn 2>/dev/null | grep -qw 'kvm'; then
  GROUPS_TO_ADD+=(kvm)
fi

if [ ${#GROUPS_TO_ADD[@]} -gt 0 ]; then
  GROUP_LIST=$(IFS=,; echo "${GROUPS_TO_ADD[*]}")
  info "Ajout de $(whoami) aux groupes: $GROUP_LIST"
  if [ "$DRY_RUN" = true ]; then
    info "[DRY-RUN] sudo usermod -aG $GROUP_LIST $(whoami)"
  elif confirm; then
    sudo usermod -aG "$GROUP_LIST" "$(whoami)"
    ok "Groupes ajoutés — REDÉMARRAGE DE SESSION REQUIS (newgrp libvirt ou logout/login)"
  fi
fi

# Start libvirtd if needed
if [ ${#SERVICES_TO_START[@]} -gt 0 ]; then
  for svc in "${SERVICES_TO_START[@]}"; do
    info "Démarrage du service $svc..."
    if [ "$DRY_RUN" = true ]; then
      info "[DRY-RUN] sudo systemctl enable --now $svc"
    elif confirm; then
      sudo systemctl enable --now "$svc"
      ok "Service $svc activé et démarré"
    fi
  done
fi

# Generate SSH key if missing
if [ ! -f "$HOME/.ssh/id_ed25519" ] && [ ! -f "$HOME/.ssh/id_rsa" ]; then
  info "Génération d'une clé SSH Ed25519..."
  if [ "$DRY_RUN" = true ]; then
    info "[DRY-RUN] ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -N ''"
  elif confirm; then
    ssh-keygen -t ed25519 -f "$HOME/.ssh/id_ed25519" -N '' -C "ztna-lab@$(hostname)"
    ok "Clé SSH générée: $HOME/.ssh/id_ed25519"
  fi
fi

# ----- Summary --------------------------------------------------------------
echo ""
echo -e "${CYAN}${BOLD}═══════════════════════════════════════════════════${NC}"
echo -e "${CYAN}${BOLD}               RÉSUMÉ                              ${NC}"
echo -e "${CYAN}${BOLD}═══════════════════════════════════════════════════${NC}"
echo ""

echo -e "  ${GREEN}✓ Installés:${NC}   ${#ALREADY_OK[@]} dépendances déjà présentes"
if [ ${#MISSING_REQUIRED[@]} -gt 0 ]; then
  echo -e "  ${RED}✗ Requis:${NC}      ${#MISSING_REQUIRED[@]} à installer"
fi
if [ ${#MISSING_RECOMMENDED[@]} -gt 0 ]; then
  echo -e "  ${YELLOW}! Recommandés:${NC} ${#MISSING_RECOMMENDED[@]} à installer"
fi
if [ ${#MISSING_OPTIONAL[@]} -gt 0 ]; then
  echo -e "  ${BLUE}○ Optionnels:${NC}  ${#MISSING_OPTIONAL[@]} non installés"
fi
echo ""

if [ "$DRY_RUN" = true ]; then
  info "Mode dry-run — aucune modification effectuée."
  echo "  Relancez sans --dry-run pour installer."
else
  echo -e "${BOLD}Prochaines étapes:${NC}"
  echo "  1. make bootstrap          # Initialiser l'infra complète"
  echo "  2. make deploy && make deploy-gw"
  echo "  3. make check              # Vérifier que tout fonctionne"
  echo "  4. make demo               # Lancer la démo visuelle"
fi
echo ""
