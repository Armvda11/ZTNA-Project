# ZTNA Gateway

Le **Gateway** est le **Policy Enforcement Point (PEP)** réseau du système ZTNA. Il se trouve en DMZ, expose un port mTLS aux clients WAN, et proxifie le trafic vers les ressources internes uniquement après autorisation du Control Plane.

---

## Architecture interne

```
main.go
└── internal/
    ├── config/     # Chargement et validation de config.yaml
    ├── pep/        # Client HTTP vers le CP PEP (/api/v1/pep/authorize)
    ├── proxy/      # Listener mTLS + protocole ConnectRequest/Response + proxy TCP
    └── tlsutil/    # Utilitaires TLS : chargement CA, génération cert éphémère
```

---

## Protocole applicatif (couche au-dessus du mTLS)

La connexion suit ce séquencement **après** le handshake TLS :

```
Client                              Gateway                    Control Plane
  │                                    │                            │
  │── JSON newline ──────────────────► │                            │
  │   {"resource_type":"http",         │                            │
  │    "resource_match":"http:lan-app:80",                          │
  │    "action":"connect"}             │                            │
  │                                    │── POST /pep/authorize ───► │
  │                                    │◄── {effect:"allow",...} ── │
  │◄── JSON newline ─────────────────- │                            │
  │   {"allowed":true,                 │                            │
  │    "decision_id":"dec-..."}        │                            │
  │                                    │                            │
  │═══════ Proxy TCP brut ════════════►│════ TCP → lan-app:80 ═════►│
```

**Étapes :**
1. Le client ouvre une connexion TLS en présentant son certificat Device (signé par la CA du CP).
2. Le gateway vérifie le certificat client (mTLS : `RequireAndVerifyClientCert`).
3. Le client envoie un `ConnectRequest` JSON terminé par `\n`.
4. Le gateway extrait le sujet ZTNA depuis le certificat X.509 (`CN` = username, `O` = groups, `serialNumber` = sub).
5. Le gateway appelle le CP PEP pour obtenir une décision.
6. Le gateway répond avec `ConnectResponse` JSON terminé par `\n`.
7. Si `allowed=true` : tunnel TCP bidirectionnel vers la ressource cible.

---

## Format des messages

### ConnectRequest (client → gateway)
```json
{
  "resource_type":  "http",
  "resource_match": "http:lan-app:80",
  "action":         "connect"
}
```

**Types supportés :** `ssh`, `http`

**Format `resource_match` :** `<type>:<hostname>:<port>`

### ConnectResponse (gateway → client)
```json
{
  "allowed":     true,
  "decision_id": "dec-6f4e575d-5fdc-4be6-8ec9-118fd248ecf8",
  "reason":      "rule:2"
}
```

---

## Configuration (`config.yaml`)

```yaml
listen_addr: "0.0.0.0:4433"   # Port mTLS exposé aux clients WAN

# Identité de ce gateway auprès du Control Plane
gateway_id:  "ztna-gw-01"
pep_id:      "ztna-gw-01"
pep_token:   "ztna-lab-pep-secret-2026"  # Doit correspondre à config.lab.yaml du CP

# URL du Control Plane
cp_url:          "https://10.10.20.30:8080"
cp_tls_insecure: true   # En lab, le cert CP est self-signed → désactiver la vérification

# TLS du gateway (optionnel — auto-généré si absent)
# tls:
#   server_cert:    "certs/gw.crt"
#   server_key:     "certs/gw.key"
#   device_ca_cert: "pki/ca.crt"    # Si absent, récupéré automatiquement depuis le CP
```

---

## Résolution des routes

Le gateway résout `resource_match` en adresse TCP via la table de routes définie dans la config :

```yaml
routes:
  - match: "ssh:lan-app:22"
    target: "10.10.30.10:22"
  - match: "http:lan-app:80"
    target: "10.10.30.10:80"
  - match: "ssh:lan-admin:22"
    target: "10.10.30.11:22"
```

Si aucune route ne correspond, la connexion est refusée (pas d'accès direct par IP).

---

## Démarrage

```bash
# Build
cd gateway
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o ztna-gateway-linux-amd64 .

# Lancer
./ztna-gateway-linux-amd64 -config config.yaml
```

Au démarrage, si aucun cert Device CA n'est configuré localement, le gateway récupère automatiquement la CA depuis le CP (`GET /api/v1/cp/device-ca-cert`). Il réessaie 10 fois (backoff progressif) pour gérer le démarrage séquentiel CP → Gateway.

---

## Tests unitaires

```bash
cd gateway
go test ./...
```

**Couverture :**
- `internal/proxy/server_test.go` — `buildAuthorizeRequest` pour SSH et HTTP
- `internal/config/config_test.go` — validation de la configuration

---

## Exemple de client Python (test mTLS)

```python
import ssl, socket, json

# Contexte mTLS : présenter le certificat device
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode    = ssl.CERT_NONE
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain("~/.ztna/device_alice.crt", "~/.ztna/device_alice.key")

raw = socket.create_connection(("10.10.10.20", 4433), timeout=15)
tls = ctx.wrap_socket(raw)

# 1. Envoyer ConnectRequest
tls.sendall(json.dumps({
    "resource_type":  "http",
    "resource_match": "http:lan-app:80",
    "action":         "connect"
}).encode() + b"\n")

# 2. Lire ConnectResponse
resp = json.loads(tls.recv(4096).rstrip(b"\n"))
if resp["allowed"]:
    # 3. Tunnel TCP ouvert → envoyer HTTP directement
    tls.sendall(b"GET / HTTP/1.0\r\nHost: lan-app\r\n\r\n")
    print(tls.recv(4096).decode())
```

Voir `scripts/test-mtls-access.sh` pour le test complet avec obtention du token OIDC et du cert device.

---

## Logs structurés

Le gateway produit des logs JSON sur stdout :

```json
{"time":"...","level":"INFO","msg":"ztna gateway listening","addr":"0.0.0.0:4433"}
{"time":"...","level":"INFO","msg":"client connected","remote":"10.10.10.1:44166","username":"alice","sub":"1d584701-..."}
{"time":"...","level":"INFO","msg":"access allowed","username":"alice","target":"10.10.30.10:80","decision_id":"dec-..."}
{"time":"...","level":"ERROR","msg":"dial backend","target":"10.10.30.10:80","err":"connection refused"}
```
