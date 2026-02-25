# Index Documentation ZTNA Lab

## Demarrer vite

1. [ONBOARDING.md](ONBOARDING.md)
2. [../QUICKSTART.md](../QUICKSTART.md)
3. [../README.md](../README.md)

## Guides principaux

- [ONBOARDING.md](ONBOARDING.md)
- [../QUICKSTART.md](../QUICKSTART.md)
- [REQUIREMENTS.md](REQUIREMENTS.md)
- [SETUP.md](SETUP.md)
- [TESTING.md](TESTING.md)
- [CLI_ZTNA.md](CLI_ZTNA.md)
- [CP_GW_RUNBOOK.md](CP_GW_RUNBOOK.md)
- [CP_GW_FEATURES_TEST_MATRIX.md](CP_GW_FEATURES_TEST_MATRIX.md)
- [../ARCHITECTURE.md](../ARCHITECTURE.md)
- [../TROUBLESHOOTING.md](../TROUBLESHOOTING.md)

## Scripts utilises

- [../setup.sh](../setup.sh)
- [../scripts/check-requirements.sh](../scripts/check-requirements.sh)
- [../scripts/lab-up-simple.sh](../scripts/lab-up-simple.sh)
- [../scripts/deploy-control-plane.sh](../scripts/deploy-control-plane.sh)
- [../scripts/deploy-gateway.sh](../scripts/deploy-gateway.sh)

## Commandes recommandees

```bash
make prereq
make quickstart
make check
make test-flux1
make test-flux1-auto
make test-flux2
make test-crl-routing
make test-cp-gw-lab
```
