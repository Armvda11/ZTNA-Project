.PHONY: help up deploy destroy check check-vms check-networks check-ssh ssh-* vm-* clean

# Variables
PROJECT_DIR := $(shell pwd)
TERRAFORM_DIR := $(PROJECT_DIR)/lab/terraform

# Couleurs
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m

help: ## Affiche cette aide
	@echo "$(BLUE)╔════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(BLUE)║  ZTNA Lab - Makefile                                  ║$(NC)"
	@echo "$(BLUE)╚════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(YELLOW)Commandes Principales :$(NC)"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-20s$(NC) %s\n", $$1, $$2}'
	@echo ""
	@echo "$(YELLOW)Quick Start :$(NC)"
	@echo "  make up             # Créer le lab (5 VMs)"
	@echo "  make deploy         # Déployer control-plane + Keycloak"
	@echo "  make check          # Vérifier que tout fonctionne"
	@echo "  make ssh-cp         # Se connecter au control-plane"
	@echo "  make destroy        # Détruire tout"
	@echo ""

# ============================================================================
# GESTION DU LAB
# ============================================================================

up: ## Créer le lab complet (5 VMs, 3 réseaux)
	@bash ./scripts/lab-up-simple.sh

deploy: ## Déployer control-plane + Keycloak sur ztna-cp
	@bash ./scripts/deploy-control-plane.sh

destroy: ## Détruire toute l'infrastructure
	@echo "$(RED)[DANGER]$(NC) Destruction de l'infrastructure..."
	@cd $(TERRAFORM_DIR) && terraform destroy -auto-approve
	@echo "$(GREEN)[✓]$(NC) Infrastructure détruite"

# ============================================================================
# VÉRIFICATION
# ============================================================================

check: check-vms check-networks check-ssh ## Vérification complète du lab

check-vms: ## Lister toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) État des VMs :"
	@echo ""
	@virsh list --all
	@echo ""

check-networks: ## Lister tous les réseaux
	@echo "$(BLUE)[INFO]$(NC) Réseaux libvirt :"
	@echo ""
	@virsh net-list --all
	@echo ""

check-ssh: ## Vérifier la connectivité SSH
	@echo "$(BLUE)[INFO]$(NC) Vérification SSH :"
	@echo ""
	@timeout 3 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 ztna@10.10.10.10 'echo "✓ wan-client"' 2>/dev/null || echo "✗ wan-client"
	@timeout 3 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 ztna@10.10.10.20 'echo "✓ ztna-gw"' 2>/dev/null || echo "✗ ztna-gw"
	@timeout 3 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 ztna@10.10.20.30 'echo "✓ ztna-cp"' 2>/dev/null || echo "✗ ztna-cp"
	@echo ""

status: ## État complet du lab
	@echo "$(BLUE)╔════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(BLUE)║  État du Lab ZTNA                                      ║$(NC)"
	@echo "$(BLUE)╚════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@make check-vms
	@make check-networks
	@make check-ssh

# ============================================================================
# CONNEXIONS SSH
# ============================================================================

ssh-client: ## Se connecter à wan-client (10.10.10.10)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.10.10

ssh-gw: ## Se connecter à ztna-gw (10.10.10.20)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.10.20

ssh-cp: ## Se connecter à ztna-cp (10.10.20.30)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.20.30

ssh-app: ## Se connecter à lan-app (10.10.30.10)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.30.10

ssh-admin: ## Se connecter à lan-admin (10.10.30.11)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.30.11

# ============================================================================
# GESTION DES VMs
# ============================================================================

vm-list: check-vms ## Liste des VMs

vm-start: ## Démarrer toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) Démarrage des VMs..."
	@for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do \
		virsh start $$vm 2>/dev/null || echo "  $$vm déjà démarrée"; \
	done
	@echo "$(GREEN)[✓]$(NC) VMs démarrées"

vm-stop: ## Arrêter proprement toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) Arrêt des VMs..."
	@for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do \
		virsh shutdown $$vm 2>/dev/null || echo "  $$vm déjà arrêtée"; \
	done
	@sleep 5
	@echo "$(GREEN)[✓]$(NC) VMs arrêtées"

vm-force-stop: ## Arrêter de force toutes les VMs
	@echo "$(RED)[DANGER]$(NC) Arrêt de force des VMs..."
	@for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do \
		virsh destroy $$vm 2>/dev/null || echo "  $$vm déjà arrêtée"; \
	done
	@echo "$(GREEN)[✓]$(NC) VMs arrêtées de force"

vm-reboot: ## Redémarrer toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) Redémarrage des VMs..."
	@for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do \
		virsh reboot $$vm 2>/dev/null || echo "  $$vm non disponible"; \
	done
	@echo "$(GREEN)[✓]$(NC) Redémarrage en cours"

vm-console: ## Console d'une VM (make vm-console VM=wan-client)
	@if [ -z "$(VM)" ]; then \
		echo "$(RED)Erreur$(NC) : Spécifier VM=<nom>"; \
		echo "Exemple : make vm-console VM=wan-client"; \
	else \
		virsh console $(VM); \
	fi

# ============================================================================
# NETTOYAGE
# ============================================================================

clean: ## Nettoyer les fichiers temporaires
	@echo "$(BLUE)[INFO]$(NC) Nettoyage..."
	@cd $(TERRAFORM_DIR) && rm -rf .terraform/
	@rm -rf /tmp/ztna-*
	@echo "$(GREEN)[✓]$(NC) Nettoyage terminé"

clean-all: destroy clean ## Détruire et nettoyer tout
	@echo "$(GREEN)[✓]$(NC) Nettoyage complet terminé"

# ============================================================================
# LOGS
# ============================================================================

logs-cp: ## Logs du control-plane sur ztna-cp
	@ssh -o StrictHostKeyChecking=no ztna@10.10.20.30 'sudo journalctl -u ztna-cp -f'

logs-keycloak: ## Logs Keycloak sur ztna-cp
	@ssh -o StrictHostKeyChecking=no ztna@10.10.20.30 'cd ztna/control-plane/keycloak && docker-compose logs -f'

logs-vm: ## Logs QEMU d'une VM (make logs-vm VM=wan-client)
	@if [ -z "$(VM)" ]; then \
		echo "$(RED)Erreur$(NC) : Spécifier VM=<nom>"; \
	else \
		sudo tail -f /var/log/libvirt/qemu/$(VM).log; \
	fi

# ============================================================================
# DÉVELOPPEMENT
# ============================================================================

build-cp: ## Compiler le control-plane localement
	@echo "$(BLUE)[INFO]$(NC) Compilation du control-plane..."
	@cd control-plane && go build -o cp-linux-amd64 .
	@echo "$(GREEN)[✓]$(NC) Binaire : control-plane/cp-linux-amd64"

certs: ## Régénérer les certificats mTLS
	@bash ./scripts/gen-tls-certs.sh

# ============================================================================
# ALIASES
# ============================================================================

.DEFAULT_GOAL := help

# Raccourcis
c: check
s: status
h: help
