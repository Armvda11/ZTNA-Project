.PHONY: help setup init plan apply destroy check logs ssh-* clean git git-status git-start git-sync git-commit git-publish git-finish git-merge git-agent

# Variables
PROJECT_DIR := $(shell pwd)
TERRAFORM_DIR := $(PROJECT_DIR)/lab/terraform
TF_VARS := $(TERRAFORM_DIR)/terraform.tfvars

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
	@echo "$(YELLOW)Exemples :$(NC)"
	@echo "  make init          # Créer l'infrastructure"
	@echo "  make check         # Vérifier que tout fonctionne"
	@echo "  make ssh-client    # Se connecter au client WAN"
	@echo "  make destroy       # Détruire tout"
	@echo ""

# ============================================================================
# SETUP ET INSTALLATION
# ============================================================================

setup: ## Exécuter l'installation complète
	@echo "$(BLUE)[INFO]$(NC) Exécution du script setup.sh..."
	@bash ./setup.sh

check-requirements: ## Vérifier les prérequis système
	@echo "$(BLUE)[INFO]$(NC) Vérification des prérequis..."
	@bash ./scripts/check-requirements.sh

# ============================================================================
# GESTION DE L'INFRASTRUCTURE TERRAFORM
# ============================================================================

init: terraform-init terraform-apply check ## Initialiser le lab complet (terraform + vérification)
	@echo "$(GREEN)[✓]$(NC) Lab initialisé avec succès"
	@echo ""
	@echo "$(YELLOW)Prochaines étapes :$(NC)"
	@echo "  1. Vérifier l'infrastructure : make check"
	@echo "  2. Se connecter au client : make ssh-client"
	@echo "  3. Voir la documentation : cat README.md"
	@echo ""

terraform-init: ## Initialiser Terraform (télécharger les providers)
	@echo "$(BLUE)[INFO]$(NC) Initialisation de Terraform..."
	@cd $(TERRAFORM_DIR) && terraform init -upgrade

terraform-plan: ## Afficher le plan Terraform (quels changements vont être appliqués)
	@echo "$(BLUE)[INFO]$(NC) Plan Terraform..."
	@cd $(TERRAFORM_DIR) && terraform plan -var-file=$(TF_VARS)

terraform-apply: ## Appliquer la configuration Terraform (créer l'infrastructure)
	@echo "$(BLUE)[INFO]$(NC) Application de la configuration Terraform..."
	@cd $(TERRAFORM_DIR) && terraform apply -var-file=$(TF_VARS) -auto-approve
	@echo "$(GREEN)[✓]$(NC) Infrastructure créée"
	@sleep 30
	@echo "$(YELLOW)[⚠]$(NC) Attente du démarrage des VMs..."

terraform-destroy: ## Détruire toute l'infrastructure Terraform
	@echo "$(RED)[DANGER]$(NC) Destruction de l'infrastructure..."
	@cd $(TERRAFORM_DIR) && terraform destroy -var-file=$(TF_VARS) -auto-approve
	@echo "$(GREEN)[✓]$(NC) Infrastructure détruite"

terraform-show: ## Afficher l'état actuel
	@cd $(TERRAFORM_DIR) && terraform show

terraform-state: ## Lister les ressources Terraform
	@cd $(TERRAFORM_DIR) && terraform state list

plan: terraform-plan ## Alias pour terraform-plan

apply: terraform-apply ## Alias pour terraform-apply

destroy: terraform-destroy ## Alias pour terraform-destroy

# ============================================================================
# VÉRIFICATION ET DIAGNOSTIC
# ============================================================================

check: check-vms check-networks check-ssh ## Vérification complète du lab

check-vms: ## Lister toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) État des VMs :"
	@echo ""
	@sudo chmod 666 /var/run/libvirt/libvirt-sock 2>/dev/null || true
	@virsh list --all || echo "$(RED)Erreur : virsh non disponible$(NC)"
	@echo ""

check-networks: ## Lister tous les réseaux libvirt
	@echo "$(BLUE)[INFO]$(NC) Réseaux libvirt :"
	@echo ""
	@virsh net-list --all || echo "$(RED)Erreur : virsh non disponible$(NC)"
	@echo ""

