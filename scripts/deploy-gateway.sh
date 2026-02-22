#!/usr/bin/env bash
# Déploiement du ZTNA Gateway sur ztna-gw
# + Configuration SSH CA trust sur ztna-gw et lan-app
#
# Usage:
#   ./scripts/deploy-gateway.sh
#
# Pré-requis:
#   - Le lab Terraform est démarré (terraform apply)
#   - Le control-plane est déployé et accessible (./scripts/deploy-control-plane.sh)
#   - Le binaire gateway/ztna-gateway-linux-amd64 existe

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GW_DIR="${ROOT_DIR}/gateway"
GW_BIN="${GW_DIR}/ztna-gateway-linux-amd64"

GW_HOST="10.10.10.20"           # WAN IP de ztna-gw (accessible depuis le PC de dev)
CP_HOST="10.10.20.30"           # IP DMZ du control-plane
LAN_APP_IP="10.10.30.10"
LAN_ADMIN_IP="10.10.30.11"

USER="ztna"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=15"
SSH="ssh ${SSH_OPTS}"
SCP="scp ${SSH_OPTS}"
# Pour les VMs LAN (isolées), on passe par ztna-gw en jump host
SSH_LAN="${SSH} -J ${USER}@${GW_HOST}"

log()  { echo "[$(date +%H:%M:%S)] $*"; }
die()  { echo "[ERROR] $*" >&2; exit 1; }
step() { echo; echo "──── $* ────"; }

# ──────────────────────────────────────────────────────────────────────────────
step "1. Vérification des prérequis"
# ──────────────────────────────────────────────────────────────────────────────
if [[ ! -f "${GW_BIN}" ]]; then
  log "Binaire non trouvé, compilation..."
  (cd "${GW_DIR}" && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ztna-gateway-linux-amd64 .)
  log "✓ Binaire compilé"
fi

# ──────────────────────────────────────────────────────────────────────────────
step "2. Attente de ztna-gw"
# ──────────────────────────────────────────────────────────────────────────────
for i in {1..30}; do
  if ${SSH} ${USER}@${GW_HOST} "echo ok" >/dev/null 2>&1; then
    log "✓ ztna-gw accessible"
    break
  fi
  [[ $i -eq 30 ]] && die "ztna-gw inaccessible après 30 tentatives"
  log "Tentative $i/30..."
  sleep 5
done

# ──────────────────────────────────────────────────────────────────────────────
step "3. Configuration de ztna-gw"
# ──────────────────────────────────────────────────────────────────────────────
log "Copie du binaire gateway..."
${SCP} "${GW_BIN}" ${USER}@${GW_HOST}:/tmp/ztna-gateway
${SCP} "${GW_DIR}/config.yaml" ${USER}@${GW_HOST}:/tmp/gateway.yaml

log "Installation du gateway sur ztna-gw..."
${SSH} ${USER}@${GW_HOST} bash << 'REMOTE'
set -euo pipefail

# Binaire
sudo mv /tmp/ztna-gateway /usr/local/bin/ztna-gateway
sudo chmod +x /usr/local/bin/ztna-gateway

# Config
sudo mkdir -p /etc/ztna
sudo mv /tmp/gateway.yaml /etc/ztna/gateway.yaml

# Systemd service
sudo tee /etc/systemd/system/ztna-gateway.service > /dev/null << 'SVC'
[Unit]
Description=ZTNA Gateway (mTLS PEP)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ztna-gateway -config /etc/ztna/gateway.yaml
Restart=on-failure
RestartSec=5
User=root
WorkingDirectory=/etc/ztna

[Install]
WantedBy=multi-user.target
SVC

sudo systemctl daemon-reload
sudo systemctl enable ztna-gateway
sudo systemctl restart ztna-gateway || true
echo "✓ Gateway service installé"
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "4. Configuration SSH CA sur ztna-gw (jump host)"
# ──────────────────────────────────────────────────────────────────────────────
log "Récupération de la clé CA SSH depuis le CP..."
${SSH} ${USER}@${GW_HOST} bash << REMOTE
set -euo pipefail
for i in \$(seq 1 10); do
  if curl -sk "https://${CP_HOST}:8080/pki/ssh-ca/pubkey" -o /tmp/ztna_ca.pub 2>/dev/null \
      && [[ -s /tmp/ztna_ca.pub ]]; then
    echo "✓ CA SSH récupérée (tentative \$i)"
    break
  fi
  echo "Attente du CP (tentative \$i/10)..."
  sleep 5
done

sudo cp /tmp/ztna_ca.pub /etc/ssh/ztna_ca.pub
sudo chmod 644 /etc/ssh/ztna_ca.pub

