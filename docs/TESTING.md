# Guide de Tests Manuels — ZTNA Project

> **Statut des tests automatiques :** ✅ 25/25 CP · ✅ 13/13 Gateway (vérifié le 22/02/2026)

---

## Table des matières

1. [Tests locaux (sans lab)](#1-tests-locaux-sans-lab)
2. [Prérequis lab](#2-prérequis-lab)
3. [Démarrage du lab](#3-démarrage-du-lab)
4. [Déploiement du control-plane](#4-déploiement-du-control-plane)
5. [Déploiement du gateway](#5-déploiement-du-gateway)
6. [Flux 1 — Accès SSH par certificat](#6-flux-1--accès-ssh-par-certificat)
7. [Flux 2 — Accès mTLS via gateway](#7-flux-2--accès-mtls-via-gateway)
8. [Commandes de vérification rapide](#8-commandes-de-vérification-rapide)
9. [Dépannage](#9-dépannage)

---

## 1. Tests locaux (sans lab)

Ces commandes s'exécutent directement sur votre machine de développement.

### 1.1 Tests unitaires complets

```bash
# Depuis la racine du projet
make test

# Ou manuellement :
cd control-plane && go test ./... -v
cd ../gateway     && go test ./... -v
```

### 1.2 Compilation

```bash
# Control-plane
make build-cp
# → control-plane/cp-linux-amd64

# Gateway
make build-gw
# → gateway/ztna-gateway-linux-amd64
```

### 1.3 Vérification des binaires

```bash
file control-plane/cp-linux-amd64
file gateway/ztna-gateway-linux-amd64
# Doit afficher : ELF 64-bit LSB executable, x86-64
```

### 1.4 Vérification des policies

```bash
cat control-plane/policies.yaml
# Doit contenir 3 règles allow + 1 deny-all :
#   allow  group:ztna-admins → ssh:lan-app:22
#   allow  group:ztna-admins → http:lan-app:80
#   allow  group:ztna-admins → ssh:lan-admin:22
#   deny   *                 → *
```

### 1.5 Vérification du config gateway

```bash
cat gateway/config.yaml
# Doit contenir les 3 routes (ssh:lan-app, http:lan-app, ssh:lan-admin)
```

---

## 2. Prérequis lab

```bash
# Vérifier les outils requis
make check  # ou :

which virsh terraform libvirt ssh curl jq openssl python3
virsh version
terraform version
```

---

## 3. Démarrage du lab

```bash
# Créer les 5 VMs (wan-client, ztna-gw, ztna-cp, lan-app, lan-admin)
make up
# ou directement :
bash scripts/lab-up-simple.sh

# ~3-5 minutes. Vérifier que tout est up :
make status
```

**Résultat attendu :**

```
State des VMs:
 wan-client    running
 ztna-gw       running
 ztna-cp       running
 lan-app       running
 lan-admin     running

SSH:
 ✓ wan-client (10.10.10.10)
 ✓ ztna-gw    (10.10.10.20)
 ✓ ztna-cp    (10.10.20.30)
```

### Connectivité SSH directe (test de base)

```bash
ssh -o StrictHostKeyChecking=no ztna@10.10.10.10  # wan-client
ssh -o StrictHostKeyChecking=no ztna@10.10.10.20  # ztna-gw
ssh -o StrictHostKeyChecking=no ztna@10.10.20.30  # ztna-cp

# VMs LAN (isolées, via jump host ztna-gw)
ssh -J ztna@10.10.10.20 ztna@10.10.30.10          # lan-app
ssh -J ztna@10.10.10.20 ztna@10.10.30.11          # lan-admin
```

---

## 4. Déploiement du control-plane

```bash
make deploy
# ou :
bash scripts/deploy-control-plane.sh
```

**Vérifications après déploiement :**

```bash
# 1. Keycloak accessible
curl -sk https://10.10.20.30:8443/realms/ztna | python3 -m json.tool | head -5

# 2. Control-plane API accessible
curl -sk https://10.10.20.30:8080/health | python3 -m json.tool

# 3. SSH CA pubkey exposée (endpoint ajouté)
curl -sk https://10.10.20.30:8080/pki/ssh-ca/pubkey
# Doit afficher : ecdsa-sha2-nistp256 AAAAxxxxxx...  (clé publique SSH)

# 4. Device CA cert exposé
curl -sk https://10.10.20.30:8080/pki/device-ca/cert
# Doit afficher : -----BEGIN CERTIFICATE-----

# 5. Obtenir un token Keycloak (remplacer USER/PASS par vos credentials)
curl -sk \
  -d "client_id=ztna-client&username=alice&password=alice123&grant_type=password" \
  https://10.10.20.30:8443/realms/ztna/protocol/openid-connect/token \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('access_token','ERREUR')[:80])"
# Doit afficher les premiers caractères du JWT

# 6. Logs du control-plane
make logs-cp
```

---

## 5. Déploiement du gateway

```bash
make deploy-gw
# ou :
bash scripts/deploy-gateway.sh
```

**Vérifications après déploiement :**

```bash
# 1. Service gateway actif sur ztna-gw
ssh -o StrictHostKeyChecking=no ztna@10.10.10.20 \
  "sudo systemctl status ztna-gateway --no-pager"

# 2. Port 4433 en écoute
ssh -o StrictHostKeyChecking=no ztna@10.10.10.20 \
  "sudo ss -tlnp | grep 4433"
# Doit afficher : LISTEN  *:4433

# 3. Logs du gateway (temps réel)
ssh -o StrictHostKeyChecking=no ztna@10.10.10.20 \
  "sudo journalctl -u ztna-gateway -f"

# 4. Vérifier SSH CA configurée sur lan-app
ssh -J ztna@10.10.10.20 ztna@10.10.30.10 \
  "grep TrustedUserCAKeys /etc/ssh/sshd_config"
# Doit afficher : TrustedUserCAKeys /etc/ssh/ztna_ca.pub

# 5. nginx actif sur lan-app
ssh -J ztna@10.10.10.20 ztna@10.10.30.10 \
  "curl -s http://localhost:80 | head -3"
# Doit afficher la page HTML de bienvenue
```

---

## 6. Flux 1 — Accès SSH par certificat

### Test automatique (script)

```bash
ZTNA_USER=alice ZTNA_PASS=alice123 bash scripts/test-ssh-cert-access.sh lan-app
ZTNA_USER=alice ZTNA_PASS=alice123 bash scripts/test-ssh-cert-access.sh lan-admin
```

### Test manuel étape par étape

**Étape 1 — Obtenir un token OIDC**

```bash
TOKEN=$(curl -sk \
  -d "client_id=ztna-client&username=alice&password=alice123&grant_type=password" \
  https://10.10.20.30:8443/realms/ztna/protocol/openid-connect/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

echo "Token obtenu (longueur: ${#TOKEN})"
```

**Étape 2 — Générer une clé SSH**

```bash
mkdir -p ~/.ztna
ssh-keygen -t ed25519 -f ~/.ztna/id_ztna_alice -N "" -C "ztna-alice" -q
echo "Clé générée : $(cat ~/.ztna/id_ztna_alice.pub)"
```

**Étape 3 — Obtenir un certificat SSH signé**

```bash
PUB_KEY=$(cat ~/.ztna/id_ztna_alice.pub)

curl -sk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"public_key\": \"$PUB_KEY\"}" \
  https://10.10.20.30:8080/api/v1/credentials/ssh-cert \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('certificate','ERREUR'))" \
  > ~/.ztna/id_ztna_alice-cert.pub

cat ~/.ztna/id_ztna_alice-cert.pub
# Doit afficher : ecdsa-cert-v01@openssh.com AAAAxxx...
```

**Étape 4 — Vérifier le certificat**

```bash
ssh-keygen -L -f ~/.ztna/id_ztna_alice-cert.pub
# Doit afficher :
#   Type: ecdsa-sha2-nistp256-cert-v01@openssh.com user certificate
#   Key ID: "alice"
#   Valid: from ... to ...
#   Principals: ztna
#   Extensions: permit-pty permit-port-forwarding ...
```

**Étape 5 — Se connecter via jump host avec le certificat**

```bash
# Vers lan-app
ssh \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -i ~/.ztna/id_ztna_alice \
  -i ~/.ztna/id_ztna_alice-cert.pub \
  -J ztna@10.10.10.20 \
  ztna@10.10.30.10

# Vers lan-admin
ssh \
  -o StrictHostKeyChecking=no \
  -o UserKnownHostsFile=/dev/null \
  -i ~/.ztna/id_ztna_alice \
  -i ~/.ztna/id_ztna_alice-cert.pub \
  -J ztna@10.10.10.20 \
  ztna@10.10.30.11
```

> **Résultat attendu :** session SSH interactive ouverte sur la VM cible, sans demande de mot de passe, grâce au certificat signé.

---

## 7. Flux 2 — Accès mTLS via gateway

### Test automatique (script)

```bash
# Accès HTTP à lan-app via mTLS
ZTNA_USER=alice ZTNA_PASS=alice123 bash scripts/test-mtls-access.sh http

# Accès SSH via mTLS (tunnel TCP)
ZTNA_USER=alice ZTNA_PASS=alice123 bash scripts/test-mtls-access.sh ssh
```

### Test manuel étape par étape

**Étape 1 — Token OIDC** *(même qu'à la section 6)*

```bash
TOKEN=$(curl -sk \
  -d "client_id=ztna-client&username=alice&password=alice123&grant_type=password" \
  https://10.10.20.30:8443/realms/ztna/protocol/openid-connect/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")
```

**Étape 2 — Générer une clé ECDSA device + CSR**

```bash
mkdir -p ~/.ztna

# Clé privée
openssl ecparam -name prime256v1 -genkey -noout -out ~/.ztna/device_alice.key

# CSR (CN = username, O = groupe)
openssl req -new \
  -key ~/.ztna/device_alice.key \
  -subj "/CN=alice/O=ztna-admins" \
  -out ~/.ztna/device_alice.csr

echo "CSR :"
cat ~/.ztna/device_alice.csr
```

**Étape 3 — Obtenir le certificat device signé par le CP**

```bash
CSR_JSON=$(python3 -c "import json; print(json.dumps(open('$HOME/.ztna/device_alice.csr').read()))")

curl -sk \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"csr_pem\": $CSR_JSON}" \
  https://10.10.20.30:8080/api/v1/credentials/device-cert \
  | python3 -c "import sys,json; print(json.load(sys.stdin).get('certificate_pem','ERREUR'))" \
  > ~/.ztna/device_alice.crt

# Vérifier
openssl x509 -noout -subject -issuer -dates -in ~/.ztna/device_alice.crt
# Subject: CN=alice, O=ztna-admins
# Issuer:  CN=ztna-device-ca
```

**Étape 4 — Test de connexion mTLS vers le gateway**

```bash
# Test TLS handshake uniquement (openssl s_client)
openssl s_client \
  -connect 10.10.10.20:4433 \
  -cert ~/.ztna/device_alice.crt \
  -key ~/.ztna/device_alice.key \
  -verify_quiet \
  -noservername 2>&1 | head -20
# Doit afficher : SSL handshake has read ... bytes
```

**Étape 5 — Connexion mTLS complète + requête HTTP (Python)**

```bash
python3 << 'PYEOF'
import ssl, socket, json

GW    = "10.10.10.20"
PORT  = 4433
CERT  = f"{__import__('os').environ['HOME']}/.ztna/device_alice.crt"
KEY   = f"{__import__('os').environ['HOME']}/.ztna/device_alice.key"

ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.check_hostname = False
ctx.verify_mode    = ssl.CERT_NONE
ctx.minimum_version = ssl.TLSVersion.TLSv1_3
ctx.load_cert_chain(certfile=CERT, keyfile=KEY)

print(f"[1] Connexion mTLS vers {GW}:{PORT} ...")
raw = socket.create_connection((GW, PORT), timeout=10)
tls = ctx.wrap_socket(raw, server_hostname=GW)
print(f"[2] Handshake OK — {tls.version()}")

# Envoi ConnectRequest
req = json.dumps({"resource_type":"http","resource_match":"http:lan-app:80","action":"connect"})
tls.sendall((req + "\n").encode())
print(f"[3] ConnectRequest envoyé : {req}")

# Lecture ConnectResponse
buf = b""
while b"\n" not in buf:
    chunk = tls.recv(4096)
    if not chunk: break
    buf += chunk

resp = json.loads(buf.split(b"\n")[0])
print(f"[4] ConnectResponse : {resp}")

assert resp["allowed"], f"ACCÈS REFUSÉ : {resp.get('reason')}"
print(f"[5] Accès AUTORISÉ (decision={resp.get('decision_id')})")

# Requête HTTP à travers le tunnel
http_req = "GET / HTTP/1.1\r\nHost: lan-app\r\nConnection: close\r\n\r\n"
tls.sendall(http_req.encode())
response = b""
while True:
    d = tls.recv(4096)
    if not d: break
    response += d

print(f"\n[6] Réponse HTTP :\n{'='*50}")
print(response.decode(errors="replace")[:500])
print('='*50)

tls.close()
print("\n✓ Test mTLS réussi !")
PYEOF
```

**Résultat attendu :**

```
[1] Connexion mTLS vers 10.10.10.20:4433 ...
[2] Handshake OK — TLSv1.3
[3] ConnectRequest envoyé : {"resource_type":"http",...}
[4] ConnectResponse : {'allowed': True, 'decision_id': 'dec-...', 'reason': 'rule:0'}
[5] Accès AUTORISÉ (decision=dec-...)
[6] Réponse HTTP :
==================================================
HTTP/1.1 200 OK
...
<html><body><h1>ZTNA Lab - lan-app</h1>...
==================================================
✓ Test mTLS réussi !
```

---

## 8. Commandes de vérification rapide

### État général

```bash
make status              # VMs + réseaux + SSH
make check               # idem
virsh list --all         # toutes les VMs KVM
```

### Control-plane

```bash
# Health
curl -sk https://10.10.20.30:8080/health | python3 -m json.tool

# SSH CA pubkey
curl -sk https://10.10.20.30:8080/pki/ssh-ca/pubkey

# Device CA cert
curl -sk https://10.10.20.30:8080/pki/device-ca/cert

# Logs live
make logs-cp
# ou :
ssh ztna@10.10.20.30 "sudo journalctl -u ztna-cp -f"
```

### Gateway

```bash
# Status service
ssh ztna@10.10.10.20 "sudo systemctl status ztna-gateway --no-pager"

# Port ouvert ?
ssh ztna@10.10.10.20 "sudo ss -tlnp | grep 4433"

# Logs live
ssh ztna@10.10.10.20 "sudo journalctl -u ztna-gateway -f"

# Redémarrer le gateway
ssh ztna@10.10.10.20 "sudo systemctl restart ztna-gateway"
```

### lan-app

```bash
# nginx
ssh -J ztna@10.10.10.20 ztna@10.10.30.10 "systemctl status nginx --no-pager"
ssh -J ztna@10.10.10.20 ztna@10.10.30.10 "curl -s http://localhost/"

# SSH CA configurée ?
ssh -J ztna@10.10.10.20 ztna@10.10.30.10 \
  "cat /etc/ssh/ztna_ca.pub && grep TrustedUserCAKeys /etc/ssh/sshd_config"
```

### Keycloak

```bash
# Realm accessible
curl -sk https://10.10.20.30:8443/realms/ztna | python3 -m json.tool | grep realm

# Logs Keycloak
make logs-keycloak
```

---

## 9. Dépannage

### Le lab ne démarre pas

```bash
sudo systemctl status libvirtd
sudo virsh net-list --all      # vérifier wan-net, dmz-net, lan-net
cd lab/terraform && terraform state list  # voir l'état Terraform
```

### Connexion SSH refusée

```bash
# Vérifier que la clé SSH est autorisée
ssh-keygen -l -f lab/terraform/ssh_public_key.pub

# Console virsh si SSH ne répond pas
make vm-console VM=wan-client
# login: ztna / mot de passe: défini dans cloud-init
```

### Token OIDC non obtenu

```bash
# Vérifier Keycloak
ssh ztna@10.10.20.30 "cd ztna/control-plane/keycloak && docker-compose ps"
# Si pas running :
ssh ztna@10.10.20.30 "cd ztna/control-plane/keycloak && docker-compose up -d"
```

### Accès mTLS refusé par le gateway

```bash
# Vérifier les policies sur le CP
ssh ztna@10.10.20.30 "cat ztna/control-plane/policies.yaml"
# group:ztna-admins doit être autorisé

# Vérifier que l'utilisateur est dans le groupe ztna-admins dans Keycloak
# → Admin Keycloak : https://10.10.20.30:8443/admin (admin/admin)

# Logs du gateway pour voir la décision
ssh ztna@10.10.10.20 "sudo journalctl -u ztna-gateway --since '5 min ago'"
```

### Certificat SSH rejeté

```bash
# Vérifier la CA sur le serveur cible
ssh -J ztna@10.10.10.20 ztna@10.10.30.10 \
  "cat /etc/ssh/ztna_ca.pub"
# Comparer avec :
curl -sk https://10.10.20.30:8080/pki/ssh-ca/pubkey
# Les deux doivent être identiques

# Reconfigurer si nécessaire
bash scripts/deploy-gateway.sh  # re-fetche la CA sur toutes les VMs
```

### Rebuild complet depuis zéro

```bash
make destroy      # supprimer les VMs
make clean        # nettoyer les fichiers temporaires
make up           # recréer le lab
make deploy       # déployer CP
make deploy-gw    # déployer gateway
```

---

## Résumé des adresses IP

| VM | Réseau | Adresse | Accessible depuis |
|---|---|---|---|
| wan-client | wan-net | 10.10.10.10 | PC (SSH direct) |
| ztna-gw | wan/dmz/lan | 10.10.10.20 / 10.10.20.20 / 10.10.30.20 | PC (SSH direct) |
| ztna-cp | dmz-net | 10.10.20.30 | PC (SSH direct) |
| lan-app | lan-net | 10.10.30.10 | Via ztna-gw (-J) |
| lan-admin | lan-net | 10.10.30.11 | Via ztna-gw (-J) |

## Résumé des ports

| Service | Hôte | Port | Protocole |
|---|---|---|---|
| Control Plane API | ztna-cp | 8080 | HTTPS |
| Keycloak | ztna-cp | 8443 | HTTPS |
| SSH jump host | ztna-gw | 22 | SSH |
| ZTNA Gateway mTLS | ztna-gw | 4433 | TLS 1.3 |
| nginx (lan-app) | lan-app | 80 | HTTP (via tunnel) |
