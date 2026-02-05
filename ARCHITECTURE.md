# Architecture ZTNA Lab

## Table des Matières
1. [Vue d'Ensemble](#vue-densemble)
2. [Topologie Réseau](#topologie-réseau)
3. [Composants](#composants)
4. [Flux d'Accès](#flux-daccès)
5. [Modèles de Sécurité](#modèles-de-sécurité)
6. [Technologies Utilisées](#technologies-utilisées)
7. [Évolution Prévue](#évolution-prévue)

---

## Vue d'Ensemble

### Principes

Le lab ZTNA implémente les principes **Zero Trust** :

1. **Never Trust, Always Verify** : Chaque accès est vérifiécomplètement
2. **Least Privilege** : Accès minimum requis pour la tâche
3. **Assume Breach** : L'infrastructure suppose une compromission
4. **Verify Explicitly** : Utiliser tous les contextes disponibles
5. **Secure by Default** : Tout est bloqué par défaut

### Architecture Générale

```
┌─────────────────────────────────────────────────────┐
│          INFRASTRUCTURE ZERO TRUST                   │
├─────────────────────────────────────────────────────┤
│                                                       │
│  ┌─────────────────────────────────────────────┐   │
│  │     Policy Decision Point (PDP)              │   │
│  │  - Authentication                            │   │
│  │  - Authorization                             │   │
│  │  - Certificate Authority                     │   │
│  │  - Policy Engine                             │   │
│  │  (Serveur Control Plane)                      │   │
│  └─────────────────────────────────────────────┘   │
│               ↕ (gRPC/TLS)                          │
│  ┌─────────────────────────────────────────────┐   │
│  │  Policy Enforcement Point (PEP)              │   │
│  │  - SSH Gateway                                │   │
│  │  - Access Control                            │   │
│  │  - Audit & Logging                           │   │
│  │  - Routing                                    │   │
│  │  (Gateway ZTNA)                               │   │
│  └─────────────────────────────────────────────┘   │
│               ↕ (SSH/Tunnels)                       │
│  ┌─────────────────────────────────────────────┐   │
│  │        RESSOURCES PROTÉGÉES                  │   │
│  │  - Serveurs d'application                    │   │
│  │  - Bases de données                          │   │
│  │  - Services internes                         │   │
│  └─────────────────────────────────────────────┘   │
│                                                       │
└─────────────────────────────────────────────────────┘
```

---

## Topologie Réseau

### Réseaux Isolés

```
┌──────────────────────────────────────────────────────────────┐
│         HYPERVISEUR KVM/QEMU                                 │
├──────────────────────────────────────────────────────────────┤
│                                                                │
│  ┌──────────────────┐  ┌──────────────────┐  ┌────────────┐  │
│  │   WAN Network    │  │   DMZ Network    │  │ LAN Network│  │
│  │ (Untrusted/Public)  │ (Semi-trusted)  │  │(Trusted)   │  │
│  │ 10.10.10.0/24    │  │ 10.10.20.0/24    │  │ 10.10.30.0/│  │
│  │                  │  │                  │  │      24    │  │
│  └────────┬─────────┘  └────────┬─────────┘  └─────┬──────┘  │
│           │                     │                  │          │
│           │ DHCP: 10-31         │ Static IPs       │ Static IPs
│           │                     │                  │          │
│    ┌──────┴─────┐        ┌──────┴──────┐     ┌────┴────┐     │
│    │             │        │             │     │          │     │
│    ◆ wan-client  ◆ wan-  ◆ ztna-gw     ◆ztna-◆lan-app  ◆     │
│    │ .10         │attacker─┆ .20 (WAN) ┆ cp   ◆ .10     │     │
│    │             │ .11     ┆ .20 (DMZ) ┆.30   │          │     │
│    │ (Legitime)  │(Attaque)┆           ┆     ◆lan-admin │     │
│    │             │         ◆ ┆         ┆     │ .11      │     │
│    │             │ ┆       ┆ ┆         ┆     │          │     │
│    └─────────────┘ ┆       ┆ ┆         ┆     └──────────┘     │
│                    ┆       ┆ ┆         ┆                      │
│                    └─────┬─┆──────────┘                       │
│                          │ ┆                                  │
│                      Routes Control Plane                     │
│                          │ ┆                                  │
│                          ▼ ▼                                  │
│                    ┌──────────────────┐                       │
│                    │   Contrôleur     │                       │
│                    │ (+ Policy Engine)│                       │
│                    │  ztna-cp@10..30  │                       │
│                    └──────────────────┘                       │
└──────────────────────────────────────────────────────────────┘

Legend:
  ◆ = VM
  ┆ = Communication de contrôle
  → = Accès utilisateur
```

### Caractéristiques des Réseaux

| Réseau | CIDR | Type | Description |
|--------|------|------|-------------|
| **WAN** | 10.10.10.0/24 | Untrusted | Accès externe, clients, attaquants potentiels |
| **DMZ** | 10.10.20.0/24 | Semi-trusted | Contrôleur et Gateway ZTNA |
| **LAN** | 10.10.30.0/24 | Trusted | Applications protégées, ressources internes |

### Isolation Réseau

- **Aucune route par défaut** entre WAN ↔ LAN
- **Seule la Gateway ZTNA** peut router entre ces réseaux
- **Le Control Plane** reste en DMZ, isolated
- **Firewall virtuel** implicite par segmentation de réseau

---

## Composants

### 1. Machines Virtuelles

#### Client WAN (wan-client)
- **IP** : 10.10.10.10
- **Réseau** : WAN
- **Role** : Utilisateur légitime tentant d'accéder aux services
- **Configuration** : Ubuntu 22.04, 1 GB RAM, 1 vCPU
- **OS Utilisateur** : ztna / clé SSH

**Flux** :
```
Client → SSH vers Gateway ZTNA
       → Authentification
       → Récupère certificat SSH temporaire du CP
       → Ouvre tunnel SSH via Gateway
       → Accède à l'application LAN
```

#### Attaquant WAN (wan-attacker)
- **IP** : 10.10.10.11
- **Réseau** : WAN
- **Role** : Simule une tentative d'accès non autorisée
- **Configuration** : Ubuntu 22.04, 1 GB RAM, 1 vCPU

**Comportement** :
- Aucun accès au Control Plane
- Accès bloqué à la Gateway sans authentification
- Tentatives de connexion directe aux applications = bloquées

#### Gateway ZTNA (ztna-gw)
- **IPs** : 10.10.10.20 (WAN), 10.10.20.20 (DMZ)
- **Réseau** : WAN + DMZ (dual-homed)
- **Role** : Policy Enforcement Point (PEP)
- **Configuration** : Ubuntu 22.04, 2 GB RAM, 2 vCPU
- **Services** :
  - SSH Server (réception des connexions)
  - SSH Client (routage vers applications)
  - Intercepteur de commandes
  - Loggueur d'audit

**Responsabilités** :
```
1. Accepter connexions SSH des clients WAN
2. Vérifier les certificats SSH signés
3. Exécuter les politiques de routage
4. Rediriger vers application correcte
5. Logger toutes les actions
6. Mesurer les métriques d'accès
```

#### Control Plane (ztna-cp)
- **IP** : 10.10.20.30
- **Réseau** : DMZ
- **Role** : Policy Decision Point (PDP)
- **Configuration** : Ubuntu 22.04, 2 GB RAM, 2 vCPU
- **Responsabilités** :
  - Authentification des utilisateurs
  - Évaluation des politiques d'accès
  - Génération de certificats SSH temporaires (15 minutes)
  - Audit et logging
  - Gestion de la configuration

**API** :
```bash
POST /api/v1/auth/login
     Authentifie l'utilisateur, retourne JWT

POST /api/v1/certs/request
     Génère un certificat SSH temporaire

GET /api/v1/policies/{resource}
     Récupère les politiques d'accès

POST /api/v1/audit
     Log un événement d'accès
```

#### Application LAN (lan-app)
- **IP** : 10.10.30.10
- **Réseau** : LAN
- **Role** : Ressource protégée
- **Configuration** : Ubuntu 22.04, 1 GB RAM, 1 vCPU
- **Services** : 
  - SSH Server (connectivité simple)
  - Applications déployées (TBD)

#### Admin LAN (lan-admin)
- **IP** : 10.10.30.11
- **Réseau** : LAN
- **Role** : Administration et monitoring
- **Configuration** : Ubuntu 22.04, 1 GB RAM, 1 vCPU

---

## Flux d'Accès

### Scénario 1 : Accès Légitime Autorisé

```
Étape 1 : AUTHENTIFICATION
┌──────────────┐
│ Client WAN   │ (10.10.10.10)
└───────┬──────┘
        │ SSH Client → Control Plane
        │ POST /api/v1/auth/login
        │ Payload: {username: alice, password: ...}
        ▼
┌──────────────────┐
│ Control Plane    │ (10.10.20.30)
│ - Vérifie MFA    │
│ - Vérifie posture│ Vérifications:
│ - Check policies │  ✓ Utilisateur existe
└───────┬──────────┘  ✓ MFA valide
        │ Retour: JWT token
        ▼
┌──────────────┐
│ Client WAN   │ Token stocké
└──────────────┘


Étape 2 : CERTIFICAT SSH TEMPORAIRE
┌──────────────┐
│ Client WAN   │
└───────┬──────┘
        │ JWT Token → Control Plane
        │ POST /api/v1/certs/request
        │ Payload: {resource: app, duration: 900s}
        ▼
┌──────────────────┐
│ Control Plane    │
│ - Génère clé pri-│ Génération:
│   vée temporaire │  - Nouvelle clé Ed25519
│ - Signe certificat  - Valide 15 minutes
│ - Log requête    │  - Signé avec clé CA
└───────┬──────────┘
        │ Retour: certificat SSH signé
        ▼
┌──────────────┐
│ Client WAN   │ Certificat stocké en RAM
└──────────────┘


Étape 3 : ACCÈS À LA RESSOURCE
┌──────────────┐
│ Client WAN   │
└───────┬──────┘
        │ SSH Connection + Certificat
        │ SSH ztna@10.10.10.20 (Gateway ZTNA)
        ▼
┌────────────────────┐
│ Gateway ZTNA       │ (10.10.10.20)
│ - Vérifie cert SSH │ Vérifications:
│ - Interprete commandes  ✓ Certificat valide
│ - Extrait resource │  ✓ Source IP autorisée
│ - Extract user     │  ✓ Durée certificat OK
└───────┬────────────┘
        │ Décision: ALLOW
        │ SSH → lan-app:22 (10.10.30.10)
        ▼
┌──────────────┐
│ Application  │ (10.10.30.10)
│ - Accepte SSH│ Utilisateur ztna connecté
│ - Session ouverte
└──────────────┘

Résultat: ACCÈS AUTORISÉ ✓
```

### Scénario 2 : Accès Non Autorisé (Attaquant)

```
Étape 1 : TENTATIVE DE CONNEXION
┌──────────────┐
│ Attaquant WAN│ (10.10.10.11)
└───────┬──────┘
        │ SSH ztna@10.10.20.30 (Direct vers Control Plane)
        ▼
┌──────────────────┐
│ Gateway ZTNA     │ Rejet en couche réseau
│ - Pas de route   │ (pas de connectivité directe WAN→DMZ)
└──────────────────┘ Exception levée


Étape 2 : TENTATIVE DE CERTIFICAT FRAUDULEUX
┌──────────────┐
│ Attaquant WAN│
└───────┬──────┘
        │ SSH avec certificat généré localement
        │ SSH ztna@10.10.10.20 -i fake-cert
        ▼
┌────────────────────┐
│ Gateway ZTNA       │
│ - Vérifie signature│ Vérifications:
│ - Check timestamp  │  ✗ Signature invalide (pas de CA)
│                    │  ✗ Log attempt
└────────────────────┘ Connexion refusée

Résultat: ACCÈS REFUSÉ ✗
```

---

## Modèles de Sécurité

### 1. Authentification

- **Méthode** : Certificats SSH temporaires (short-lived)
- **Durée** : 15 minutes par défaut
- **Génération** : Control Plane uniquement
- **Signature** : Clé CA du Control Plane

### 2. Autorisation

- **Modèle** : RBAC (Role-Based Access Control) + Attributs
- **Vérifications** :
  - Rôle utilisateur
  - Contexte (IP source, heure, device)
  - Ressource demandée
  - Sensibilité des données

### 3. Audit et Logging

Chaque accès est enregistré :
```json
{
  "timestamp": "2026-02-01T21:00:00Z",
  "user": "alice",
  "source_ip": "10.10.10.10",
  "action": "SSH_CONNECT",
  "resource": "app@10.10.30.10",
  "status": "ALLOWED",
  "duration_seconds": 1234,
  "commands_count": 45
}
```

### 4. Ségrégation Réseau

```
WAN Network          DMZ Network         LAN Network
┌──────────┐        ┌──────────┐        ┌──────────┐
│ Clients  │        │ ZTNA GW  │        │ Apps     │
│ Public   │        │ CP       │        │ Internal │
└──────────┘        └──────────┘        └──────────┘
     │                   │ | │               │
     └───────────────────┘ | └───────────────┘
                           │
                    Firewall Virtuel
                    (Routes ZTNA only)
```

---

## Technologies Utilisées

### Infrastructure

| Composant | Technologie | Version | Rôle |
|-----------|-------------|---------|------|
| **Hyperviseur** | KVM/QEMU | 7.0+ | Virtualisation des VMs |
| **Provisioning** | Terraform | 1.14+ | Infrastructure-as-Code |
| **VM Image** | Ubuntu Cloud | 22.04 | Base OS |
| **Networking** | libvirt | 8.0+ | Gestion des réseaux virtuels |
| **Boot** | cloud-init | Latest | Configuration VM initiale |

### Application

| Composant | Technologie | Version | Rôle |
|-----------|-------------|---------|------|
| **Language** | Go | 1.21+ | Développement PDP/PEP |
| **RPC** | gRPC | v1 | Communication PDP ↔ PEP |
| **TLS** | TLS 1.3 | 1.3 | Chiffrement |
| **SSH** | OpenSSH | 8.9+ | Protocol d'accès |
| **Auth** | JWT | RS256 | Token stateless |
| **Cert** | SSH CA | Custom | Certificats temporaires |
| **DB** | SQLite | 3.40+ | Audit logging |

---

## Évolution Prévue

### Phase 1 : Fondamentaux (v1.0) ✅
- [x] Infrastructure KVM/Terraform
- [x] Gateway ZTNA basique
- [x] Control Plane avec authentification
- [x] Certificats SSH temporaires

### Phase 2 : Sécurité Avancée (v2.0)
- [ ] MFA (TOTP, U2F)
- [ ] Device posture checking
- [ ] Contexte géographique
- [ ] ML anomaly detection

### Phase 3 : Enterprise (v3.0)
- [ ] LDAP/AD integration
- [ ] Okta SSO
- [ ] mTLS mutual authentication
- [ ] Policy as Code (OPA)

### Phase 4 : Multi-Cloud (v4.0)
- [ ] AWS support
- [ ] Azure support
- [ ] Kubernetes integration
- [ ] Service mesh integration (Istio)

---

## Dépannage Architectural

### Le Client ne peut pas atteindre la Gateway

**Cause** : Problème de routage ou iptables

**Solution** :
```bash
# Sur ztna-gw, vérifier les routes
ip route

# Vérifier l'écoute SSH
ss -tlnp | grep :22

# Vérifier la règle iptables
iptables -L -n | grep 10.10
```

### Le Control Plane n'est pas accessible de la Gateway

**Cause** : Routes réseau manquantes

**Solution** :
```bash
# Sur ztna-gw
ping 10.10.20.30

# Sur ztna-cp
ping 10.10.20.20
```

### Certificats SSH expire trop tôt

**Cause** : Configuration durée certificat

**Solution** : Éditer la configuration du Control Plane
```go
const CertValidityDuration = 15 * time.Minute  // Augmenter si nécessaire
```

---

**Version** : 1.0  
**Date** : 1 février 2026  
**Status** : Production Ready ✅
