# Flux 2 — mTLS HTTP : test manuel depuis `wan-client`

## Vue d'ensemble

Flux 2 est l'accès à une ressource HTTP interne (lan-app) depuis un poste
client externe (wan-client), sans VPN traditionnel.
Le client obtient un **certificat X.509 signé par le Control Plane** puis
ouvre une connexion **mTLS vers la gateway ZTNA** qui proxifie vers la ressource.

```
wan-client (10.10.10.10)
  │
  │  1. POST /realms/ztna/protocol/openid-connect/token  → Keycloak
  │     ← access_token JWT
  │
  │  2. POST /api/v1/credentials/device-cert  → Control Plane (10.10.20.30:8080)
  │     Corps : { "csr_pem": "<CSR ECDSA>" }
  │     ← { "certificate_pem": "<cert X.509 signé par Device CA>" }
  │
  │  3. Connexion TLS 1.3 + certificat client  → ztna-gw (10.10.10.20:4433)
  │     Handshake mTLS (gateway vérifie le cert via Device CA)
  │
  │  4. ConnectRequest JSON (dans le tunnel TLS)
  │     { "resource_type":"http", "resource_match":"http:lan-app:80", "action":"connect" }
  │
  │  5. Gateway  → CP PEP /api/v1/pep/authorize
  │     Vérifie que alice a le droit sur http:lan-app:80
  │     ← { "effect":"allow", "decision_id":"..." }
  │
  │  6. ConnectResponse JSON (dans le tunnel TLS)
  │     { "allowed":true, "decision_id":"..." }
  │
  │  7. GET / HTTP/1.0   (toujours dans le tunnel TLS)
  │     Proxifié vers lan-app:80
  │     ← HTTP/1.0 200 OK ...
  │
ztna-gw (10.10.10.20) → lan-app (10.10.30.10:80)
```

---

## Prérequis une seule fois : configurer le routage sur `ztna-gw`

### Pourquoi ?

`wan-client` possède déjà une route statique vers `10.10.20.0/24 via 10.10.10.20`
(configurée par cloud-init). `ztna-gw` a `ip_forward=1`.

**Problème sans MASQUERADE :**
- wan-client envoie un paquet vers 10.10.20.30 avec source 10.10.10.10
- ztna-gw le forward, mais `ztna-cp` répond via sa gateway par défaut `10.10.20.1` (bridge KVM) sans repasser par ztna-gw → TCP SYN sans SYN-ACK → timeout

**Solution MASQUERADE WAN→DMZ uniquement :**
- ztna-gw réécrit la source : 10.10.10.10 → 10.10.20.20 (son IP DMZ)
- ztna-cp répond à 10.10.20.20 → ztna-gw → wan-client ✓

**Pas de MASQUERADE WAN→LAN** — inutile et dangereux :
- Pour Flux 2, la GW initie elle-même les connexions LAN (proxy mTLS → TCP)
- La source est déjà l'IP LAN de la GW, aucun NAT nécessaire
- Ajouter ce NAT transformerait la GW en routeur généraliste et ouvrirait un accès LAN direct bypassant le PEP

**Politique FORWARD = DROP par défaut** — Zero Trust :
- Seuls les ports strictement nécessaires à l'auth/enrollment sont ouverts
- `10.10.10.10 → 10.10.20.30:8081` TCP (Keycloak)
- `10.10.10.10 → 10.10.20.30:8080` TCP (Control Plane)
- Tout le reste WAN→DMZ/LAN : DROP

```
                  SANS MASQUERADE         |   AVEC MASQUERADE
─────────────────────────────────────────┼──────────────────────────────────
wan-client  → [src:10.10.10.10]          │  wan-client → [src:10.10.10.10]
              → ztna-gw forward          │               → ztna-gw SNAT
              → ztna-cp reçoit           │               → ztna-cp reçoit
                source: 10.10.10.10      │                 source: 10.10.20.20 ✓
ztna-cp répond via 10.10.20.1 (KVM)     │  ztna-cp répond à 10.10.20.20 ✓
  → paquet PERDU ✗                       │  ztna-gw DNAT → wan-client ✓
```

