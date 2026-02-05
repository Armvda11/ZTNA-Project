#!/bin/bash
###############################################################################
# ZTNA Gateway - Script de Build et Déploiement
###############################################################################

set -e

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
VM_HOST="10.10.20.20"
VM_USER="ztna"
BINARY_NAME="ztna-gw"
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

# Vérifier que nous sommes dans le bon répertoire
if [ ! -f "main.go" ]; then
    log_error "main.go not found. Please run from gateway/ directory"
    exit 1
fi

# 1. Build le binaire
log_info "Building Gateway for Linux..."
GOOS=linux GOARCH=amd64 go build -o ${BINARY_NAME} main.go
log_success "Build completed: ${BINARY_NAME}"

# 2. Copier le binaire vers la VM
log_info "Copying binary to ${VM_USER}@${VM_HOST}:${REMOTE_DIR}..."
scp ${BINARY_NAME} ${VM_USER}@${VM_HOST}:/tmp/
ssh ${VM_USER}@${VM_HOST} "sudo mv /tmp/${BINARY_NAME} ${REMOTE_DIR}/${BINARY_NAME} && chmod +x ${REMOTE_DIR}/${BINARY_NAME}"
log_success "Binary copied and made executable"

# 3. Copier la configuration
log_info "Copying configuration..."
scp config.yaml ${VM_USER}@${VM_HOST}:${REMOTE_DIR}/
log_success "Configuration copied"

# 4. Générer host key si nécessaire
log_info "Checking SSH host key..."
ssh ${VM_USER}@${VM_HOST} << 'EOF'
if [ ! -f /etc/ztna/gateway_host_key ]; then
    echo "Generating SSH host key..."
    sudo mkdir -p /etc/ztna
    sudo ssh-keygen -t ed25519 -f /etc/ztna/gateway_host_key -N '' -q
    sudo chown ztna:ztna /etc/ztna/gateway_host_key*
    sudo chmod 600 /etc/ztna/gateway_host_key
    echo "Host key generated"
else
    echo "Host key already exists"
fi
EOF
log_success "Host key ready"

# 5. Créer le service systemd et démarrer
log_info "Setting up systemd service..."
ssh ${VM_USER}@${VM_HOST} << 'EOF'
# Créer les répertoires nécessaires
sudo mkdir -p /etc/ztna
sudo mkdir -p /var/lib/ztna
sudo chown -R ztna:ztna /etc/ztna /var/lib/ztna

# Créer le service systemd
sudo tee /etc/systemd/system/ztna-gw.service > /dev/null <<SERVICE
[Unit]
Description=ZTNA Gateway (Policy Enforcement Point)
After=network.target

[Service]
Type=simple
User=ztna
WorkingDirectory=/home/ztna
ExecStart=/home/ztna/ztna-gw -config /home/ztna/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
SERVICE

# Recharger systemd et démarrer le service
sudo systemctl daemon-reload
sudo systemctl enable ztna-gw
sudo systemctl restart ztna-gw
EOF

log_success "Systemd service configured and started"

# 6. Vérifier que le service tourne
sleep 2
log_info "Checking service status..."
if ssh ${VM_USER}@${VM_HOST} 'systemctl is-active ztna-gw' >/dev/null 2>&1; then
    log_success "Gateway is running!"
else
    log_error "Gateway failed to start. Check logs with: ssh ${VM_USER}@${VM_HOST} 'sudo journalctl -u ztna-gw -n 50'"
    exit 1
fi

# 7. Afficher les logs récents
log_info "Recent logs:"
ssh ${VM_USER}@${VM_HOST} 'sudo journalctl -u ztna-gw -n 10 --no-pager' || true

echo ""
log_success "=========================================="
log_success "  Gateway Deployment Complete!"
log_success "=========================================="
echo ""
echo "Next steps:"
echo "  1. Check logs: ssh ${VM_USER}@${VM_HOST} 'sudo journalctl -u ztna-gw -f'"
echo "  2. Test SSH: ssh -p 2222 alice@${VM_HOST}"
echo "  3. View config: ssh ${VM_USER}@${VM_HOST} 'cat ${REMOTE_DIR}/config.yaml'"
echo ""
