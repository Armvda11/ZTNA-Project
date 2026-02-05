# ZTNA Lab - Zero Trust Network Access

Infrastructure production-ready pour tester et développer **Zero Trust Network Access**.  
**5 min d'installation. 6 VMs opérationnelles. Prêt à développer.**

<p align="center">
  <strong><a href="#démarrage-rapide">Quick Start</a></strong> •
  <strong><a href="#commandes">Commandes</a></strong> •
  <strong><a href="#documentation">Docs</a></strong> •
  <strong><a href="#support">Aide</a></strong>
</p>

---

## Démarrage Rapide

```bash
git clone https://github.com/your-org/ZTNA.git && cd ZTNA
./setup.sh              # Auto-installation (15 min)
newgrp libvirt          # Activer les permissions
make init               # Créer infrastructure (10 min)
make check              # Vérifier tout fonctionne
```

**C'est tout.** Vous avez une infrastructure ZTNA complète et opérationnelle.

---

## Qu'est-ce que vous obtenez ?

| Élément | Détails |
|--------|---------|
| **6 VMs** | wan-client, wan-attacker, ztna-gw, ztna-cp, lan-app, lan-admin |
| **3 Réseaux** | WAN (10.10.10.0/24), DMZ (10.10.20.0/24), LAN (10.10.30.0/24) |
| **Infrastructure** | Terraform IaC, KVM/QEMU, Ubuntu 22.04 |
| **Automatisation** | setup.sh + Makefile (40+ commandes) |
| **Documentation** | ARCHITECTURE.md, QUICKSTART.md, etc. |

---

## Commandes Principales

```bash
make init               # Créer l'infrastructure
make check              # Vérifier l'état
make ssh-client         # SSH vers wan-client
make ssh-gw             # SSH vers la gateway
make ssh-cp             # SSH vers control-plane
make destroy            # Tout détruire
make help               # Voir toutes les commandes
```

---

## Architecture Réseau

```
        ┌─── WAN Network (10.10.10.0/24) ───┐
        │                                     │
   [Client]  [Attacker]              Internet (NAT)
    .10         .11
        │                                     │
        └──────────┬──────────────────────────┘
                   │
             ┌─────┴─────┐
             │           │
        [Gateway]    [DMZ]
         10.10.20    (dmz-net)
             │           │
             │    [Control Plane]
             │       10.10.20.30
             │
        ┌────┴────┐
        │   LAN   │ (10.10.30.0/24)
        │         │
    [App]   [Admin]
     .10      .11
```

---

## Documentation Complète

- **Prérequis** : [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md)
- **Installation manuelle** : [docs/SETUP.md](docs/SETUP.md)
- **Architecture technique** : [ARCHITECTURE.md](ARCHITECTURE.md)
- **Contribuer** : [CONTRIBUTING.md](CONTRIBUTING.md)
- **Dépannage** : [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- **Quick Start** : [QUICKSTART.md](QUICKSTART.md)
- **Plus de docs** : Voir [docs/](docs/)

---

## Accès SSH

```bash
# Utilisateur: ztna | Clé: ~/.ssh/id_ed25519

ssh ztna@10.10.10.10    # wan-client
ssh ztna@10.10.10.11    # wan-attacker
ssh ztna@10.10.20.30    # ztna-cp (control-plane)
ssh ztna@10.10.20.20    # ztna-gw (gateway)
ssh ztna@10.10.30.10    # lan-app
ssh ztna@10.10.30.11    # lan-admin
```

---

## Structure
Structure

```
ZTNA/
├── README.md
├── QUICKSTART.md
├── ARCHITECTURE.md
├── CONTRIBUTING.md
├── TROUBLESHOOTING.md
├── Makefile
├── setup.sh
├── .gitignore, .editorconfig, .pre-commit-config.yaml
│
├── scripts/
│   ├── init-lab.sh
│   └── cleanup.sh
│
├── lab/
│   └── terraform/
│       ├── main.tf, variables.tf, outputs.tf
│       └── terraform.tfvars
│
├── control-plane/
│   ├── main.go
│   ├── go.mod
│   └── internal/
│
├── gateway/
│   ├── main.go
│   ├── go.mod
│   └── internal/
│
└── docs/
    ├── SETUP.md
    ├── REQUIREMENTS.md
    ├── CHANGELOG.md
    └── ONBOARDING.md
```

---

##
- **OS** : Ubuntu 22.04+ LTS
- **RAM** : 16 GB minimum
- **CPU** : 4+ cores avec VT-x/AMD-V
- **Disque** : 100 GB libres

**VProchaines Étapes

1. Lire [ARCHITECTURE.md](ARCHITECTURE.md) pour comprendre le design
2. Explorer `control-plane/` et `gateway/` pour développer
3. Consulter [TROUBLESHOOTING.md](TROUBLESHOOTING.md) en cas de problème
4. Lire [CONTRIBUTING.md](CONTRIBUTING.md) pour contribuer

---

## Prochaines Étapes

1. **C'est bon ?** → Lire [ARCHITECTURE.md](ARCHITECTURE.md) pour comprendre
2. **Veux développer ?** → Voir `control-plane/` et `gateway/`
3. **Problème ?** → Voir [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
4. **Veux contribuer ?** → Lire [CONTRIBUTING.md](CONTRIBUTING.md)

---

## License

MIT - Voir [LICENSE](LICENSE)

---

**Status** : Production Ready
---

## License

MIT - Voir [LICENSE](LICENSE)

---

**Status** : Production Ready  
**Version** : 1.0.0  
**Dernière MAJ** : 1 février 2026