### Appliquer le routage (une seule commande depuis le host KVM)

```bash
make setup-routing
```

Ou manuellement, en SSH sur `ztna-gw` :

```bash
ssh -i ~/.ssh/id_ed25519 ztna@10.10.10.20
```

Puis sur `ztna-gw` :

```bash
# Activer le forwarding
sudo sysctl -w net.ipv4.ip_forward=1

# Politique par défaut : DROP tout ce qui n'est pas autorisé explicitement
sudo iptables -P FORWARD DROP

# Autoriser les connexions déjà établies (retour des réponses)
sudo iptables -I FORWARD 1 -m state --state RELATED,ESTABLISHED -j ACCEPT

# Autoriser wan-client → ztna-cp:8081 (Keycloak OIDC)
sudo iptables -A FORWARD -i ens3 -o ens4 \
  -s 10.10.10.10 -d 10.10.20.30 -p tcp --dport 8081 \
  -m state --state NEW,ESTABLISHED -j ACCEPT

# Autoriser wan-client → ztna-cp:8080 (Control Plane device-cert)
sudo iptables -A FORWARD -i ens3 -o ens4 \
  -s 10.10.10.10 -d 10.10.20.30 -p tcp --dport 8080 \
  -m state --state NEW,ESTABLISHED -j ACCEPT

# MASQUERADE WAN→DMZ uniquement (corriger l'asymétrie de routage)
# PAS de MASQUERADE WAN→LAN : inutile pour Flux 2 et dangereux
sudo iptables -t nat -A POSTROUTING -s 10.10.10.0/24 -o ens4 -j MASQUERADE

# Vérifier
sudo iptables -L FORWARD -n --line-numbers -v
sudo iptables -t nat -L POSTROUTING -n --line-numbers
```

**Tester que wan-client atteint maintenant ztna-cp :**

```bash
# Depuis wan-client (10.10.10.10)
ssh -i ~/.ssh/id_ed25519 ztna@10.10.10.10
# puis :
curl -sk https://10.10.20.30:8080/healthz
```

Résultat attendu : `{"status":"ok"}` ou similaire.

---

## Test manuel Flux 2 sur `wan-client`

Se connecter sur `wan-client` :

```bash
ssh -i ~/.ssh/id_ed25519 ztna@10.10.10.10
```

Toutes les commandes suivantes sont à taper **dans cette session SSH sur wan-client**.

### Variables (exécuter en premier)

```bash
export ZTNA_DIR="$HOME/.ztna"
export CP_URL="https://10.10.20.30:8080"
export KC_URL="http://10.10.20.30:8081"
export KC_REALM="ztna"
export KC_CLIENT="ztna-control-plane"
export GW_HOST="10.10.10.20"
export GW_PORT="4433"
export ZTNA_USER="alice"
export ZTNA_PASS='Password123!'

mkdir -p "$ZTNA_DIR" && chmod 700 "$ZTNA_DIR"
```

---

### Étape 1 — Obtenir le token OIDC (Keycloak)

```bash
TOKEN_RESP=$(curl -sk \
  -d "client_id=${KC_CLIENT}&username=${ZTNA_USER}&password=${ZTNA_PASS}&grant_type=password" \
  "${KC_URL}/realms/${KC_REALM}/protocol/openid-connect/token")

ACCESS_TOKEN=$(echo "$TOKEN_RESP" | \
  python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('access_token',''))")

if [ -z "$ACCESS_TOKEN" ]; then
  echo "ERREUR : token non obtenu"
  echo "$TOKEN_RESP"
else
  echo "Token OIDC obtenu — ${#ACCESS_TOKEN} caractères"
fi
```

**Succès attendu :** `Token OIDC obtenu — 1200+ caractères`

**Échec → vérifier :**
```bash
# Test réseau : wan-client peut-il atteindre Keycloak ?
curl -v http://10.10.20.30:8081/realms/ztna
# Si timeout : routing non configuré sur ztna-gw → faire make setup-routing
```

