# ZTNA Control Plane

Le **Control Plane (CP)** est le cerveau de l'architecture ZTNA. Il est responsable de :
- L'authentification des utilisateurs via OIDC (Keycloak)
- La délivrance des certificats SSH et Device (mTLS)
- L'évaluation des politiques d'accès (PEP — Policy Enforcement Point)
- L'audit de toutes les décisions d'accès

---

## Architecture interne

```
main.go
└── internal/
    ├── app/            # Bootstrap : wiring des dépendances (DI manuel)
    ├── config/         # Chargement et validation de config.yaml
    ├── api/
    │   ├── httpserver/ # Routeur HTTP (chi), enregistrement des routes
    │   ├── handlers/   # Handlers HTTP (PEP, credentials, admin, audit)
    │   └── middleware/ # Auth utilisateur, auth PEP, request-ID, erreurs
    ├── service/
    │   ├── decision/   # Orchestration : reçoit une demande → renvoie allow/deny
    │   ├── policy/     # CRUD des versions de politique + moteur d'évaluation
    │   ├── credentials/# Signe les certificats SSH et Device
    │   └── audit/      # Persistance des événements d'audit
    ├── domain/
    │   ├── model/      # Structures métier : Subject, Resource, Decision, Policy…
    │   ├── policy/     # Moteur d'évaluation pur (sans DB, testable)
    │   ├── errors/     # Erreurs métier typées (ErrUnauthorized, ErrNotFound…)
    │   └── port/       # Interfaces (Repository) — découplage infra/domaine
    ├── store/sqlite/   # Implémentation SQLite des interfaces Repository
    ├── crypto/
    │   ├── sshca/      # Signature de certificats SSH (OpenSSH format)
    │   └── deviceca/   # CA X.509 pour les certificats Device (mTLS)
    ├── security/oidc/  # Validation des tokens JWT (JWKS discovery)
    └── logger/         # Logger structuré JSON (slog)
```

---

## Endpoints HTTP

Le serveur écoute sur le port **8080** (HTTPS).

### Endpoints publics (authentification OIDC requise)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| `GET`   | `/healthz` | Santé du service (pas d'auth) |
| `GET`   | `/api/v1/whoami` | Infos du token OIDC courant |
| `POST`  | `/api/v1/credentials/ssh-cert` | Demander un certificat SSH signé |
| `POST`  | `/api/v1/credentials/device-cert` | Demander un certificat Device X.509 |

### Endpoints PEP (authentification PEP token requise)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| `POST`  | `/api/v1/pep/authorize` | Décision d'autorisation pour une ressource |
| `POST`  | `/api/v1/pep/heartbeat` | Heartbeat gateway → CP |

### Endpoints admin (rôle `ztna-admins` requis)

| Méthode | Chemin | Description |
|---------|--------|-------------|
| `GET`   | `/api/v1/admin/policies` | Lister les versions de politique |
| `POST`  | `/api/v1/admin/policies` | Créer une nouvelle version de politique |
| `PUT`   | `/api/v1/admin/policies/:id/activate` | Activer une version |
| `GET`   | `/api/v1/admin/audit` | Consulter les événements d'audit |
| `GET`   | `/api/v1/cp/ssh-ca-pubkey` | Clé publique de la CA SSH (pour `TrustedUserCAKeys`) |

---

## Format de la requête PEP `/api/v1/pep/authorize`

```json
{
  "subject": {
    "sub":      "1d584701-ee5e-4176-a0e2-3cf72e2b8b35",
    "username": "alice",
    "groups":   ["ztna-admins"]
  },
  "action": "connect",
  "resource": {
    "type": "ssh",
    "ssh":  { "host": "lan-app", "port": 22 }
  }
}
```

**Types de ressources supportés :** `ssh`, `http`

**Réponse :**
```json
{
  "effect":         "allow",
  "ttl_seconds":    60,
  "reason":         "rule:1",
  "policy_version": 1,
  "decision_id":    "dec-6f4e575d-..."
}
```

---

## Format des politiques (`policies.yaml` / API admin)

```yaml
created_by: seed
rules:
  # Autoriser le groupe ztna-admins à accéder à lan-app via SSH
  - effect:         allow
    subject_match:  group:ztna-admins
    action:         connect
    resource_type:  ssh
    resource_match: "ssh:lan-app:22"

  # Autoriser le groupe ztna-admins à accéder à lan-app via HTTP
  - effect:         allow
    subject_match:  group:ztna-admins
    action:         connect
    resource_type:  http
    resource_match: "http:lan-app:80"

  # Deny-all obligatoire en dernière position
  - effect:         deny
    subject_match:  "*"
    action:         "*"
    resource_type:  "*"
    resource_match: "*"
```

**Syntaxe `subject_match` :**
- `group:<nom>` — correspond si l'utilisateur appartient au groupe
- `user:<nom>` — correspond sur le `username` exact
- `sub:<uuid>` — correspond sur le `sub` (UUID OIDC)
- `*` — correspond toujours

**Syntaxe `resource_match` :**
- `ssh:lan-app:22` — exact
- `http:*` — glob : tout accès HTTP (préfixe)
- `*` — wildcard total

---

## Configuration (`config.yaml`)

```yaml
server:
  addr: "0.0.0.0:8080"      # Adresse d'écoute HTTPS
  tls:
    cert: "certs/server.crt"
    key:  "certs/server.key"

database:
  path: "control-plane.db"   # SQLite (chemin relatif au répertoire de travail)

oidc:
  issuer: "http://10.10.20.30:8081/realms/ztna"
  # L'issuer doit correspondre exactement au champ `iss` dans les tokens JWT.
  # En lab, utiliser l'IP de ztna-cp (pas 127.0.0.1).

sshca:
  private_key:  "ssh_ca/id_ztna_ca"
  allowed_principals:
    - "ztna"         # Utilisateur unix sur les VMs cibles
    - "${username}"  # Nom d'utilisateur OIDC (interpolé dynamiquement)

device_ca:
  cert: "pki/ca.crt"
  key:  "pki/ca.key"
  allowed_key_types: ["ecdsa-p256", "ed25519"]  # Types de clés acceptés dans le CSR
  cert_duration: "168h"                          # Durée de validité : 7 jours

pep:
  auth_mode: token   # Authentification des gateways via token statique
  tokens:
    ztna-gw-01: "ztna-lab-pep-secret-2026"

logging:
  level:  "info"
  format: "json"
```

---

## Démarrage

```bash
# Build
cd control-plane
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o cp-linux-amd64 .

# Lancer (en local, avec config de lab)
./cp-linux-amd64 -config config.lab.yaml
```

Le CP charge les politiques depuis `policies.yaml` au démarrage (seed) si aucune version n'existe en base.

---

## Tests unitaires

```bash
cd control-plane
go test ./...
```

**Couverture :**
- `internal/api/middleware` — authentification utilisateur et PEP
- `internal/config` — validation de la config
- `internal/crypto/deviceca` — émission des certificats device
- `internal/domain/policy` — moteur d'évaluation des politiques (25+ cas)

---

## Structure de la base de données (SQLite)

| Table | Description |
|-------|-------------|
| `policy_versions` | Versions de politique (`id`, `is_active`, `created_by`) |
| `policy_rules` | Règles associées à une version |
| `device_certs` | Certificats Device émis (historique) |
| `audit_events` | Événements d'audit des décisions PEP |
| `gateways` | Gateways enregistrées (heartbeat) |
| `users` | Utilisateurs et credentials locaux (usage futur) |

Les migrations sont versionnées dans `store/sqlite/migrations/`.
