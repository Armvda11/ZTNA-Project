# ZTNA Lab - Guide de Démarrage Rapide

Guide rapide pour démarrer avec le lab ZTNA sur votre machine.

## Étape 1 : Vérifier les Prérequis (2 min)

Avant de commencer, assure-toi que ta machine a :

```bash
# Vérifie la CPU (doit afficher vmx ou svm)
grep -E 'vmx|svm' /proc/cpuinfo

# Vérifie la RAM (besoin minimum 16 GB)
free -h | grep Mem

# Vérifie l'espace disque (besoin minimum 100 GB)
df -h /
```

**Résultat attendu** :
- CPU : vmx ou svm présent
- RAM : 16+ GB
- Disque : 100+ GB libres

Si tu as des problèmes, voir [REQUIREMENTS.md](REQUIREMENTS.md).

---

## Étape 2 : Installation Automatique (5-10 min)

Le script d'installation fait **tout automatiquement** :

```bash
# 1. Clone le projet
git clone https://github.com/your-org/ZTNA.git
cd ZTNA

# 2. Exécute le setup (demande ton mot de passe sudo)
./setup.sh

# 3. Active les permissions (IMPORTANT)
newgrp libvirt

# 4. Vérifie que c'est OK
./scripts/check-requirements.sh
```

**Qu'est-ce que setup.sh fait ?**
- Installe QEMU/KVM
- Installe Terraform
- Configure libvirt
- Génère les clés SSH
- Crée les répertoires
- Prépare AppArmor

Aucune intervention manuelle nécessaire !

---

## Étape 3 : Créer le Lab (5-10 min)

Maintenant que tout est installé, crée l'infrastructure :

```bash
# Initialise et crée tout (VMs + réseaux)
make init
```

Ça fera :
1. Télécharger les images Ubuntu
2. Créer 3 réseaux isolés
3. Créer 6 VMs
4. Configurer chaque VM avec cloud-init
5. Vérifier que tout marche

**Temps d'attente** : 5-10 minutes

---

## Étape 4 : Vérifier que Tout Marche (2 min)

Après le `make init`, vérifie que l'infrastructure est opérationnelle :

```bash
# Lance le script de vérification
./scripts/check-lab.sh

# Ou utilise make
make check
```

**Résultat attendu** :
```
✓ 6 VMs en cours d'exécution
✓ 4 réseaux actifs
✓ wan-client accessible
✓ wan-attacker accessible
✓ ztna-gw accessible
```

---

## Utilisation Basique

### Se Connecter à une VM

```bash
# Client WAN (le point de départ)
make ssh-client
# ou
ssh ztna@10.10.10.10

# Gateway ZTNA (le routeur de sécurité)
make ssh-gw
# ou
ssh ztna@10.10.10.20

# Control Plane (le cœur du système)
make ssh-cp
# ou
ssh ztna@10.10.20.30

# Application protégée (dans le LAN)
make ssh-app
# ou
ssh ztna@10.10.30.10
```

**Note** : Aucun mot de passe requis (clé SSH utilisée)

### Voir l'État du Lab

```bash
# État complet
make status

# Ou spécifiquement
make check-vms           # Lister les VMs
make check-networks      # Lister les réseaux
make check-ssh           # Tester les connexions SSH
```

### Arrêter / Redémarrer

```bash
# Arrêter proprement (les VMs peuvent redémarrer)
make vm-stop

# Redémarrer
make vm-start

# Arrêter TOUT le lab
make destroy

# Recréer après destroy
make init
```

---

## Structure du Projet

