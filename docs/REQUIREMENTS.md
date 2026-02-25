# Requirements ZTNA Lab

## OS supporte

- Ubuntu 22.04 LTS ou 24.04 LTS

## Ressources minimum

- CPU avec virtualisation activee (VT-x ou AMD-V)
- RAM: 16 GB recommandes
- Disque: 100 GB libres minimum

Verification rapide:

```bash
grep -E 'vmx|svm' /proc/cpuinfo
free -h | grep Mem
df -h /
```

## Outils requis pour `make quickstart`

- `terraform`
- `virsh`
- `ssh` / `scp`
- `curl`
- `go` (build local des binaires CP/GW)

Verification automatique:

```bash
make prereq
# ou:
./scripts/check-requirements.sh
```

## Outils optionnels (tests/debug)

- `openssl`
- `python3`
- `jq`

## Installation assistee

```bash
./setup.sh
newgrp libvirt
make prereq
```

## Remarques importantes

- L'utilisateur doit appartenir au groupe `libvirt`.
- Si `libvirtd` est inactif:

```bash
sudo systemctl start libvirtd
```
