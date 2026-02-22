# Tests Manuels du Lab ZTNA

Ce fichier décrit les commandes à exécuter **manuellement** sur chaque VM pour valider
ou invalider un comportement du système ZTNA. Chaque test indique le **résultat attendu**
(✅ succès / ❌ échec) et la **raison** qui explique ce résultat.

---

## Prérequis — Connexion aux VMs

```bash
# Depuis l'hôte KVM
SSH="ssh -o StrictHostKeyChecking=no -i ~/.ssh/id_ed25519"

# VMs WAN (accessibles directement)
$SSH ztna@10.10.10.10    # wan-client
$SSH ztna@10.10.10.20    # ztna-gw
$SSH ztna@10.10.20.30    # ztna-cp

# VMs LAN (via jump host ztna-gw)
$SSH -J ztna@10.10.10.20 ztna@10.10.30.10   # lan-app
$SSH -J ztna@10.10.10.20 ztna@10.10.30.11   # lan-admin
```

Raccourcis Makefile disponibles : `make ssh-cp`, `make ssh-gw`, `make ssh-app`, etc.

---

## 1. Santé des services

### Depuis l'hôte KVM

```bash
# ── Tester la santé du control-plane ──────────────────────────────────────────
curl -sfk https://10.10.20.30:8080/healthz
# ✅ ATTENDU : ok
# ❌ ÉCHEC   : "curl: (7) Failed to connect" → le service ztna-cp n'est pas démarré
#             Solution : ssh ztna@10.10.20.30 "sudo systemctl restart ztna-cp"

# ── Vérifier que Keycloak répond ──────────────────────────────────────────────
curl -sf http://10.10.20.30:8081/realms/ztna/.well-known/openid-configuration | python3 -m json.tool | head -5
# ✅ ATTENDU : JSON avec "issuer", "authorization_endpoint", etc.
# ❌ ÉCHEC   : connexion refusée → docker-compose pas démarré sur ztna-cp

# ── Vérifier que le gateway écoute ────────────────────────────────────────────
nc -zv 10.10.10.20 4433
# ✅ ATTENDU : "Connection to 10.10.10.20 4433 port [tcp/*] succeeded!"
# ❌ ÉCHEC   : "Connection refused" → sudo systemctl restart ztna-gateway sur ztna-gw
```

---

## 2. Authentification OIDC (Keycloak)

### Depuis l'hôte KVM ou wan-client

```bash
# ── Token valide pour alice ────────────────────────────────────────────────────
TOKEN=$(curl -s -d 'grant_type=password&client_id=ztna-control-plane&client_secret=ztna-client-secret&username=alice&password=Password123%21' \
  'http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
echo "Token: ${#TOKEN} chars"
# ✅ ATTENDU : "Token: 1177 chars" (JWT valide)

# ── Vérifier les claims du token (whoami) ─────────────────────────────────────
curl -sk -H "Authorization: Bearer $TOKEN" https://10.10.20.30:8080/api/v1/whoami
# ✅ ATTENDU : {"sub":"1d584701-...","username":"alice","groups":["ztna-admins"]}

# ── Mot de passe incorrect ────────────────────────────────────────────────────
curl -s -d 'grant_type=password&client_id=ztna-control-plane&client_secret=ztna-client-secret&username=alice&password=MAUVAIS_MDP' \
  'http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token'
# ❌ ATTENDU : {"error":"invalid_grant","error_description":"Invalid user credentials"}

# ── Utilisateur inexistant ────────────────────────────────────────────────────
curl -s -d 'grant_type=password&client_id=ztna-control-plane&client_secret=ztna-client-secret&username=hacker&password=test' \
  'http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token'
# ❌ ATTENDU : {"error":"invalid_grant","error_description":"Invalid user credentials"}

# ── Token expiré (simulé avec un JWT arbitraire) ──────────────────────────────
curl -sk -H "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.FAUX.SIGNATURE" \
  https://10.10.20.30:8080/api/v1/whoami
# ❌ ATTENDU : {"error":"unauthorized"}

# ── Appel sans token ──────────────────────────────────────────────────────────
curl -sk https://10.10.20.30:8080/api/v1/whoami
# ❌ ATTENDU : {"error":"unauthorized"}
```

