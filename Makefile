# ============================================================================
# ZTNA Lab – Makefile
# ============================================================================
# Toutes les cibles sont déclarées PHONY pour éviter qu'un fichier du même
# nom ne masque une règle (comportement make standard).
.PHONY: help \
        up deploy deploy-gw destroy \
        check check-vms check-networks check-ssh healthz \
        status \
        ssh-client ssh-gw ssh-cp ssh-app ssh-admin \
        vm-list vm-start vm-stop vm-force-stop vm-reboot vm-console \
        build-cp build-gw \
        test test-unit test-flux1 test-flux2 test-flux2-local setup-routing \
        fmt lint \
        logs-cp logs-gw logs-keycloak logs-vm \
        certs \
        clean clean-all \
        c s h

# ── Chemins ────────────────────────────────────────────────────────────────
PROJECT_DIR   := $(shell pwd)
TERRAFORM_DIR := $(PROJECT_DIR)/lab/terraform

# ── Clé SSH utilisée pour toutes les connexions aux VMs ───────────────────
# Surcharger avec : make ssh-cp SSH_KEY=~/.ssh/autre_cle
SSH_KEY ?= $(HOME)/.ssh/id_ed25519

# ── IPs des VMs ────────────────────────────────────────────────────────────
CP_IP     := 10.10.20.30
GW_IP     := 10.10.10.20
CLIENT_IP := 10.10.10.10
APP_IP    := 10.10.30.10
ADMIN_IP  := 10.10.30.11

# ── Raccourci SSH (StrictHostKeyChecking désactivé pour le lab) ───────────
SSH := ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i $(SSH_KEY)
# SSH via jump host pour les VMs sur le réseau LAN (10.10.30.x)
SSH_J := $(SSH) -J ztna@$(GW_IP)

