#!/usr/bin/env bash
# Déploiement SIMPLE du control-plane + Keycloak sur ztna-cp
# Architecture: PC → SSH direct vers DMZ (NAT)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CP_DIR="${ROOT_DIR}/control-plane"
TARGET_HOST="10.10.20.30"
TARGET_USER="ztna"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10"

log() { echo "[$(date +%H:%M:%S)] $*"; }

# 1. Build binaire EN LOCAL
log "Build du control-plane..."
(cd "${CP_DIR}" && go build -o cp-linux-amd64 .)
log "✓ Binaire compilé"

# 2. Générer les certificats mTLS si nécessaire
if [[ ! -f "${CP_DIR}/certs/ca.crt" ]]; then
  log "Génération des certificats mTLS..."
  mkdir -p "${CP_DIR}/certs"
  (cd "${CP_DIR}/certs" && \
    openssl genrsa -out ca.key 4096 2>/dev/null && \
    openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 -subj "/CN=ztna-ca" -out ca.crt 2>/dev/null && \
    openssl genrsa -out server.key 2048 2>/dev/null && \
    openssl req -new -key server.key -subj "/CN=ztna-cp" -out server.csr 2>/dev/null && \
    openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt -days 365 -sha256 2>/dev/null && \
    openssl genrsa -out pep.key 2048 2>/dev/null && \
    openssl req -new -key pep.key -subj "/CN=pep-gw-1" -out pep.csr 2>/dev/null && \
    openssl x509 -req -in pep.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out pep.crt -days 365 -sha256 2>/dev/null)
  log "✓ Certificats générés"
fi

# 3. Attendre que ztna-cp soit accessible
log "Test de connectivité vers ztna-cp..."
for i in {1..30}; do
  if ssh ${SSH_OPTS} ${TARGET_USER}@${TARGET_HOST} "echo ok" >/dev/null 2>&1; then
    log "✓ ztna-cp accessible"
    break
  fi
  if [[ $i -eq 30 ]]; then
    log "✗ ztna-cp inaccessible après 30 tentatives"
    exit 1
  fi
  sleep 2
done

# 4. Vérifier Docker
log "Vérification Docker sur ztna-cp..."
if ! ssh ${SSH_OPTS} ${TARGET_USER}@${TARGET_HOST} "docker --version" >/dev/null 2>&1; then
  log "Installation de Docker..."
  ssh ${SSH_OPTS} ${TARGET_USER}@${TARGET_HOST} "sudo apt update && sudo DEBIAN_FRONTEND=noninteractive apt install -y docker.io docker-compose && sudo systemctl enable --now docker && sudo usermod -aG docker ztna"
fi
log "✓ Docker opérationnel"

# 5. Copier les fichiers
log "Copie des fichiers vers ztna-cp..."
ssh ${SSH_OPTS} ${TARGET_USER}@${TARGET_HOST} "mkdir -p ztna/control-plane"

tar -czf /tmp/ztna-deploy.tar.gz -C "${ROOT_DIR}" \
  control-plane/cp-linux-amd64 \
  control-plane/keycloak \
  control-plane/certs \
  control-plane/config.lab.yaml \
  control-plane/policies.yaml

scp ${SSH_OPTS} /tmp/ztna-deploy.tar.gz ${TARGET_USER}@${TARGET_HOST}:/tmp/
ssh ${SSH_OPTS} ${TARGET_USER}@${TARGET_HOST} "cd ztna && tar -xzf /tmp/ztna-deploy.tar.gz && chmod +x control-plane/cp-linux-amd64"
log "✓ Fichiers copiés"

# 6. Lancer Keycloak
log "Démarrage de Keycloak..."
ssh ${SSH_OPTS} ${TARGET_USER}@${TARGET_HOST} "cd ztna/control-plane/keycloak && docker-compose up -d"
sleep 5
log "✓ Keycloak démarré"

# 7. Créer et démarrer le service control-plane
log "Configuration du service control-plane..."
ssh ${SSH_OPTS} ${TARGET_USER}@${TARGET_HOST} "sudo tee /etc/systemd/system/ztna-cp.service >/dev/null" <<'EOF'
[Unit]
Description=ZTNA Control Plane
After=network.target docker.service

[Service]
Type=simple
User=ztna
WorkingDirectory=/home/ztna/ztna/control-plane
ExecStart=/home/ztna/ztna/control-plane/cp-linux-amd64 -config /home/ztna/ztna/control-plane/config.lab.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

ssh ${SSH_OPTS} ${TARGET_USER}@${TARGET_HOST} "sudo systemctl daemon-reload && sudo systemctl enable --now ztna-cp"
sleep 3
log "✓ Control-plane démarré"

# 8. Tests de santé
log "Vérification des services..."
if curl -sf "http://${TARGET_HOST}:8081/realms/ztna" >/dev/null 2>&1; then
  log "✓ Keycloak: http://${TARGET_HOST}:8081"
else
  log "⚠ Keycloak pas encore prêt (attendre 30s)"
fi

if curl -sfk "https://${TARGET_HOST}:8080/healthz" >/dev/null 2>&1; then
  log "✓ Control-plane: https://${TARGET_HOST}:8080"
else
  log "⚠ Control-plane: vérifier les logs avec 'make ssh-cp' puis 'sudo journalctl -u ztna-cp -f'"
fi

log ""
log "=== Déploiement terminé ==="
log "Keycloak:       http://${TARGET_HOST}:8081  (admin/admin)"
log "Control-plane:  https://${TARGET_HOST}:8080"
log "SSH:            ssh ${TARGET_USER}@${TARGET_HOST}"
