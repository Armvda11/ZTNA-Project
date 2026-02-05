#!/bin/bash
###############################################################################
# ZTNA Lab - Setup Automatisé Complet
# 
# Ce script installe tous les prérequis et configure le lab ZTNA.
# Compatible : Ubuntu 22.04 LTS et 24.04 LTS
#
# Usage : ./setup.sh [--skip-vm] [--skip-docker]
###############################################################################

set -e

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_NAME="ZTNA"
TERRAFORM_VERSION="1.14.3"
GO_VERSION="1.21.0"

# Variables optionnelles
SKIP_VM=${SKIP_VM:-false}
SKIP_GO=${SKIP_GO:-false}

###############################################################################
# Fonctions Utilitaires
###############################################################################

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[⚠]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

check_root() {
    if [ "$EUID" -eq 0 ]; then
        log_error "Ne pas exécuter ce script en tant que root"
        exit 1
    fi
}

check_os() {
    log_info "Vérification du système d'exploitation..."
    
    if ! grep -E 'ubuntu|debian' /etc/os-release > /dev/null; then
        log_error "Ce script nécessite Ubuntu ou Debian"
        exit 1
    fi
    
    log_success "OS compatible détecté"
}

check_virtualization() {
    log_info "Vérification de la virtualisation..."
    
    if ! grep -E 'vmx|svm' /proc/cpuinfo > /dev/null; then
        log_error "VT-x ou AMD-V non détecté. Activez la virtualisation dans le BIOS."
        exit 1
    fi
    
    log_success "Virtualisation CPU détectée"
}

check_hardware() {
    log_info "Vérification des ressources..."
    
    RAM_GB=$(free -g | awk 'NR==2 {print $2}')
    DISK_GB=$(df / | awk 'NR==2 {printf "%.0f", $4/1024/1024}')
    
    if [ "$RAM_GB" -lt 16 ]; then
        log_warn "RAM faible (${RAM_GB} GB, recommandé 16 GB)"
    else
        log_success "RAM OK (${RAM_GB} GB)"
    fi
    
    if [ "$DISK_GB" -lt 100 ]; then
        log_error "Pas assez d'espace disque (${DISK_GB} GB, besoin 100 GB)"
        exit 1
    fi
    
    log_success "Espace disque OK (${DISK_GB} GB)"
}

###############################################################################
# Installation des Dépendances
###############################################################################

install_system_packages() {
    log_info "Mise à jour des paquets système..."
    sudo apt update
    sudo apt upgrade -y
    
    log_info "Installation des dépendances système..."
    sudo apt install -y \
        curl wget git make vim net-tools ssh \
        build-essential pkg-config \
        qemu-kvm qemu-system-x86 qemu-utils \
        libvirt-daemon libvirt-clients libvirt-daemon-system \
        virt-manager apparmor apparmor-utils \
        cloud-utils cloud-initramfs-growroot \
        python3 python3-pip python3-venv
    
    log_success "Dépendances système installées"
}

install_terraform() {
    log_info "Installation de Terraform..."
    
    if command -v terraform &> /dev/null; then
        CURRENT_VERSION=$(terraform version | grep Terraform | awk '{print $2}')
        log_success "Terraform déjà installé ($CURRENT_VERSION)"
        return
    fi
    
    TF_URL="https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip"
    TMP_DIR=$(mktemp -d)
    
    cd "$TMP_DIR"
    log_info "Téléchargement Terraform ${TERRAFORM_VERSION}..."
    wget -q "$TF_URL"
    unzip -q terraform_${TERRAFORM_VERSION}_linux_amd64.zip
    sudo mv terraform /usr/local/bin/
    cd - > /dev/null
    rm -rf "$TMP_DIR"
    
    log_success "Terraform ${TERRAFORM_VERSION} installé"
}

install_go() {
    if [ "$SKIP_GO" = true ]; then
        log_warn "Installation de Go skippée"
        return
    fi
    
    log_info "Installation de Go..."
    
    if command -v go &> /dev/null; then
        CURRENT_VERSION=$(go version | awk '{print $3}')
        log_success "Go déjà installé ($CURRENT_VERSION)"
        return
    fi
    
    GO_URL="https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
    TMP_DIR=$(mktemp -d)
    
    cd "$TMP_DIR"
    log_info "Téléchargement Go ${GO_VERSION}..."
    wget -q "$GO_URL"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
    cd - > /dev/null
    rm -rf "$TMP_DIR"
    
    # Ajouter au PATH
    if ! grep -q '/usr/local/go/bin' ~/.bashrc; then
        echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
    fi
    
    log_success "Go ${GO_VERSION} installé"
    log_warn "Exécutez : source ~/.bashrc"
}

###############################################################################
# Configuration KVM/Libvirt
###############################################################################

