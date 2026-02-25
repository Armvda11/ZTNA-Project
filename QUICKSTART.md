# ZTNA Lab - Quickstart

## 1. Prerequis

```bash
make prereq
```

Si des outils manquent, installer via:

```bash
./setup.sh
newgrp libvirt
```

## 2. Parcours unique recommande

```bash
make quickstart
```

Ce parcours cree l'infra puis deploie control-plane et gateway.

## 3. Verification rapide

```bash
make check
make status
```

Checks attendus:
- VMs visibles
- SSH vers `wan-client`, `ztna-gw`, `ztna-cp`
- `https://10.10.20.30:8080/healthz` joignable

## 4. Tests fonctionnels

```bash
make test-flux1
make test-flux2
make test-crl-routing
```

## 5. Connexions SSH utiles

```bash
make ssh-client
make ssh-gw
make ssh-cp
make ssh-app
make ssh-admin
```

## 6. Nettoyage

```bash
make destroy
```