check-ssh: ## Vérifier la connectivité SSH aux VMs
	@echo "$(BLUE)[INFO]$(NC) Vérification SSH :"
	@echo ""
	@timeout 3 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 ztna@10.10.10.10 'echo "✓ wan-client"' 2>/dev/null || echo "✗ wan-client - FAIL"
	@timeout 3 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 ztna@10.10.10.11 'echo "✓ wan-attacker"' 2>/dev/null || echo "✗ wan-attacker - FAIL"
	@timeout 3 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 ztna@10.10.10.20 'echo "✓ ztna-gw"' 2>/dev/null || echo "✗ ztna-gw - FAIL"
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

ssh-attacker: ## Se connecter à wan-attacker (10.10.10.11)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.10.11

ssh-gw: ## Se connecter à ztna-gw (10.10.10.20)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.10.20

ssh-cp: ## Se connecter à ztna-cp (10.10.20.30)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.20.30

ssh-app: ## Se connecter à lan-app (10.10.30.10)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.30.10

ssh-admin: ## Se connecter à lan-admin (10.10.30.11)
	@ssh -o StrictHostKeyChecking=no ztna@10.10.30.11

# ============================================================================
# GESTION DES VMs (VIRSH)
# ============================================================================

vm-list: check-vms ## Liste complète des VMs

vm-start: ## Démarrer toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) Démarrage des VMs..."
	@for vm in wan-client wan-attacker ztna-gw ztna-cp lan-app lan-admin; do \
		virsh start $$vm 2>/dev/null || echo "  $$vm déjà démarrée"; \
	done
	@echo "$(GREEN)[✓]$(NC) VMs démarrées"

vm-stop: ## Arrêter proprement toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) Arrêt des VMs..."
	@for vm in wan-client wan-attacker ztna-gw ztna-cp lan-app lan-admin; do \
		virsh shutdown $$vm 2>/dev/null || echo "  $$vm déjà arrêtée"; \
	done
	@sleep 5
	@echo "$(GREEN)[✓]$(NC) VMs arrêtées"

vm-force-stop: ## Arrêter de force toutes les VMs (dangereux)
	@echo "$(RED)[DANGER]$(NC) Arrêt de force des VMs..."
	@for vm in wan-client wan-attacker ztna-gw ztna-cp lan-app lan-admin; do \
		virsh destroy $$vm 2>/dev/null || echo "  $$vm déjà arrêtée"; \
	done
	@echo "$(GREEN)[✓]$(NC) VMs arrêtées de force"

vm-reboot: ## Redémarrer toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) Redémarrage des VMs..."
	@for vm in wan-client wan-attacker ztna-gw ztna-cp lan-app lan-admin; do \
		virsh reboot $$vm 2>/dev/null || echo "  $$vm non disponible"; \
	done
	@echo "$(GREEN)[✓]$(NC) Redémarrage en cours"

vm-console: ## Accéder à la console d'une VM (usage: make vm-console VM=wan-client)
	@if [ -z "$(VM)" ]; then \
		echo "$(RED)Erreur$(NC) : Spécifier la VM avec VM="; \
		echo "Exemple : make vm-console VM=wan-client"; \
	else \
		virsh console $(VM); \
	fi

# ============================================================================
# LOGS ET DIAGNOSTIC
# ============================================================================

logs-libvirtd: ## Voir les logs libvirtd
	@sudo journalctl -u libvirtd -n 50 -f

logs-vm: ## Voir les logs d'une VM (usage: make logs-vm VM=wan-client)
	@if [ -z "$(VM)" ]; then \
		echo "$(RED)Erreur$(NC) : Spécifier la VM avec VM="; \
		echo "Exemple : make logs-vm VM=wan-client"; \
	else \
		sudo tail -f /var/log/libvirt/qemu/$(VM).log; \
	fi

logs-apparmor: ## Voir les logs AppArmor
	@sudo journalctl -u apparmor -n 50 -f

# ============================================================================
# NETTOYAGE
# ============================================================================

