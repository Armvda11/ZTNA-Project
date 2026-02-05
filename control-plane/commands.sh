#!/bin/bash
###############################################################################
# ZTNA Control Plane - Quick Reference & Commands
# 
# Ce fichier contient toutes les commandes utiles pour gérer le système
###############################################################################

# ┌─────────────────────────────────────────────────────────────────────┐
# │  🚀 DÉMARRAGE RAPIDE                                                │
# └─────────────────────────────────────────────────────────────────────┘

# 1. Compiler le binaire
go build -o ztna-cp main.go

# 2. Lancer en local (dev)
./ztna-cp -config config.yaml

# 3. Déployer sur VM
./deploy.sh

# 4. Vérifier l'état du service
ssh ztna@10.10.20.30 'sudo systemctl status ztna-cp.service'


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🔐 SÉCURITÉ                                                         │
# └─────────────────────────────────────────────────────────────────────┘

# Audit de sécurité complet
./security-audit.sh

# Mise à jour des dépendances
go get -u all && go mod tidy

# Vérifier les vulnérabilités connues (nécessite govulncheck)
# go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...

# Générer un JWT secret fort pour production
openssl rand -base64 32

# Configurer JWT secret dans systemd
ssh ztna@10.10.20.30 'sudo systemctl edit ztna-cp.service'
# Ajouter:
# [Service]
# Environment="ZTNA_JWT_SECRET=<votre_secret>"


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🧪 TESTS                                                            │
# └─────────────────────────────────────────────────────────────────────┘

# Tous les tests unitaires
go test -v ./...

# Tests E2E Standard (HTTP)
./e2e-test.sh

# Tests E2E HTTPS
./e2e-test.sh --https

# Tests E2E avec rapport JSON (pour CI/CD, archivage)
./e2e-test.sh --report json

# Tests E2E avec rapport Markdown (pour documentation)
./e2e-test.sh --report markdown

# Tests E2E HTTPS + rapport
./e2e-test.sh --https --report json
./e2e-test.sh --https --report markdown

# Corriger les clés SSH hôte (après recréation des VMs)
./e2e-test.sh --fix-known-hosts

# Custom base URL (pour tester différents endpoints)
BASE_URL=https://10.10.20.30:9443 ./e2e-test.sh --https

# Tests avec coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Tests d'un package spécifique
go test -v ./internal/config/
go test -v ./internal/logger/
go test -v ./internal/storage/

# Benchmarks
go test -bench=. ./...

# Race detection
go test -race ./...


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🔍 DEBUGGING & LOGS                                                │
# └─────────────────────────────────────────────────────────────────────┘

# Logs en temps réel
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp.service -f'

# Logs des 100 dernières lignes
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp.service -n 100 --no-pager'

# Logs filtrés par niveau ERROR
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp.service | grep "ERROR"'

# Logs depuis une date
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp.service --since "2026-02-04 20:00"'

# Vérifier la config actuelle
ssh ztna@10.10.20.30 'cat /home/ztna/config.yaml'

# Lister les certificats générés
ssh ztna@10.10.20.30 'ls -la /etc/ztna/'


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🔄 GESTION DU SERVICE                                               │
# └─────────────────────────────────────────────────────────────────────┘

# Démarrer
ssh ztna@10.10.20.30 'sudo systemctl start ztna-cp.service'

# Arrêter
ssh ztna@10.10.20.30 'sudo systemctl stop ztna-cp.service'

# Redémarrer
ssh ztna@10.10.20.30 'sudo systemctl restart ztna-cp.service'

# Recharger config systemd (après modification du .service)
ssh ztna@10.10.20.30 'sudo systemctl daemon-reload'

# Activer au démarrage
ssh ztna@10.10.20.30 'sudo systemctl enable ztna-cp.service'

# Désactiver au démarrage
ssh ztna@10.10.20.30 'sudo systemctl disable ztna-cp.service'

# Voir la config systemd
ssh ztna@10.10.20.30 'sudo systemctl cat ztna-cp.service'


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🌐 TESTS API                                                        │
# └─────────────────────────────────────────────────────────────────────┘

# Health check
curl http://10.10.20.30:8443/health | jq .