---

## 3. Flux 1 — Certificat SSH

### Depuis l'hôte KVM (simule wan-client)

```bash
# ── Préparer : obtenir un token frais ─────────────────────────────────────────
TOKEN=$(curl -s -d 'grant_type=password&client_id=ztna-control-plane&client_secret=ztna-client-secret&username=alice&password=Password123%21' \
  'http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

# ── Générer une clé SSH Ed25519 ───────────────────────────────────────────────
mkdir -p ~/.ztna && ssh-keygen -t ed25519 -f ~/.ztna/test_key -N "" -q
PUB_KEY=$(cat ~/.ztna/test_key.pub)

# ── Demander un certificat SSH avec les bons principals ───────────────────────
CERT_RESP=$(curl -sk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"$PUB_KEY\", \"principals\": [\"ztna\", \"alice\"]}" \
  https://10.10.20.30:8080/api/v1/credentials/ssh-cert)
echo "$CERT_RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['certificate'])" > ~/.ztna/test_key-cert.pub

ssh-keygen -L -f ~/.ztna/test_key-cert.pub
# ✅ ATTENDU : Principals = ztna, alice  /  Valid = now → +15min

# ── Connexion SSH autorisée : lan-app ─────────────────────────────────────────
ssh -o StrictHostKeyChecking=no \
    -i ~/.ztna/test_key -i ~/.ztna/test_key-cert.pub \
    -J ztna@10.10.10.20 ztna@10.10.30.10 \
    'echo "✅ CONNEXION RÉUSSIE"; id; hostname'
# ✅ ATTENDU :
#   ✅ CONNEXION RÉUSSIE
#   uid=1000(ztna) gid=1001(ztna) groups=1001(ztna),...
#   lan-app

# ── Connexion SSH autorisée : lan-admin ───────────────────────────────────────
ssh -o StrictHostKeyChecking=no \
    -i ~/.ztna/test_key -i ~/.ztna/test_key-cert.pub \
    -J ztna@10.10.10.20 ztna@10.10.30.11 \
    'echo "✅ CONNEXION RÉUSSIE"; hostname'
# ✅ ATTENDU : lan-admin

# ── Connexion sans certificat (clé seule) ─────────────────────────────────────
ssh -o StrictHostKeyChecking=no \
    -o PreferredAuthentications=publickey \
    -i ~/.ztna/test_key \
    -J ztna@10.10.10.20 ztna@10.10.30.10 \
    'echo FAIL' 2>&1
# ❌ ATTENDU : "Permission denied (publickey)" car TrustedUserCAKeys est requis

# ── Certificat sans le principal "ztna" ───────────────────────────────────────
CERT_NO_ZTNA=$(curl -sk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"$PUB_KEY\", \"principals\": [\"alice\"]}" \
  https://10.10.20.30:8080/api/v1/credentials/ssh-cert \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['certificate'])")
echo "$CERT_NO_ZTNA" > /tmp/cert_alice_only.pub

ssh -o StrictHostKeyChecking=no \
    -i ~/.ztna/test_key -i /tmp/cert_alice_only.pub \
    -J ztna@10.10.10.20 ztna@10.10.30.10 \
    'echo FAIL' 2>&1
# ❌ ATTENDU : "Permission denied" — le cert a le principal "alice" mais
#             l'utilisateur unix cible est "ztna", donc le cert est rejeté par sshd

# ── Demande de cert sans token ────────────────────────────────────────────────
curl -sk -H "Content-Type: application/json" \
  -d "{\"public_key\": \"$PUB_KEY\"}" \
  https://10.10.20.30:8080/api/v1/credentials/ssh-cert
# ❌ ATTENDU : {"error":"unauthorized"}
```

---

## 4. Flux 2 — mTLS / Certificat Device

### Depuis l'hôte KVM (simule wan-client)