# ── Couleurs terminal ──────────────────────────────────────────────────────
RED    := \033[0;31m
GREEN  := \033[0;32m
YELLOW := \033[1;33m
BLUE   := \033[0;34m
NC     := \033[0m

# ── Cible par défaut ───────────────────────────────────────────────────────
.DEFAULT_GOAL := help

help: ## Affiche cette aide
	@echo "$(BLUE)╔══════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(BLUE)║  ZTNA Lab – Makefile                                     ║$(NC)"
	@echo "$(BLUE)╚══════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@echo "$(YELLOW)Quick Start :$(NC)"
	@echo "  make up             # Créer le lab complet (5 VMs KVM)"
	@echo "  make deploy         # Déployer control-plane + Keycloak"
	@echo "  make deploy-gw      # Déployer la gateway ZTNA"
	@echo "  make check          # Vérifier SSH + santé des services"
	@echo "  make test-flux1     # Test bout en bout : SSH cert"
	@echo "  make setup-routing  # Configurer MASQUERADE sur ztna-gw (1 fois)"
	@echo "  make test-flux2     # Test Flux 2 mTLS exécuté depuis wan-client"
	@echo "  make destroy        # Détruire toute l'infra"
	@echo ""
	@echo "$(YELLOW)Toutes les cibles :$(NC)"
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-22s$(NC) %s\n", $$1, $$2}'
	@echo ""

# ============================================================================
# GESTION DU LAB
# ============================================================================

up: ## Créer le lab complet (5 VMs KVM, 3 réseaux)
	@bash ./scripts/lab-up-simple.sh

deploy: ## Déployer control-plane + Keycloak sur ztna-cp
	@bash ./scripts/deploy-control-plane.sh

deploy-gw: build-gw ## Compiler + déployer le gateway sur ztna-gw
	@bash ./scripts/deploy-gateway.sh

destroy: ## Détruire toute l'infrastructure (IRRÉVERSIBLE)
	@echo "$(RED)[DANGER]$(NC) Destruction de l'infrastructure..."
	@cd $(TERRAFORM_DIR) && terraform destroy -auto-approve
	@echo "$(GREEN)[✓]$(NC) Infrastructure détruite"

# ============================================================================
# VÉRIFICATION
# ============================================================================

check: check-vms check-networks check-ssh healthz ## Vérification complète du lab

check-vms: ## Lister l'état de toutes les VMs
	@echo "$(BLUE)[INFO]$(NC) État des VMs :"
	@echo ""
	@virsh list --all
	@echo ""

check-networks: ## Lister tous les réseaux libvirt
	@echo "$(BLUE)[INFO]$(NC) Réseaux libvirt :"
	@echo ""
	@virsh net-list --all
	@echo ""

# check-ssh utilise la clé SSH configurée dans la variable SSH_KEY
check-ssh: ## Vérifier la connectivité SSH vers les 3 VMs accessibles
	@echo "$(BLUE)[INFO]$(NC) Vérification SSH (clé: $(SSH_KEY)) :"
	@echo ""
	@timeout 5 $(SSH) -o ConnectTimeout=3 ztna@$(CLIENT_IP) 'echo "  $(GREEN)✓$(NC) wan-client  ($(CLIENT_IP))"' 2>/dev/null \
	  || echo "  $(RED)✗$(NC) wan-client  ($(CLIENT_IP)) - inaccessible"
	@timeout 5 $(SSH) -o ConnectTimeout=3 ztna@$(GW_IP) 'echo "  $(GREEN)✓$(NC) ztna-gw     ($(GW_IP))"' 2>/dev/null \
	  || echo "  $(RED)✗$(NC) ztna-gw     ($(GW_IP)) - inaccessible"
	@timeout 5 $(SSH) -o ConnectTimeout=3 ztna@$(CP_IP) 'echo "  $(GREEN)✓$(NC) ztna-cp     ($(CP_IP))"' 2>/dev/null \
	  || echo "  $(RED)✗$(NC) ztna-cp     ($(CP_IP)) - inaccessible"
	@echo ""

healthz: ## Vérifier la santé du control-plane et du gateway
	@echo "$(BLUE)[INFO]$(NC) Santé des services :"
	@echo ""
	@result=$$(curl -sfk --max-time 3 https://$(CP_IP):8080/healthz 2>/dev/null) && \
	  echo "  $(GREEN)✓$(NC) CP  https://$(CP_IP):8080/healthz  → $$result" || \
	  echo "  $(RED)✗$(NC) CP  https://$(CP_IP):8080/healthz  → non joignable"
	@timeout 3 $(SSH) -o ConnectTimeout=2 ztna@$(GW_IP) \
	  'systemctl is-active ztna-gateway >/dev/null 2>&1 && echo "  \033[0;32m✓\033[0m GW  ztna-gateway.service  → active" || echo "  \033[0;31m✗\033[0m GW  ztna-gateway.service  → inactif"' \
	  2>/dev/null || echo "  $(RED)✗$(NC) GW  ztna-gateway.service  → VM inaccessible"
	@echo ""

status: ## Tableau de bord complet du lab (VMs + réseaux + SSH + santé)
	@echo "$(BLUE)╔══════════════════════════════════════════════════════════╗$(NC)"
	@echo "$(BLUE)║  État du Lab ZTNA                                        ║$(NC)"
	@echo "$(BLUE)╚══════════════════════════════════════════════════════════╝$(NC)"
	@echo ""
	@$(MAKE) check-vms
	@$(MAKE) check-networks
	@$(MAKE) check-ssh
	@$(MAKE) healthz

# ============================================================================
# CONNEXIONS SSH
# ============================================================================
# Les VMs WAN (wan-client, ztna-gw, ztna-cp) sont directement accessibles.
# Les VMs LAN (lan-app, lan-admin) passent par ztna-gw comme jump host.

ssh-client: ## Se connecter à wan-client (10.10.10.10)
	@$(SSH) ztna@$(CLIENT_IP)

ssh-gw: ## Se connecter à ztna-gw (10.10.10.20)
	@$(SSH) ztna@$(GW_IP)

ssh-cp: ## Se connecter à ztna-cp (10.10.20.30)
	@$(SSH) ztna@$(CP_IP)

ssh-app: ## Se connecter à lan-app via jump ztna-gw (10.10.30.10)
	@$(SSH_J) ztna@$(APP_IP)

ssh-admin: ## Se connecter à lan-admin via jump ztna-gw (10.10.30.11)
	@$(SSH_J) ztna@$(ADMIN_IP)

# ============================================================================
# GESTION DES VMs
# ============================================================================

vm-list: check-vms ## Liste les VMs (alias → check-vms)

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

vm-console: ## Ouvrir la console série d'une VM (make vm-console VM=wan-client)
	@if [ -z "$(VM)" ]; then \
		echo "$(RED)Erreur$(NC) : Spécifier VM=<nom>"; \
		echo "VMs disponibles : wan-client ztna-gw ztna-cp lan-app lan-admin"; \
		echo "Exemple : make vm-console VM=wan-client"; \
	else \
		virsh console $(VM); \
	fi

# ============================================================================
# NETTOYAGE
# ============================================================================

clean: ## Nettoyer les artefacts de build et fichiers temporaires
	@echo "$(BLUE)[INFO]$(NC) Nettoyage des artefacts..."
	@rm -f $(PROJECT_DIR)/control-plane/cp-linux-amd64
	@rm -f $(PROJECT_DIR)/gateway/ztna-gateway-linux-amd64
	@rm -rf $(TERRAFORM_DIR)/.terraform/
	@rm -rf /tmp/ztna-*
	@echo "$(GREEN)[✓]$(NC) Nettoyage terminé"

clean-all: destroy clean ## Détruire l'infra + nettoyer tous les artefacts
	@echo "$(GREEN)[✓]$(NC) Nettoyage complet terminé"

# ============================================================================
# LOGS
# ============================================================================

logs-cp: ## Suivre les logs du control-plane sur ztna-cp
	@$(SSH) ztna@$(CP_IP) 'sudo journalctl -u ztna-cp -f --no-pager'

logs-gw: ## Suivre les logs du gateway sur ztna-gw
	@$(SSH) ztna@$(GW_IP) 'sudo journalctl -u ztna-gateway -f --no-pager'

logs-keycloak: ## Suivre les logs Keycloak sur ztna-cp
	@$(SSH) ztna@$(CP_IP) 'cd ztna/control-plane/keycloak && docker-compose logs -f'

logs-vm: ## Logs QEMU d'une VM locale (make logs-vm VM=wan-client)
	@if [ -z "$(VM)" ]; then \
		echo "$(RED)Erreur$(NC) : Spécifier VM=<nom>  ex: make logs-vm VM=wan-client"; \
	else \
		sudo tail -f /var/log/libvirt/qemu/$(VM).log; \
	fi

# ============================================================================
# DÉVELOPPEMENT
# ============================================================================
# Les binaires sont compilés pour Linux amd64 (cible: VMs KVM).
# Surcharger avec GOOS/GOARCH si nécessaire.

build-cp: ## Compiler le control-plane (Linux amd64)
	@echo "$(BLUE)[INFO]$(NC) Compilation du control-plane..."
	@cd control-plane && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o cp-linux-amd64 .
	@echo "$(GREEN)[✓]$(NC) Binaire : control-plane/cp-linux-amd64 ($$(du -sh control-plane/cp-linux-amd64 | cut -f1))"

build-gw: ## Compiler le gateway ZTNA (Linux amd64)
	@echo "$(BLUE)[INFO]$(NC) Compilation du gateway..."
	@cd gateway && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ztna-gateway-linux-amd64 .
	@echo "$(GREEN)[✓]$(NC) Binaire : gateway/ztna-gateway-linux-amd64 ($$(du -sh gateway/ztna-gateway-linux-amd64 | cut -f1))"

test-unit: ## Lancer les tests unitaires (control-plane + gateway)
	@echo "$(BLUE)[INFO]$(NC) Tests control-plane..."
	@cd control-plane && go test ./... 2>&1
	@echo "$(BLUE)[INFO]$(NC) Tests gateway..."
	@cd gateway && go test ./... 2>&1
	@echo "$(GREEN)[✓]$(NC) Tests terminés"

test: test-unit ## Alias pour test-unit

# test-flux1 : validation bout en bout du flux SSH cert (nécessite les VMs up)
test-flux1: ## Test d'intégration Flux 1 – SSH par certificat
	@echo "$(BLUE)[INFO]$(NC) Flux 1 : SSH cert (alice → ztna-gw → lan-app)"
	@ZTNA_USER=alice ZTNA_PASS='Password123!' bash ./scripts/test-ssh-cert-access.sh lan-app

# setup-routing : configure MASQUERADE sur ztna-gw pour que wan-client puisse
# atteindre les services DMZ (Keycloak 8081, CP 8080) à travers le gateway.
# À exécuter UNE SEULE FOIS après la création du lab (ou après reboot de ztna-gw).
setup-routing: ## Configurer iptables MASQUERADE WAN→DMZ sur ztna-gw
	@echo "$(BLUE)[INFO]$(NC) Configuration du routage WAN→DMZ sur ztna-gw ($(GW_IP))..."
	@$(SSH) ztna@$(GW_IP) 'bash -s' < ./scripts/setup-gw-routing.sh
	@echo "$(GREEN)[✓]$(NC) Routage configuré — wan-client peut désormais atteindre le CP et Keycloak."

# test-flux2 : validation bout en bout du flux mTLS HTTP depuis wan-client.
# Le script est copié sur wan-client puis exécuté depuis la VM elle-même,
# exactement comme dans un vrai système ZTNA (utilisateur distant → gateway).
test-flux2: ## Test Flux 2 mTLS exécuté depuis wan-client ($(CLIENT_IP))
	@echo "$(BLUE)[INFO]$(NC) Flux 2 : copie du script sur wan-client ($(CLIENT_IP))..."
	@$(SSH) ztna@$(CLIENT_IP) 'mkdir -p /home/ztna/ztna-scripts'
	@scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
	  -i $(SSH_KEY) \
	  ./scripts/test-mtls-access.sh \
	  ztna@$(CLIENT_IP):/home/ztna/ztna-scripts/test-mtls-access.sh
	@echo "$(BLUE)[INFO]$(NC) Exécution du test depuis wan-client..."
	@$(SSH) ztna@$(CLIENT_IP) \
	  'ZTNA_USER=alice ZTNA_PASS='"'"'Password123!'"'"' bash /home/ztna/ztna-scripts/test-mtls-access.sh http'

# test-flux2-local : version locale (tourne sur le host KVM, utile pour debug)
test-flux2-local: ## Test Flux 2 mTLS local (depuis le host, pour debug)
	@echo "$(BLUE)[INFO]$(NC) Flux 2 local : mTLS HTTP (depuis host → ztna-gateway:4433 → lan-app:80)"
	@ZTNA_USER=alice ZTNA_PASS='Password123!' bash ./scripts/test-mtls-access.sh http

fmt: ## Formater le code Go (control-plane + gateway)
	@echo "$(BLUE)[INFO]$(NC) go fmt control-plane..."
	@cd control-plane && go fmt ./...
	@echo "$(BLUE)[INFO]$(NC) go fmt gateway..."
	@cd gateway && go fmt ./...
	@echo "$(GREEN)[✓]$(NC) Formatage terminé"

lint: ## Analyser le code Go avec go vet
	@echo "$(BLUE)[INFO]$(NC) go vet control-plane..."
	@cd control-plane && go vet ./...
	@echo "$(BLUE)[INFO]$(NC) go vet gateway..."
	@cd gateway && go vet ./...
	@echo "$(GREEN)[✓]$(NC) Analyse terminée"

certs: ## Régénérer les certificats mTLS server/CA/PEP
	@bash ./scripts/gen-tls-certs.sh

# ============================================================================
# RACCOURCIS
# ============================================================================

c: check  ## Alias → check
s: status ## Alias → status
h: help   ## Alias → help
