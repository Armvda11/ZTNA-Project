# Guide de Contribution au Projet ZTNA

Merci de votre intérêt pour contribuer au projet ZTNA Lab ! Ce document décrit comment configurer votre environnement de développement, contribuer du code, et maintenir les bonnes pratiques du projet.

## Table des Matières

- [Code de Conduite](#code-de-conduite)
- [Configuration du Développement](#configuration-du-développement)
- [Flux de Travail](#flux-de-travail)
- [Bonnes Pratiques](#bonnes-pratiques)
- [Structure du Code](#structure-du-code)
- [Tester votre Code](#tester-votre-code)
- [Documentation](#documentation)
- [Processus de Review](#processus-de-review)

---

## Code de Conduite

Nous nous engageons à fournir un environnement accueillant et inspirant pour tous les contributeurs. Veuillez :

- Traiter tout le monde avec respect
- Être constructif dans les critiques
- Accueillir les perspectives différentes
- Signaler les comportements abusifs aux mainteneurs

---

## Configuration du Développement

### 1. Cloner le Projet

```bash
git clone https://github.com/your-org/ZTNA.git
cd ZTNA
```

### 2. Configurer votre Environnement

```bash
# Installation automatique
./setup.sh

# Ou manuel
cat docs/REQUIREMENTS.md
cat docs/SETUP.md
```

### 3. Initialiser le Lab

```bash
# Créer l'infrastructure de test
make init

# Vérifier que tout fonctionne
make check
```

### 4. Installer les Outils de Développement

```bash
# Go (pour Control Plane et Gateway)
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Terraform (pour infrastructure)
# (Déjà installé par setup.sh)

# Ansible (optionnel, pour configuration)
pip3 install ansible

# Python dev tools
pip3 install pylint black pytest
```

### 5. Configurer Git

```bash
# Ajouter votre signature
git config user.name "Your Name"
git config user.email "your.email@example.com"

# Optionnel : ajouter la signature GPG
# git config user.signingkey YOUR_GPG_KEY
# git config commit.gpgsign true
```

---

## Flux de Travail

### Option Rapide: Assistant Git (recommandé)

Pour éviter de taper beaucoup de commandes Git manuelles:

```bash
# Interface interactive colorée
make git

# Ou commandes directes
make git-start TYPE=feat NAME=auth-cache
make git-sync
make git-commit TYPE=feat MSG="Add auth cache"
make git-publish
make git-finish
```

Commande agent-friendly:

```bash
make git-agent
```

Auto-check à l'ouverture du terminal VSCode:

```bash
./scripts/git-assistant.sh on-open
```

Vous pouvez l'attacher à un profil terminal VSCode (workspace) avec:

```json
{
  "terminal.integrated.profiles.linux": {
    "ZTNA Git Shell": {
      "path": "bash",
      "args": [
        "-lc",
        "cd \"${workspaceFolder}\" && ./scripts/git-assistant.sh on-open; exec bash -l"
      ]
    }
  },
  "terminal.integrated.defaultProfile.linux": "ZTNA Git Shell"
}
```

### 1. Créer une Branche

```bash
# Pour une feature
git checkout -b feature/description-feature

# Pour un bug
git checkout -b fix/description-bug

# Pour de la documentation
git checkout -b docs/description-documentation

# Pour une refactoring
git checkout -b refactor/description-refactoring
```

### 2. Effectuer des Changements

```bash
# Travailler sur votre code

# Vérifier vos changements
git status
git diff

# Ajouter vos fichiers
git add file1.go file2.sh

# Ou ajouter tout
git add .
```

### 3. Écrire un Commit Message

```bash
# Format
git commit -m "Type: Description courte

Description détaillée si nécessaire
- Point 1
- Point 2

Fixes #123
Related to #456"

# Types acceptés
# - feat: nouvelle feature
# - fix: correction de bug
# - docs: documentation
# - refactor: refactorisation
# - test: tests
# - chore: maintenance
# - perf: performance
```

**Exemple** :
```bash
git commit -m "feat: Add policy decision endpoint

Implements the policy decision point (PDP) API
- GET /api/v1/decisions
- POST /api/v1/decisions
- Full unit tests
- API documentation

Fixes #42"
```

### 4. Pusher vos Changements

```bash
# Pousser votre branche
git push origin feature/description-feature

# Ou si c'est la première fois
git push -u origin feature/description-feature
```

### 5. Créer une Pull Request

1. Allez sur GitHub
2. Cliquez sur "New Pull Request"
3. Sélectionnez votre branche
4. Remplissez le template PR avec :
   - **Description** : Qu'est-ce que changez ? Pourquoi ?
   - **Testing** : Comment tester ?
   - **Issues** : Quels tickets cela ferme-t-il ?
   - **Checklist** : Avez-vous testé ? Documentation à jour ?

**Template PR** :
```markdown
## Description
Courte description de ce PR

## Type de Changement
- [ ] Nouvelle Feature
- [ ] Correction de Bug
- [ ] Documentation
- [ ] Refactoring

## Testing
- [ ] J'ai testé localement
- [ ] Tests unitaires passent
- [ ] Couverture de code vérifiée

## Documentation
- [ ] Documentation mise à jour
- [ ] Changelog mis à jour
- [ ] Architecture documentée si nécessaire

## Checklist
- [ ] J'ai rebasé sur main
- [ ] Commits sont propres
- [ ] Pas de fichiers inutiles committés

Fixes #123
```

---

## Bonnes Pratiques

### 1. Code Bash

```bash
# ✅ BON
#!/bin/bash
set -euo pipefail

main() {
    local var="value"
    
    if [[ ! -f "$var" ]]; then
        echo "Error: file not found"
        return 1
    fi
    
    # Utiliser des quotes
    echo "$var"
}

main "$@"
```

```bash
# ❌ MAUVAIS
#!/bin/bash

set -e  # Pas assez strict

var=value  # Pas de quotes

if [ ! -f $var ]; then  # Pas de double brackets
    echo Error: file not found
    exit 1
fi
```

### 2. Code Go

```go
// ✅ BON
package main

import (
    "fmt"
    "log"
)

// PolicyDecision represents a policy decision
type PolicyDecision struct {
    Allow  bool
    Reason string
}

// MakeDecision evaluates if access is allowed
func MakeDecision(user, resource string) (PolicyDecision, error) {
    if user == "" {
        return PolicyDecision{}, fmt.Errorf("user cannot be empty")
    }
    
    return PolicyDecision{Allow: true}, nil
}

func main() {
    decision, err := MakeDecision("user@example.com", "resource")
    if err != nil {
        log.Fatalf("Failed to make decision: %v", err)
    }
    
    fmt.Printf("Decision: %+v\n", decision)
}
```

```go
// ❌ MAUVAIS
package main

import "fmt"

type Decision struct {
    allow bool  // Pas d'export
    reason string  // Pas de doc
}

func makeDecision(user, resource string) Decision {  // Pas d'error handling
    return Decision{allow: true}
}

func main() {
    d := makeDecision("user", "resource")  // Pas de vérification d'erreur
    fmt.Println(d)
}
```

### 3. Terraform

```hcl
# ✅ BON
terraform {
  required_version = ">= 1.0"
  required_providers {
    libvirt = {
      source  = "dmacvicar/libvirt"
      version = ">= 0.7"
    }
  }
}

# Variables bien documentées
variable "vm_count" {
  description = "Number of VMs to create"
  type        = number
  default     = 6
  
  validation {
    condition     = var.vm_count > 0
    error_message = "vm_count must be > 0"
  }
}

# Resources avec locals pour réutilisation
locals {
  vm_config = {
    wan-client = {
      network = "wan-net"
      ip      = "10.10.10.10"
    }
  }
}

resource "libvirt_domain" "vm" {
  for_each = local.vm_config
  
  name   = each.key
  memory = 2048
  vcpu   = 4
  
  tags = {
    Environment = "development"
    Project     = "ztna"
  }
}
```

```hcl
# ❌ MAUVAIS
terraform {
  required_version = "1.0"  # Trop strict
}

# Pas de validation
variable "vm_count" {
  type = number
}

# IP codée en dur
resource "libvirt_domain" "wan_client" {
  name = "wan-client"
  # ...
}

resource "libvirt_domain" "ztna_gw" {
  name = "ztna-gw"
  # ... copie-collé
}
```

### 4. Documentation

```markdown
# ✅ BON
## Installation

Prerequisites:
- Ubuntu 22.04+ (see docs/REQUIREMENTS.md)
- 16GB RAM minimum

Steps:
1. Clone the repo
   ```bash
   git clone https://github.com/your-org/ZTNA.git
   ```

2. Run setup
   ```bash
   ./setup.sh
   ```

3. Create infrastructure
   ```bash
   make init
   ```

## Troubleshooting

If you encounter issues, see TROUBLESHOOTING.md
```

```markdown
# ❌ MAUVAIS
## How to use

Just run setup.sh lol

If it doesn't work, try rebooting
```

---

## Structure du Code

```
ZTNA/
├── control-plane/              # Control Plane (Policy Decision Point)
│   ├── main.go                # Entrée principale
│   ├── api/                   # Endpoints HTTP/HTTPS
│   │   ├── decisions.go       # API de décisions
│   │   └── policies.go        # API de gestion de politiques
│   ├── core/                  # Logique métier
│   │   ├── engine.go          # Moteur de décision
│   │   └── policy.go          # Structure des politiques
│   ├── test/                  # Tests
│   │   ├── engine_test.go
│   │   └── api_test.go
│   ├── go.mod                 # Dépendances
│   ├── Dockerfile             # Image Docker
│   └── README.md              # Documentation
│
├── gateway/                    # Gateway ZTNA (Policy Enforcement Point)
│   ├── main.go
│   ├── proxy/                 # Proxy logic
│   ├── auth/                  # Authentication
│   ├── test/
│   ├── go.mod
│   ├── Dockerfile
│   └── README.md
│
├── lab/                        # Infrastructure de lab
│   ├── terraform/             # Infrastructure as Code
│   │   ├── main.tf            # Configuration principale
│   │   ├── networks.tf        # Réseaux
│   │   ├── vms.tf             # VMs
│   │   ├── variables.tf       # Variables
│   │   ├── outputs.tf         # Outputs
│   │   └── terraform.tfvars   # Valeurs par défaut
│   └── ansible/               # Configuration (optionnel)
│
├── scripts/                    # Scripts utilitaires
│   ├── setup.sh               # Installation
│   ├── init-lab.sh            # Initialisation lab
│   ├── lab-up.sh              # Tout-en-un (init + apply + check)
│   └── cleanup.sh             # Nettoyage
│
├── docs/                       # Documentation
│   ├── REQUIREMENTS.md        # Dépendances
│   ├── SETUP.md               # Guide d'installation
│   ├── ARCHITECTURE.md        # Architecture
│   ├── TROUBLESHOOTING.md     # Dépannage
│   ├── API.md                 # API REST
│   └── DEVELOPMENT.md         # Développement
│
├── .github/                    # Actions GitHub
│   └── workflows/
│       ├── test.yml           # Tests
│       └── release.yml        # Release
│
├── .gitignore
├── Makefile                    # Tâches principales
├── README.md                   # Accueil
├── docker-compose.yml          # Dev environment
└── CONTRIBUTING.md            # Ce fichier
```

---

## Tester votre Code

### Tests Bash

```bash
# Utiliser bats pour tester des scripts bash
pip3 install bats

# Créer tests/test-setup.sh
#!/usr/bin/env bats

@test "setup.sh checks virtualization" {
    # Le test doit vérifier que grep vmx/svm fonctionne
    grep -E 'vmx|svm' /proc/cpuinfo
}

# Exécuter les tests
bats tests/*.sh
```

### Tests Go

```bash
# Écrire des tests
cd control-plane
cat > core/engine_test.go <<'EOF'
package core

import (
    "testing"
)

func TestMakeDecision(t *testing.T) {
    decision := MakeDecision("user", "resource")
    if !decision.Allow {
        t.Error("Expected decision to allow access")
    }
}
EOF

# Exécuter les tests
go test ./...

# Avec couverture
go test -cover ./...
```

### Tests Terraform

```bash
# Valider la syntaxe Terraform
cd lab/terraform
terraform fmt -check .
terraform validate

# Plan pour vérifier
terraform plan -var-file=terraform.tfvars
```

### Tests d'Intégration

```bash
# Utiliser le Makefile
make clean-all  # Nettoyer complètement
make init       # Créer l'infra
make check      # Vérifier que tout fonctionne
make test-network  # Tester la connectivité
```

---

## Documentation

### Ajouter une Feature

Créer un fichier `docs/feature-name.md` :

```markdown
# Feature: Policy Decision

## Overview
Brief description of the feature

## Architecture
How does it work internally?

## Usage
```bash
# Code example
```

## API Reference
Document the API endpoints

## Configuration
How to configure it?

## Testing
How to test it?

## Known Limitations
What doesn't work yet?

## Future Improvements
What's planned?
```

### Mettre à Jour le README

1. Vérifier que votre feature est expliquée
2. Ajouter des exemples d'utilisation
3. Mettre à jour la table des matières
4. Ajouter des liens vers la documentation détaillée

### Mettre à Jour le CHANGELOG

```markdown
## [1.1.0] - 2026-02-15

### Added
- New policy decision endpoint
- Support for RBAC policies

### Fixed
- Fixed authentication bug in gateway
- Improved error logging

### Changed
- Updated Terraform provider version

### Deprecated
- Old policy format (migrate using tool X)

### Security
- Fixed critical SQL injection vulnerability
```

---

## Processus de Review

### Ce que les Reviewers Vérifieront

1. **Qualité du Code**
   - Suit les bonnes pratiques
   - Cohérent avec le reste du codebase
   - Pas de code copié-collé

2. **Tests**
   - Tests unitaires présents
   - Couverture acceptable (>80%)
   - Tests d'intégration réussissent

3. **Documentation**
   - Code commenté quand c'est nécessaire
   - README/docs mis à jour
   - API documentée

4. **Performance**
   - Pas de régressions
   - Optimisations si applicables

5. **Sécurité**
   - Pas de secrets committé
   - Pas de vulnérabilités évidentes
   - Validation des entrées

### Répondre aux Commentaires

1. Lire attentivement les commentaires
2. Demander des clarifications si nécessaire
3. Faire les changements demandés
4. Répondre aux commentaires
5. Forcer le push de votre branche

```bash
git add .
git commit -m "Address review feedback"
git push origin feature/description-feature
```

### Après l'Approbation

1. Rebasez sur main si nécessaire
2. Le mainteneur mergera votre PR
3. Votre branche sera supprimée
4. Mettez à jour votre local

```bash
git checkout main
git pull origin main
git branch -d feature/description-feature
```

---

## Ressources Utiles

- [Go Documentation](https://pkg.go.dev/)
- [Terraform Docs](https://www.terraform.io/docs)
- [Bash Best Practices](https://mywiki.wooledge.org/BashGuide)
- [GitHub Docs](https://docs.github.com/)
- [Our Architecture](ARCHITECTURE.md)
- [Troubleshooting Guide](TROUBLESHOOTING.md)

---

**Version** : 1.0  
**Dernière mise à jour** : 1 février 2026  
**Mainteneurs** : ZTNA Lab Team
