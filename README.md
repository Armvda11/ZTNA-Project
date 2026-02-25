# ZTNA Lab

Lab Zero Trust Network Access pour tester un Control Plane (PDP) et un Gateway (PEP) sur un reseau WAN/DMZ/LAN local.

## Demarrage recommande

```bash
# Optionnel: installer les dependances systeme
./setup.sh

# Verifier les prerequis minimum
make prereq

# Parcours complet (recommande)
make quickstart
```

`make quickstart` enchaine:
1. `make prereq`
2. `make up`
3. `make deploy`
4. `make deploy-gw`
5. `make check`

## Commandes utiles

```bash
make help
make up
make lab-start
make deploy
make deploy-gw
make build-cli
make check
make test-flux1
make test-flux2
make test-crl-routing
make destroy
```

## Topologie (lab actuel)

- WAN: `10.10.10.0/24`
- DMZ: `10.10.20.0/24`
- LAN: `10.10.30.0/24`

VMs deployees par Terraform:
- `wan-client` (`10.10.10.10`)
- `ztna-gw` (`10.10.10.20`)
- `ztna-cp` (`10.10.20.30`)
- `lan-app` (`10.10.30.10`)
- `lan-admin` (`10.10.30.11`)

## Documentation

- Guide rapide: [QUICKSTART.md](QUICKSTART.md)
- Onboarding: [docs/ONBOARDING.md](docs/ONBOARDING.md)
- Prerequis: [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md)
- Installation manuelle: [docs/SETUP.md](docs/SETUP.md)
- Tests manuels: [docs/TESTING.md](docs/TESTING.md)
- CLI client ZTNA: [docs/CLI_ZTNA.md](docs/CLI_ZTNA.md)
- Runbook CP/GW: [docs/CP_GW_RUNBOOK.md](docs/CP_GW_RUNBOOK.md)
- Index docs: [docs/INDEX.md](docs/INDEX.md)
- Architecture globale: [ARCHITECTURE.md](ARCHITECTURE.md)

## Compatibilite commandes legacy

Les alias suivants existent temporairement et affichent un message `DEPRECATED`:
- `make init` -> `make up`
- `make check-requirements` -> `make prereq`
- `make apply` -> `make up`
- `make plan` -> `bash scripts/tf-lab plan -var-file=terraform.tfvars`
- `make test-flux2-local` -> `make test-flux2`
