#!/usr/bin/env bash
# ============================================================================
# Déploiement PostgreSQL sur lan-app pour la démo ZTNA
# ============================================================================
# Installe PostgreSQL sur lan-app (10.10.30.10), crée la base "appdb" avec
# des données réalistes, et ajoute la route db:pg-staging sur le gateway.
#
# Usage:
#   bash scripts/deploy-postgresql.sh
#
# Pré-requis:
#   - Lab démarré (make lab-start)
#   - Gateway déployée (make deploy-gw)
#   - Connectivité SSH via jump host
# ============================================================================

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GW_HOST="10.10.10.20"
CP_HOST="10.10.20.30"
LAN_APP_IP="10.10.30.10"
USER="ztna"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=15"
SSH="ssh ${SSH_OPTS} -i ${SSH_KEY}"
SSH_LAN="${SSH} -J ${USER}@${GW_HOST}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; BLUE='\033[0;34m'; NC='\033[0m'

log()  { echo -e "[$(date +%H:%M:%S)] $*"; }
die()  { echo -e "${RED}[ERREUR]${NC} $*" >&2; exit 1; }
step() { echo; echo -e "${BLUE}──── $* ────${NC}"; }

# ──────────────────────────────────────────────────────────────────────────────
step "1/6 — Vérification de la connectivité"
# ──────────────────────────────────────────────────────────────────────────────
log "Test SSH vers ztna-gw (jump host)..."
${SSH} ${USER}@${GW_HOST} "echo ok" >/dev/null 2>&1 || die "ztna-gw inaccessible"
log "${GREEN}✓${NC} ztna-gw accessible"

log "Test SSH vers lan-app (via jump host)..."
for i in {1..10}; do
  if ${SSH_LAN} ${USER}@${LAN_APP_IP} "echo ok" >/dev/null 2>&1; then
    log "${GREEN}✓${NC} lan-app accessible"
    break
  fi
  [[ $i -eq 10 ]] && die "lan-app inaccessible après 10 tentatives"
  log "Tentative $i/10..."
  sleep 3
done

# ──────────────────────────────────────────────────────────────────────────────
step "2/6 — Configuration du routage LAN (NAT + DNS)"
# ──────────────────────────────────────────────────────────────────────────────
# lan-app est une VM isolée sans route par défaut vers internet.
# ztna-gw (10.10.30.20 côté LAN) fait le NAT masquerade.
# ip_forward est déjà activé sur ztna-gw.

log "Configuration NAT masquerade sur ztna-gw pour le sous-réseau LAN..."
${SSH} ${USER}@${GW_HOST} bash << 'GW_REMOTE'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
# Idempotent: vérifier si la règle existe déjà avant de l'ajouter
if ! sudo iptables -t nat -C POSTROUTING -s 10.10.30.0/24 ! -o lo -j MASQUERADE 2>/dev/null; then
  sudo iptables -t nat -A POSTROUTING -s 10.10.30.0/24 ! -o lo -j MASQUERADE
  echo "✓ NAT masquerade ajouté"
else
  echo "✓ NAT masquerade déjà en place"
fi
# Persistance via iptables-save
if command -v iptables-save >/dev/null 2>&1; then
  sudo mkdir -p /etc/iptables
  sudo iptables-save | sudo tee /etc/iptables/rules.v4 >/dev/null
  echo "✓ Règles sauvegardées dans /etc/iptables/rules.v4"
fi
# Assurer le rechargement au boot (iptables-persistent) — non-interactif
if ! systemctl is-active --quiet netfilter-persistent 2>/dev/null; then
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
    -o Dpkg::Options::="--force-confdef" \
    -o Dpkg::Options::="--force-confold" \
    iptables-persistent 2>/dev/null || true
fi
GW_REMOTE
log "${GREEN}✓${NC} NAT ztna-gw configuré"

log "Configuration route par défaut + DNS sur lan-app..."
${SSH_LAN} ${USER}@${LAN_APP_IP} bash << 'LAN_REMOTE'
set -euo pipefail

# ── Route par défaut ──
if ! ip route show default | grep -q default 2>/dev/null; then
  sudo ip route add default via 10.10.30.20
  echo "✓ Route par défaut ajoutée (via 10.10.30.20)"
else
  echo "✓ Route par défaut déjà présente"
fi

