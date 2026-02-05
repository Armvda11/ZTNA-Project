#!/bin/bash
###############################################################################
# ZTNA Lab - Script d'Initialisation du Lab
# 
# Ce script crée et configure le lab complet avec Terraform
#
# Usage : ./scripts/init-lab.sh
###############################################################################

set -e

# Couleurs
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_DIR="$( cd "$SCRIPT_DIR/.." && pwd )"
TERRAFORM_DIR="$PROJECT_DIR/lab/terraform"

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_error() { echo -e "${RED}[✗]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[⚠]${NC} $1"; }

###############################################################################
# Vérifications préalables
###############################################################################

log_info "Vérifications préalables..."

# Vérifier que Terraform est installé
if ! command -v terraform &> /dev/null; then
    log_error "Terraform n'est pas installé"
    echo "Exécutez : ./setup.sh"
    exit 1
fi

# Vérifier que libvirt est disponible
if ! command -v virsh &> /dev/null; then
    log_error "libvirt n'est pas disponible"
    exit 1
fi

# Fixer les permissions du socket libvirt
sudo chmod 666 /var/run/libvirt/libvirt-sock 2>/dev/null || true

log_success "Vérifications terminées"

###############################################################################
# Initialisation Terraform
###############################################################################

cd "$TERRAFORM_DIR"

log_info "Initialisation de Terraform..."
terraform init -upgrade

log_info "Génération du plan..."
terraform plan -out=tfplan -var-file=terraform.tfvars

echo -e ""
log_warn "Vérifiez le plan ci-dessus"
read -p "Appuyez sur Entrée pour créer l'infrastructure..."

###############################################################################
# Application de la Configuration
###############################################################################

log_info "Création de l'infrastructure (cette étape peut prendre 2-5 minutes)..."

if terraform apply tfplan; then
    log_success "Infrastructure créée avec succès"
    rm -f tfplan
else
    log_error "Erreur lors de la création"
    exit 1
fi

###############################################################################
# Attendre le démarrage des VMs
###############################################################################

log_info "Attente du démarrage des VMs et configuration cloud-init..."
sleep 30

# Attendre que cloud-init finisse
log_info "Attente de la finalisation de cloud-init (30-60 secondes)..."
for i in {1..60}; do
    if timeout 2 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=1 ztna@10.10.10.10 'cloud-init status' > /dev/null 2>&1; then
        STATUS=$(ssh -o StrictHostKeyChecking=no -o ConnectTimeout=1 ztna@10.10.10.10 'cloud-init status' 2>/dev/null || echo "not started")
        if echo "$STATUS" | grep -q "done"; then
            log_success "Cloud-init finalisé"
            break
        fi
    fi
    echo -n "."
    sleep 1
done
echo ""

###############################################################################
# Vérification Finale
###############################################################################

log_info "Vérification finale..."

cd "$PROJECT_DIR"

# Lister les VMs
log_info "VMs en cours d'exécution :"
echo ""
virsh list --all || true
echo ""

# Tester SSH
log_info "Test de connectivité SSH :"
echo ""
FAIL_COUNT=0
for ip in 10.10.10.10 10.10.10.11 10.10.10.20; do
    NAME=$(timeout 3 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 ztna@$ip hostname 2>/dev/null || echo "FAIL")
    if [ "$NAME" != "FAIL" ]; then
        log_success "$ip accessible ($NAME)"
    else
        log_error "$ip non accessible"
        ((FAIL_COUNT++))
    fi
done
echo ""

###############################################################################
# Résumé
###############################################################################

if [ $FAIL_COUNT -eq 0 ]; then
    echo -e "${GREEN}"
    echo "╔════════════════════════════════════════════════════════╗"
    echo "║  ✓ LAB INITIALISÉ AVEC SUCCÈS !                       ║"
    echo "╚════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
    echo ""
    echo -e "${YELLOW}Prochaines étapes :${NC}"
    echo "  1. Vérifier le lab complet : make status"
    echo "  2. Se connecter au client : make ssh-client"
    echo "  3. Lire la documentation : cat README.md"
    echo ""
    echo -e "${YELLOW}Informations importantes :${NC}"
    echo "  - Utilisateur SSH : ztna"
    echo "  - Pas de mot de passe (clé SSH utilisée)"
    echo "  - Client WAN : 10.10.10.10"
    echo "  - Gateway ZTNA : 10.10.10.20 (WAN) + 10.10.20.20 (DMZ)"
    echo "  - Control Plane : 10.10.20.30"
    echo "  - Application : 10.10.30.10"
    echo ""
else
    log_warn "Le lab a été créé mais certaines VMs ne sont pas accessibles en SSH"
    log_warn "Attendez 30-60 secondes et réessayez : make check"
fi
