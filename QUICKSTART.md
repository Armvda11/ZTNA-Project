# ZTNA Lab - Guide de demarrage rapide

## Etape 1 - Prerequis (2 min)

```bash
grep -E 'vmx|svm' /proc/cpuinfo
free -h | grep Mem
df -h /
```

Details: [docs/REQUIREMENTS.md](docs/REQUIREMENTS.md)

## Etape 2 - Installation automatique (5-10 min)

```bash
git clone https://github.com/your-org/ZTNA.git
cd ZTNA
./setup.sh
newgrp libvirt
make check-requirements
```

## Etape 3 - Creer le lab (5-10 min)

```bash
make init
```

## Etape 4 - Verifier (2 min)

```bash
make check
```

## Acces rapide SSH

```bash
make ssh-client   # 10.10.10.10
make ssh-gw       # 10.10.10.20
make ssh-cp       # 10.10.20.30
make ssh-app      # 10.10.30.10
make ssh-admin    # 10.10.30.11
```

## Commandes utiles

```bash
make status
make vm-start
make vm-stop
make destroy
make help
```

## Suite

- Architecture: [ARCHITECTURE.md](ARCHITECTURE.md)
- Depannage: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- Installation manuelle: [docs/SETUP.md](docs/SETUP.md)
