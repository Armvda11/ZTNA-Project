# ZTNA Gateway

Le gateway (GW) est le Policy Enforcement Point (PEP) reseau.

Il fait 5 choses critiques:
1. Exposer un endpoint mTLS pour les clients ZTNA
2. Verifier le cert device client
3. Appeler le control-plane pour la decision (`authorize`)
4. Ouvrir le proxy TCP vers la ressource autorisee
5. Appliquer CRL + heartbeat + telemetrie de session

## 1) Comment il fonctionne (vue runtime)

Pour chaque connexion client:
1. Handshake TLS 1.3 avec cert client device
2. Lecture d'un `ConnectRequest` JSON
3. Resolution route locale (`resource_match -> target`)
4. Appel CP `POST /api/v1/pep/authorize`
5. Reponse `ConnectResponse` au client
6. Si allowed: tunnel TCP bidirectionnel

Boucles de fond:
- register + heartbeat vers CP
- refresh CRL + blocage certs revoques
- kill sessions actives revoquees
- cache de decisions (si active)

## 2) Protocole applicatif

### ConnectRequest (client -> GW)

```json
{
  "resource_type": "http",
  "resource_match": "http:lan-app:80",
  "action": "connect"
}
```

### ConnectResponse (GW -> client)

```json
{
  "allowed": true,
  "decision_id": "dec-...",
  "reason": "rule:2"
}
```

Types supportes:
- `http`
- `ssh`

## 3) Build et run

```bash
cd gateway
GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o ztna-gateway-linux-amd64 .
./ztna-gateway-linux-amd64 -config config.yaml
```

## 4) Config a connaitre

Champs cles dans `gateway/config.yaml`:
- `listen_addr`: socket mTLS client
- `gateway_id`: identite de ce GW
- `cp_url`: URL CP
- `cp_auth_mode`: `mtls` (cible) ou `token` (fallback lab)
- `cp_ca_cert`, `cp_client_cert`, `cp_client_key`: trust CP
- `cp_tls_insecure`: bypass TLS verification (lab uniquement)
- `strict_revocation`: fail-close CRL
- `decision_cache_ttl`, `decision_cache_max_entries`: cache authorize
- `cp_down_mode`: `deny` ou `cache_allow`
- `routes`: mapping des ressources vers targets LAN

## 5) Exemples concrets - scripts

Depuis la racine du projet:

```bash
make up
make deploy
make deploy-gw
make check
```

Tests qui prouvent le comportement gateway:

```bash
# Flux 2 nominal (mTLS + authorize + proxy)
make test-flux2

# Register + heartbeat strict
make test-pep-register

# CRL + kill sessions + routage
make test-crl-routing
```

Logs runtime:

```bash
make logs-gw
```

## 6) Exemples concrets - ztna-cli

```bash
cd /home/hermas/Documents/ZTNA-Project
make build-cli
export ZTNA=./ztna-cli/ztna-linux-amd64

$ZTNA init --profile lab
$ZTNA login --username alice --password 'Password123!'
$ZTNA enroll

# Test one-shot HTTP via gateway
$ZTNA connect http:lan-app:80 --http-probe --http-path /

# Test tunnel local
$ZTNA connect http:lan-app:80 --local-port 18080
curl -i http://127.0.0.1:18080/
```

## 7) Cas de test utiles pour le gateway

### Cas deny (policy/route)

```bash
$ZTNA connect http:unknown:80 --http-probe --http-path /
echo $?
```

Attendu:
- `allowed=false`
- code de sortie `6` (policy deny) ou `5` selon la cause

### Cas cert revoque

```bash
$ZTNA revoke-status
$ZTNA connect http:lan-app:80 --http-probe --http-path /
echo $?
```

Attendu:
- connexion refusee si cert present en CRL

## 8) Ou regarder dans le code

- `main.go`: bootstrap, register/heartbeat/CRL
- `internal/config/config.go`: config validation
- `internal/proxy/server.go`: listener mTLS, authorize, proxy, session tracking
- `internal/pep/client.go`: appels HTTP vers CP (authorize/heartbeat/register/sessions)
- `internal/crl/*`: cache CRL + refresh + kill revoked
- `internal/decisioncache/*`: cache decisions
- `internal/tlsutil/tlsutil.go`: cert pools, fingerprint, device CA fetch

## 9) Notes de securite

Pour un mode plus durci:
- `cp_auth_mode=mtls`
- `cp_tls_insecure=false`
- `strict_revocation=true`
- `cp_down_mode=deny`
