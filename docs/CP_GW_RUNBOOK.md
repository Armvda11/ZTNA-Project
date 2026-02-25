# Runbook Control-Plane / Gateway

Ce document est le runbook operationnel pour demarrer, diagnostiquer et comprendre les flux CP/GW.

## 1. Ordre de boot recommande

1. Infra Terraform: `make up`
2. Control-plane + Keycloak: `make deploy`
3. Gateway: `make deploy-gw`
4. Verification globale: `make check`

Raison: le gateway depend du control-plane pour l'autorisation PEP et la recuperation de la Device CA.

## 2. Flux runtime (vue rapide)

### 2.1 Auth utilisateur (OIDC)

1. Le client obtient un token depuis Keycloak.
2. Le token est presente au control-plane pour les endpoints utilisateur/admin.

### 2.2 Autorisation PEP (gateway -> control-plane)

1. Le client ouvre une connexion mTLS vers le gateway (`:4433`) avec un cert device.
2. Le gateway extrait le sujet depuis le cert (CN, O, SerialNumber).
3. Le gateway appelle `POST /api/v1/pep/authorize` sur le control-plane.
4. Le CP renvoie `allow` ou `deny`.
5. Si `allow`, le gateway ouvre le proxy TCP vers la cible routee.

### 2.3 Session telemetry

1. Au debut de session: `POST /api/v1/pep/sessions/start`.
2. En fin de session: `POST /api/v1/pep/sessions/end` avec bytes/duree/raison.

### 2.4 CRL refresh

1. Le gateway recupere periodiquement la CRL CP.
2. Si un cert est revoque, les nouvelles connexions sont refusees.
3. Les sessions actives du cert revoque peuvent etre fermees.

## 3. Endpoints reels a connaitre

## Control-plane public

- `GET /healthz`
- `GET /pki/ssh-ca/pubkey`
- `GET /pki/device-ca/cert`
- `GET /pki/device-ca/crl`

## Control-plane API utilisateur/admin

- `POST /api/v1/credentials/ssh-cert`
- `POST /api/v1/credentials/device-cert`
- `POST /api/v1/admin/policies`
- `POST /api/v1/admin/policies/{id}/activate`
- `GET /api/v1/admin/policies/active`
- `GET /api/v1/admin/audit`
- `GET /api/v1/admin/sessions`
- `DELETE /api/v1/admin/device-certs/{serial}`

## PEP API

- `POST /api/v1/pep/register`
- `POST /api/v1/pep/authorize`
- `POST /api/v1/pep/heartbeat` (statut strict: `registered|unregistered|revoked`)
- `POST /api/v1/pep/sessions/start`
- `POST /api/v1/pep/sessions/end`

## 4. Commandes diagnostic

```bash
# Sante CP
curl -sk https://10.10.20.30:8080/healthz

# PKI CP
curl -sk https://10.10.20.30:8080/pki/ssh-ca/pubkey
curl -sk https://10.10.20.30:8080/pki/device-ca/cert
curl -sk https://10.10.20.30:8080/pki/device-ca/crl -o /tmp/current.crl

# Services
make logs-cp
make logs-gw

# Etat rapide
make check
make check-ssh
```

## 5. Playbook incidents

## Incident A: CP indisponible

Symptomes:
- `make healthz` en echec sur CP
- erreurs authorize/heartbeat cote gateway

Actions:
1. `make logs-cp`
2. verifier service sur VM CP: `sudo systemctl status ztna-cp`
3. verifier Keycloak: `curl -sf http://10.10.20.30:8081/realms/ztna`
4. redeployer: `make deploy`

## Incident B: Gateway refuse tout

Symptomes:
- connexions mTLS deny
- tests flux2/crl en echec

Actions:
1. `make logs-gw`
2. verifier token PEP (`gateway/config.yaml` vs `control-plane/config.lab.yaml`)
3. verifier routes gateway (`gateway/config.yaml`)
4. verifier policies actives CP

## Incident C: CRL ne bloque pas

Symptomes:
- cert revoque toujours accepte

Actions:
1. verifier serial dans CRL CP (`/pki/device-ca/crl`)
2. verifier logs refresh CRL gateway
3. attendre intervalle refresh configure
4. relancer gateway si besoin (`make deploy-gw`)

## Incident D: Route KO vers LAN

Symptomes:
- authorize=allow mais backend non joignable

Actions:
1. verifier `routes` dans `gateway/config.yaml`
2. verifier reachability depuis `ztna-gw` vers cibles LAN
3. verifier services sur `lan-app` / `lan-admin`

## 6. Ou regarder dans le code

## Control-plane

- `control-plane/main.go`: bootstrap process + signal handling
- `control-plane/internal/app/app.go`: wiring des services/repos/middlewares
- `control-plane/internal/api/httpserver/server.go`: declaration des routes HTTP
- `control-plane/internal/service/*`: logique metier (policy, decision, credentials, sessions)
- `control-plane/internal/store/sqlite/*`: persistance SQLite + migrations

## Gateway

- `gateway/main.go`: bootstrap TLS/PEP/CRL + boucle heartbeat
- `gateway/internal/proxy/server.go`: listener mTLS + authorize + proxy TCP
- `gateway/internal/pep/client.go`: client HTTP vers endpoints PEP du CP
- `gateway/internal/crl/*`: cache CRL et refresh periodique
- `gateway/internal/config/config.go`: validation config gateway
