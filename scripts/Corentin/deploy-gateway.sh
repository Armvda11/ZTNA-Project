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
log "Compilation du gateway local..."
(cd "${GW_DIR}/cmd/ztna-gateway" && GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "${GW_DIR}/ztna-gateway-linux-amd64" .)
log "✓ Binaire compilé"

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
# Sélection du fichier de config (config.yaml > config.lab.yaml)
GW_CONFIG_FILE=""
if [[ -f "${GW_DIR}/config.yaml" ]]; then
  GW_CONFIG_FILE="${GW_DIR}/config.yaml"
elif [[ -f "${GW_DIR}/config.lab.yaml" ]]; then
  GW_CONFIG_FILE="${GW_DIR}/config.lab.yaml"
  log "⚠ config.yaml absent — utilisation de config.lab.yaml"
fi
if [[ -n "${GW_CONFIG_FILE}" ]]; then
  ${SCP} "${GW_CONFIG_FILE}" ${USER}@${GW_HOST}:/tmp/gateway.yaml
else
  log "⚠ Aucun fichier config gateway trouvé — la config existante (/etc/ztna/gateway.yaml) sera conservée"
  # Créer un fichier vide pour que le remote script ne plante pas sur le mv
  touch /tmp/gateway_empty_placeholder
fi
if [[ -f "${ROOT_DIR}/control-plane/certs/ca.crt" && -f "${ROOT_DIR}/control-plane/certs/server.crt" && -f "${ROOT_DIR}/control-plane/certs/server.key" ]]; then
  # Mapping attendu par gateway/config*.yaml :
  # - gateway.{crt,key}   : certificat serveur présenté aux clients mTLS
  # - client-ca.crt       : CA utilisée pour valider les certificats device
  # - cp-ca.crt           : CA utilisée pour valider le certificat TLS du CP
  ${SCP} "${ROOT_DIR}/control-plane/certs/server.crt" ${USER}@${GW_HOST}:/tmp/gateway.crt
  ${SCP} "${ROOT_DIR}/control-plane/certs/server.key" ${USER}@${GW_HOST}:/tmp/gateway.key
  ${SCP} "${ROOT_DIR}/control-plane/certs/ca.crt" ${USER}@${GW_HOST}:/tmp/cp-ca.crt
fi

log "Installation du gateway sur ztna-gw..."
${SSH} ${USER}@${GW_HOST} bash << 'REMOTE'
set -euo pipefail

# Binaire
sudo mv /tmp/ztna-gateway /usr/local/bin/ztna-gateway
sudo chmod +x /usr/local/bin/ztna-gateway

# Config (seulement si un nouveau fichier a été envoyé)
sudo mkdir -p /etc/ztna
if [[ -f /tmp/gateway.yaml ]]; then
  sudo mv /tmp/gateway.yaml /etc/ztna/gateway.yaml
  echo "✓ Config gateway mise à jour"
else
  echo "✓ Config existante /etc/ztna/gateway.yaml conservée"
fi
if [[ -f /tmp/gateway.crt && -f /tmp/gateway.key && -f /tmp/client-ca.crt && -f /tmp/cp-ca.crt ]]; then
  sudo mkdir -p /etc/ztna/certs
  sudo mv /tmp/gateway.crt /etc/ztna/certs/gateway.crt
  sudo mv /tmp/gateway.key /etc/ztna/certs/gateway.key
  sudo mv /tmp/client-ca.crt /etc/ztna/certs/client-ca.crt
  sudo mv /tmp/cp-ca.crt /etc/ztna/certs/cp-ca.crt
  sudo chmod 644 /etc/ztna/certs/gateway.crt /etc/ztna/certs/client-ca.crt /etc/ztna/certs/cp-ca.crt
  sudo chmod 600 /etc/ztna/certs/gateway.key
fi

# Récupérer dynamiquement la Device CA depuis le CP (source de vérité pour les certs clients mTLS)
if curl -sk --max-time 10 "https://10.10.20.30:8080/pki/device-ca/cert" -o /tmp/client-ca.crt && [[ -s /tmp/client-ca.crt ]]; then
  sudo mkdir -p /etc/ztna/certs
  sudo mv /tmp/client-ca.crt /etc/ztna/certs/client-ca.crt
  sudo chmod 644 /etc/ztna/certs/client-ca.crt
  echo "✓ Device CA récupérée depuis CP (/pki/device-ca/cert)"
