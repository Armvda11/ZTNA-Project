# ZTNA Lab - Zero Trust Network Access

Lab ZTNA pour tester et developper un Control Plane (PDP) et une Gateway (PEP) sur un reseau WAN/DMZ/LAN isole.

## Demarrage rapide

```bash
git clone https://github.com/your-org/ZTNA.git && cd ZTNA
./setup.sh
newgrp libvirt
make init
make check
```

## Ce que tu obtiens

| Element | Details |
|--------|---------|
| **6 VMs** | wan-client, wan-attacker, ztna-gw, ztna-cp, lan-app, lan-admin |
| **3 reseaux** | WAN (10.10.10.0/24), DMZ (10.10.20.0/24), LAN (10.10.30.0/24) |
| **Infra** | Terraform IaC, KVM/QEMU, Ubuntu 22.04 |
| **Automatisation** | setup.sh + Makefile |
| **Docs** | Guides d'installation, depannage, architecture |

## Commandes principales

```bash
make init               # Creer l'infrastructure
make check              # Verifier l'etat
make ssh-client         # SSH vers wan-client
make ssh-gw             # SSH vers la gateway
make ssh-cp             # SSH vers le control-plane
make destroy            # Tout detruire
```

## Architecture reseau

```
        WAN (10.10.10.0/24)
   [Client]   [Attacker]
      .10         .11
         \        /
          \      /
           [Gateway] 10.10.20.20
               |
         [Control Plane] 10.10.20.30
               |
          LAN (10.10.30.0/24)
          [App]   [Admin]
           .10      .11
```

## Documentation

- **Onboarding** : [docs/ONBOARDING.md](docs/ONBOARDING.md)
- **Quick start** : [QUICKSTART.md](QUICKSTART.md)
- **Prerequis** : [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md)
- **Installation manuelle** : [docs/SETUP.md](docs/SETUP.md)
- **Architecture** : [ARCHITECTURE.md](ARCHITECTURE.md)
- **Depannage** : [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- **Contribuer** : [CONTRIBUTING.md](CONTRIBUTING.md)
- **Index docs** : [docs/INDEX.md](docs/INDEX.md)

## Acces SSH

```bash
# Utilisateur: ztna | Cle: ~/.ssh/id_ed25519
ssh ztna@10.10.10.10    # wan-client
ssh ztna@10.10.10.11    # wan-attacker
ssh ztna@10.10.20.20    # ztna-gw
ssh ztna@10.10.20.30    # ztna-cp
ssh ztna@10.10.30.10    # lan-app
ssh ztna@10.10.30.11    # lan-admin
```

## Structure (resume)

```
ZTNA/
├── README.md
├── QUICKSTART.md
├── ARCHITECTURE.md
├── CONTRIBUTING.md
├── TROUBLESHOOTING.md
├── Makefile
├── setup.sh
├── scripts/
├── lab/terraform/
├── control-plane/
├── gateway/
└── docs/
```

## Statut

- **Lab**: operationnel (infra + automation)
- **Control Plane**: MVP fonctionnel (auth, policies, CA SSH, audit, rate limit)
- **Gateway**: serveur SSH de base (validation cert + proxy en cours)

## License

MIT - Voir [LICENSE](LICENSE)
