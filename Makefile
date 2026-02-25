# ============================================================================
# ZTNA Lab — Makefile (clean interface)
# ============================================================================

.PHONY: help \
        quickstart prereq doctor doctor-dry doctor-full bootstrap \
        up lab-start destroy \
        deploy deploy-gw \
        check status check-vms check-ssh healthz \
        test-flux1 test-flux1-auto test-flux2 test-crl-routing test-pep-register test-cp-gw-lab \
        ssh-client ssh-gw ssh-cp ssh-app ssh-admin \
        vm-start vm-stop vm-force-stop vm-reboot vm-console \
        logs-cp logs-gw clean \
        build-cp build-gw build-cli test-unit test certs \
        init check-requirements plan apply test-flux2-local setup-routing

PROJECT_DIR   := $(shell pwd)
TERRAFORM_DIR := $(PROJECT_DIR)/lab/terraform
SSH_KEY       ?= $(HOME)/.ssh/id_ed25519

CP_IP     := 10.10.20.30
GW_IP     := 10.10.10.20
CLIENT_IP := 10.10.10.10
APP_IP    := 10.10.30.10
ADMIN_IP  := 10.10.30.11

SSH   := ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i $(SSH_KEY)
SSH_J := $(SSH) -J ztna@$(GW_IP)
VIRSH := bash $(PROJECT_DIR)/scripts/virsh-lab
TF    := bash $(PROJECT_DIR)/scripts/tf-lab

