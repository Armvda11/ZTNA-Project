# Guide de Tests Manuels - ZTNA Lab

## 1. Checks de base

```bash
make check
make status
```

Attendu:
- VMs visibles dans `virsh list --all`
- SSH OK vers `wan-client`, `ztna-gw`, `ztna-cp`
- Health CP OK sur `/healthz`

## 2. Verification control-plane

```bash
# Keycloak
curl -sf http://10.10.20.30:8081/realms/ztna >/dev/null && echo "Keycloak OK"

# Health CP
curl -sfk https://10.10.20.30:8080/healthz && echo "CP healthz OK"

# Endpoints PKI publics
curl -sfk https://10.10.20.30:8080/pki/ssh-ca/pubkey | head -n 1
curl -sfk https://10.10.20.30:8080/pki/device-ca/cert | head -n 1
```

## 3. Verification gateway

```bash
ssh -o StrictHostKeyChecking=no ztna@10.10.10.20 "sudo systemctl is-active ztna-gateway"
ssh -o StrictHostKeyChecking=no ztna@10.10.10.20 "sudo ss -tlnp | grep 4433"
```

## 4. Flux fonctionnels automatiques

```bash
make test-flux1
make test-flux1-auto
make test-flux2
make test-crl-routing
make test-pep-register
make test-cp-gw-lab
```

Pour la matrice complete "fonctionnalite -> test -> preuve", voir:
- `docs/CP_GW_FEATURES_TEST_MATRIX.md`

## 5. Flux de deploiement pas a pas

```bash
make up
make deploy
make deploy-gw
make check
```

## 6. Logs utiles

```bash
make logs-cp
make logs-gw
```

## 7. Depannage rapide

- Si SSH KO juste apres `make up`: attendre 30-60s puis relancer `make check-ssh`.
- Si CP KO: verifier `make logs-cp` puis endpoint `https://10.10.20.30:8080/healthz`.
- Si gateway KO: verifier `make logs-gw` et la connectivite CP depuis `ztna-gw`.

## 8. Nettoyage / reset

```bash
make destroy
make up
make deploy
make deploy-gw
make check
```