else
  echo "⚠ Impossible de récupérer /pki/device-ca/cert (client-ca.crt inchangée)"
fi

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

# Résolution locale des ressources LAN utilisées dans les policies (resource_match)
sudo sed -i '/# ZTNA-LAB-HOSTS-BEGIN/,/# ZTNA-LAB-HOSTS-END/d' /etc/hosts
cat <<'HOSTS' | sudo tee -a /etc/hosts >/dev/null
# ZTNA-LAB-HOSTS-BEGIN
10.10.30.10 lan-app
10.10.30.11 lan-admin
# ZTNA-LAB-HOSTS-END
HOSTS

echo "✓ Gateway service installé"
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "4. Configuration SSH CA sur ztna-gw (jump host)"
# ──────────────────────────────────────────────────────────────────────────────
log "Récupération de la clé CA SSH depuis le CP..."
${SSH} ${USER}@${GW_HOST} bash << REMOTE
set -euo pipefail
for i in \$(seq 1 10); do
  if curl -sk --max-time 5 "https://${CP_HOST}:8080/pki/ssh-ca/pubkey" -o /tmp/ztna_ca.pub 2>/dev/null \
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
  if curl -sk --max-time 5 "https://${CP_HOST}:8080/pki/ssh-ca/pubkey" -o /tmp/ztna_ca.pub 2>/dev/null \
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

# S'assurer qu'un service HTTP répond sur :80 pour les tests Flux2.
if ! curl -sf --max-time 3 http://127.0.0.1/ >/dev/null 2>&1; then
  echo "⚠ Aucun HTTP local détecté sur lan-app:80, activation fallback"
  sudo mkdir -p /var/www/html
  echo '<html><body><h1>ZTNA Lab - lan-app</h1><p>Acces autorise via ZTNA</p></body></html>' | \
    sudo tee /var/www/html/index.html >/dev/null

  if command -v nginx >/dev/null 2>&1; then
    sudo systemctl enable --now nginx >/dev/null 2>&1 || true
  fi

  if ! curl -sf --max-time 3 http://127.0.0.1/ >/dev/null 2>&1; then
    sudo pkill -f "python3 -m http.server 80 --directory /var/www/html" >/dev/null 2>&1 || true
    sudo nohup python3 -m http.server 80 --directory /var/www/html \
      >/var/log/ztna-lan-app-http.log 2>&1 &
    sleep 1
  fi
fi

if curl -sf --max-time 3 http://127.0.0.1/ >/dev/null 2>&1; then
  echo "✓ Backend HTTP lan-app:80 opérationnel"
else
  echo "⚠ Backend HTTP lan-app:80 toujours indisponible (tests Flux2 risquent d'échouer)"
fi
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "6. Attente et configuration de lan-admin"
# ──────────────────────────────────────────────────────────────────────────────
log "Configuration SSH CA sur lan-admin..."
${SSH_LAN} ${USER}@${LAN_ADMIN_IP} bash << REMOTE
set -euo pipefail
for i in \$(seq 1 10); do
  if curl -sk --max-time 5 "https://${CP_HOST}:8080/pki/ssh-ca/pubkey" -o /tmp/ztna_ca.pub 2>/dev/null \
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
step "7. Routage WAN→DMZ (ip_forward + iptables Keycloak/CP)"
# ──────────────────────────────────────────────────────────────────────────────
log "Configuration du routage sur ztna-gw (nécessaire pour OIDC depuis wan-client)..."
cat "$(dirname "${BASH_SOURCE[0]}")/setup-gw-routing.sh" | \
  ${SSH} -o BatchMode=yes ${USER}@${GW_HOST} 'bash --norc -s' 2>&1 | grep -E "✓|✗|===" || true
log "✓ Routage WAN→DMZ configuré"

# ──────────────────────────────────────────────────────────────────────────────
step "8. Vérification finale"
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
