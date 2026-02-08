#!/bin/bash
###############################################################################
# ZTNA Lab - Script de Nettoyage
# 
# Ce script nettoie complètement le lab (VMs + files)
#
# Usage : ./scripts/cleanup.sh [--force]
###############################################################################

set -e

# Couleurs
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

FORCE=${1:-}
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_DIR="$( cd "$SCRIPT_DIR/.." && pwd )"
TERRAFORM_DIR="$PROJECT_DIR/lab/terraform"

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[⚠]${NC} $1"; }

###############################################################################

log_info "Nettoyage du Lab ZTNA"

if [ "$FORCE" != "--force" ]; then
    echo -e ""
    log_warn "Ceci supprimera :"
    echo "  - Toutes les VMs du lab"
    echo "  - Tous les réseaux libvirt"
    echo "  - Tous les fichiers images"
    echo "  - L'état Terraform"
    echo ""
    read -p "Êtes-vous sûr ? (tapez 'oui') : " CONFIRM
    if [ "$CONFIRM" != "oui" ]; then
        log_info "Annulation"
        exit 0
    fi
fi

echo ""

###############################################################################
# Détruire avec Terraform
###############################################################################

if cd "$TERRAFORM_DIR" && [ -f terraform.tfstate ]; then
    log_info "Destruction via Terraform..."
    if terraform destroy -var-file=terraform.tfvars -auto-approve; then
        log_success "Ressources Terraform détruite"
    else
        log_warn "Erreur lors de la destruction Terraform (nettoyage manuel)"
    fi
    cd - > /dev/null
else
    log_info "Pas d'état Terraform trouvé, nettoyage manuel..."
fi

echo ""

###############################################################################
# Nettoyage manuel des VMs
###############################################################################

log_info "Nettoyage manuel des VMs..."

for vm in wan-client wan-attacker ztna-gw ztna-cp lan-app lan-admin; do
    if virsh list --all 2>/dev/null | grep -q "$vm"; then
        log_info "Suppression de $vm..."
        virsh destroy "$vm" 2>/dev/null || true
        virsh undefine "$vm" --nvram 2>/dev/null || true
        log_success "$vm supprimé"
    fi
done

echo ""

###############################################################################
# Nettoyage des Réseaux
###############################################################################

log_info "Nettoyage des réseaux..."

for net in wan-net dmz-net lan-net; do
    if virsh net-list 2>/dev/null | grep -q "$net"; then
        log_info "Suppression du réseau $net..."
        virsh net-destroy "$net" 2>/dev/null || true
        virsh net-undefine "$net" 2>/dev/null || true
        log_success "Réseau $net supprimé"
    fi
done

echo ""

###############################################################################
# Nettoyage du Pool
###############################################################################

log_info "Nettoyage du pool de stockage..."

if virsh pool-list 2>/dev/null | grep -q "ztna-lab"; then
    log_info "Suppression du pool ztna-lab..."
    virsh pool-destroy ztna-lab 2>/dev/null || true
    virsh pool-undefine ztna-lab 2>/dev/null || true
    log_success "Pool ztna-lab supprimé"
fi

echo ""

###############################################################################
# Nettoyage des Fichiers
###############################################################################

log_info "Nettoyage des fichiers..."

# Images
if sudo test -d /var/lib/libvirt/images/ztna-lab; then
    log_info "Suppression des images..."
    sudo rm -rf /var/lib/libvirt/images/ztna-lab
    log_success "Images supprimées"
fi

# Terraform state
if [ -d "$TERRAFORM_DIR/.terraform" ]; then
    rm -rf "$TERRAFORM_DIR/.terraform"
    log_success "Cache Terraform supprimé"
fi

# State files
if [ -f "$TERRAFORM_DIR/terraform.tfstate" ]; then
    rm -f "$TERRAFORM_DIR/terraform.tfstate"*
    log_success "État Terraform supprimé"
fi

# Fichiers temporaires
rm -rf /tmp/ztna-* 2>/dev/null || true
log_success "Fichiers temporaires supprimés"

echo ""

###############################################################################
# Résumé
###############################################################################

echo -e "${GREEN}"
echo "╔════════════════════════════════════════════════════════╗"
echo "║  Nettoyage Terminé ✓                                  ║"
echo "╚════════════════════════════════════════════════════════╝"
echo -e "${NC}"

log_info "Pour vérifier :"
echo "  virsh list --all     # Doit être vide"
echo "  virsh net-list       # Doit afficher que 'default'"
echo ""

log_info "Pour recréer le lab :"
echo "  make init"
echo ""
