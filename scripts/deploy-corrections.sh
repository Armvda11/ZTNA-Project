#!/bin/bash
# Déploiement du control plane corrigé sur le lab
set -euo pipefail

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║     Déploiement Control Plane Corrigé - Lab ZTNA              ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# Configuration
CP_HOST="10.10.20.30"
CP_USER="ztna"
CP_DIR="/home/ztna/ztna/control-plane"

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Vérifier que nous sommes dans le bon répertoire
if [ ! -f "control-plane/main.go" ]; then
    echo -e "${RED}Erreur:${NC} Exécuter depuis le répertoire racine ZTNA/"
    exit 1
fi

echo -e "${BLUE}[1/6]${NC} Compilation du control plane..."
cd control-plane
if go build -o cp-linux-amd64 .; then
    echo -e "  ${GREEN}✓${NC} Compilation réussie"
else
    echo -e "  ${RED}✗${NC} Échec de compilation"
    exit 1
fi
cd ..

echo ""
echo -e "${BLUE}[2/6]${NC} Vérification de l'accès à ztna-cp..."
if timeout 3 ssh -o StrictHostKeyChecking=no -o ConnectTimeout=2 $CP_USER@$CP_HOST 'echo ok' >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓${NC} SSH accessible"
else
    echo -e "  ${RED}✗${NC} Impossible de se connecter à $CP_HOST"
    exit 1
fi

echo ""
echo -e "${BLUE}[3/6]${NC} Arrêt du service control plane..."
ssh -o StrictHostKeyChecking=no $CP_USER@$CP_HOST 'sudo systemctl stop ztna-cp' 2>/dev/null || true
echo -e "  ${GREEN}✓${NC} Service arrêté"

echo ""
echo -e "${BLUE}[4/6]${NC} Déploiement du nouveau binaire..."
scp -o StrictHostKeyChecking=no control-plane/cp-linux-amd64 $CP_USER@$CP_HOST:$CP_DIR/ 2>&1 | grep -v "Warning:" || true
echo -e "  ${GREEN}✓${NC} Binaire copié"

echo ""
echo -e "${BLUE}[5/6]${NC} Déploiement de la configuration..."
scp -o StrictHostKeyChecking=no control-plane/config.lab.yaml $CP_USER@$CP_HOST:$CP_DIR/ 2>&1 | grep -v "Warning:" || true
echo -e "  ${GREEN}✓${NC} Configuration copiée"

echo ""
echo -e "${BLUE}[6/6]${NC} Redémarrage du service..."
ssh -o StrictHostKeyChecking=no $CP_USER@$CP_HOST 'sudo systemctl start ztna-cp'
sleep 2

# Vérifier le status
if ssh -o StrictHostKeyChecking=no $CP_USER@$CP_HOST 'sudo systemctl is-active ztna-cp' | grep -q "active"; then
    echo -e "  ${GREEN}✓${NC} Service démarré"
else
    echo -e "  ${RED}✗${NC} Échec du démarrage"
    echo ""
    echo "Logs du service:"
    ssh -o StrictHostKeyChecking=no $CP_USER@$CP_HOST 'sudo journalctl -u ztna-cp -n 20 --no-pager'
    exit 1
fi

echo ""
echo -e "${GREEN}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║              Déploiement Terminé avec Succès                   ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Test rapide
echo -e "${YELLOW}Test de santé du control plane...${NC}"
sleep 2
if HEALTH=$(curl -sSfk --max-time 5 https://$CP_HOST:8080/healthz 2>&1); then
    if echo "$HEALTH" | grep -q "ok"; then
        echo -e "  ${GREEN}✓${NC} Control plane répond correctement"
        echo ""
        echo "Commandes utiles:"
        echo "  • Logs en temps réel : ssh $CP_USER@$CP_HOST 'sudo journalctl -u ztna-cp -f'"
        echo "  • Status du service  : ssh $CP_USER@$CP_HOST 'sudo systemctl status ztna-cp'"
        echo "  • Tester les endpoints : bash scripts/ztna-diagnostic.sh"
        echo ""
    else
        echo -e "  ${YELLOW}⚠${NC} Réponse inattendue: $HEALTH"
    fi
else
    echo -e "  ${RED}✗${NC} Pas de réponse du control plane"
    echo "  Vérifier les logs: ssh $CP_USER@$CP_HOST 'sudo journalctl -u ztna-cp -n 50'"
fi