# ── Persistance route via rc.local ──
if ! grep -q "ip route add default via 10.10.30.20" /etc/rc.local 2>/dev/null; then
  # Créer /etc/rc.local s'il n'existe pas
  if [[ ! -f /etc/rc.local ]]; then
    printf '#!/bin/sh\nexit 0\n' | sudo tee /etc/rc.local >/dev/null
    sudo chmod +x /etc/rc.local
  fi
  # Insérer avant la dernière ligne (exit 0)
  sudo sed -i '/^exit 0/i ip route add default via 10.10.30.20 2>/dev/null || true' /etc/rc.local
  echo "✓ Route persistante dans /etc/rc.local"
else
  echo "✓ Route déjà persistante"
fi

# ── DNS upstream (systemd-resolved) ──
sudo mkdir -p /etc/systemd/resolved.conf.d
UPSTREAM_CONF=/etc/systemd/resolved.conf.d/upstream.conf
if [[ ! -f "$UPSTREAM_CONF" ]] || ! grep -q "8.8.8.8" "$UPSTREAM_CONF" 2>/dev/null; then
  printf '[Resolve]\nDNS=8.8.8.8 1.1.1.1\nFallbackDNS=8.8.4.4\n' \
    | sudo tee "$UPSTREAM_CONF" >/dev/null
  sudo systemctl restart systemd-resolved
  sleep 2
  echo "✓ DNS upstream configuré (8.8.8.8)"
else
  echo "✓ DNS upstream déjà configuré"
fi

# ── Vérification finale ──
if getent hosts archive.ubuntu.com >/dev/null 2>&1; then
  echo "✓ DNS OK → archive.ubuntu.com résolu"
else
  echo "ERREUR: DNS encore injoignable après configuration" >&2
  exit 1
fi
HTTP_CODE=$(curl -s --max-time 10 -o /dev/null -w "%{http_code}" http://archive.ubuntu.com/ || echo "0")
if [[ "$HTTP_CODE" == "200" || "$HTTP_CODE" == "302" || "$HTTP_CODE" == "301" ]]; then
  echo "✓ HTTP OK → archive.ubuntu.com accessible (${HTTP_CODE})"
else
  echo "ERREUR: accès HTTP archive.ubuntu.com échoué (code=${HTTP_CODE})" >&2
  exit 1
fi
LAN_REMOTE
log "${GREEN}✓${NC} Routage LAN opérationnel — lan-app a accès à internet"

# ──────────────────────────────────────────────────────────────────────────────
step "3/6 — Installation de PostgreSQL sur lan-app"
# ──────────────────────────────────────────────────────────────────────────────
${SSH_LAN} ${USER}@${LAN_APP_IP} bash << 'REMOTE'
set -euo pipefail

# Vérification si déjà installé et actif
if command -v psql >/dev/null 2>&1 && systemctl is-active --quiet postgresql 2>/dev/null; then
  echo "✓ PostgreSQL déjà installé et actif"
  # Vérifier si la base existe
  if sudo -u postgres psql -lqt | grep -qw appdb; then
    echo "✓ Base appdb existe déjà"
    exit 0
  fi
else
  echo "Installation de PostgreSQL..."
  export DEBIAN_FRONTEND=noninteractive

  echo "  - apt-get update (timeout 8 min)"
  sudo timeout 480 apt-get update -o Acquire::Retries=3 -o DPkg::Lock::Timeout=120
  echo "  - apt-get install postgresql postgresql-client (timeout 12 min)"
  sudo timeout 720 apt-get install -y \
    -o Acquire::Retries=3 \
    -o DPkg::Lock::Timeout=180 \
    -o Dpkg::Use-Pty=0 \
    postgresql postgresql-client
  echo "  - systemctl enable --now postgresql"
  sudo systemctl enable --now postgresql
  echo "✓ PostgreSQL installé"
fi
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "4/6 — Configuration réseau PostgreSQL"
# ──────────────────────────────────────────────────────────────────────────────
${SSH_LAN} ${USER}@${LAN_APP_IP} bash << 'REMOTE'
set -euo pipefail

PG_CONF=$(find /etc/postgresql -name postgresql.conf -type f 2>/dev/null | head -1)
PG_HBA=$(find /etc/postgresql -name pg_hba.conf -type f 2>/dev/null | head -1)

if [[ -z "$PG_CONF" || -z "$PG_HBA" ]]; then
  echo "ERREUR: fichiers PostgreSQL non trouvés" >&2
  exit 1
fi

# Écouter sur toutes les interfaces
if ! grep -q "^listen_addresses = '\*'" "$PG_CONF" 2>/dev/null; then
  # Commenter la ligne existante et ajouter la nouvelle
  sudo sed -i "s/^#*listen_addresses.*/# &/" "$PG_CONF"
  echo "listen_addresses = '*'" | sudo tee -a "$PG_CONF" >/dev/null
  echo "✓ listen_addresses = '*'"
fi

# Permettre les connexions depuis le réseau LAN et gateway
NEEDS_RESTART=false
for CIDR in "10.10.30.0/24" "10.10.20.0/24" "10.10.10.0/24"; do
  if ! grep -q "$CIDR" "$PG_HBA" 2>/dev/null; then
    echo "host    all    all    ${CIDR}    md5" | sudo tee -a "$PG_HBA" >/dev/null
    echo "✓ pg_hba.conf: ajouté ${CIDR}"
    NEEDS_RESTART=true
  fi
done

if $NEEDS_RESTART; then
  sudo systemctl restart postgresql
  echo "✓ PostgreSQL redémarré avec nouvelle config réseau"
else
  echo "✓ Configuration réseau déjà en place"
fi
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "5/6 — Création de la base de données et des données"
# ──────────────────────────────────────────────────────────────────────────────
${SSH_LAN} ${USER}@${LAN_APP_IP} bash << 'REMOTE'
set -euo pipefail

# Créer l'utilisateur alice
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='alice'" | grep -q 1 || \
  sudo -u postgres psql -c "CREATE USER alice WITH PASSWORD 'ztna2026' CREATEDB LOGIN;"
echo "✓ Utilisateur alice créé"

# Créer la base appdb
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='appdb'" | grep -q 1 || \
  sudo -u postgres createdb -O alice appdb
echo "✓ Base appdb créée"

# Créer les tables et insérer des données
sudo -u postgres psql -d appdb << 'SQL'
-- ============================================================================
-- Schema ZTNA — Base de données de démonstration
-- ============================================================================

CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL UNIQUE,
    email VARCHAR(128) NOT NULL,
    groups TEXT[] DEFAULT '{}',
    role VARCHAR(32) DEFAULT 'user',
    department VARCHAR(64),
    created_at TIMESTAMP DEFAULT NOW(),
    last_login TIMESTAMP,
    active BOOLEAN DEFAULT TRUE
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id INTEGER REFERENCES users(id),
    resource_name VARCHAR(128) NOT NULL,
    backend VARCHAR(128) NOT NULL,
    cert_serial VARCHAR(128),
    decision_id VARCHAR(128),
    started_at TIMESTAMP DEFAULT NOW(),
    ended_at TIMESTAMP,
    bytes_in BIGINT DEFAULT 0,
    bytes_out BIGINT DEFAULT 0,
    end_reason VARCHAR(64)
);

