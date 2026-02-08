# Onboarding - ZTNA Lab

Objectif: avoir un lab fonctionnel en moins de 15-20 minutes.

## TL;DR

```bash
git clone https://github.com/your-org/ZTNA.git
cd ZTNA
./setup.sh
newgrp libvirt
make init
make check
```

## Liens utiles

- Guide rapide: [../QUICKSTART.md](../QUICKSTART.md)
- Vue d'ensemble: [../README.md](../README.md)
- Architecture: [../ARCHITECTURE.md](../ARCHITECTURE.md)
- Depannage: [../TROUBLESHOOTING.md](../TROUBLESHOOTING.md)
- Contribution: [../CONTRIBUTING.md](../CONTRIBUTING.md)

## Commandes essentielles

```bash
make init
make check
make ssh-client
make ssh-gw
make ssh-cp
```

## En cas de probleme

- Depannage: [../TROUBLESHOOTING.md](../TROUBLESHOOTING.md)
- Installation manuelle: [SETUP.md](SETUP.md)

## Vous etes pret

- Control Plane: [../control-plane](../control-plane)
- Gateway: [../gateway](../gateway)
- Infrastructure: [../lab/terraform](../lab/terraform)
- Contribution: [../CONTRIBUTING.md](../CONTRIBUTING.md)
