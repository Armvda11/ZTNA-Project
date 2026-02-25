# Onboarding ZTNA Lab

Objectif: avoir un lab fonctionnel avec le chemin le plus simple possible.

## TL;DR

```bash
./setup.sh              # optionnel si la machine n'est pas preparee
newgrp libvirt          # recharger groupe libvirt
make quickstart         # prereq -> up -> deploy -> deploy-gw -> check
```

## Commandes essentielles

```bash
make help
make check
make ssh-client
make ssh-gw
make ssh-cp
make destroy
```

## Ce que fait quickstart

1. Verifie les prerequis minimum (`make prereq`)
2. Cree/met a jour les VMs (`make up`)
3. Deploie control-plane + keycloak (`make deploy`)
4. Deploie gateway (`make deploy-gw`)
5. Verifie sante et SSH (`make check`)

## Liens utiles

- Guide rapide: [../QUICKSTART.md](../QUICKSTART.md)
- Prerequis: [REQUIREMENTS.md](REQUIREMENTS.md)
- Installation manuelle: [SETUP.md](SETUP.md)
- Tests: [TESTING.md](TESTING.md)
- Runbook CP/GW: [CP_GW_RUNBOOK.md](CP_GW_RUNBOOK.md)