```bash
# ── Préparer : obtenir un token et générer une clé ECDSA ──────────────────────
TOKEN=$(curl -s -d 'grant_type=password&client_id=ztna-control-plane&client_secret=ztna-client-secret&username=alice&password=Password123%21' \
  'http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

mkdir -p ~/.ztna
openssl ecparam -name prime256v1 -genkey -noout -out ~/.ztna/device.key 2>/dev/null
openssl req -new -key ~/.ztna/device.key -subj "/CN=alice/O=ztna-admins" -out ~/.ztna/device.csr 2>/dev/null

# ── Demander un certificat Device ─────────────────────────────────────────────
CSR=$(cat ~/.ztna/device.csr)
curl -sk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"csr_pem\": $(python3 -c "import json; print(json.dumps(open('$HOME/.ztna/device.csr').read()))")}" \
  https://10.10.20.30:8080/api/v1/credentials/device-cert \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['certificate_pem'])" > ~/.ztna/device.crt

openssl x509 -noout -subject -enddate -in ~/.ztna/device.crt
# ✅ ATTENDU :
#   subject=O=ztna-admins, CN=alice, serialNumber=<UUID>
#   notAfter=Mar  1 ... (7 jours)

# ── Test complet mTLS → HTTP lan-app (accès autorisé) ─────────────────────────
ZTNA_USER=alice ZTNA_PASS='Password123!' bash scripts/test-mtls-access.sh http
# ✅ ATTENDU :
#   [mTLS] Handshake OK — protocole : TLSv1.3
#   │  allowed     : True
#   │  reason      : rule:2
#   HTTP/1.0 200 OK
#   ✓ Test mTLS terminé avec succès.

# ── Essayer d'accéder à une ressource non autorisée par les politiques ─────────
python3 - << 'EOF'
import ssl, socket, json, sys
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False; ctx.verify_mode = ssl.CERT_NONE
ctx.load_cert_chain(f"{__import__('os').path.expanduser('~')}/.ztna/device.crt",
                    f"{__import__('os').path.expanduser('~')}/.ztna/device.key")
tls = ctx.wrap_socket(socket.create_connection(("10.10.10.20", 4433), timeout=10))
tls.sendall(json.dumps({"resource_type":"http","resource_match":"http:lan-admin:80","action":"connect"}).encode()+b"\n")
resp = json.loads(tls.recv(4096))
print(f"allowed={resp['allowed']}, reason={resp.get('reason')}")
# ❌ ATTENDU : allowed=False, reason=default-deny ou rule:<deny-all>
# Raison     : lan-admin:80 n'a pas de règle allow dans policies.yaml
EOF

# ── Sans certificat client (connexion TLS sans cert) ──────────────────────────
python3 - << 'EOF'
import ssl, socket
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False; ctx.verify_mode = ssl.CERT_NONE
# Pas de load_cert_chain → pas de certificat client
try:
    tls = ctx.wrap_socket(socket.create_connection(("10.10.10.20", 4433), timeout=5))
    print("FAIL - connexion établie sans cert")
except ssl.SSLError as e:
    print(f"❌ ATTENDU - Handshake rejeté : {e}")
EOF
# ❌ ATTENDU : SSLError / CERTIFICATE_REQUIRED

# ── Certificat auto-signé (non émis par la CA du CP) ─────────────────────────
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 \
  -keyout /tmp/fake.key -out /tmp/fake.crt \
  -days 1 -nodes -subj "/CN=hacker/O=hackers" 2>/dev/null

python3 - << 'EOF'
import ssl, socket
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False; ctx.verify_mode = ssl.CERT_NONE
ctx.load_cert_chain("/tmp/fake.crt", "/tmp/fake.key")
try:
    tls = ctx.wrap_socket(socket.create_connection(("10.10.10.20", 4433), timeout=5))
    print("FAIL - connexion établie avec un faux cert")
except ssl.SSLError as e:
    print(f"❌ ATTENDU - Rejeté par mTLS : {e}")
EOF
# ❌ ATTENDU : SSLError — le cert n'est pas signé par la Device CA du CP
```

---

## 5. Tests sur ztna-cp (services internes)

### Depuis ztna-cp