CREATE TABLE IF NOT EXISTS resources (
    id SERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL UNIQUE,
    display_name VARCHAR(256),
    type VARCHAR(32) NOT NULL,
    backend VARCHAR(128) NOT NULL,
    access_mode VARCHAR(32),
    description TEXT,
    gateway_id VARCHAR(64) DEFAULT 'ztna-gw-01'
);

CREATE TABLE IF NOT EXISTS policies (
    id SERIAL PRIMARY KEY,
    effect VARCHAR(16) NOT NULL,
    subject_match VARCHAR(128),
    action VARCHAR(32),
    resource_type VARCHAR(32),
    resource_match VARCHAR(128),
    description TEXT
);

CREATE TABLE IF NOT EXISTS audit_log (
    id SERIAL PRIMARY KEY,
    ts TIMESTAMP DEFAULT NOW(),
    actor VARCHAR(64),
    action VARCHAR(64),
    target VARCHAR(128),
    result VARCHAR(16),
    source_ip INET,
    details JSONB
);

-- ============================================================================
-- Données réalistes
-- ============================================================================

-- Utilisateurs nommés (équipe ZTNA)
INSERT INTO users (username, email, groups, role, department, last_login, active) VALUES
    ('alice',   'alice@corp.example.com',   '{ztna-admins,ztna-dba}', 'admin',    'Security',    NOW() - interval '12 minutes', TRUE),
    ('bob',     'bob@corp.example.com',     '{ztna-users}',           'user',     'Engineering', NOW() - interval '2 hours',    TRUE),
    ('charlie', 'charlie@corp.example.com', '{ztna-admins}',          'operator', 'DevOps',      NOW() - interval '1 day',      TRUE),
    ('diana',   'diana@corp.example.com',   '{ztna-dba}',             'dba',      'Data',        NOW() - interval '3 hours',    TRUE),
    ('eve',     'eve@corp.example.com',     '{ztna-users}',           'user',     'Engineering', NOW() - interval '5 days',     TRUE)
ON CONFLICT (username) DO NOTHING;