clean: ## Nettoyer les fichiers temporaires
	@echo "$(BLUE)[INFO]$(NC) Nettoyage des fichiers temporaires..."
	@cd $(TERRAFORM_DIR) && rm -rf .terraform/
	@rm -rf /tmp/ztna-*
	@find . -name "*.tfstate*" -delete
	@echo "$(GREEN)[✓]$(NC) Nettoyage terminé"

clean-all: destroy clean ## Nettoyer tout (destroy + temporary files)
	@echo "$(GREEN)[✓]$(NC) Nettoyage complet terminé"

# ============================================================================
# DOCUMENTATION ET AIDE
# ============================================================================

docs-open: ## Ouvrir la documentation
	@if command -v xdg-open > /dev/null; then \
		xdg-open README.md; \
	elif command -v open > /dev/null; then \
		open README.md; \
	else \
		echo "Ouvrez README.md manuellement"; \
	fi

docs-troubleshoot: ## Ouvrir le guide de dépannage
	@if command -v xdg-open > /dev/null; then \
		xdg-open docs/TROUBLESHOOTING.md; \
	elif command -v open > /dev/null; then \
		open docs/TROUBLESHOOTING.md; \
	else \
		echo "Ouvrez docs/TROUBLESHOOTING.md manuellement"; \
	fi

# ============================================================================
# DÉVELOPPEMENT
# ============================================================================

dev-setup: ## Configurer l'environnement de développement
	@echo "$(BLUE)[INFO]$(NC) Configuration de l'environnement de développement..."
	@mkdir -p control-plane gateway
	@echo "✓ Répertoires créés"

dev-build-cp: ## Compiler le Control Plane (Go)
	@echo "$(BLUE)[INFO]$(NC) Compilation du Control Plane..."
	@cd control-plane && go mod tidy && go build -o ztna-cp main.go
	@echo "$(GREEN)[✓]$(NC) Control Plane compilé"

dev-build-gw: ## Compiler la Gateway (Go)
	@echo "$(BLUE)[INFO]$(NC) Compilation de la Gateway..."
	@cd gateway && go mod tidy && go build -o ztna-gw main.go
	@echo "$(GREEN)[✓]$(NC) Gateway compilée"

# ============================================================================
# TESTS
# ============================================================================

test: ## Exécuter les tests
	@echo "$(BLUE)[INFO]$(NC) Exécution des tests..."
	@echo "TODO: Ajouter les tests"

test-network: ## Tester la connectivité réseau entre VMs
	@echo "$(BLUE)[INFO]$(NC) Test de connectivité réseau..."
	@ssh -o StrictHostKeyChecking=no ztna@10.10.10.10 'echo "Client WAN:" && hostname && ping -c 1 10.10.10.20 2>/dev/null && echo "Accès WAN->GW: OK" || echo "Accès WAN->GW: FAIL"'

# ============================================================================
# WORKFLOW GIT
# ============================================================================

git: ## Ouvrir l'assistant Git interactif
	@./scripts/git-assistant.sh menu

git-status: ## Afficher le statut Git enrichi
	@./scripts/git-assistant.sh status

git-start: ## Créer une branche (usage: make git-start TYPE=feat NAME=my-feature)
	@./scripts/git-assistant.sh start "$(TYPE)" "$(NAME)"

git-sync: ## Rebaser la branche courante sur main
	@./scripts/git-assistant.sh sync

git-commit: ## Commit rapide (usage: make git-commit TYPE=feat MSG="message")
	@./scripts/git-assistant.sh commit "$(TYPE)" "$(MSG)"

git-publish: ## Push la branche courante
	@./scripts/git-assistant.sh publish

git-finish: ## Préparer la branche pour PR propre
	@./scripts/git-assistant.sh finish

git-merge: ## Merge direct vers main (avec confirmation)
	@./scripts/git-assistant.sh merge

git-agent: ## Afficher un prompt à donner à un agent
	@./scripts/git-assistant.sh agent

# ============================================================================
# ALIASES
# ============================================================================

.DEFAULT_GOAL := help

# Aliases courants
i: init
p: plan
a: apply
d: destroy
c: check
s: status
h: help
g: git