```bash
# Se connecter
ssh -o StrictHostKeyChecking=no -i ~/.ssh/id_ed25519 ztna@10.10.20.30

# ── Vérifier que le CP tourne ─────────────────────────────────────────────────
sudo systemctl status ztna-cp --no-pager | grep -E 'Active|Main PID'
# ✅ ATTENDU : Active: active (running)

# ── Consulter les logs récents ────────────────────────────────────────────────
sudo journalctl -u ztna-cp -n 20 --no-pager
# ✅ ATTENDU : entrées JSON avec "app ready" et "starting public server"

# ── Vérifier Keycloak ─────────────────────────────────────────────────────────
cd ztna/control-plane/keycloak
docker-compose ps
# ✅ ATTENDU : keycloak   ...   Up   0.0.0.0:8081->8080/tcp

# ── Lister les politiques actives (API admin) ─────────────────────────────────
TOKEN=$(curl -s -d 'grant_type=password&client_id=ztna-control-plane&client_secret=ztna-client-secret&username=alice&password=Password123%21' \
  'http://127.0.0.1:8081/realms/ztna/protocol/openid-connect/token' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
# ⚠️  ATTENTION : ici on est sur ztna-cp donc 127.0.0.1 est OK (token local)
#    MAIS l'issuer du token sera 127.0.0.1, pas 10.10.20.30.
#    L'endpoint CP refusera ce token si l'issuer de config est 10.10.20.30 !
# → Utiliser toujours l'IP 10.10.20.30 même depuis ztna-cp :
TOKEN=$(curl -s -d 'grant_type=password&client_id=ztna-control-plane&client_secret=ztna-client-secret&username=alice&password=Password123%21' \
  'http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

curl -sk -H "Authorization: Bearer $TOKEN" https://10.10.20.30:8080/api/v1/admin/policies
# ✅ ATTENDU : [{"id":1,"is_active":true,"rules":[...]}]

# ── Appeler l'endpoint admin sans être dans ztna-admins ───────────────────────
# (simuler un utilisateur sans le rôle admin — ici on utilise alice qui EST admin)
# Pour tester le refus, créer un user sans groupe dans Keycloak. En attendant :
curl -sk https://10.10.20.30:8080/api/v1/admin/policies
# ❌ ATTENDU : {"error":"unauthorized"} — pas de token

# ── Appeler PEP avec un token PEP invalide ────────────────────────────────────
curl -sk -X POST \
  -H "X-PEP-ID: ztna-gw-01" \
  -H "X-PEP-TOKEN: MAUVAIS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"subject":{"username":"alice","groups":["ztna-admins"]},"action":"connect","resource":{"type":"ssh","host":"lan-app","port":22}}' \
  https://10.10.20.30:8080/api/v1/pep/authorize
# ❌ ATTENDU : {"error":"unauthorized"}

# ── Appeler PEP avec input invalide (resource type inconnu) ───────────────────
curl -sk -X POST \
  -H "X-PEP-ID: ztna-gw-01" \
  -H "X-PEP-TOKEN: ztna-lab-pep-secret-2026" \
  -H "Content-Type: application/json" \
  -d '{"subject":{"username":"alice","groups":["ztna-admins"]},"action":"connect","resource":{"type":"ftp","host":"lan-app","port":21}}' \
  https://10.10.20.30:8080/api/v1/pep/authorize
# ❌ ATTENDU : {"error":"invalid input"} — type "ftp" non supporté

# ── Appeler PEP avec une ressource autorisée ──────────────────────────────────
curl -sk -X POST \
  -H "X-PEP-ID: ztna-gw-01" \
  -H "X-PEP-TOKEN: ztna-lab-pep-secret-2026" \
  -H "Content-Type: application/json" \
  -d '{"subject":{"username":"alice","groups":["ztna-admins"]},"action":"connect","resource":{"type":"http","http":{"host":"lan-app","port":80}}}' \
  https://10.10.20.30:8080/api/v1/pep/authorize
# ✅ ATTENDU : {"effect":"allow","reason":"rule:2",...}

# ── Appeler PEP avec une ressource non autorisée ──────────────────────────────
curl -sk -X POST \
  -H "X-PEP-ID: ztna-gw-01" \
  -H "X-PEP-TOKEN: ztna-lab-pep-secret-2026" \
  -H "Content-Type: application/json" \
  -d '{"subject":{"username":"alice","groups":["ztna-admins"]},"action":"connect","resource":{"type":"http","http":{"host":"lan-admin","port":80}}}' \
  https://10.10.20.30:8080/api/v1/pep/authorize
# ❌ ATTENDU : {"effect":"deny","reason":"default-deny",...}
# Raison     : aucune règle allow pour http:lan-admin:80 dans policies.yaml
```