# Ajouter TrustedUserCAKeys si pas encore présent
grep -q "TrustedUserCAKeys" /etc/ssh/sshd_config || \
  echo 'TrustedUserCAKeys /etc/ssh/ztna_ca.pub' | sudo tee -a /etc/ssh/sshd_config

# Autoriser le jump host à relayer vers n'importe quelle destination
grep -q "PermitOpen" /etc/ssh/sshd_config || \
  echo 'PermitOpen any' | sudo tee -a /etc/ssh/sshd_config

sudo systemctl restart sshd
echo "✓ SSH CA configurée sur ztna-gw"
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "5. Attente et configuration de lan-app"
# ──────────────────────────────────────────────────────────────────────────────
log "Test de connectivité vers lan-app (via ztna-gw)..."
for i in {1..20}; do
  if ${SSH_LAN} ${USER}@${LAN_APP_IP} "echo ok" >/dev/null 2>&1; then
    log "✓ lan-app accessible"
    break
  fi
  [[ $i -eq 20 ]] && die "lan-app inaccessible après 20 tentatives"
  log "Tentative $i/20..."
  sleep 5
done

log "Configuration SSH CA sur lan-app..."
${SSH_LAN} ${USER}@${LAN_APP_IP} bash << REMOTE
set -euo pipefail
for i in \$(seq 1 10); do
  if curl -sk "https://${CP_HOST}:8080/pki/ssh-ca/pubkey" -o /tmp/ztna_ca.pub 2>/dev/null \
      && [[ -s /tmp/ztna_ca.pub ]]; then
    echo "✓ CA SSH récupérée"
    break
  fi
  echo "Attente du CP (tentative \$i/10)..."
  sleep 5
done

sudo cp /tmp/ztna_ca.pub /etc/ssh/ztna_ca.pub
sudo chmod 644 /etc/ssh/ztna_ca.pub

grep -q "TrustedUserCAKeys" /etc/ssh/sshd_config || \
  echo 'TrustedUserCAKeys /etc/ssh/ztna_ca.pub' | sudo tee -a /etc/ssh/sshd_config

sudo systemctl restart sshd
echo "✓ SSH CA configurée sur lan-app"
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "6. Attente et configuration de lan-admin"
# ──────────────────────────────────────────────────────────────────────────────
log "Configuration SSH CA sur lan-admin..."
${SSH_LAN} ${USER}@${LAN_ADMIN_IP} bash << REMOTE
set -euo pipefail
for i in \$(seq 1 10); do
  if curl -sk "https://${CP_HOST}:8080/pki/ssh-ca/pubkey" -o /tmp/ztna_ca.pub 2>/dev/null \
      && [[ -s /tmp/ztna_ca.pub ]]; then
    echo "✓ CA SSH récupérée"
    break
  fi
  echo "Attente du CP (tentative \$i/10)..."
  sleep 5
done

sudo cp /tmp/ztna_ca.pub /etc/ssh/ztna_ca.pub
sudo chmod 644 /etc/ssh/ztna_ca.pub

grep -q "TrustedUserCAKeys" /etc/ssh/sshd_config || \
  echo 'TrustedUserCAKeys /etc/ssh/ztna_ca.pub' | sudo tee -a /etc/ssh/sshd_config

sudo systemctl restart sshd
echo "✓ SSH CA configurée sur lan-admin"
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "7. Vérification finale"
# ──────────────────────────────────────────────────────────────────────────────
log "Status du service ztna-gateway..."
${SSH} ${USER}@${GW_HOST} "sudo systemctl status ztna-gateway --no-pager -l | head -20"

echo
log "╔═══════════════════════════════════════════════════════════════╗"
log "║             DÉPLOIEMENT GATEWAY TERMINÉ ✓                    ║"
log "╠═══════════════════════════════════════════════════════════════╣"
log "║  Gateway mTLS   : ${GW_HOST}:4433                          ║"
log "║  SSH Jump Host  : ${USER}@${GW_HOST}:22                    ║"
log "║  lan-app HTTP   : 10.10.30.10:80 (via gateway mTLS)         ║"
log "║  lan-app SSH    : 10.10.30.10:22 (cert ou jump host)        ║"
log "║  lan-admin SSH  : 10.10.30.11:22 (cert ou jump host)        ║"
log "╠═══════════════════════════════════════════════════════════════╣"
log "║  Test SSH cert : ./scripts/test-ssh-cert-access.sh          ║"
log "║  Test mTLS     : ./scripts/test-mtls-access.sh              ║"
log "╚═══════════════════════════════════════════════════════════════╝"