RED    := \033[0;31m
GREEN  := \033[0;32m
YELLOW := \033[1;33m
BLUE   := \033[0;34m
NC     := \033[0m

.DEFAULT_GOAL := help

help:
	@echo "$(BLUE)ZTNA Lab — commandes principales$(NC)"
	@echo ""
	@echo "$(YELLOW)Onboarding$(NC)"
	@echo "  make bootstrap     Installation complète + démarrage VMs (1 seule commande ← COMMENCER ICI)"
	@echo "  make prereq        Vérifier les prérequis minimum"
	@echo "  make doctor        Diagnostic + réparation automatique (si VMs ne démarrent pas)"
	@echo "  make doctor-dry    Diagnostic seul - lecture seule, aucune modification"
	@echo "  make doctor-full   Diagnostic + réparation + tentative de démarrage des VMs"
	@echo "  make quickstart    Parcours recommandé: prereq -> up -> deploy -> deploy-gw -> check"
	@echo ""
	@echo "$(YELLOW)Infra$(NC)"
	@echo "  make up            Créer / mettre à jour les VMs avec Terraform"
	@echo "  make lab-start     Démarrer les VMs existantes + check SSH de base"
	@echo "  make destroy       Détruire toute l'infrastructure"
	@echo ""
	@echo "$(YELLOW)Deploy$(NC)"
	@echo "  make deploy        Déployer control-plane + Keycloak"
	@echo "  make deploy-gw     Déployer gateway"
	@echo ""
	@echo "$(YELLOW)Checks$(NC)"
	@echo "  make check         Check global (VMs + SSH + healthz)"
	@echo "  make status        Alias de check"
	@echo "  make check-ssh     Vérifier SSH WAN/DMZ"
	@echo "  make healthz       Vérifier API CP + service gateway"
	@echo ""
	@echo "$(YELLOW)Tests$(NC)"
	@echo "  make test-flux1"
	@echo "  make test-flux1-auto"
	@echo "  make test-flux2"
	@echo "  make test-crl-routing"
	@echo "  make test-pep-register"
	@echo "  make test-cp-gw-lab"
	@echo ""
	@echo "$(YELLOW)Ops$(NC)"
	@echo "  make ssh-client | ssh-gw | ssh-cp | ssh-app | ssh-admin"
	@echo "  make vm-start | vm-stop | vm-reboot | vm-force-stop | vm-console VM=<nom>"
	@echo "  make logs-cp | logs-gw"
	@echo "  make clean"
	@echo ""
	@echo "$(YELLOW)Dev (optionnel)$(NC)"
	@echo "  make build-cp | build-gw | build-cli | test-unit | test | certs"
	@echo ""
	@echo "$(BLUE)Compatibilité legacy:$(NC) init, check-requirements, plan, apply, test-flux2-local"

# ============================================================================
# ONBOARDING
# ============================================================================

prereq:
	@bash ./scripts/check-requirements.sh

doctor:
	@bash ./scripts/ztna-doctor.sh

doctor-dry:
	@bash ./scripts/ztna-doctor.sh --dry

doctor-full:
	@bash ./scripts/ztna-doctor.sh --full

bootstrap:
	@bash ./scripts/bootstrap.sh

quickstart: doctor prereq up deploy deploy-gw check
	@echo "$(GREEN)[✓]$(NC) Quickstart terminé"

# ============================================================================
# INFRA
# ============================================================================

up:
	@bash ./scripts/lab-up-simple.sh

lab-start:
	@bash ./scripts/lab-start.sh

destroy:
	@echo "$(RED)[DANGER]$(NC) Cette opération est irréversible. Ctrl+C pour annuler, Entrée pour continuer."
	@read _
	@$(TF) destroy -auto-approve

# ============================================================================
# DEPLOY
# ============================================================================

deploy:
	@bash ./scripts/deploy-control-plane.sh

deploy-gw: build-gw
	@bash ./scripts/deploy-gateway.sh

# ============================================================================
# CHECKS
# ============================================================================

check: check-vms check-ssh healthz

status: check

check-vms:
	@$(VIRSH) list --all

check-ssh:
	@echo "$(BLUE)[SSH]$(NC)"
	@timeout 5 $(SSH) -o ConnectTimeout=3 ztna@$(CLIENT_IP) 'echo "  ✓ wan-client ($(CLIENT_IP))"' 2>/dev/null || echo "  ✗ wan-client ($(CLIENT_IP))"
	@timeout 5 $(SSH) -o ConnectTimeout=3 ztna@$(GW_IP) 'echo "  ✓ ztna-gw ($(GW_IP))"' 2>/dev/null || echo "  ✗ ztna-gw ($(GW_IP))"
	@timeout 5 $(SSH) -o ConnectTimeout=3 ztna@$(CP_IP) 'echo "  ✓ ztna-cp ($(CP_IP))"' 2>/dev/null || echo "  ✗ ztna-cp ($(CP_IP))"

healthz:
	@echo "$(BLUE)[HEALTH]$(NC)"
	@curl -sfk --max-time 3 https://$(CP_IP):8080/healthz >/dev/null 2>&1 \
	  && echo "  ✓ control-plane https://$(CP_IP):8080/healthz" \
	  || echo "  ✗ control-plane https://$(CP_IP):8080/healthz"
	@timeout 4 $(SSH) -o ConnectTimeout=3 ztna@$(GW_IP) \
	  'systemctl is-active ztna-gateway >/dev/null && echo "  ✓ gateway ztna-gateway.service" || echo "  ✗ gateway ztna-gateway.service"' \
	  2>/dev/null || echo "  ✗ gateway (VM inaccessible)"

# ============================================================================
# TESTS
# ============================================================================

test-flux1:
	@ZTNA_USER=alice ZTNA_PASS='Password123!' bash ./scripts/test-ssh-cert-access.sh lan-app

test-flux1-auto:
	@ZTNA_USER=alice ZTNA_PASS='Password123!' SSH_TEST_CMD='hostname && id -un' \
	  bash ./scripts/test-ssh-cert-access.sh lan-app

test-flux2:
	@$(SSH) ztna@$(CLIENT_IP) 'mkdir -p /home/ztna/ztna-scripts'
	@scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i $(SSH_KEY) \
	  ./scripts/test-mtls-access.sh ztna@$(CLIENT_IP):/home/ztna/ztna-scripts/
	@$(SSH) ztna@$(CLIENT_IP) \
	  'ZTNA_USER=alice ZTNA_PASS='"'"'Password123!'"'"' bash /home/ztna/ztna-scripts/test-mtls-access.sh http'

test-crl-routing:
	@$(SSH) ztna@$(CLIENT_IP) 'mkdir -p /home/ztna/ztna-scripts /home/ztna/.ssh'
	@scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i $(SSH_KEY) \
	  ./scripts/test-crl-sessions-routing.sh ztna@$(CLIENT_IP):/home/ztna/ztna-scripts/
	@scp -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -i $(SSH_KEY) \
	  $(SSH_KEY) ztna@$(CLIENT_IP):/home/ztna/.ssh/id_ed25519
	@$(SSH) ztna@$(CLIENT_IP) 'chmod 600 /home/ztna/.ssh/id_ed25519'
	@$(SSH) ztna@$(CLIENT_IP) \
	  'ZTNA_USER=alice ZTNA_PASS='"'"'Password123!'"'"' bash /home/ztna/ztna-scripts/test-crl-sessions-routing.sh'

test-pep-register:
	@bash ./scripts/test-pep-register-heartbeat.sh

test-cp-gw-lab: test-flux1-auto test-flux2 test-crl-routing test-pep-register

# ============================================================================
# OPS
# ============================================================================

ssh-client:
	@$(SSH) ztna@$(CLIENT_IP)

ssh-gw:
	@$(SSH) ztna@$(GW_IP)

ssh-cp:
	@$(SSH) ztna@$(CP_IP)

ssh-app:
	@$(SSH_J) ztna@$(APP_IP)

ssh-admin:
	@$(SSH_J) ztna@$(ADMIN_IP)

vm-start:
	@for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do \
		$(VIRSH) start $$vm >/dev/null 2>&1 && echo "  → $$vm démarré" || echo "  → $$vm déjà en marche"; \
	done

vm-stop:
	@for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do \
		$(VIRSH) shutdown $$vm >/dev/null 2>&1 || true; \
	done

vm-force-stop:
	@for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do \
		$(VIRSH) destroy $$vm >/dev/null 2>&1 || true; \
	done

vm-reboot:
	@for vm in wan-client ztna-gw ztna-cp lan-app lan-admin; do \
		$(VIRSH) reboot $$vm >/dev/null 2>&1 || true; \
	done

vm-console:
	@[ -n "$(VM)" ] || { echo "Usage : make vm-console VM=<nom>"; exit 1; }
	@$(VIRSH) console $(VM)

logs-cp:
	@$(SSH) ztna@$(CP_IP) 'sudo journalctl -u ztna-cp -f --no-pager'

logs-gw:
	@$(SSH) ztna@$(GW_IP) 'sudo journalctl -u ztna-gateway -f --no-pager'

clean:
	@rm -f $(PROJECT_DIR)/control-plane/cp-linux-amd64
	@rm -f $(PROJECT_DIR)/gateway/ztna-gateway-linux-amd64
	@rm -f $(PROJECT_DIR)/ztna-cli/ztna-linux-amd64
	@rm -f $(PROJECT_DIR)/ztna-cli/ztna
	@rm -rf $(TERRAFORM_DIR)/.terraform/
	@rm -rf /tmp/ztna-*
	@echo "$(GREEN)[✓]$(NC) Nettoyage terminé"

# ============================================================================
# DEV (OPTIONNEL)
# ============================================================================

build-cp:
	@cd control-plane && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o cp-linux-amd64 .
	@echo "$(GREEN)[✓]$(NC) control-plane/cp-linux-amd64"

build-gw:
	@cd gateway && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ztna-gateway-linux-amd64 .
	@echo "$(GREEN)[✓]$(NC) gateway/ztna-gateway-linux-amd64"

build-cli:
	@cd ztna-cli && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ztna-linux-amd64 .
	@echo "$(GREEN)[✓]$(NC) ztna-cli/ztna-linux-amd64"

test-unit:
	@cd control-plane && go test ./...
	@cd gateway && go test ./...
	@cd ztna-cli && go test ./...

test: test-unit

certs:
	@bash ./scripts/gen-tls-certs.sh

# ============================================================================
# LEGACY ALIASES (temporaires)
# ============================================================================

init:
	@echo "$(YELLOW)[DEPRECATED]$(NC) 'make init' -> utilisez 'make up'"
	@$(MAKE) up

check-requirements:
	@echo "$(YELLOW)[DEPRECATED]$(NC) 'make check-requirements' -> utilisez 'make prereq'"
	@$(MAKE) prereq

plan:
	@echo "$(YELLOW)[DEPRECATED]$(NC) 'make plan' -> utilisez 'bash scripts/tf-lab plan -var-file=terraform.tfvars'"
	@$(TF) plan -var-file=terraform.tfvars

apply:
	@echo "$(YELLOW)[DEPRECATED]$(NC) 'make apply' -> utilisez 'make up'"
	@$(MAKE) up

test-flux2-local:
	@echo "$(YELLOW)[DEPRECATED]$(NC) 'make test-flux2-local' -> utilisez 'make test-flux2'"
	@$(MAKE) test-flux2

# Conservé pour compatibilité avec docs/tests existants.
setup-routing:
	@$(SSH) ztna@$(GW_IP) 'bash -s' < ./scripts/setup-gw-routing.sh
	@echo "$(GREEN)[✓]$(NC) Routage WAN→DMZ configuré"