---

## 6. Tests sur ztna-gw (gateway)

### Depuis ztna-gw

```bash
ssh -o StrictHostKeyChecking=no -i ~/.ssh/id_ed25519 ztna@10.10.10.20

# ── État du service gateway ───────────────────────────────────────────────────
sudo systemctl status ztna-gateway --no-pager | grep -E 'Active|Main PID'
# ✅ ATTENDU : Active: active (running)

# ── Confirmer que le port 4433 est en écoute ──────────────────────────────────
ss -tlnp | grep 4433
# ✅ ATTENDU : LISTEN   0   128   0.0.0.0:4433   0.0.0.0:*   users:(("ztna-gateway",...))

# ── Joindre le CP depuis la gateway ──────────────────────────────────────────
curl -sfk https://10.10.20.30:8080/healthz
# ✅ ATTENDU : ok

# ── Vérifier que les ressources LAN sont accessibles depuis la gateway ─────────
nc -zv 10.10.30.10 22 2>&1    # SSH lan-app
# ✅ ATTENDU : succeeded!   (sshd écoute sur lan-app)
nc -zv 10.10.30.10 80 2>&1    # HTTP lan-app
# ✅ ATTENDU : succeeded!   (nginx ou python http.server)
nc -zv 10.10.30.11 22 2>&1    # SSH lan-admin
# ✅ ATTENDU : succeeded!

# ── Vérifier que la gateway ne peut PAS joindre le réseau WAN arbitraire ──────
nc -zv 1.1.1.1 443 -w 3 2>&1
# ❌ ATTENDU : timeout / Network unreachable  (les VMs n'ont pas d'accès Internet direct)

# ── Logs en direct pendant un test mTLS ───────────────────────────────────────
sudo journalctl -u ztna-gateway -f --no-pager &
# Lancer depuis l'hôte : ZTNA_USER=alice ZTNA_PASS='Password123!' bash scripts/test-mtls-access.sh http
# ✅ ATTENDU dans les logs :
#   "client connected", "access allowed", "username":"alice", "target":"10.10.30.10:80"
```

---

## 7. Tests sur lan-app (ressource protégée)

### Depuis lan-app

```bash
ssh -o StrictHostKeyChecking=no -i ~/.ssh/id_ed25519 -J ztna@10.10.10.20 ztna@10.10.30.10

# ── Vérifier la configuration SSH (TrustedUserCAKeys) ────────────────────────
grep TrustedUserCAKeys /etc/ssh/sshd_config
# ✅ ATTENDU : TrustedUserCAKeys /etc/ssh/ztna_ca.pub
cat /etc/ssh/ztna_ca.pub
# ✅ ATTENDU : clé publique SSH-CA du CP (format ssh-ed25519 ...)

# ── Vérifier que lan-app ne peut PAS joindre le WAN ──────────────────────────
curl --max-time 3 http://1.1.1.1 2>&1
# ❌ ATTENDU : timeout / connection refused
# Raison    : pas de route vers Internet (réseau LAN isolé)

# ── Vérifier que le serveur HTTP tourne ───────────────────────────────────────
ss -tlnp | grep ':80'
# ✅ ATTENDU : LISTEN ... (python3 -m http.server ou nginx)
# Si absent : sudo python3 -m http.server 80 </dev/null >/tmp/http.log 2>&1 &

# ── Accès direct depuis l'hôte KVM serait impossible ─────────────────────────
# Depuis l'hôte, tenter :
#   curl http://10.10.30.10/  avec routage direct (pas de NAT configuré)
# ❌ ATTENDU : timeout ou "No route to host"
# → lan-app n'est accessible que depuis ztna-gw (réseau LAN isolé)
```