---

### Étape 2 — Générer la clé ECDSA et le CSR

```bash
DEVICE_KEY="$ZTNA_DIR/device_${ZTNA_USER}.key"
DEVICE_CSR="$ZTNA_DIR/device_${ZTNA_USER}.csr"
DEVICE_CRT="$ZTNA_DIR/device_${ZTNA_USER}.crt"

# Générer la clé (P-256, équivalent RSA-3072, très rapide)
if [ ! -f "$DEVICE_KEY" ]; then
  openssl ecparam -name prime256v1 -genkey -noout -out "$DEVICE_KEY"
  echo "Clé ECDSA générée → $DEVICE_KEY"
else
  echo "Clé existante réutilisée → $DEVICE_KEY"
fi

# Créer le CSR avec le CN=alice et l'organisation ztna-admins
openssl req -new -key "$DEVICE_KEY" \
  -subj "/CN=${ZTNA_USER}/O=ztna-admins" \
  -out "$DEVICE_CSR"

echo "CSR généré → $DEVICE_CSR"
cat "$DEVICE_CSR"
```

---

### Étape 3 — Obtenir le certificat device auprès du Control Plane

```bash
CERT_RESP=$(curl -sk \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"csr_pem\": $(python3 -c "import json; print(json.dumps(open('${DEVICE_CSR}').read()))")}" \
  "${CP_URL}/api/v1/credentials/device-cert")

CERT_PEM=$(echo "$CERT_RESP" | \
  python3 -c "import sys,json;d=json.load(sys.stdin);print(d.get('certificate_pem',''))")

if [ -z "$CERT_PEM" ]; then
  echo "ERREUR : certificat non obtenu"
  echo "$CERT_RESP"
else
  echo "$CERT_PEM" > "$DEVICE_CRT"
  chmod 600 "$DEVICE_CRT"
  echo "Certificat device écrit → $DEVICE_CRT"
  echo ""
  openssl x509 -noout -subject -issuer -dates -in "$DEVICE_CRT"
fi
```

**Succès attendu :**
```
Certificat device écrit → /home/ztna/.ztna/device_alice.crt
subject=CN=alice, O=ztna-admins
issuer=CN=ztna-device-ca
notBefore=...
notAfter=...
```

---

### Étape 4 — Connexion mTLS + requête HTTP (Python inline)

```bash
python3 - "$GW_HOST" "$GW_PORT" "$DEVICE_CRT" "$DEVICE_KEY" << 'EOF'
import sys, ssl, socket, json

host, port, cert, key = sys.argv[1:]
port = int(port)

# Contexte mTLS : le client présente son certificat device
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE          # lab self-signed, ok pour le test
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=cert, keyfile=key)

print(f"[1/4] Connexion TLS vers {host}:{port} ...")
raw = socket.create_connection((host, port), timeout=15)
tls = ctx.wrap_socket(raw, server_hostname=host)
print(f"[1/4] Handshake mTLS OK  — TLS: {tls.version()}")
print(f"      Certificat présenté : {cert}")

# ConnectRequest : demande de tunnel vers http:lan-app:80
req = json.dumps({
    "resource_type":  "http",
    "resource_match": "http:lan-app:80",
    "action":         "connect",
})
tls.sendall((req + "\n").encode())
print(f"[2/4] ConnectRequest envoyé : {req}")

# ConnectResponse : le gateway demande au PEP (CP) si c'est autorisé
buf = b""
while b"\n" not in buf:
    chunk = tls.recv(4096)
    if not chunk:
        break
    buf += chunk

resp = json.loads(buf.split(b"\n")[0])
print()
print("┌─ ConnectResponse (gateway → client) ──────────────────────")
print(f"│  allowed     : {resp.get('allowed')}")
print(f"│  decision_id : {resp.get('decision_id', '—')}")
print(f"│  reason      : {resp.get('reason', '—')}")
print("└───────────────────────────────────────────────────────────")

if not resp.get("allowed"):
    print("\n[REFUSÉ] Policy deny ou ressource inconnue.")
    sys.exit(1)

print(f"\n[3/4] Accès autorisé — tunnel TCP ouvert vers http:lan-app:80")

# Requête HTTP sur le tunnel TLS (HTTP/1.0 ferme après la réponse)
http_req = "GET / HTTP/1.0\r\nHost: lan-app\r\n\r\n"
tls.sendall(http_req.encode())
print(f"[4/4] Requête HTTP envoyée : GET / HTTP/1.0")

tls.settimeout(10.0)
response = b""
try:
    while True:
        data = tls.recv(4096)
        if not data:
            break
        response += data
except Exception:
    pass

tls.close()

if not response:
    print("[ERREUR] Aucune réponse HTTP (lan-app accessible ? HTTP server up ?)")
    sys.exit(1)

print()
print("─── Réponse HTTP depuis lan-app ────────────────────────────")
print(response.decode(errors="replace")[:2000])
print("────────────────────────────────────────────────────────────")
print()
print("✅ Test Flux 2 mTLS réussi depuis wan-client !")
EOF
```