-- Utilisateurs générés (simulant une organisation de ~1250 personnes)
INSERT INTO users (username, email, groups, role, department, last_login, active)
SELECT
    'user_' || i,
    'user_' || i || '@corp.example.com',
    CASE
        WHEN i <= 50  THEN ARRAY['ztna-admins', 'ztna-dba']
        WHEN i <= 200 THEN ARRAY['ztna-admins']
        ELSE ARRAY['ztna-users']
    END,
    CASE
        WHEN i <= 10  THEN 'admin'
        WHEN i <= 100 THEN 'operator'
        ELSE 'user'
    END,
    (ARRAY['Security','Engineering','DevOps','Data','Support','Management'])[1 + (i % 6)],
    NOW() - (random() * interval '30 days'),
    (random() > 0.05)  -- 95% actifs
FROM generate_series(1, 1247) AS i
ON CONFLICT (username) DO NOTHING;

-- Ressources publiées (miroir de resources.yaml du CP)
INSERT INTO resources (name, display_name, type, backend, access_mode, description) VALUES
    ('grafana-internal', 'Grafana Monitoring',  'web', '10.10.30.15:3000', 'http-proxy',  'Dashboard Grafana interne'),
    ('ssh-dev-01',       'Dev Server SSH',      'ssh', '10.10.30.10:22',   'ssh-cert',    'Serveur de développement SSH'),
    ('pg-staging',       'PostgreSQL Staging',  'db',  '10.10.30.10:5432', 'tcp-tunnel',  'Base PostgreSQL staging')
ON CONFLICT (name) DO NOTHING;

-- Politiques (miroir de policies.yaml du CP)
INSERT INTO policies (effect, subject_match, action, resource_type, resource_match, description) VALUES
    ('allow', 'group:ztna-admins', 'connect', 'ssh',  'ssh:*',  'Admins: accès SSH complet'),
    ('allow', 'group:ztna-admins', 'connect', 'http', 'http:*', 'Admins: accès HTTP complet'),
    ('allow', 'group:ztna-admins', 'connect', 'web',  'web:*',  'Admins: accès Web complet'),
    ('allow', 'group:ztna-admins', 'connect', 'db',   'db:*',   'Admins: accès DB complet'),
    ('allow', 'group:ztna-dba',    'connect', 'db',   'db:*',   'DBA: accès DB complet'),
    ('deny',  '*',                 '*',       '*',    '*',       'Deny par défaut')
ON CONFLICT DO NOTHING;

-- Sessions récentes (historique réaliste)
INSERT INTO sessions (user_id, resource_name, backend, cert_serial, decision_id, started_at, ended_at, bytes_in, bytes_out, end_reason)
SELECT
    u.id,
    r.name,
    r.backend,
    md5(random()::text),
    'dec-' || gen_random_uuid()::text,
    NOW() - (random() * interval '24 hours'),
    NOW() - (random() * interval '23 hours'),
    (random() * 50000)::bigint,
    (random() * 100000)::bigint,
    (ARRAY['client_close','timeout','admin_kill','cert_revoked'])[1 + (random()*3)::int]
FROM users u
CROSS JOIN resources r
WHERE u.username IN ('alice','bob','charlie')
LIMIT 15
ON CONFLICT DO NOTHING;

-- Audit log
INSERT INTO audit_log (ts, actor, action, target, result, source_ip, details) VALUES
    (NOW() - interval '10 minutes', 'alice',   'connect',     'ssh-dev-01',       'allow', '10.10.10.10', '{"decision_id":"dec-001","groups":["ztna-admins"]}'),
    (NOW() - interval '8 minutes',  'alice',   'connect',     'pg-staging',       'allow', '10.10.10.10', '{"decision_id":"dec-002","groups":["ztna-admins","ztna-dba"]}'),
    (NOW() - interval '5 minutes',  'bob',     'connect',     'ssh-dev-01',       'deny',  '10.10.10.10', '{"reason":"group mismatch","required":"ztna-admins"}'),
    (NOW() - interval '3 minutes',  'alice',   'cert_request','device-cert',      'allow', '10.10.10.10', '{"serial":"3A:2B:1C:0D"}'),
    (NOW() - interval '1 minute',   'system',  'crl_refresh', 'gateway:ztna-gw-01','ok',   '10.10.10.20', '{"revoked_count":0}')
ON CONFLICT DO NOTHING;

-- Permissions
GRANT ALL ON ALL TABLES IN SCHEMA public TO alice;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO alice;

SQL

echo "✓ Tables créées et données insérées"

