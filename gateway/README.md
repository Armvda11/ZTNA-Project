# Gateway ZTNA

Point d'application des politiques (PEP) du système Zero Trust Network Access.
La Gateway reçoit les connexions mTLS des clients, vérifie leur identité localement
et demande une décision d'autorisation au Control Plane avant de proxier le trafic.

## Architecture

```
                            ┌──────────────────────┐
                            │    Control Plane      │
                            │    :8443 (PEP)        │
                            │                       │
                            │  POST /api/v1/pep/    │
                            │       authorize       │
                            └──────────┬───────────┘
                                       │ authz
                                       │ (X-PEP-ID/TOKEN)
┌──────────┐   mTLS     ┌─────────────┴───────────┐    TCP     ┌───────────┐
│  Client   │═══════════▷│      Gateway             │──────────▷│ Resource  │
│  ZTNA     │  :9443     │                          │           │ (SSH,     │
│           │◁═══════════│  1. Authn local (cert)   │◁──────────│  HTTP...) │
└──────────┘   tunnel    │  2. Authz via CP         │           └───────────┘
                         │  3. Proxy TCP            │
                         └──────────────────────────┘
```

## Flux de traitement

### 1. Authentification locale (mTLS)

La Gateway configure un listener TLS avec `ClientAuth: RequireAndVerifyClientCert`.
Le certificat client est vérifié localement grâce à la CA client (`client_ca_file`)
qui correspond à la CA utilisée par le Control Plane pour émettre les certificats.

L'identité du sujet est extraite du certificat :
- **Priorité 1** : SAN URI avec schéma `oidc:` (identifiant OIDC)
- **Priorité 2** : SAN DNS
- **Priorité 3** : Subject CN (Common Name)

### 2. Autorisation via Control Plane

Après extraction de l'identité, la Gateway :
1. Lit la requête CONNECT du client (action, resource, context)
2. Appelle `POST /api/v1/pep/authorize` sur le CP avec les headers PEP
3. Reçoit une décision `allow` ou `deny` avec un TTL et un ID de décision

### 3. Proxy TCP

Si la décision est `allow` :
- La Gateway établit une connexion TCP vers la ressource cible
- Le trafic est relayé bidirectionnellement entre le client et la ressource
- La session est limitée au TTL retourné par le CP

Si la décision est `deny` :
- La Gateway retourne une réponse d'erreur au client
- La connexion est fermée
- L'événement est journalisé

## Sécurité

### Anti-pivoting
- **1 connexion = 1 ressource** : chaque connexion mTLS ne donne accès qu'à
  une seule ressource (host:port) autorisée par le CP
- Pas de forwarding arbitraire ni de changement de cible en cours de session
- La validation de la cible est faite AVANT l'établissement de la connexion proxy

### Journalisation
- Chaque décision (allow/deny) est journalisée avec l'identité du sujet
- Les sessions sont tracées de bout en bout (ouverture, durée, fermeture)
- Les erreurs de handshake TLS sont journalisées (tentatives non autorisées)

### TLS vers les ressources LAN (évolution future)
- Actuellement : connexion TCP en clair vers les ressources internes
- Futur : support mTLS/TLS optionnel vers les ressources backend
- Le chiffrement de bout en bout (client → ressource) est assuré par le tunnel mTLS

## Structure du code

```
gateway/
├── cmd/ztna-gateway/main.go       # Point d'entrée (config, logger, signal)
├── config.lab.yaml                # Configuration de lab
├── go.mod                         # Module Go
├── internal/
│   ├── bootstrap/app.go           # Orchestration applicative
│   ├── config/
│   │   ├── config.go              # Chargement YAML + validation
│   │   └── config_test.go         # Tests
│   ├── core/
│   │   ├── domain/
│   │   │   ├── errors.go          # Erreurs sentinelles
│   │   │   └── model.go           # SubjectRef, ResourceRef
│   │   └── ports/
│   │       └── ports.go           # Interfaces métier (évolution)
│   ├── infra/
│   │   ├── authz/
│   │   │   ├── client.go          # Client HTTP vers CP /pep/authorize
│   │   │   └── types.go           # Types décision (allow/deny)
│   │   ├── mtls/
│   │   │   ├── identity.go        # Extraction d'identité depuis cert X.509
│   │   │   └── listener.go        # Listener TLS avec mTLS
│   │   ├── proxy/
│   │   │   ├── http.go            # Proxy HTTP L7 (stub futur)
│   │   │   └── tcp.go             # Proxy TCP L4
│   │   ├── session/
│   │   │   └── manager.go         # Gestionnaire de sessions actives
│   │   ├── tls/
│   │   │   └── tlsutil.go         # Helpers TLS/HTTPS partagés (CP + mTLS)
│   │   ├── revocation/
│   │   │   └── crl.go             # Store CRL local (refresh TODO)
│   │   └── cache/
│   │       └── cache.go           # Cache local de décisions (intégration progressive)
│   ├── observability/
│   │   └── logger/
│   │       └── logger.go          # Logger structuré slog
│   └── usecase/
│       ├── connect/
│       │   ├── connect.go         # Structures CONNECT request/response
│       │   └── handler.go         # Handler de connexion (CONNECT → authz → proxy)
│       └── lifecycle/
│           └── lifecycle.go       # Hooks cycle de vie (TODO)
└── README.md
```

Ces packages sont branchés de façon progressive :
- `infra/mtls/listener` utilise `infra/tls` pour centraliser le chargement TLS
- `infra/authz/client` prépare le client HTTPS CP via `infra/tls`
- `bootstrap/app` initialise `infra/revocation` et `infra/cache` (activation métier complète en TODO)

> Version active unique: `cmd/ztna-gateway/main.go` + couche `internal/bootstrap/*`.

## État d'implémentation

| Composant | État | Notes |
|-----------|------|-------|
| CLI / main.go | ✅ Squelette | Config, logger, signal handling |
| Config YAML | ✅ Fonctionnel | Load + Validate + tests |
| Logger | ✅ Fonctionnel | Même pattern que le Control Plane |
| mTLS Listener | 🔲 TODO | TLS config prête, accept loop à implémenter |
| Identity extraction | ✅ Squelette | SAN URI/DNS/CN avec priorité |
| Protocol (CONNECT) | 🔲 TODO | Structures définies, framing à implémenter |
| Connection handler | 🔲 TODO | Flux décrit, implémentation à faire |
| Authorize client | 🔲 TODO | Types définis, appel HTTP à implémenter |
| TCP Proxy | 🔲 TODO | io.Copy bidirectionnel à implémenter |
| HTTP Proxy (L7) | 🔲 Stub | Placeholder pour évolution future |
| Session manager | ✅ Squelette | Register/Unregister, compteurs |

## Configuration

Voir [config.lab.yaml](config.lab.yaml) pour un exemple complet.

Champs requis :
- `server.tls.cert_file` / `key_file` / `client_ca_file` : certificats TLS
- `control_plane.base_url` : URL du Control Plane (endpoint PEP)
- `pep.id` / `pep.token` : identifiants pour s'authentifier auprès du CP

## Build et exécution

```bash
cd gateway
GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o ztna-gateway-linux-amd64 ./cmd/ztna-gateway
./ztna-gateway-linux-amd64 -config config.lab.yaml
```

## Tests utiles (lab)

```bash
make test-flux2
make test-pep-register
make test-crl-routing
make logs-gw
```
