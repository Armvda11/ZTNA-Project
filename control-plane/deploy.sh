#!/bin/bash
###############################################################################
# ZTNA Control Plane - Script de Build et Déploiement
###############################################################################

set -e

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
VM_HOST="10.10.20.30"
VM_USER="ztna"
BINARY_NAME="ztna-cp"
REMOTE_DIR="/home/ztna"

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

detect_tls_mode() {
    if grep -A4 -E '^[[:space:]]*tls:[[:space:]]*$' config.yaml | grep -Eq '^[[:space:]]*enabled:[[:space:]]*true([[:space:]]*#.*)?$'; then
        echo "true"
    else
        echo "false"
    fi
}

# Vérifier que nous sommes dans le bon répertoire
if [ ! -f "main.go" ]; then
    log_error "main.go not found. Run this script from control-plane directory."
    exit 1
fi

# 1. Build le binaire
log_info "Building Control Plane for Linux..."
GOOS=linux GOARCH=amd64 go build -o ${BINARY_NAME} main.go
log_success "Build completed: ${BINARY_NAME}"

# 2. Copier le binaire vers la VM
log_info "Copying binary to ${VM_USER}@${VM_HOST}:/tmp..."
scp ${BINARY_NAME} ${VM_USER}@${VM_HOST}:/tmp/${BINARY_NAME}.new
log_success "Binary copied to /tmp"

# 3. Copier la configuration
log_info "Copying configuration..."
scp config.yaml ${VM_USER}@${VM_HOST}:${REMOTE_DIR}/
log_success "Configuration copied"

# 4. Créer le service systemd et démarrer
log_info "Setting up systemd service..."
ssh ${VM_USER}@${VM_HOST} << 'EOF'
# Arrêter le service avant remplacement du binaire (évite text file busy)
sudo systemctl stop ztna-cp 2>/dev/null || true

# Installer le nouveau binaire de manière atomique
chmod +x /tmp/ztna-cp.new
mv -f /tmp/ztna-cp.new /home/ztna/ztna-cp
chmod +x /home/ztna/ztna-cp

# Créer les répertoires nécessaires
sudo mkdir -p /etc/ztna
sudo mkdir -p /var/lib/ztna
sudo chown -R ztna:ztna /etc/ztna /var/lib/ztna

# Créer le service systemd
sudo tee /etc/systemd/system/ztna-cp.service > /dev/null <<SERVICE
[Unit]
Description=ZTNA Control Plane
After=network.target

[Service]
Type=simple
User=ztna
WorkingDirectory=/home/ztna
ExecStart=/home/ztna/ztna-cp -config /home/ztna/config.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
SERVICE

# Recharger systemd
sudo systemctl daemon-reload

# Activer et démarrer le service
sudo systemctl enable ztna-cp
sudo systemctl start ztna-cp

# Attendre 2 secondes que le service démarre
sleep 2

# Afficher le statut
sudo systemctl status ztna-cp --no-pager
EOF

log_success "Service configured and started"

# 5. Vérifier que le service fonctionne
log_info "Checking service health..."
sleep 2

TLS_ENABLED="$(detect_tls_mode)"
PROTOCOL="http"
CURL_OPTS=()

if [ "$TLS_ENABLED" = "true" ]; then
    PROTOCOL="https"
    CURL_OPTS=(-k)
fi

if curl "${CURL_OPTS[@]}" -s "${PROTOCOL}://${VM_HOST}:8443/health" | grep -q "healthy"; then
    log_success "Control Plane is healthy and running!"
    echo ""
    echo "==================================="
    echo "  ZTNA Control Plane Deployed! ✅"
    echo "==================================="
    echo ""
    echo "API URL: ${PROTOCOL}://${VM_HOST}:8443"
    echo ""
    echo "Test with:"
    if [ "$TLS_ENABLED" = "true" ]; then
        echo "  curl -k https://${VM_HOST}:8443/health"
    else
        echo "  curl http://${VM_HOST}:8443/health"
    fi
    echo ""
    echo "View logs:"
    echo "  ssh ${VM_USER}@${VM_HOST} 'sudo journalctl -u ztna-cp -f'"
else
    log_error "Health check failed. Check logs with:"
    echo "  ssh ${VM_USER}@${VM_HOST} 'sudo journalctl -u ztna-cp -n 50'"
    exit 1
fi
