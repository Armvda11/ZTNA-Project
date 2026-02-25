# ZTNA Control Plane

Le control-plane (CP) est le coeur de decision de ton architecture ZTNA.

Il fait 4 choses critiques:
1. Authentifier les utilisateurs (OIDC/JWT)
2. Emettre les identites temporaires (cert SSH + cert device X.509)
3. Decider allow/deny pour les requetes gateway (`/api/v1/pep/authorize`)
4. Produire des preuves (audit, sessions, CRL)

## 1) Comment il fonctionne (vue runtime)

Flux principal:
1. Le client obtient un token OIDC (Keycloak)
2. Le client demande un cert SSH ou device au CP
3. La GW appelle le CP pour chaque acces (`pep/authorize`)
4. Le CP renvoie `effect=allow|deny` + `decision_id`
5. Le CP journalise audit + sessions et gere la revocation via CRL

## 2) Endpoints utiles

### Public
- `GET /healthz`
- `GET /pki/ssh-ca/pubkey`
- `GET /pki/device-ca/cert`
- `GET /pki/device-ca/crl`

### API user (OIDC requis)
- `GET /api/v1/whoami`
- `POST /api/v1/credentials/ssh-cert`
- `POST /api/v1/credentials/device-cert`

### API PEP (GW)
- `POST /api/v1/pep/register`
- `POST /api/v1/pep/heartbeat`
- `POST /api/v1/pep/authorize`
- `POST /api/v1/pep/sessions/start`
- `POST /api/v1/pep/sessions/end`

### API admin
- `POST /api/v1/admin/policies`
- `POST /api/v1/admin/policies/{id}/activate`
- `GET /api/v1/admin/policies/active`
- `GET /api/v1/admin/audit`
- `GET /api/v1/admin/sessions`
- `DELETE /api/v1/admin/device-certs/{serial}`

## 3) Build et run

```bash
cd control-plane
GOOS=linux GOARCH=amd64 go build -ldflags='-s -w' -o cp-linux-amd64 .
./cp-linux-amd64 -config config.lab.yaml
```

## 4) Exemples concrets - scripts (recommandes pour demo)

Depuis la racine du projet:

```bash
make up
make deploy
make deploy-gw
make check
```

Preuves fonctionnelles CP:

```bash
# Flux 1: OIDC + cert SSH + acces SSH
make test-flux1-auto

# Flux 2: OIDC + cert device + authorize + tunnel mTLS
make test-flux2

# Register/heartbeat strict GW -> CP
make test-pep-register

# Revocation CRL + sessions + routage
make test-crl-routing
```

Campagne complete:

```bash
make test-cp-gw-lab
```

## 5) Exemples concrets - ztna-cli

```bash
cd /home/hermas/Documents/ZTNA-Project
make build-cli
export ZTNA=./ztna-cli/ztna-linux-amd64

$ZTNA init --profile lab
$ZTNA login --username alice --password 'Password123!'
$ZTNA whoami
$ZTNA enroll
$ZTNA connect http:lan-app:80 --http-probe --http-path /
$ZTNA status
$ZTNA revoke-status
```

## 6) Exemple revocation admin reelle

```bash
export STATE=~/.ztna/state.json
export TOKEN=$(jq -r '.token.access_token' "$STATE")
export SERIAL=$(jq -r '.device.serial' "$STATE")

curl -sk -X DELETE \
  "https://10.10.20.30:8080/api/v1/admin/device-certs/${SERIAL}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'Content-Type: application/json' \
  -d '{"reason":"manual-test-cp"}'

$ZTNA revoke-status
$ZTNA connect http:lan-app:80 --http-probe --http-path /
```

Attendu:
- `revoke-status` indique `revoked: true`
- `connect` retourne un deny (cert revoque)

Retour etat sain:

```bash
$ZTNA enroll --force-new-key
```

## 7) Ou regarder dans le code

- `main.go`: bootstrap process
- `internal/app/app.go`: wiring dependances
- `internal/api/httpserver/server.go`: routes HTTP
- `internal/api/handlers/*`: endpoints user/admin/pep/pki
- `internal/service/*`: logique metier (decision, policy, credentials, sessions)
- `internal/store/sqlite/*`: persistance SQLite + migrations

## 8) Notes de securite importantes

- En mode lab, certaines options sont plus permissives (`allow_http_issuer`, token mode PEP, TLS insecure).
- Pour un profil durci, preferer:
  - OIDC HTTPS strict
  - PEP en mTLS
  - `cp_tls_insecure=false`
  - `strict_revocation=true`