---

## 8. Tests de sécurité avancés

### Depuis l'hôte KVM

```bash
# ── Connexion SSH directe sur lan-app (sans certificat ZTNA) ─────────────────
ssh -o StrictHostKeyChecking=no -i ~/.ssh/id_ed25519 -J ztna@10.10.10.20 ztna@10.10.30.10
# ✅ ATTENDU : connexion réussie — l'accès SSH via jump host avec la clé cloud-init
#            est autorisé (c'est la clé provisionnée par cloud-init, pas ZTNA).
# ⚠️  Note : cet accès "admin" de l'opérateur est normal.
#            En production, ce canal serait restreint à certaines IPs + bastion.

# ── Scan de port sur le gateway depuis l'extérieur ───────────────────────────
nmap -p 4433,8080,22 10.10.10.20 2>/dev/null | grep -E 'open|closed|filtered'
# ✅ ATTENDU :
#   22/tcp    open   ssh     (SSH administratif)
#   4433/tcp  open   ssl/mTLS (ZTNA gateway)
# Le port 8080 (CP) n'est sur ztna-gw, donc filtered/closed ici

nmap -p 8080 10.10.20.30 2>/dev/null | grep -E 'open|closed|filtered'
# ✅ ATTENDU : 8080/tcp open (mais sur réseau DMZ, pas WAN)

# ── Accès brut au port 4433 sans TLS ─────────────────────────────────────────
echo "GET / HTTP/1.0" | nc 10.10.10.20 4433 2>&1 | head -3
# ❌ ATTENDU : connexion fermée immédiatement (le serveur attend un ClientHello TLS)

# ── Replay attack : réutiliser un decision_id ─────────────────────────────────
# Le gateway n'implémente pas (encore) de cache de replay.
# Le decision_id est un UUID unique généré à chaque appel PEP → pas de replay côté CP.
# Ce comportement est documenté : chaque ConnectRequest génère un nouvel appel PEP.
#
# Validation : lancer 2 fois le test mTLS → observer 2 decision_id différents dans les logs.
ZTNA_USER=alice ZTNA_PASS='Password123!' bash scripts/test-mtls-access.sh http 2>&1 | grep decision_id
ZTNA_USER=alice ZTNA_PASS='Password123!' bash scripts/test-mtls-access.sh http 2>&1 | grep decision_id
# ✅ ATTENDU : 2 UUID différents dans les ConnectResponse
```

---

## 9. Récapitulatif

| # | Test | VM d'où exécuter | Résultat attendu |
|---|------|-----------------|------------------|
| 1 | `curl healthz` | hôte KVM | ✅ `ok` |
| 2 | Token alice valide | hôte / wan-client | ✅ JWT 1177 chars |
| 3 | Token mauvais MDP | hôte / wan-client | ❌ `invalid_grant` |
| 4 | SSH cert → lan-app (principals ztna+alice) | hôte | ✅ connexion SSH |
| 5 | SSH sans cert client | hôte | ❌ Permission denied |
| 6 | mTLS HTTP → lan-app | hôte | ✅ HTTP 200 |
| 7 | mTLS vers ressource non listée (lan-admin:80) | hôte | ❌ `allowed=False` |
| 8 | mTLS sans cert client | hôte | ❌ SSLError handshake |
| 9 | mTLS cert auto-signé | hôte | ❌ SSLError handshake |
| 10 | PEP call token invalide | ztna-cp | ❌ `unauthorized` |
| 11 | PEP call type invalide (ftp) | ztna-cp | ❌ `invalid input` |
| 12 | PEP call ressource autorisée | ztna-cp | ✅ `effect=allow` |
| 13 | API admin sans token | ztna-cp | ❌ `unauthorized` |
| 14 | Accès WAN depuis lan-app | lan-app | ❌ timeout |
| 15 | Port 4433 brut sans TLS | hôte | ❌ connexion fermée |