setup_kvm() {
    if [ "$SKIP_VM" = true ]; then
        log_warn "Configuration KVM skippée"
        return
    fi
    
    log_info "Configuration KVM/libvirt..."
    
    # Charger les modules KVM
    log_info "Chargement des modules KVM..."
    sudo modprobe kvm
    if grep -q "^flags.*vmx" /proc/cpuinfo; then
        sudo modprobe kvm_intel
    elif grep -q "^flags.*svm" /proc/cpuinfo; then
        sudo modprobe kvm_amd
    fi
    
    # Ajouter l'utilisateur au groupe libvirt
    log_info "Configuration des permissions..."
    sudo usermod -aG libvirt,kvm "$USER"
    
    # Démarrer libvirtd
    log_info "Démarrage de libvirtd..."
    sudo systemctl enable libvirtd
    sudo systemctl start libvirtd || true
    
    # Configurer AppArmor
    log_info "Configuration AppArmor..."
    setup_apparmor
    
    # Fixer les permissions du socket libvirt
    sudo chmod 666 /var/run/libvirt/libvirt-sock 2>/dev/null || true
    
    log_success "KVM/libvirt configuré"
    log_warn "IMPORTANT: Exécutez 'newgrp libvirt' ou reconnectez-vous pour que les permissions prennent effet"
}

setup_apparmor() {
    # Créer le répertoire s'il n'existe pas
    sudo mkdir -p /etc/apparmor.d/local
    
    # Ajouter le chemin ztna-lab au profil local libvirt
    if ! grep -q 'ztna-lab' /etc/apparmor.d/local/usr.sbin.libvirtd 2>/dev/null; then
        echo '  /var/lib/libvirt/images/ztna-lab/** rwk,' | sudo tee -a /etc/apparmor.d/local/usr.sbin.libvirtd > /dev/null
    fi
    
    # Recharger AppArmor
    sudo systemctl reload apparmor 2>/dev/null || true
}

###############################################################################
# Configuration SSH
###############################################################################

setup_ssh() {
    log_info "Configuration SSH..."
    
    if [ ! -f ~/.ssh/id_ed25519 ]; then
        log_info "Génération de la clé SSH..."
        mkdir -p ~/.ssh
        chmod 700 ~/.ssh
        ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -N "" -C "$USER@$(hostname)"
        log_success "Clé SSH créée"
    else
        log_success "Clé SSH déjà existante"
    fi
    
    # Exporter la clé publique pour les VMs
    cp ~/.ssh/id_ed25519.pub "$SCRIPT_DIR/lab/terraform/ssh_public_key.pub"
    log_success "Clé SSH publique exportée"
}

###############################################################################
# Initialisation Terraform
###############################################################################

setup_terraform() {
    if [ "$SKIP_VM" = true ]; then
        log_warn "Initialisation Terraform skippée"
        return
    fi
    
    log_info "Initialisation Terraform..."
    
    cd "$SCRIPT_DIR/lab/terraform"
    
    # Vérifier si le provider libvirt est disponible
    if ! terraform init -backend=false 2>/dev/null; then
        log_warn "Les providers Terraform doivent être téléchargés"
        log_info "Exécutez manuellement : cd $SCRIPT_DIR/lab/terraform && terraform init"
    else
        log_success "Terraform initialisé"
    fi
    
    cd - > /dev/null
}

###############################################################################
# Création des Répertoires
###############################################################################

setup_directories() {
    log_info "Création de la structure de répertoires..."
    
    mkdir -p "$SCRIPT_DIR/lab/terraform/cloudinit"
    mkdir -p "$SCRIPT_DIR/control-plane"
    mkdir -p "$SCRIPT_DIR/gateway"
    mkdir -p "$SCRIPT_DIR/docs"
    mkdir -p "$SCRIPT_DIR/scripts"
    mkdir -p "$SCRIPT_DIR/.terraform"
    
    log_success "Répertoires créés"
}

###############################################################################
# Script Principal
###############################################################################

main() {
    clear
    
    echo -e "${BLUE}"
    echo "╔════════════════════════════════════════════════════════╗"
    echo "║   ZTNA Lab - Setup Automatisé Complet                 ║"
    echo "║   Zero Trust Network Access Infrastructure Lab         ║"
    echo "╚════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
    
    log_info "Début de l'installation..."
    echo ""
    
    # Vérifications
    check_root
    check_os
    check_virtualization
    check_hardware
    echo ""
    
    # Installation
    log_info "Phase 1 : Installation des dépendances"
    echo ""
    install_system_packages
    install_terraform
    install_go
    echo ""
    
    # Configuration
    log_info "Phase 2 : Configuration du système"
    echo ""
    setup_directories
    setup_ssh
    setup_kvm
    setup_terraform
    echo ""
    
    # Résumé
    echo -e "${GREEN}"
    echo "╔════════════════════════════════════════════════════════╗"
    echo "║   Installation Terminée avec Succès ! ✓               ║"
    echo "╚════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
    
    echo -e "${YELLOW}Prochaines étapes :${NC}"
    echo "  1. Mettez à jour le groupe : newgrp libvirt"
    echo "  2. Vérifiez les prérequis : ./scripts/check-requirements.sh"
    echo "  3. Lancez le lab : make init"
    echo "  4. Vérifiez l'infrastructure : ./scripts/check-lab.sh"
    echo ""
    
    log_info "Documentation :"
    echo "  - Architecture : ARCHITECTURE.md"
    echo "  - Dépannage : docs/TROUBLESHOOTING.md"
    echo "  - Configuration : SETUP.md"
    echo ""
}

# Exécution
main "$@"