# Login (obtenir JWT token)
TOKEN=$(curl -s -X POST http://10.10.20.30:8443/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"alice123"}' | jq -r '.token')
echo "Token: $TOKEN"

# Demander un certificat SSH
ssh-keygen -t ed25519 -f /tmp/test_key -N ""
PUBKEY=$(cat /tmp/test_key.pub)
curl -s -X POST http://10.10.20.30:8443/api/v1/certs/request \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"public_key\":\"$PUBKEY\"}" | jq .

# Vérifier une politique
curl -s -X GET http://10.10.20.30:8443/api/v1/policies/lan-app \
  -H "Authorization: Bearer $TOKEN" | jq .

# Récupérer les audit logs
curl -s -X GET http://10.10.20.30:8443/api/v1/audit \
  -H "Authorization: Bearer $TOKEN" | jq .

# Test de charge (nécessite apache2-utils)
# apt-get install apache2-utils
ab -n 1000 -c 10 http://10.10.20.30:8443/health


# ┌─────────────────────────────────────────────────────────────────────┐
# │  💾 BASE DE DONNÉES                                                  │
# └─────────────────────────────────────────────────────────────────────┘

# Accéder à la base SQLite
ssh ztna@10.10.20.30 'sqlite3 /var/lib/ztna/control-plane.db'

# Lister les utilisateurs
ssh ztna@10.10.20.30 'sqlite3 /var/lib/ztna/control-plane.db "SELECT * FROM users;"'

# Lister les audit logs
ssh ztna@10.10.20.30 'sqlite3 /var/lib/ztna/control-plane.db "SELECT * FROM audit_logs ORDER BY timestamp DESC LIMIT 10;"'

# Compter les logs par action
ssh ztna@10.10.20.30 'sqlite3 /var/lib/ztna/control-plane.db "SELECT action, COUNT(*) FROM audit_logs GROUP BY action;"'

# Backup de la base
ssh ztna@10.10.20.30 'sudo cp /var/lib/ztna/control-plane.db /var/lib/ztna/control-plane.db.backup-$(date +%Y%m%d)'

# Restore depuis backup
ssh ztna@10.10.20.30 'sudo cp /var/lib/ztna/control-plane.db.backup-YYYYMMDD /var/lib/ztna/control-plane.db'


# ┌─────────────────────────────────────────────────────────────────────┐
# │  📦 BUILD & DÉPLOIEMENT                                              │
# └─────────────────────────────────────────────────────────────────────┘

# Build pour Linux (cross-compilation depuis Mac/Windows)
GOOS=linux GOARCH=amd64 go build -o ztna-cp main.go

# Build avec version
VERSION="0.1.0"
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build -ldflags "-X main.version=$VERSION -X main.buildDate=$BUILD_DATE" -o ztna-cp main.go

# Build optimisé pour production (size reduction)
go build -ldflags="-s -w" -o ztna-cp main.go

# ═════════════════════════════════════════════════════════════════════
# DÉPLOIEMENT - Deux options:
# ═════════════════════════════════════════════════════════════════════

# OPTION 1: Déploiement COMPLET (Build + Config + Service)
# Utilisé pour: premier déploiement, changements de code, mise à jour dépendances
./deploy.sh

# OPTION 2: Déploiement CONFIG-ONLY (Rapide, sans rebuild)
# Utilisé pour: TLS, rate limiting, policies, log level
# ⏱️  2-5 secondes vs 15-20 secondes pour full deploy
./deploy-config-only.sh

# Workflow d'itération rapide:
# 1. vim config.yaml              # Éditer la config
# 2. ./deploy-config-only.sh      # Deploy (fast)
# 3. ./e2e-test.sh --https        # Test
# 4. Répéter

# ═════════════════════════════════════════════════════════════════════

# Déploiement manuel étape par étape
scp ztna-cp ztna@10.10.20.30:/tmp/
ssh ztna@10.10.20.30 'sudo systemctl stop ztna-cp.service'
ssh ztna@10.10.20.30 'sudo mv /tmp/ztna-cp /home/ztna/ztna-cp'
ssh ztna@10.10.20.30 'sudo chown ztna:ztna /home/ztna/ztna-cp'
ssh ztna@10.10.20.30 'sudo chmod +x /home/ztna/ztna-cp'
ssh ztna@10.10.20.30 'sudo systemctl start ztna-cp.service'


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🔧 MAINTENANCE                                                      │
# └─────────────────────────────────────────────────────────────────────┘

# Rotation des logs (logrotate)
ssh ztna@10.10.20.30 'sudo logrotate -f /etc/logrotate.d/ztna-cp'

# Nettoyer les anciens logs
ssh ztna@10.10.20.30 'sudo journalctl --vacuum-time=7d'

# Vérifier l'espace disque
ssh ztna@10.10.20.30 'df -h'

# Vérifier la mémoire utilisée
ssh ztna@10.10.20.30 'free -h'

# Processes ZTNA
ssh ztna@10.10.20.30 'ps aux | grep ztna-cp'

# Nettoyer les certificats SSH temporaires (si besoin)
ssh ztna@10.10.20.30 'sudo rm -f /etc/ztna/ssh_ca*'

# Réinitialiser complètement (⚠️ DANGER - efface tout)
ssh ztna@10.10.20.30 'sudo systemctl stop ztna-cp.service && \
  sudo rm -rf /var/lib/ztna/control-plane.db /etc/ztna/ssh_ca* && \
  sudo systemctl start ztna-cp.service'


# ┌─────────────────────────────────────────────────────────────────────┐
# │  📊 MONITORING                                                       │
# └─────────────────────────────────────────────────────────────────────┘

# Vérifier que le service répond
while true; do 
  curl -s http://10.10.20.30:8443/health | jq -r '.status' || echo "DOWN"
  sleep 5
done

# Surveiller les logs en temps réel avec alertes
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp.service -f' | \
  grep --line-buffered "ERROR" | \
  while read line; do 
    echo "🚨 ALERT: $line"
    # Envoyer notification (email, Slack, etc.)
  done

# Statistiques de performance
ssh ztna@10.10.20.30 'sudo systemctl status ztna-cp.service' | grep -E "Memory|CPU"


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🗂️  FICHIERS IMPORTANTS                                            │
# └─────────────────────────────────────────────────────────────────────┘

# Binaire principal
# /home/ztna/ztna-cp

# Configuration
# /home/ztna/config.yaml

# Service systemd
# /etc/systemd/system/ztna-cp.service

# Clé privée CA (⚠️ SENSIBLE)
# /etc/ztna/ssh_ca (permissions 600)

# Clé publique CA
# /etc/ztna/ssh_ca.pub

# TrustedUserCAKeys (pour sshd_config)
# /etc/ztna/ssh_ca.trustedkeys

# Base de données
# /var/lib/ztna/control-plane.db

# Répertoire logs (si file logging activé)
# /var/log/ztna/


# ┌─────────────────────────────────────────────────────────────────────┐
# │  📖 DOCUMENTATION                                                    │
# └─────────────────────────────────────────────────────────────────────┘

# Ouvrir la documentation
cat README.md | less
cat DEPLOYMENT.md | less
cat SECURITY.md | less
cat CHANGELOG.md | less

# Générer la doc godoc (nécessite godoc)
# go install golang.org/x/tools/cmd/godoc@latest
godoc -http=:6060
# Ouvrir http://localhost:6060/pkg/github.com/ztna/control-plane/


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🐛 TROUBLESHOOTING                                                  │
# └─────────────────────────────────────────────────────────────────────┘

# Service ne démarre pas
# 1. Vérifier les logs
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp.service -n 50'

# 2. Vérifier les permissions
ssh ztna@10.10.20.30 'ls -la /home/ztna/ztna-cp /home/ztna/config.yaml'

# 3. Tester manuellement
ssh ztna@10.10.20.30 'cd /home/ztna && ./ztna-cp -config config.yaml'

# API ne répond pas
# 1. Vérifier que le service est actif
ssh ztna@10.10.20.30 'sudo systemctl is-active ztna-cp.service'

# 2. Vérifier le port
ssh ztna@10.10.20.30 'sudo netstat -tlnp | grep 8443'

# 3. Tester depuis la VM elle-même
ssh ztna@10.10.20.30 'curl http://localhost:8443/health'

# Certificats SSH invalides
# 1. Vérifier la clé CA
ssh ztna@10.10.20.30 'ls -la /etc/ztna/ssh_ca*'

# 2. Régénérer la clé CA
ssh ztna@10.10.20.30 'sudo systemctl stop ztna-cp.service && \
  sudo rm -f /etc/ztna/ssh_ca* && \
  sudo systemctl start ztna-cp.service'

# Base de données corrompue
# 1. Vérifier l'intégrité
ssh ztna@10.10.20.30 'sqlite3 /var/lib/ztna/control-plane.db "PRAGMA integrity_check;"'

# 2. Restore depuis backup
ssh ztna@10.10.20.30 'sudo systemctl stop ztna-cp.service && \
  sudo cp /var/lib/ztna/control-plane.db.backup-YYYYMMDD /var/lib/ztna/control-plane.db && \
  sudo systemctl start ztna-cp.service'


# ┌─────────────────────────────────────────────────────────────────────┐
# │  🔗 URLS UTILES                                                      │
# └─────────────────────────────────────────────────────────────────────┘

# Control Plane API
# http://10.10.20.30:8443

# Endpoints:
# GET  http://10.10.20.30:8443/health
# POST http://10.10.20.30:8443/api/v1/auth/login
# POST http://10.10.20.30:8443/api/v1/certs/request
# GET  http://10.10.20.30:8443/api/v1/policies/{resource}
# GET  http://10.10.20.30:8443/api/v1/audit


echo "📚 Voir ce fichier pour toutes les commandes utiles:"
echo "    cat commands.sh | less"
echo ""
echo "🔍 Rechercher une commande:"
echo "    grep -i 'mot-clé' commands.sh"
