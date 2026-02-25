# CLI ZTNA (`ztna`)

Client ZTNA minimal pour remplacer les scripts E2E tout en conservant le flux reel:

1. identite utilisateur OIDC
2. identite device X.509
3. canal mTLS vers la gateway
4. demande d'acces + tunnel/port-forward

## Build

```bash
make build-cli
```

Binaire genere: `ztna-cli/ztna-linux-amd64`

## Flux rapide lab

```bash
./ztna-cli/ztna-linux-amd64 init --profile lab
./ztna-cli/ztna-linux-amd64 login --username alice --password 'Password123!'
./ztna-cli/ztna-linux-amd64 enroll
./ztna-cli/ztna-linux-amd64 connect http:lan-app:80 --local-port 18080
curl http://127.0.0.1:18080/
```

## Commandes

### `ztna init`
Configure CP/GW/IdP et le repertoire d'etat local (`~/.ztna` par defaut).

Flags utiles:
- `--cp-url`
- `--gw-addr`
- `--idp-base --idp-realm --idp-client-id`
- `--token-renew-before` (ex: `2m`)
- `--cert-renew-before` (ex: `24h`)
- `--auto-rotate-cert`

### `ztna login`
Obtient un token OIDC (grant password pour le lab).

Flags utiles:
- `--username --password`
- `--password-stdin`

### `ztna enroll`
Genere/reutilise une cle device locale, cree un CSR, demande un cert au CP.

Flags utiles:
- `--ttl-seconds`
- `--force-new-key`
- `--groups` (override CSV)

### `ztna connect <resource>`
Etablit un canal mTLS vers la gateway, envoie `ConnectRequest`, applique la decision `ConnectResponse`, puis relaie le trafic.

Formats supportes:
- `http:<host>:<port>`
- `ssh:<host>:<port>`

Modes:
- `--local-port <N>`: port-forward local
- `--http-probe --http-path /`: test HTTP one-shot

### `ztna whoami`
Affiche l'identite resolue par le control-plane (`/api/v1/whoami`).

### `ztna status`
Affiche:
- expiration token/cert
- besoin de rotation
- reachability CP (`/healthz`) et GW (TCP)

### `ztna revoke-status`
Telecharge la CRL (`/pki/device-ca/crl`) et indique si le serial local est revoque.

## Rotation / expiration / erreurs / logs

- Rotation token: refresh automatique avant expiration (`token_renew_before`).
- Rotation cert device: renouvellement automatique avant expiration (`cert_renew_before`) si `auto_rotate_cert=true`.
- Codes d'erreur explicites (auth, revoke, expiry, policy deny, connect).
- Logs:
  - `--verbose` pour debug
  - config `logging.format=text|json`
  - config `logging.file` pour journaliser dans un fichier