# Vérification
echo ""
echo "── Vérification ──"
sudo -u postgres psql -d appdb -c "SELECT count(*) AS total_users FROM users;"
sudo -u postgres psql -d appdb -c "SELECT name, type, backend, access_mode FROM resources ORDER BY name;"
sudo -u postgres psql -d appdb -c "SELECT count(*) AS total_audit_entries FROM audit_log;"
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
step "6/6 — Ajout de la route DB sur le gateway"
# ──────────────────────────────────────────────────────────────────────────────
log "Ajout de la route db:pg-staging → ${LAN_APP_IP}:5432 sur ztna-gw..."

${SSH} ${USER}@${GW_HOST} bash << 'REMOTE'
set -euo pipefail

CONFIG="/etc/ztna/gateway.yaml"

if [[ ! -f "$CONFIG" ]]; then
  echo "⚠ Config gateway non trouvée à $CONFIG"
  echo "  La route DB devra être ajoutée manuellement."
  exit 0
fi

# ── Réparer les routes orphelines (appended sans 'routes:' key) ──
ORPHAN_LINE=$(grep -n "^  - resource_type:" "$CONFIG" 2>/dev/null | head -1 | cut -d: -f1 || true)
if [[ -n "$ORPHAN_LINE" ]]; then
  PREV_LINE=$(sed -n "$((ORPHAN_LINE-1))p" "$CONFIG")
  if ! echo "$PREV_LINE" | grep -q "^routes:"; then
    sudo sed -i "${ORPHAN_LINE},\$d" "$CONFIG"
    sudo sed -i '/^[[:space:]]*$/{ /./!d }' "$CONFIG"
    echo "⚒ Route orpheline retirée du gateway.yaml"
  fi
fi

# ── Ajouter pg-staging avec la bonne structure ──
if grep -q "pg-staging" "$CONFIG" 2>/dev/null; then
  # Mettre à jour le target si besoin
  sudo sed -i '/pg-staging/{n;s|target:.*|target: "10.10.30.10:5432"|}' "$CONFIG"
  echo "✓ Route pg-staging déjà présente (target vérifié)"
elif grep -q "^routes:" "$CONFIG"; then
  # Section routes: existe — ajouter l'entrée
  printf '  - resource_type: "db"\n    resource_match: "db:pg-staging"\n    target: "10.10.30.10:5432"\n' \
    | sudo tee -a "$CONFIG" >/dev/null
  echo "✓ Route db:pg-staging ajoutée (section routes: existante)"
else
  # Pas de section routes: — créer la section complète
  printf '\nroutes:\n  - resource_type: "db"\n    resource_match: "db:pg-staging"\n    target: "10.10.30.10:5432"\n' \
    | sudo tee -a "$CONFIG" >/dev/null
  echo "✓ Section routes: créée avec db:pg-staging"
fi

# ── Redémarrer le gateway ──
sudo systemctl restart ztna-gateway
sleep 2
if systemctl is-active --quiet ztna-gateway; then
  echo "✓ ztna-gateway actif avec route db:pg-staging"
else
  echo "⚠ Gateway ne redémarre pas — journalctl:" >&2
  sudo journalctl -u ztna-gateway --no-pager -n 10 >&2
  exit 1
fi
REMOTE

# ──────────────────────────────────────────────────────────────────────────────
echo ""
log "${GREEN}╔══════════════════════════════════════════════════════════════╗${NC}"
log "${GREEN}║         PostgreSQL déployé sur lan-app ✓                    ║${NC}"
log "${GREEN}╠══════════════════════════════════════════════════════════════╣${NC}"
log "${GREEN}║  Host     : ${LAN_APP_IP}          Port: 5432              ║${NC}"
log "${GREEN}║  Database : appdb                User: alice / ztna2026    ║${NC}"
log "${GREEN}║  Tables   : users (~1252), sessions, resources,            ║${NC}"
log "${GREEN}║             policies, audit_log                            ║${NC}"
log "${GREEN}╠══════════════════════════════════════════════════════════════╣${NC}"
log "${GREEN}║  Gateway route: db:pg-staging → ${LAN_APP_IP}:5432         ║${NC}"
log "${GREEN}║                                                            ║${NC}"
log "${GREEN}║  Test direct (via SSH tunnel ZTNA):                        ║${NC}"
log "${GREEN}║    make test-flux1  # obtenir cert SSH                     ║${NC}"
log "${GREEN}║    ssh -L 15432:${LAN_APP_IP}:5432 -J ztna@${GW_HOST} \\   ║${NC}"
log "${GREEN}║        ztna@${LAN_APP_IP} -N &                             ║${NC}"
log "${GREEN}║    PGPASSWORD=ztna2026 psql -h localhost -p 15432 \\       ║${NC}"
log "${GREEN}║        -U alice appdb                                      ║${NC}"
log "${GREEN}╚══════════════════════════════════════════════════════════════╝${NC}"
