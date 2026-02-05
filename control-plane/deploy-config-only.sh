#!/bin/bash
###############################################################################
# ZTNA Control Plane - Configuration-Only Deployment (Fast Update)
#
# Usage: ./deploy-config-only.sh
#
# This script deploys ONLY the configuration file without rebuilding the binary.
# Useful for quick config updates (TLS settings, rate limits, policies, etc.)
###############################################################################

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
VM_HOST="10.10.20.30"
VM_USER="ztna"
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

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Check main.go exists to verify we're in correct directory
if [ ! -f "config.yaml" ]; then
    log_error "config.yaml not found. Run this script from control-plane directory."
    exit 1
fi

log_info "=== ZTNA Control Plane - Config-Only Deployment ==="
log_info "Target: ${VM_USER}@${VM_HOST}"
echo ""

# 1. Verify service is running before update
log_info "Checking current service status..."
if ! ssh -o ConnectTimeout=5 ${VM_USER}@${VM_HOST} 'systemctl is-active ztna-cp.service' >/dev/null 2>&1; then
    log_warn "Service appears to be stopped. Proceeding anyway..."
fi

# 2. Copy configuration file
log_info "Deploying configuration file..."
scp config.yaml ${VM_USER}@${VM_HOST}:${REMOTE_DIR}/
log_success "Configuration uploaded"

# 3. Reload and restart service
log_info "Restarting service to apply changes..."
ssh ${VM_USER}@${VM_HOST} << 'EOF'
sudo systemctl restart ztna-cp
sleep 2
EOF

log_success "Service restarted"

# 4. Verify service is running
log_info "Verifying service health..."
sleep 2

if curl -s -m 5 http://${VM_HOST}:8443/health >/dev/null 2>&1; then
    log_success "Control Plane is healthy"
else
    log_error "Health check failed"
    log_info "Check logs with: ssh ${VM_USER}@${VM_HOST} 'sudo journalctl -u ztna-cp -n 20'"
    exit 1
fi

echo ""
echo "==================================="
echo "  Config Update Complete! ✅"
echo "==================================="
echo ""
echo "What changed:"
grep -E "^(enabled|requests_per_minute|burst|cert:|key:)" config.yaml || echo "  (No breaking changes detected)"
echo ""
echo "View service logs:"
echo "  ssh ${VM_USER}@${VM_HOST} 'sudo journalctl -u ztna-cp -n 30 --no-pager'"
