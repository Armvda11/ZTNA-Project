# Control Plane / Gateway - Fonctionnalites et Tests Lab

## Objectif

Ce document mappe les fonctionnalites reelles du control-plane (CP) et du gateway (GW)
aux tests lab executables sur ton infra Terraform (wan-client, ztna-gw, ztna-cp, lan-app, lan-admin).

## Prerequis lab

```bash
make up
make deploy
make deploy-gw
make check
```

## Matrice fonctionnalite -> preuve de fonctionnement

| Composant | Fonctionnalite | Endpoint / comportement | Test lab recommande | Preuve attendue |
|---|---|---|---|---|
| CP | Auth utilisateur OIDC (JWT RS256 + JWKS) | middleware `RequireUser` | `curl -sk https://10.10.20.30:8080/api/v1/whoami -H "Authorization: Bearer <token>"` | `200` + `sub/username/groups` |
| CP | Emission cert SSH | `POST /api/v1/credentials/ssh-cert` | `make test-flux1` (interactif) ou `make test-flux1-auto` | cert SSH emis puis login SSH vers `lan-app`/`lan-admin` |
| CP | Emission cert device X.509 | `POST /api/v1/credentials/device-cert` | `make test-flux2` | cert device emis + connexion mTLS autorisee |
| CP | Moteur policy allow/deny + default deny | `POST /api/v1/pep/authorize` | `make test-flux2` puis test ressource non routee/non autorisee | `effect=allow` ou `deny` + `reason` |
| CP | Auth PEP par token | `X-PEP-ID` + `X-PEP-TOKEN` | verification via logs GW + CP pendant `test-flux2` | appels PEP acceptes, pas de `401` |
| CP | Enregistrement PEP + heartbeat strict | `POST /api/v1/pep/register` + `POST /api/v1/pep/heartbeat` | `make test-pep-register` | statut `registered` obligatoire (sinon `403`) |
| CP | Audit des decisions | `GET /api/v1/admin/audit` | appel admin apres `test-flux1/2` | evenements `issue_*`, `policy_*`, decisions PEP |
| CP | Revocation cert device + CRL | `DELETE /api/v1/admin/device-certs/{serial}` + `/pki/device-ca/crl` | `make test-crl-routing` bloc A | serial present en CRL puis cert refuse |
| CP | Telemetrie session PEP | `POST /api/v1/pep/sessions/start/end` + `GET /api/v1/admin/sessions` | `make test-crl-routing` bloc B | session visible avec `decision_id`, `bytes_*`, `end_reason` |
| GW | Listener mTLS TLS1.3 (client cert obligatoire) | `:4433` + `RequireAndVerifyClientCert` | `make test-flux2` | handshake OK avec cert valide, refus sinon |
| GW | Resolution route + proxy TCP | `routes` dans `gateway/config.yaml` | `make test-flux2` | acces HTTP a `lan-app` via tunnel GW |
| GW | Appel PEP authorize | `POST /api/v1/pep/authorize` | `make test-flux2` | `decision_id` present dans reponse GW |
| GW | Heartbeat CP | `POST /api/v1/pep/heartbeat` periodique | `make logs-gw` | logs heartbeat (ou warning si CP indispo) |
| GW | Refresh CRL + kill sessions revoquees | refresh periodique + `KillRevoked` | `make test-crl-routing` bloc A4/A5 | nouvelles connexions refusees + session active coupee |
| Infra | Segmentation WAN/DMZ/LAN | iptables/routage GW | `make test-crl-routing` bloc C | WAN->LAN bloque, WAN->DMZ CP autorise |

## Campagne de test recommandee (ordre)

```bash
make test-flux1
make test-flux2
make test-crl-routing
make test-pep-register
```

Ou en une commande:

```bash
make test-cp-gw-lab
```

## Verifications complementaires (evidence exploitable demo)

```bash
# Sessions (admin)
curl -sk -H "Authorization: Bearer <token_admin>" \
  "https://10.10.20.30:8080/api/v1/admin/sessions?limit=20" | jq

# Audit
curl -sk -H "Authorization: Bearer <token_admin>" \
  "https://10.10.20.30:8080/api/v1/admin/audit?limit=20" | jq

# CRL
curl -sk "https://10.10.20.30:8080/pki/device-ca/crl" -o /tmp/current.crl
openssl crl -inform DER -text -noout -in /tmp/current.crl | sed -n '1,80p'
```

## Limites actuelles a connaitre

- Le mode PEP `mtls` existe cote CP, mais le client PEP du GW est actuellement en mode token.
- Les tests unitaires existent surtout sur config, policy engine, middleware PEP et proxy; la plupart des handlers/services restent couverts via tests lab E2E.
- Le script `test-crl-sessions-routing.sh` doit etre lance depuis `wan-client` (la cible Makefile `test-crl-routing` le fait deja).