```
ZTNA/
├── README.md               ← Documentation générale
├── REQUIREMENTS.md         ← Dépendances et prérequis
├── ARCHITECTURE.md         ← Architecture détaillée
├── setup.sh                ← Script d'installation (À EXÉCUTER D'ABORD)
├── Makefile                ← Commandes principales (make init, make check, etc.)
│
├── lab/terraform/          ← Définition de l'infrastructure (Terraform)
│   ├── main.tf
│   ├── networks.tf
│   ├── vms.tf
│   ├── variables.tf
│   ├── terraform.tfvars    ← Config à personnaliser si besoin
│   └── cloudinit/          ← Scripts de configuration des VMs
│
├── scripts/                ← Utilitaires
│   ├── check-requirements.sh  ← Vérifier les prérequis
│   ├── check-lab.sh           ← Vérifier le lab
│   ├── init-lab.sh            ← Initialiser le lab (appelé par make init)
│   └── cleanup.sh             ← Nettoyer complètement
│
├── control-plane/          ← Control Plane (à développer en Go)
│   ├── main.go
│   └── go.mod
│
├── gateway/                ← Gateway ZTNA (à développer en Go)
│   ├── main.go
│   └── go.mod
│
└── docs/
    ├── TROUBLESHOOTING.md  ← Problèmes courants et solutions
    ├── API.md              ← Documentation API
    └── DEVELOPMENT.md      ← Guide de développement
```

---

## Commandes Principales

```bash
# INSTALLATION (À FAIRE UNE FOIS)
./setup.sh                 # Installer tout

# CRÉATION DU LAB
make init                  # Créer l'infrastructure
make plan                  # Voir ce qui sera créé
make apply                 # Appliquer les changements

# VÉRIFICATION
make check                 # Vérifier l'état
make status                # État détaillé
make check-vms             # Lister les VMs
make check-ssh             # Tester SSH

# CONNEXIONS SSH
make ssh-client            # Connecter au client WAN
make ssh-gw                # Connecter à la gateway
make ssh-cp                # Connecter au Control Plane
make ssh-app               # Connecter à l'app
make ssh-admin             # Connecter à l'admin

# GESTION DES VMs
make vm-start              # Démarrer toutes les VMs
make vm-stop               # Arrêter proprement
make vm-reboot             # Redémarrer
make vm-force-stop         # Arrêter de force (dangereux)

# LOGS
make logs-libvirtd         # Logs de libvirt
make logs-vm VM=wan-client # Logs d'une VM

# NETTOYAGE
make clean                 # Nettoyer fichiers temporaires
make destroy               # Détruire l'infrastructure
make clean-all             # Destroy + clean

# AIDE
make help                  # Afficher toutes les commandes
```

---

## Problèmes Courants

### "Permission Denied" sur libvirt

```bash
# Solution
newgrp libvirt
# ou redémarre ta session
```

### Les VMs ne démarrent pas

```bash
# Attends quelques secondes
sleep 30

# Puis vérifie
make check-vms
```

### SSH ne marche pas

```bash
# C'est normal au démarrage (cloud-init configure SSH)
sleep 60

# Puis réessaie
make ssh-client
```

### "terraform not found"

```bash
./setup.sh  # Réexécute le setup complet
```

**Pour plus de problèmes**, voir [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md).

---

## Prochaines Lectures

1. **[ARCHITECTURE.md](ARCHITECTURE.md)** - Comprendre le design
2. **[REQUIREMENTS.md](REQUIREMENTS.md)** - Tous les détails techniques
3. **[docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)** - Dépannage
4. **[README.md](README.md)** - Vue d'ensemble complète

---

## Commande TL;DR

```bash
git clone https://github.com/your-org/ZTNA.git && cd ZTNA
./setup.sh
newgrp libvirt
make init
make check
make ssh-client
```

Vous avez maintenant un lab ZTNA complètement opérationnel !

---

## Tips Utiles

```bash
# Alias raccourcis dans le Makefile
make i          # = make init
make c          # = make check
make s          # = make status
make d          # = make destroy
make h          # = make help

# Voir tout
make help       # Affiche toutes les commandes avec descriptions

# Tester la connectivité réseau depuis le client
ssh ztna@10.10.10.10 'ping 10.10.10.20'

# Accéder à la console d'une VM (Ctrl+] pour quitter)
virsh console wan-client

# Voir l'arborescence des VMs et réseaux
virsh domtree
```

---

## Questions ?

- Consulte **[TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)** d'abord
- Ouvre une **issue GitHub**
- Demande à **[@author]**

---

**Prêt ?** Exécute `./setup.sh` et suis les instructions !

---

*Dernière mise à jour : 1 février 2026*
