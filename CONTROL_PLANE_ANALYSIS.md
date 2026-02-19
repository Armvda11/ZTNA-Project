# Analyse du Control Plane ZTNA

## 📋 Vue d'ensemble

Le control plane ZTNA implémente un système Zero Trust complet avec authentification OIDC, autorisation basée sur des politiques, émission de certificats SSH et audit.

## 🏗️ Architecture

### Composants principaux

```
┌─────────────────────────────────────────────────────────────┐
│                     Control Plane                            │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │   API      │────│  Middleware  │────│  Handlers    │   │
│  │  Server    │    │  (Auth)      │    │  (Endpoints) │   │
│  └────────────┘    └──────────────┘    └──────────────┘   │
│        │                   │                    │           │
│        │                   │                    │           │
│  ┌────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │   Config   │────│   Services   │────│    Store     │   │
│  │  (YAML)    │    │  (Business)  │    │   (SQLite)   │   │
│  └────────────┘    └──────────────┘    └──────────────┘   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
         │                          │                   │
         ├──────────────────────────┼───────────────────┤
         │                          │                   │
    ┌─────────┐              ┌───────────┐      ┌───────────┐
    │Keycloak │              │    PEP    │      │SSH Client │
    │  OIDC   │              │ (Gateway) │      │  (User)   │
    └─────────┘              └───────────┘      └───────────┘
```

### Ports et endpoints

- **8080** : HTTPS public (certificats utilisateurs, authentification OIDC)
  - `/healthz` - Health check
  - `/api/v1/whoami` - Identité utilisateur
  - `/api/v1/credentials/ssh-cert` - Émission de certificats SSH
  - `/api/v1/admin/policies` - Gestion des politiques (admin)
  - `/api/v1/admin/audit` - Consultation des logs d'audit (admin)

- **8443** : HTTPS PEP (mTLS ou token-based)
  - `/api/v1/pep/authorize` - Décisions d'autorisation

## 🔐 Authentification et Autorisation

### 1. Authentification OIDC (Utilisateurs)

**Implémentation** : [internal/security/oidc/validator.go](control-plane/internal/security/oidc/validator.go)

