# ZTNA Control Plane

Control Plane (Policy Decision Point) du système ZTNA.

## Documentation

- **[TESTING.md](TESTING.md)** - Guide complet de test (HTTP, HTTPS, rapports)
- **[DEPLOYMENT.md](DEPLOYMENT.md)** - Rapport de déploiement complet et statut du système
- **[SECURITY.md](SECURITY.md)** - Vulnérabilités corrigées et procédures de sécurité
- **[security-audit.sh](security-audit.sh)** - Script d'audit de sécurité automatisé
- **[deploy.sh](deploy.sh)** - Script de déploiement complet (build + config + service)
- **[deploy-config-only.sh](deploy-config-only.sh)** - Déploiement rapide de config (sans rebuild)
- **[e2e-test.sh](e2e-test.sh)** - Tests E2E complets avec rapports JSON/Markdown

## Fonctionnalités

- **Authentification JWT** - Login sécurisé avec tokens JWT
- **Autorité de Certification SSH** - Génération de certificats SSH temporaires (15 min)
- **Moteur de Politiques** - Règles d'accès configurables (ABAC)
- **Audit Logging** - Traçabilité complète de tous les accès
- **API REST** - Endpoints documentés pour authentication, certificats, et politiques
- **Base de données SQLite** - Stockage des utilisateurs et audit logs

## Démarrage Rapide

### 1. Installer les dépendances

```bash
cd control-plane
go mod download
```

### 2. Lancer le serveur

```bash
# Avec configuration par défaut
go run main.go

# Avec configuration personnalisée
go run main.go -config /path/to/config.yaml
```

Le serveur démarre sur `http://0.0.0.0:8443`

### 3. Tester l'API

```bash
# Health check
curl http://localhost:8443/health

# Login
curl -X POST http://localhost:8443/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"alice123"}'

# Demande de certificat SSH (nécessite token JWT)
curl -X POST http://localhost:8443/api/v1/certs/request \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"public_key":"ssh-ed25519 AAAA..."}'
```

## API Endpoints

### Public Endpoints

| Méthode | Endpoint | Description |
|---------|----------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/auth/login` | Authentification |

### Protected Endpoints (JWT requis)

| Méthode | Endpoint | Description |
|---------|----------|-------------|
| POST | `/api/v1/certs/request` | Demander certificat SSH |
| GET | `/api/v1/policies/{resource}` | Vérifier politique d'accès |
| GET | `/api/v1/audit` | Récupérer logs d'audit |
| GET | `/api/v1/ca/public-key` | Obtenir clé publique CA |

## Configuration

Fichier `config.yaml` :

```yaml
server:
  host: "0.0.0.0"
  port: 8443
  tls:
    enabled: false
    cert: "/etc/ztna/tls/server.crt"
    key: "/etc/ztna/tls/server.key"

auth:
  jwt_secret: "change-me"  # Override avec ZTNA_JWT_SECRET
  token_expiry: "15m"
  rate_limit:
    enabled: true
    requests_per_minute: 5
    burst: 10

ssh:
  ca_key_path: "/etc/ztna/ssh_ca"
  cert_validity: "15m"
  cert_principals:
    - "ztna-user"

policies:
  default_deny: true
  rules:
    - user: "alice"
      resources: ["lan-app", "lan-admin"]
      allowed: true
```

## Architecture

```
control-plane/
├── main.go                 # Point d'entrée
├── config.yaml             # Configuration
├── go.mod                  # Dépendances Go
└── internal/
    ├── api/                # API REST handlers
    │   └── server.go
    ├── config/             # Configuration management
    │   └── config.go
    ├── logger/             # Logging structuré
    │   └── logger.go
    ├── sshca/              # SSH Certificate Authority
    │   └── sshca.go
    └── storage/            # Database operations
        └── storage.go
