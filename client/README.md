# Client ZTNA

Client CLI pour le système Zero Trust Network Access (Method 3 : mTLS tunnel vers Gateway).

## Architecture

```
┌─────────┐     OIDC      ┌───────────┐
│  Client  │──────────────▷│ Keycloak  │
│  (CLI)   │               └───────────┘
│          │   mTLS cert    ┌──────────────┐
│          │───────────────▷│Control Plane │
│          │  (stub/TODO)   │  :8080       │
│          │                └──────────────┘
│          │   mTLS tunnel  ┌──────────────┐    TCP     ┌──────────┐
│          │═══════════════▷│   Gateway    │───────────▷│ Resource │
└─────────┘                 │  :9443       │            └──────────┘
                            └──────────────┘
```

## Flux d'utilisation

### 1. Authentification OIDC

```bash
ztna -config config.lab.yaml login
```

Le client s'authentifie auprès de Keycloak et stocke les tokens localement.

- **Lab** : Resource Owner Password Grant (flux simplifié, non sécurisé)
- **Production** : Device Authorization Flow ou Authorization Code + PKCE

### 2. Demande de certificat mTLS

```bash
ztna -config config.lab.yaml cert
```

Le client génère une paire de clés ECDSA P-256 localement, construit un CSR
et l'envoie au Control Plane pour obtenir un certificat X.509 signé de courte durée.

> **⚠️ STUB** : L'endpoint CP `POST /api/v1/credentials/mtls-cert` n'existe pas encore.
> Cette commande retournera une erreur jusqu'à l'implémentation côté Control Plane.

### 3. Connexion à une ressource

```bash
ztna -config config.lab.yaml connect ssh-server-1
```

Le client établit un tunnel mTLS vers la Gateway avec le certificat obtenu,
envoie une requête CONNECT et, si autorisé, relaie le trafic.

## Structure du code

```
client/
├── cmd/ztna/main.go            # Point d'entrée CLI
├── config.lab.yaml             # Configuration de lab
├── go.mod                      # Module Go
├── internal/
│   ├── app/app.go              # Câblage applicatif (RunLogin, RunCert, RunConnect)
│   ├── config/
│   │   ├── config.go           # Chargement YAML + validation
│   │   └── config_test.go      # Tests de configuration
│   ├── credentials/
│   │   └── mtls_cert.go        # Demande de certificat mTLS au CP (STUB)
│   ├── domain/
│   │   ├── errors.go           # Erreurs sentinelles
│   │   └── model.go            # Modèles partagés
│   ├── logger/
│   │   └── logger.go           # Logger structuré slog
│   ├── oidc/
│   │   ├── oidc.go             # Client OIDC (TODO)
│   │   └── token_store.go      # Stockage sécurisé des tokens (TODO)
│   └── tunnel/
│       ├── protocol.go         # Structures CONNECT request/response
│       └── tunnel.go           # Gestionnaire de tunnel mTLS (TODO)
└── README.md
```

## État d'implémentation

| Composant | État | Notes |
|-----------|------|-------|
| CLI scaffolding | ✅ Squelette | Sous-commandes login, cert, connect |
| Config YAML | ✅ Fonctionnel | Load + Validate + tests |
| Logger | ✅ Fonctionnel | Même pattern que le Control Plane |
| OIDC client | 🔲 TODO | Flux Password Grant et Device Flow |
| Token storage | 🔲 TODO | Placeholder fichier, chiffrement à ajouter |
| mTLS cert request | 🔲 TODO | Endpoint CP inexistant |
| Tunnel mTLS | 🔲 TODO | TLS config prête, handshake à implémenter |
| Protocol (CONNECT) | 🔲 TODO | Structures définies, framing à implémenter |
| Relay trafic | 🔲 TODO | io.Copy bidirectionnel |

## Configuration

Voir [config.lab.yaml](config.lab.yaml) pour un exemple complet.

Champs requis :
- `oidc.issuer` : URL du provider OIDC
- `oidc.client_id` : identifiant du client OIDC
- `control_plane.base_url` : URL du Control Plane
- `gateway.address` : adresse `host:port` de la Gateway

## Sécurité

- Les clés privées ne quittent **jamais** le client
- TLS 1.3 minimum pour toutes les connexions
- Les tokens sont stockés localement (chiffrement à implémenter)
- Les certificats mTLS sont de courte durée de vie (15 min)