- **Provider** : Keycloak (http://10.10.20.30:8081/realms/ztna)
- **Algorithme** : RS256 uniquement (validation stricte)
- **Validation** : 
  - Signature JWT via JWKS (go-jose/v4)
  - Claims : `sub`, `username`, `groups`, `aud`/`azp`
  - Cache JWKS : 1 heure (configurable)
  - Mode audience : `aud_or_azp` (support Keycloak)

**Middleware** : [internal/api/middleware/user_auth.go](control-plane/internal/api/middleware/user_auth.go)

```go
// Extraction du token Bearer
Authorization: Bearer <JWT_TOKEN>

// Claims extraits et injectés dans le contexte
type Subject {
    Sub      string   // UUID utilisateur
    Username string   // alice
    Groups   []string // ["ztna-admins"]
}
```

### 2. Authentification PEP (Gateways)

**Implémentation** : [internal/api/middleware/pep_auth.go](control-plane/internal/api/middleware/pep_auth.go)

**Mode 1 : Token-based** (actuellement utilisé)
```bash
X-PEP-ID: ztna-gw-1
X-PEP-TOKEN: CHANGE_ME_LONG_RANDOM
```

**Mode 2 : mTLS** (optionnel)
- Certificat client vérifié
- CN extrait du certificat → PEP ID

**Configuration** : [config.lab.yaml](control-plane/config.lab.yaml)
```yaml
pep:
  auth_mode: token
  tokens:
    ztna-gw-1: CHANGE_ME_LONG_RANDOM
```

## 📜 Moteur de Politiques

**Implémentation** : [internal/service/policy/service.go](control-plane/internal/service/policy/service.go)

### Structure d'une politique

```json
{
  "version_id": 2,
  "rules": [
    {
      "effect": "allow",
      "subject_match": "group:ztna-admins",
      "action": "connect",
      "resource_type": "ssh",
      "resource_match": "ssh:lan-app:22"
    },
    {
      "effect": "deny",
      "subject_match": "*",
      "action": "*",
      "resource_type": "*",
      "resource_match": "*"
    }
  ]
}
```

### Évaluation

1. **Matching de règles** : Les règles sont évaluées dans l'ordre
2. **Default deny** : Si aucune règle ne correspond, accès refusé
3. **Actions supportées** : `connect`, `*` (wildcard)
4. **Resource types** : `ssh`, `*`

**Endpoint d'autorisation** : [internal/api/handlers/pep_authorize.go](control-plane/internal/api/handlers/pep_authorize.go)

```bash
POST /api/v1/pep/authorize
{
  "subject": {
    "username": "alice",
    "groups": ["ztna-admins"]
  },
  "action": "connect",
  "resource": {
    "type": "ssh",
    "host": "lan-app",
    "port": 22
  },
  "context": {
    "src_ip": "1.2.3.4"
  }
}

# Response
{
  "decision": "allow",
  "reason": "rule:2",
  "ttl_seconds": 3600
}
```

## 🎫 SSH Certificate Authority

**Implémentation** : 
- [internal/crypto/sshca/sshca.go](control-plane/internal/crypto/sshca/sshca.go)
- [internal/service/credentials/service.go](control-plane/internal/service/credentials/service.go)

### Processus d'émission

1. **Utilisateur authentifié** (JWT OIDC valide)
2. **Génération clé SSH** : `ssh-keygen -t ed25519`
3. **Requête de certificat** :

```bash
POST /api/v1/credentials/ssh-cert
Authorization: Bearer <JWT_TOKEN>
{
  "public_key": "ssh-ed25519 AAAA...",
  "principals": ["${username}", "admin"],
  "ttl_seconds": 3600
}
```

4. **Validation** :
   - Vérification du format de la clé publique
   - Résolution des principals (`${username}` → `alice`, `${sub}` → UUID)
   - Validation du TTL (min: 60s, max: 86400s, default: 3600s)

5. **Signature par la CA** :
   - Type : `ED25519`
   - Fichier : `~/.ssh/ssh_ca` (généré au démarrage si absent)
   - KeyId : `sub` ou `username`

6. **Réponse** :

```json
{
  "certificate": "ssh-ed25519-cert-v01@openssh.com AAAA...",
  "valid_before": 1708185600,
  "key_id": "alice",
  "principals": ["alice", "admin"]
}
```

### Configuration CA

```yaml
sshca:
  key_path: "./ssh_ca"
  default_ttl: 3600
  min_ttl: 60
  max_ttl: 86400
  allowed_principals:
    - "${username}"
    - "${sub}"
    - "admin"
    - "guest"
```

## 📊 Audit et Traçabilité

**Implémentation** : 
- [internal/service/audit/service.go](control-plane/internal/service/audit/service.go)
- [internal/api/handlers/admin_audit.go](control-plane/internal/api/handlers/admin_audit.go)

### Événements audités

| Action | Contexte | Champs |
|--------|----------|--------|
| `issue_ssh_cert` | Émission certificat SSH | subject, principals, ttl, public_key_fingerprint |
| `connect` | Décision d'autorisation PEP | subject, resource, decision, reason, pep_id, src_ip |
| `create_policy` | Création politique (admin) | subject, version_id |
| `activate_policy` | Activation politique | subject, version_id |

### Extraction IP source

**Implémentation** : [internal/api/handlers/audit_helpers.go](control-plane/internal/api/handlers/audit_helpers.go)

Ordre de priorité :
1. `X-Forwarded-For` (premier IP)
2. `X-Real-IP`
3. `RemoteAddr` (connexion TCP directe)
4. `src_ip` du contexte PEP (envoyé par le gateway)

### Consultation

```bash
GET /api/v1/admin/audit?limit=50&action=connect
Authorization: Bearer <ADMIN_JWT>

# Response
[
  {
    "id": 42,
    "timestamp": "2026-02-17T10:30:00Z",
    "action": "connect",
    "subject": "alice|b013a054-95aa-4d6c-8429-c02366356b7c",
    "resource": "ssh:lan-app:22",
    "decision": "allow",
    "reason": "rule:2",
    "pep_id": "ztna-gw-1",
    "src_ip": "10.10.10.10",
    "metadata": {...}
  }
]
```

## 🔧 Configuration

### Structure du fichier [config.lab.yaml](control-plane/config.lab.yaml)

```yaml
server:
  public_address: "0.0.0.0:8080"
  pep_address: "0.0.0.0:8443"
  tls:
    cert: "./certs/server.crt"
    key: "./certs/server.key"
    ca: "./certs/ca.crt"

oidc:
  issuer: "http://10.10.20.30:8081/realms/ztna"
  audience: "ztna-control-plane"
  allowed_algs: ["RS256"]
  jwks_cache_ttl: "1h"
  audience_mode: "aud_or_azp"

pep:
  auth_mode: "token"
  mtls_enabled: false
  tokens:
    ztna-gw-1: "CHANGE_ME_LONG_RANDOM"

sshca:
  key_path: "./ssh_ca"
  default_ttl: 3600
  min_ttl: 60
  max_ttl: 86400
  allowed_principals:
    - "${username}"
    - "${sub}"
    - "admin"

database:
  driver: "sqlite"
  path: "./ztna.db"

logger:
  level: "info"
  format: "json"
```

## 🧪 Tests et Validation

### Tests unitaires

```bash
cd control-plane
go test ./internal/config -v
```

**Résultat** : 2/2 tests passants
- `TestLoadConfig`
- `TestValidateConfig`

### Tests d'intégration (Lab)

**Script** : [scripts/ztna-lab-test.sh](scripts/ztna-lab-test.sh)

**Couverture** :
1. ✅ Connectivité réseau (wan-client → ztna-cp)
2. ✅ Health check (`/healthz`)
3. ✅ Obtention token OIDC (Keycloak)
4. ✅ Endpoint whoami (`/api/v1/whoami`)
5. ✅ Vérification/création politique
6. ✅ Émission certificat SSH (`/api/v1/credentials/ssh-cert`)
7. ✅ Autorisation PEP (`/api/v1/pep/authorize`)
8. ✅ Logs d'audit (`/api/v1/admin/audit`)

### Exécution manuelle des endpoints

#### 1. Obtenir un token OIDC

```bash
TOKEN=$(curl -sS -X POST \
  http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=ztna-control-plane" \
  -d "client_secret=demo-secret" \
  -d "username=alice" \
  -d "password=Password123!" \
  -d "grant_type=password" | jq -r '.access_token')
```

#### 2. Tester whoami

```bash
curl -k -H "Authorization: Bearer ${TOKEN}" \
  https://10.10.20.30:8080/api/v1/whoami | jq
```

**Réponse attendue** :
```json
{
  "sub": "b013a054-95aa-4d6c-8429-c02366356b7c",
  "username": "alice",
  "groups": ["ztna-admins"]
}
```

#### 3. Demander un certificat SSH

```bash
# Générer une clé temporaire
ssh-keygen -t ed25519 -f /tmp/test_key -N "" -q

# Demander le certificat
curl -k -X POST https://10.10.20.30:8080/api/v1/credentials/ssh-cert \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"$(cat /tmp/test_key.pub)\"}" | jq

# Nettoyer
rm -f /tmp/test_key /tmp/test_key.pub
```

**Réponse attendue** :
```json
{
  "certificate": "ssh-ed25519-cert-v01@openssh.com AAAA...",
  "valid_before": 1708185600,
  "key_id": "alice",
  "principals": ["alice"]
}
```

#### 4. Tester l'autorisation PEP

```bash
curl -k -X POST https://10.10.20.30:8080/api/v1/pep/authorize \
  -H "X-PEP-ID: ztna-gw-1" \
  -H "X-PEP-TOKEN: CHANGE_ME_LONG_RANDOM" \
  -H "Content-Type: application/json" \
  -d '{
    "subject": {
      "username": "alice",
      "groups": ["ztna-admins"]
    },
    "action": "connect",
    "resource": {
      "type": "ssh",
      "host": "lan-app",
      "port": 22
    },
    "context": {
      "src_ip": "10.10.10.10"
    }
  }' | jq
```

**Réponse attendue** :
```json
{
  "decision": "allow",
  "reason": "rule:2",
  "ttl_seconds": 3600
}
```

#### 5. Consulter les logs d'audit

```bash
curl -k -H "Authorization: Bearer ${TOKEN}" \
  https://10.10.20.30:8080/api/v1/admin/audit | jq
```

## 🐛 Dépannage

### Control plane ne démarre pas

```bash
# Vérifier les logs
sudo journalctl -u ztna-cp -n 50 --no-pager

# Vérifier les ports
sudo netstat -tlnp | grep -E '8080|8443'

# Vérifier la configuration
cd /home/ztna/ztna/control-plane
./cp-linux-amd64 -config config.lab.yaml -validate
```

### Keycloak inaccessible

```bash
ssh ztna@10.10.20.30
cd ztna/control-plane/keycloak
docker-compose ps
docker-compose logs -f keycloak
```

### Politique retourne "deny"

```bash
# Vérifier la politique active
curl -k -H "Authorization: Bearer ${TOKEN}" \
  https://10.10.20.30:8080/api/v1/admin/policies/active | jq

# Vérifier que l'action est "connect" (pas "ssh")
# Vérifier que le group match est correct (group:ztna-admins)
```

### Certificat SSH invalide

```bash
# Vérifier la clé CA publique sur les serveurs cibles
cat /etc/ssh/sshca.pub

# Vérifier le certificat émis
ssh-keygen -L -f /tmp/cert-alice.pub

# Vérifier la configuration SSH du serveur
grep -A5 "TrustedUserCAKeys" /etc/ssh/sshd_config
```

## 📈 Statut Actuel

### ✅ Fonctionnalités Complètes

- Authentification OIDC (JWKS RS256)
- Autorisation PEP (token-based)
- Émission certificats SSH CA
- Moteur de politiques avec versioning
- Audit complet (8 types d'événements)
- Endpoint whoami
- Health checks
- Configuration YAML complète
- Déploiement lab automatisé

### ⚠️ Limitations Connues

- **Secrets** : Actuellement en clair dans les fichiers de config (prévoir intégration avec HashiCorp Vault)
- **High Availability** : Instance unique (pas de clustering)
- **Cache JWKS** : En mémoire uniquement (perdu au redémarrage)
- **Certificats SSH** : CA non renouvelable à chaud (nécessite redémarrage)

### 🔜 Améliorations Futures

1. **Sécurité**
   - Intégration coffre-fort de secrets (Vault/AWS Secrets Manager)
   - Rate limiting sur les endpoints publics
   - Rotation automatique des tokens PEP

2. **Résilience**
   - Clustering multi-instances
   - Health checks avancés (DB, Keycloak, latence)
   - Circuit breaker sur JWKS fetch

3. **Observabilité**
   - Métriques Prometheus (`/metrics`)
   - Traces distribuées (OpenTelemetry)
   - Dashboard Grafana

4. **Fonctionnalités**
   - Support OIDC dynamique (multi-tenants)
   - Certificats x509 pour mTLS client
   - Politique basée sur le temps (time-based rules)
   - Webhooks d'audit vers SIEM

## 📚 Références

- [FEATURES.md](control-plane/FEATURES.md) - Documentation technique détaillée
- [config.lab.yaml](control-plane/config.lab.yaml) - Configuration du lab
- [QUICKSTART.md](QUICKSTART.md) - Guide de démarrage rapide
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) - Guide de dépannage

---

**Dernière mise à jour** : 17 février 2026  
**Version du control plane** : 0.1.0 (dev)  
**Statut** : ✅ Opérationnel en lab