```

## 🔒 Sécurité

### Production Checklist

- [ ] Changer `jwt_secret` (utiliser variable d'environnement `ZTNA_JWT_SECRET`)
- [ ] Activer TLS (certificat/clé valides) et désactiver HTTP
- [ ] Vérifier le stockage bcrypt des passwords et migrer les anciens comptes
- [ ] Protéger la clé privée CA SSH (permissions 600)
- [ ] Ajuster rate limiting (`auth.rate_limit`) selon la charge

### Certificats SSH

Le Control Plane génère automatiquement une paire de clés Ed25519 au premier démarrage :

- Clé privée : `/etc/ztna/ssh_ca`
- Clé publique : `/etc/ztna/ssh_ca.pub`
- Trusted CA keys : `/etc/ztna/ssh_ca.trustedkeys`

Pour activer la validation des certificats sur les serveurs SSH :

```bash
# Ajouter à /etc/ssh/sshd_config
TrustedUserCAKeys /etc/ztna/ssh_ca.trustedkeys
```

## Tests

### Tests Unitaires

```bash
# Tests unitaires
go test ./...

# Tests avec couverture
go test -cover ./...

# Tests avec verbose
go test -v ./...
```

### Tests d'Intégration E2E

```bash
# Test HTTP standard
./e2e-test.sh

# Test HTTPS (self-signed)
./e2e-test.sh --https

# Générer rapport JSON (pour CI/CD)
./e2e-test.sh --report json

# Générer rapport Markdown (pour documentation)
./e2e-test.sh --report markdown

# Test HTTPS + rapport JSON
./e2e-test.sh --https --report json

# Corriger les clés SSH hôtes (après recréation des VMs)
./e2e-test.sh --fix-known-hosts
```

**Voir [TESTING.md](TESTING.md) pour des scénarios de test détaillés**

## Build & Déploiement

### Build

```bash
# Build local
go build -o ztna-cp main.go

# Build pour Linux (cross-compilation)
GOOS=linux GOARCH=amd64 go build -o ztna-cp main.go
```

### Déploiement sur VM

#### Option 1: Déploiement Complet (Build + Config + Service)

```bash
./deploy.sh
```

Utilisé pour:
- Premier déploiement
- Changements de code Go
- Méises à jour de dépendances
- Recréation d'une VM

#### Option 2: Déploiement Configuration Uniquement (Rapide)

```bash
./deploy-config-only.sh
```

Utilisé pour:
- Mise à jour TLS (enabled/disabled)
- Ajustement du rate limiting
- Modification des politiques d'accès
- Changements de log level
- Itération rapide lors des tests

**Workflow d'itération rapide:**

```bash
# 1. Editer config.yaml
vim config.yaml

# 2. Déployer (2-5 secondes)
./deploy-config-only.sh

# 3. Tester
./e2e-test.sh --report json

# 4. Répéter
```

#### Déploiement Manuel (Avancé)

```bash
# Copier le binaire
scp ztna-cp ztna@10.10.20.30:/home/ztna/

# Copier la configuration
scp config.yaml ztna@10.10.20.30:/home/ztna/

# SSH vers la VM
ssh ztna@10.10.20.30

# Créer le service systemd
sudo tee /etc/systemd/system/ztna-cp.service <<EOF
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
EOF

# Activer et démarrer
sudo systemctl daemon-reload
sudo systemctl enable ztna-cp
sudo systemctl start ztna-cp
sudo systemctl status ztna-cp
```

## Monitoring

```bash
# Logs en temps réel
sudo journalctl -u ztna-cp -f

# Vérifier la santé
curl http://localhost:8443/health

# Métriques (TODO: Prometheus integration)
curl http://localhost:8443/metrics
```

## Debugging

### Activer le mode debug

```yaml
logging:
  level: "debug"
  format: "json"
```

### Logs structurés

```bash
# Filtrer les logs par niveau
sudo journalctl -u ztna-cp | grep '"level":"ERROR"'

# Logs d'authentification
sudo journalctl -u ztna-cp | grep 'login'
```

## Roadmap

- [ ] Intégration OPA pour politiques avancées
- [ ] Support LDAP/Active Directory
- [ ] Multi-factor authentication (MFA)
- [ ] Rate limiting et protection DDoS
- [ ] Métriques Prometheus
- [ ] Dashboards Grafana
- [ ] Rotation automatique des clés CA
- [ ] Support PostgreSQL
- [ ] API GraphQL
- [ ] WebSocket pour notifications temps réel

## 📝 Licence

MIT License - Voir [LICENSE](../LICENSE)