**Succès attendu :**
```
[1/4] Connexion TLS vers 10.10.10.20:4433 ...
[1/4] Handshake mTLS OK  — TLS: TLSv1.3
[2/4] ConnectRequest envoyé : {...}
┌─ ConnectResponse ──
│  allowed     : True
│  decision_id : ...
└────────────────────
[3/4] Accès autorisé — tunnel TCP ouvert vers http:lan-app:80
[4/4] Requête HTTP envoyée : GET / HTTP/1.0
─── Réponse HTTP depuis lan-app ─────
HTTP/1.0 200 OK
...
✅ Test Flux 2 mTLS réussi depuis wan-client !
```

---

## Commandes de diagnostic

### Sur `wan-client`

```bash
# Joignabilité des services
curl -sk ${CP_URL}/healthz && echo "CP: OK" || echo "CP: KO"
curl -s  ${KC_URL}/realms/${KC_REALM} | python3 -c "import sys,json;d=json.load(sys.stdin);print('KC:', d.get('realm','KO'))"
nc -vz ${GW_HOST} ${GW_PORT} && echo "GW: OK" || echo "GW: KO"

# Vérifier le certificat device déjà obtenu
openssl x509 -noout -text -in "$ZTNA_DIR/device_${ZTNA_USER}.crt" | grep -E "Subject:|Issuer:|Not"

# Inspecter la réponse TLS du gateway (sans cert client, doit refuser)
openssl s_client -connect 10.10.10.20:4433 -tls1_3 </dev/null 2>&1 | head -20
```

### Sur `ztna-gw` (depuis le host KVM)

```bash
ssh -i ~/.ssh/id_ed25519 ztna@10.10.10.20

# Vérifier les règles NAT
sudo iptables -t nat -L POSTROUTING -n --line-numbers

# Vérifier forwarding
cat /proc/sys/net/ipv4/ip_forward

# Logs gateway
sudo journalctl -u ztna-gateway.service -f --no-pager
```

### Sur `ztna-cp` (depuis le host KVM via jump)

```bash
ssh -J ztna@10.10.10.20 -i ~/.ssh/id_ed25519 ztna@10.10.20.30

# Logs CP
sudo journalctl -u ztna-cp.service -f --no-pager

# Vérifier les policies (qui autorise alice sur http:lan-app:80 ?)
curl -sk -H "Content-Type: application/json" \
  https://localhost:8080/api/v1/admin/policies | python3 -m json.tool
```

---

## Tests d'échec attendus (sécurité)

### 1. Sans certificat client → rejeté au handshake TLS

```bash
# Sur wan-client — connexion sans cert client
python3 -c "
import ssl, socket
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
# Pas de ctx.load_cert_chain() ici
raw = socket.create_connection(('10.10.10.20', 4433), timeout=5)
tls = ctx.wrap_socket(raw)
print(tls.version())  # ne devrait pas arriver ici
" 2>&1
```

