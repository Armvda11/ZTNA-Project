# Test manuel — Flux 1 : SSH via certificat signé par le Control Plane

Ce document décrit, pas à pas, comment reproduire manuellement ce que fait
`scripts/test-ssh-cert-access.sh` : obtenir un token OIDC, demander un
certificat SSH au Control Plane, puis se connecter via jump host `ztna-gw`.

Prérequis
- Accès réseau aux VMs du lab (WAN + jump)
- Clé SSH locale (nous utilisons un couple dédié dans `~/.ztna`)
- `curl`, `ssh`, `ssh-keygen`, `python3` installés

Adresses (lab par défaut)
- wan-client : 10.10.10.10
- ztna-gw    : 10.10.10.20  (jump host)
- ztna-cp    : 10.10.20.30  (control-plane + Keycloak)
- lan-app    : 10.10.30.10
- lan-admin  : 10.10.30.11

Variables utilitaires

```bash
ZTNA_DIR="$HOME/.ztna"
CP_URL="https://10.10.20.30:8080"
KC_URL="http://10.10.20.30:8081"
KC_REALM="ztna"
KC_CLIENT="ztna-control-plane"
GW_HOST="10.10.10.20"
TARGET_IP="10.10.30.10"   # lan-app (ou 10.10.30.11 pour lan-admin)
USER=alice
PASS='Password123!'
```

Étapes manuelles (copier/collez) — depuis votre poste (wan-client)

1) Créer le dossier de travail

```bash
mkdir -p "$ZTNA_DIR" && chmod 700 "$ZTNA_DIR"
```

2) Obtenir un token OIDC (Keycloak)

```bash
TOKEN_RESP=$(curl -sk \
  -d "client_id=${KC_CLIENT}&username=${USER}&password=${PASS}&grant_type=password" \
  "${KC_URL}/realms/${KC_REALM}/protocol/openid-connect/token")

ACCESS_TOKEN=$(echo "$TOKEN_RESP" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('access_token',''))")

if [ -z "$ACCESS_TOKEN" ]; then
  echo "Échec obtention token ; réponse Keycloak:" >&2
  echo "$TOKEN_RESP" >&2
  exit 1
fi

echo "Token OIDC obtenu — ${#ACCESS_TOKEN} caractères"
```

3) Générer une clé SSH (si nécessaire)

```bash
KEY_FILE="$ZTNA_DIR/id_ztna_${USER}"
if [ ! -f "$KEY_FILE" ]; then
  ssh-keygen -t ed25519 -f "$KEY_FILE" -N "" -C "ztna-${USER}"
fi
PUB_KEY=$(cat "${KEY_FILE}.pub")
echo "Clé publique : ${KEY_FILE}.pub"
```

4) Demander un certificat SSH au Control Plane

```bash
CERT_RESP=$(curl -sk \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"${PUB_KEY}\", \"principals\": [\"ztna\", \"${USER}\"]}" \
  "${CP_URL}/api/v1/credentials/ssh-cert")

CERT=$(echo "$CERT_RESP" | python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('certificate',''))")
if [ -z "$CERT" ]; then
  echo "Échec obtention certificat ; réponse CP:" >&2
  echo "$CERT_RESP" >&2
  exit 1
fi

CERT_FILE="${KEY_FILE}-cert.pub"
echo "$CERT" > "$CERT_FILE"
chmod 600 "$CERT_FILE"
echo "Certificat SSH écrit → $CERT_FILE"

# Afficher les informations du certificat
ssh-keygen -L -f "$CERT_FILE" | sed 's/^/  /'
```

5) Connexion SSH via jump host `ztna-gw`

```bash
ssh -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -i "${KEY_FILE}" \
    -i "${CERT_FILE}" \
    -J "ztna@${GW_HOST}" \
    "ztna@${TARGET_IP}"
```

Sur succès vous ouvrez une session sur la VM cible (lan-app ou lan-admin)

Résultats attendus
- Le token OIDC doit être non vide (Keycloak valide)
- `ssh-keygen -L -f <cert>` montre les principals `ztna` et `alice`
- La connexion SSH réussit et vous obtenez un shell `ztna@lan-app`

Échecs fréquents et solutions
- « Impossible d'obtenir le token OIDC » : vérifier `KC_URL`, `USER`/`PASS`, et que Keycloak est up.
- « Certificat SSH non obtenu » : vérifier que le CP est démarré (`${CP_URL}/healthz`) et que la policy autorise la délivrance.
- Échec SSH :
  - Le certificat doit contenir le principal `ztna` (sinon l'utilisateur remote n'acceptera pas la clé).
  - Vérifier que `ztna-gw` et les machines LAN ont bien la CA publique dans `AuthorizedKeysCommand`/`TrustedUserCAKeys`.
  - Tester reachabilité du jump: `ssh -J ztna@${GW_HOST} -i ${KEY_FILE} -i ${CERT_FILE} ztna@${TARGET_IP}` en mode verbeux (`-vvv`).

Commandes de diagnostic rapides

```bash
# CP health
curl -sk ${CP_URL}/healthz | jq -r .

# Gateway (port mTLS) — vérification basique
nc -vz 10.10.10.20 4433

# Afficher le certificat obtenu
ssh-keygen -L -f "${CERT_FILE}"

# Pour retenter rapidement : supprimer cert et regénérer
rm -f "${CERT_FILE}" && # refaire l'étape 4
```

Notes
- Le script original `scripts/test-ssh-cert-access.sh` encapsule ces mêmes étapes et gère l'interaction si `ZTNA_USER`/`ZTNA_PASS` ne sont pas fournis.
- Pour tester `lan-admin` remplacez `TARGET_IP` par `10.10.30.11`.

Si vous voulez que j'ajoute des commandes prêtes à coller par VM (ex : ce qu'il faut taper **sur** `ztna-gw` ou **sur** `ztna-cp`), dites-moi lesquelles vous voulez inclure ; je peux ajouter une section par VM.
