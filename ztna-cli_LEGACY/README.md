# ztna-cli

CLI client ZTNA pour piloter le flux utilisateur/device/gateway sans Terraform.

Le binaire couvre les responsabilites client suivantes:
- authentification utilisateur via OIDC (`login`)
- enrolement identite device X.509 via control-plane (`enroll`)
- connexion mTLS vers gateway + demande d'acces (`connect`)
- verification operationnelle (expiration, revocation, etat, identite)

## 1) Ce que fait le client

Le client automatise le chemin complet:
1. `login`: recupere un access token (et refresh token si fourni par l'IdP)
2. `enroll`: genere CSR local + obtient un certificat device signe par le CP
3. `connect <resource>`: ouvre TLS client-auth vers la GW, envoie `ConnectRequest`, lit `ConnectResponse`, puis relaie le trafic
4. `status` et `revoke-status`: controle l'etat reel du client (token/cert/CRL)

## 2) Build

Depuis la racine du projet:

```bash
make build-cli
```

Binaire:
- `ztna-cli/ztna-linux-amd64`

## 3) Quick start (lab)

```bash
cd /home/hermas/Documents/ZTNA-Project
export ZTNA=./ztna-cli/ztna-linux-amd64

$ZTNA init --profile lab
$ZTNA login --username alice --password 'Password123!'
$ZTNA enroll
$ZTNA connect http:lan-app:80 --http-probe --http-path /
```

## 4) Commandes

### `ztna init`
Initialise la config client locale (`~/.ztna/config.json` par defaut).

Options utiles:
- `--profile lab|prod`
- `--config <path>`
- `--state-dir <path>`
- `--cp-url`, `--gw-addr`, `--idp-base`, `--idp-realm`, `--idp-client-id`
- `--cp-insecure`, `--gw-insecure`, `--idp-insecure` (lab)
- `--token-renew-before` (ex: `2m`)
- `--cert-renew-before` (ex: `24h`)
- `--auto-rotate-cert`

### `ztna login`
Recupere un token OIDC (grant password dans le lab actuel).

Options utiles:
- `--username`
- `--password`
- `--password-stdin`
- `--config`

### `ztna enroll`
Genere/reutilise une cle device locale, cree un CSR, demande un cert device.

Options utiles:
- `--ttl-seconds`
- `--force-new-key`
- `--groups` (CSV)
- `--config`

### `ztna connect <resource>`
Connexion mTLS vers la GW + authorization + tunnel de trafic.

Formats de ressource:
- `http:<host>:<port>`
- `ssh:<host>:<port>`

Modes:
- `--http-probe --http-path /` (one-shot HTTP)
- `--local-port <N>` (port-forward local)
- `--listen-host <ip>` (par defaut `127.0.0.1`)
- `--action <verb>` (par defaut `connect`)
- `--no-auto-rotate` (desactive auto-renouvellement cert pour cette commande)

Exemple:

```bash
ztna connect http:lan-app:80 --local-port 18080
curl http://127.0.0.1:18080/
```

### `ztna whoami`
Affiche l'identite retournee par `GET /api/v1/whoami` du control-plane.

### `ztna status`
Affiche:
- etat token (presence, expiration, besoin de rotation)
- etat cert device (presence, expiration, besoin de rotation)
- reachability CP (`/healthz`) et GW (TCP)

`--json` disponible pour sortie machine-readable.

### `ztna revoke-status`
Telecharge la CRL (`/pki/device-ca/crl`) et indique si le serial local est revoque.

## 5) Fichiers locaux geres par le CLI

Par defaut sous `~/.ztna/`:
- `config.json`: configuration CP/GW/IdP + runtime
- `state.json`: token + metadonnees device
- `device.key`: cle privee device
- `device.crt`: certificat device
- `device-ca.crt`: CA device (retour CP)

## 6) Rotation, expiration, erreurs, logs

### Rotation / expiration
- Token: refresh automatique si proche expiration (`runtime.token_renew_before`)
- Certificat device: renouvellement automatique si proche expiration (`runtime.cert_renew_before`) quand `runtime.auto_rotate_cert=true`
- Si refresh impossible et token expire: erreur explicite demandant `ztna login`
- Si cert absent/expire et auto-rotate desactive: erreur explicite demandant `ztna enroll`

### Erreurs (codes de sortie)
- `1`: generic
- `2`: config
- `3`: auth
- `4`: enroll
- `5`: connect
- `6`: policy denied
- `7`: revoked
- `8`: expired
- `9`: network

### Logs
- `--verbose` active le niveau debug
- `logging.format`: `text` ou `json`
- `logging.file`: ecrit en fichier en plus de stderr

## 7) Scenarios concrets de test manuel

### A. Flux 2 nominal

```bash
export ZTNA=./ztna-cli/ztna-linux-amd64
$ZTNA init --profile lab
$ZTNA login --username alice --password 'Password123!'
$ZTNA enroll
$ZTNA revoke-status
$ZTNA connect http:lan-app:80 --http-probe --http-path /
```

### B. Flux 2 avec tunnel local (cas applicatif)

```bash
$ZTNA connect http:lan-app:80 --local-port 18080
```

Dans un autre terminal:

```bash
curl -i http://127.0.0.1:18080/
```

### C. Cas deny (ressource invalide/non routee)

```bash
$ZTNA connect http:unknown:80 --http-probe --http-path /
echo $?
```

### D. Cas revocation reelle

```bash
export STATE=~/.ztna/state.json
export TOKEN=$(jq -r '.token.access_token' "$STATE")
export SERIAL=$(jq -r '.device.serial' "$STATE")

curl -sk -X DELETE \
  "https://10.10.20.30:8080/api/v1/admin/device-certs/${SERIAL}" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"reason":"manual-test-cli"}'

$ZTNA revoke-status
$ZTNA connect http:lan-app:80 --http-probe --http-path /
echo $?
```

Retour etat sain:

```bash
$ZTNA enroll --force-new-key
```

## 8) Limites actuelles

- `login` est base sur grant password (coherent avec le lab Keycloak actuel)
- pas encore de device posture check (OS posture, EDR, etc.)
- pas encore de plugin MFA cote client (depend de la politique IdP)

Ces limites n'empechent pas un flux ZTNA complet dans ton lab VM actuel.
