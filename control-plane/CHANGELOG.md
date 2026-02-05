# Changelog

Tous les changements notables du ZTNA Control Plane sont documentés ici.

Le format est basé sur [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/),
et ce projet suit [Semantic Versioning](https://semver.org/lang/fr/).

## [0.1.0] - 2026-02-04

### 🎉 Release Initiale

#### Added
- **Authentification JWT** avec tokens de 15 minutes
  - Endpoint POST `/api/v1/auth/login`
  - Middleware d'authentification Bearer token
  - Support variable d'environnement `ZTNA_JWT_SECRET`

- **SSH Certificate Authority (CA)**
  - Génération automatique clé Ed25519 au premier démarrage
  - Émission de certificats SSH temporaires (15 min)
  - Endpoint POST `/api/v1/certs/request`
  - Principals: `ztna-user`, `app-access`
  - Génération fichier TrustedUserCAKeys pour sshd

- **API RESTful**
  - GET `/health` - Health check
  - POST `/api/v1/auth/login` - Authentification
  - POST `/api/v1/certs/request` - Demande de certificat SSH
  - GET `/api/v1/policies/{resource}` - Vérification de politique
  - GET `/api/v1/audit` - Récupération logs d'audit

- **Moteur de Politiques ABAC**
  - Configuration YAML des règles d'accès
  - Support policies par resource et utilisateur
  - Default deny avec allow explicite

- **Base de données SQLite**
  - Table `users` (id, username, password, created_at)
  - Table `audit_logs` (id, timestamp, username, action, resource, result, client_ip)
  - Création automatique du schéma
  - Seed utilisateurs par défaut (alice/bob)

- **Logging Structuré**
  - Format JSON pour parsing automatique
  - Niveaux: DEBUG, INFO, WARN, ERROR
  - Sortie stdout + fichier optionnel
  - Champs: timestamp, level, message, key-value pairs

- **Configuration YAML**
  - Server (port, timeouts, TLS)
  - Auth (JWT secret, token expiry)
  - SSH (CA key path, cert validity, principals)
  - Policies (ABAC rules)
  - Logging (level, format, file)
  - Database (path, SQLite)

- **Déploiement Automatisé**
  - Script `deploy.sh` pour VM
  - Service systemd `ztna-cp.service`
  - Build cross-compilation Linux
  - Health check post-déploiement

- **Tests Unitaires**
  - 12 tests couvrant: config (3), logger (7), storage (5)
  - `go test -v ./internal/...`

- **Documentation**
  - README.md avec exemples API
  - DEPLOYMENT.md avec rapport complet
  - SECURITY.md avec CVE et procédures
  - Commentaires inline dans le code

- **Outils de Sécurité**
  - Script `security-audit.sh` automatisé
  - Vérification versions dépendances
  - Check configuration sécurité
  - Analyse permissions fichiers
  - Détection secrets hardcodés

#### Security
- **Patches CVE Appliqués**
  - CVE-2024-45337 (Critical CVSS 9.1) - Authorization bypass
  - CVE-2025-22869 (High CVSS 7.5) - DoS attack
  - CVE-2025-30204 (High CVSS 7.5) - JWT memory exhaustion
  - CVE-2025-47914 (Medium CVSS 5.3)
  - CVE-2025-58181 (Medium CVSS 5.3)

- **Dépendances Sécurisées**
  - golang.org/x/crypto v0.47.0 (was v0.18.0)
  - github.com/golang-jwt/jwt/v5 v5.3.1 (was v5.2.0)
  - github.com/mattn/go-sqlite3 v1.14.33 (was v1.14.19)

- **Bonnes Pratiques**
  - Clé CA privée avec permissions 600
  - JWT secret via variable d'environnement
  - Service systemd en utilisateur non-root (ztna)
  - Timeout de 15 minutes sur tokens/certificats
  - Audit logging de toutes les actions

#### Fixed
- **Bug SSH CA Key Format** (Critique)
  - Problème: Clé Ed25519 générée en format binaire invalide
  - Symptôme: Service crash au redémarrage avec "Failed to initialize SSH CA"
  - Cause: `ssh.Marshal(pemBlock)` au lieu de `pem.EncodeToMemory(pemBlock)`
  - Fix: Ajout import `encoding/pem` et utilisation correcte de `pem.EncodeToMemory()`
  - Résultat: Clé au format OpenSSH PEM standard, rechargeable au redémarrage

#### Changed
- **Go Toolchain** upgradé de 1.21 → 1.24.13 (requis par crypto v0.47.0)

#### Known Issues
- ⚠️ Mots de passe stockés en clair (TODO: bcrypt)
- ⚠️ TLS désactivé par défaut (TODO: activer en production)
- ⚠️ Pas de rate limiting sur login (TODO: ajouter middleware)
- ⚠️ JWT secret par défaut en config.yaml (Utiliser env var en prod)

---

## [Unreleased] - Roadmap

### À Venir - v0.2.0

#### Planned
- [ ] **TLS/HTTPS Activé**
  - Génération certificats auto-signés pour dev
  - Support Let's Encrypt pour production
  - Configuration TLS 1.3 uniquement

- [ ] **Bcrypt Password Hashing**
  - Migration depuis plaintext
  - Script de migration utilisateurs existants

- [ ] **Rate Limiting**
  - Middleware `golang.org/x/time/rate`
  - 5 tentatives login / minute / IP
  - Headers HTTP X-RateLimit-*

- [ ] **Open Policy Agent (OPA) Integration**
  - Remplacement policies YAML hardcodées
  - Fichiers .rego pour policies
  - Context-aware policies (IP, time, device, MFA)
  - API OPA sidecar ou embedded

- [ ] **Gateway (PEP) Implementation**
  - Proxy SSH entre WAN clients et LAN apps
  - Validation certificats signés par CA
  - Enforcement policies depuis Control Plane
  - Routing vers lan-app/lan-admin
  - SSH session logging

### À Venir - v0.3.0

#### Planned
- [ ] **Multi-Factor Authentication (MFA)**
  - TOTP (Time-based One-Time Password)
  - QR code enrollment
  - Backup codes
  - MFA required per-resource

- [ ] **Certificate Revocation**
  - CRL (Certificate Revocation List)
  - Revocation API endpoint
  - Propagation aux Gateways

- [ ] **Monitoring & Observability**
  - Métriques Prometheus (/metrics)
  - Traces OpenTelemetry
  - Dashboards Grafana
  - Alerting (failed logins, cert expirations)

- [ ] **Centralized Logging**
  - Export vers ELK ou Loki
  - Structured log parsing
  - Search & analytics

### À Venir - v0.4.0

#### Planned
- [ ] **High Availability (HA)**
  - Multiple Control Plane instances
  - Load balancing
  - Session replication
  - Health checks

- [ ] **Database Migration**
  - PostgreSQL pour production
  - Migration scripts depuis SQLite
  - Connection pooling
  - Backup/restore automatisé

- [ ] **Advanced Features**
  - Device trust scoring
  - Adaptive policies (risk-based)
  - Integration SIEM
  - Compliance reporting (SOC2, ISO 27001)

---

## Notes de Version

### Versioning
- **MAJOR** (X.0.0): Breaking changes
- **MINOR** (0.X.0): New features, backwards compatible
- **PATCH** (0.0.X): Bug fixes, security patches

### Tags Git
```bash
git tag -a v0.1.0 -m "Initial Release - Control Plane MVP"
git push origin v0.1.0
```

### Build Info
```bash
# Current version build
go build -ldflags "-X main.version=0.1.0 -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o ztna-cp main.go
```

---

## Contributeurs

- **hermas** - Développement initial et déploiement

## License

Voir le fichier [LICENSE](../LICENSE) à la racine du projet.
