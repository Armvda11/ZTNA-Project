# ZTNA Gateway (Policy Enforcement Point)

SSH Gateway pour l'infrastructure Zero Trust Network Access.

## Version

**0.1.0** - Phase 1: Basic SSH Server

## Fonctionnalités

### Implémenté
- [x] Configuration YAML
- [x] Logging structuré (JSON/text)
- [x] Client HTTP vers Control Plane
- [x] SSH server de base
- [x] Health check Control Plane

### En Développement
- [ ] Validation des certificats SSH
- [ ] SSH proxy (WAN → LAN)
- [ ] Policy enforcement
- [ ] Session logging
- [ ] Routing dynamique

## Architecture

```
WAN Client → Gateway (SSH) → Control Plane (policies) → Backend SSH Servers
   |               |              |                           |
10.10.10.10    10.10.20.20    10.10.20.30            10.10.30.10/11
```

## Installation

### 1. Compiler

```bash
cd gateway
go build -o ztna-gw main.go
```

### 2. Générer Host Key

```bash
sudo mkdir -p /etc/ztna
sudo ssh-keygen -t ed25519 -f /etc/ztna/gateway_host_key -N ''
sudo chown ztna:ztna /etc/ztna/gateway_host_key*
```

### 3. Déployer

```bash
./deploy.sh
```

## Configuration

Fichier: `config.yaml`

```yaml
server:
  ssh_port: 2222  # SSH proxy port

controlplane:
  url: "https://10.10.20.30:8443"
  
routing:
  targets:
    - name: "lan-app"
      host: "10.10.30.10"
      port: 22
```

## Tests

### Test SSH Connection

```bash
# Depuis wan-client (10.10.10.10)
ssh -p 2222 alice@10.10.20.20
```

### Test Health Check

```bash
curl http://10.10.20.20:9090/health
```

## Logs

```bash
# Voir les logs du service
sudo journalctl -u ztna-gw -f

# Logs JSON
{"timestamp":"2026-02-05T00:00:00Z","level":"INFO","message":"SSH connection established","user":"alice"}
```

## Roadmap

### v0.1.0 (Current) - SSH Server
- Accept SSH connections
- Log connections
- Basic health check endpoint

### v0.2.0 - Certificate Validation
- [ ] Validate SSH certs signed by Control Plane CA
- [ ] Extract user identity from cert
- [ ] Reject invalid certs

### v0.3.0 - SSH Proxy
- [ ] Forward SSH traffic to backend servers
- [ ] Support shell, exec, subsystem
- [ ] Handle terminal sessions

### v0.4.0 - Policy Enforcement
- [ ] Query Control Plane for access decisions
- [ ] Enforce allow/deny policies
- [ ] Audit all access attempts

### v1.0.0 - Production Ready
- [ ] Session timeout enforcement
- [ ] Metrics & monitoring
- [ ] Rate limiting
- [ ] Connection pooling

## Troubleshooting

### Gateway ne démarre pas

```bash
# Vérifier la config
./ztna-gw -config config.yaml

# Vérifier que le host key existe
ls -la /etc/ztna/gateway_host_key
```

### SSH handshake échoue

```bash
# Vérifier les logs
sudo journalctl -u ztna-gw -n 50

# Tester avec verbose
ssh -vvv -p 2222 user@10.10.20.20
```

### Control Plane injoignable

```bash
# Test manuel
curl -k https://10.10.20.30:8443/health

# Vérifier network routing
ping 10.10.20.30
```

## Développement

```bash
# Build
go build -o ztna-gw main.go

# Run locally
./ztna-gw -config config.yaml

# Tests (TODO)
go test ./...
```

## Sécurité

- TLS pour Control Plane
- Logging de toutes les connexions
- SSH certificate validation (v0.2.0)
- Policy enforcement (v0.4.0)

## License

MIT