**Résultat attendu :** `ssl.SSLError: [...] certificate required` ou connexion fermée.

### 2. Avec un certificat auto-signé (non signé par Device CA) → rejeté

```bash
# Générer un cert self-signed
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout /tmp/fake.key -out /tmp/fake.crt -days 1 \
  -subj "/CN=evil/O=attacker" -nodes 2>/dev/null

python3 -c "
import ssl, socket, json
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
ctx.load_cert_chain(certfile='/tmp/fake.crt', keyfile='/tmp/fake.key')
raw = socket.create_connection(('10.10.10.20', 4433), timeout=5)
tls = ctx.wrap_socket(raw)
print('Handshake:', tls.version())
req = json.dumps({'resource_type':'http','resource_match':'http:lan-app:80','action':'connect'})
tls.sendall((req+'\n').encode())
buf = b''
while b'\n' not in buf:
    buf += tls.recv(4096)
print(json.loads(buf.split(b'\n')[0]))
"
```

**Résultat attendu :** Handshake échoue (`certificate verify failed` côté gateway) ou ConnectResponse `{"allowed": false}`.

### 3. Ressource non autorisée → PEP refuse

```bash
# Sur wan-client — remplacer resource_match par une ressource non couverte par les policies
python3 - "$GW_HOST" "$GW_PORT" "$DEVICE_CRT" "$DEVICE_KEY" << 'EOF'
import sys, ssl, socket, json
host, port, cert, key = sys.argv[1:]
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
ctx.load_cert_chain(certfile=cert, keyfile=key)
raw = socket.create_connection((host, int(port)), timeout=15)
tls = ctx.wrap_socket(raw, server_hostname=host)
req = json.dumps({"resource_type":"http","resource_match":"http:secret-server:443","action":"connect"})
tls.sendall((req + "\n").encode())
buf = b""
while b"\n" not in buf:
    buf += tls.recv(4096)
print("ConnectResponse:", json.loads(buf.split(b"\n")[0]))
tls.close()
EOF
```

**Résultat attendu :** `ConnectResponse: {'allowed': False, 'reason': 'denied'}`

---

## Via Makefile (depuis le host KVM)

```bash
# 1. Configurer le routage (une seule fois)
make setup-routing

# 2. Lancer le test complet depuis wan-client
make test-flux2

# 3. Version locale (pour debug rapide depuis le host)
make test-flux2
```

---

## Résumé des décisions d'architecture

| Decision | Choix | Raison |
|---|---|---|
| **Certificat client mTLS** | X.509 ECDSA P-256 | Petit, rapide, standard TLS 1.3 |
| **Port gateway** | 4433 | Ne pas utiliser 443 en lab (évite conflits) |
| **TLS version minimum** | TLS 1.3 | Supprime cipher suites obsolètes, forward secrecy obligatoire |
| **Vérification cert serveur** | désactivée en lab | Cert self-signed sur ztna-gw ; en prod : fournir la CA du gateway |
| **FORWARD policy** | DROP par défaut | Zero Trust : tout ce qui n'est pas explicitement autorisé est refusé |
| **FORWARD WAN→DMZ** | ports 8080/8081 uniquement, src=wan-client | Expose seulement les services d'auth/enrollment, pas toute la DMZ |
| **MASQUERADE WAN→DMZ** | Oui | Corrige l'asymétrie de routage (ztna-cp répond via sa GW par défaut sinon) |
| **MASQUERADE WAN→LAN** | Non | Inutile (GW proxifie elle-même), et ouvrirait un accès LAN direct bypassant le PEP |
| **ConnectRequest JSON** | `{"resource_type", "resource_match", "action"}` | Structure extensible, séparation type/identifiant |
| **HTTP/1.0 pour le test** | HTTP/1.0 au lieu de 1.1 | HTTP/1.0 ferme la connexion après la réponse, pas besoin de parser `Content-Length` |
