# Changelog

Tous les changements notables de ce projet sont documentés dans ce fichier.

Le format est basé sur [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
et ce projet adhère à [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-02-01

### Added
- Initial ZTNA Lab infrastructure
- 6 VMs (wan-client, wan-attacker, ztna-gw, ztna-cp, lan-app, lan-admin)
- 3 isolated networks (WAN, DMZ, LAN)
- Terraform IaC for complete infrastructure
- Automated setup script (`setup.sh`)
- Comprehensive documentation:
  - README.md: Project overview
  - QUICKSTART.md: 4-line quick start
  - SETUP.md: Manual installation guide
  - ARCHITECTURE.md: System design
  - REQUIREMENTS.md: Dependencies
  - TROUBLESHOOTING.md: Common issues & solutions
  - CONTRIBUTING.md: Developer guide
- Make-based command orchestration (40+ targets)
- Shell scripts for lab initialization and cleanup
- EditorConfig for consistent formatting
- Pre-commit hooks for code quality

## [1.1.0] - 2026-02-07

### Added
- Control Plane MVP (JWT, policies, SSH CA, audit, rate limiting)
- E2E test coverage for Control Plane (health, auth, policies, certs, audit)
- Requirements check script (scripts/check-requirements.sh)

### Changed
- Documentation cleanup and link fixes


### Infrastructure
- KVM/QEMU hypervisor support
- libvirt management
- Terraform 1.14.3 with libvirt provider
- Ubuntu 22.04 cloud images with cloud-init
- Ed25519 SSH key authentication
- Cloud-init user provisioning

### Documentation Quality
- Professional GitHub-ready structure
- Clear quick start for collaborators
- Comprehensive troubleshooting (40+ solutions)
- Architecture diagrams and flows
- Code examples and best practices

---

## [Upcoming Features]

### Control Plane Development
- [ ] Policy Decision Point (PDP) API
- [ ] Policy engine implementation
- [ ] Role-Based Access Control (RBAC)
- [ ] Audit logging
- [ ] Admin dashboard

### Gateway Development
- [ ] Policy Enforcement Point (PEP)
- [ ] Proxy/reverse proxy functionality
- [ ] Session management
- [ ] Zero Trust verification
- [ ] Performance monitoring

### Infrastructure Enhancements
- [ ] Ansible provisioning (optional)
- [ ] Docker support for dev environment
- [ ] Multi-environment support (dev, staging, prod)
- [ ] Monitoring stack (Prometheus, Grafana)
- [ ] Logging stack (ELK)

### Documentation
- [ ] API reference documentation
- [ ] Deployment guides (AWS, Azure, GCP)
- [ ] Security guidelines
- [ ] Performance tuning guide

---

## Notes for Contributors

When adding changes, update this file with:
- New section for each version
- Subsections: Added, Changed, Fixed, Removed, Deprecated, Security
- Links to related issues (#123)
- Release date in ISO 8601 format (YYYY-MM-DD)

Example:
```markdown
## [1.1.0] - 2026-03-15

### Added
- New feature description (#123)

### Fixed
- Bug fix description (#456)

### Security
- Security issue description
```
