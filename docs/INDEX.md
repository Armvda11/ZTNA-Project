# Index ZTNA Lab

Navigation complète et rapide de tous les fichiers du projet.

---

## Vous êtes NOUVEAU?

1. **[ONBOARDING.md](ONBOARDING.md)** ← Commencez ici ! (5 min)
   - Checklist de setup
   - 4 commandes essentielles
   - Problèmes courants

2. **[QUICKSTART.md](QUICKSTART.md)** ← Puis ici (10 min)
   - Installation étape-par-étape
   - Vérification
   - Dépannage rapide

3. **[README.md](README.md)** ← Ensuite ici (5 min)
   - Vue d'ensemble
   - Architecture
   - Exemples

---

## Documentation Complète

| Fichier | Description | Durée | Public |
|---------|-------------|-------|--------|
| [ONBOARDING.md](ONBOARDING.md) | Setup en 5 min | 5 min | **Nouveaux** |
| [QUICKSTART.md](QUICKSTART.md) | Installation rapide | 10 min | Tous |
| [README.md](README.md) | Vue d'ensemble | 5 min | Tous |
| [REQUIREMENTS.md](REQUIREMENTS.md) | Dépendances | 5 min | Admins |
| [SETUP.md](SETUP.md) | Installation manuelle | 30 min | Avancés |
| [ARCHITECTURE.md](ARCHITECTURE.md) | Design technique | 15 min | **Devs** |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Problèmes & solutions | À la demande | Tous |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Guide contribution | 15 min | **Contributors** |
| [CHANGELOG.md](CHANGELOG.md) | Historique versions | 5 min | Tous |
| [MANIFEST.md](MANIFEST.md) | Explication fichiers | 10 min | Tous |
| [LICENSE](LICENSE) | Licence MIT | 1 min | Tous |

---

## Configuration

| Fichier | Fonction |
|---------|----------|
| [.gitignore](.gitignore) | Fichiers ignorés par Git |
| [.editorconfig](.editorconfig) | Format éditeur cohérent |
| [.pre-commit-config.yaml](.pre-commit-config.yaml) | Vérifications avant commit |
| [Makefile](Makefile) | 40+ commandes utiles |

---

## Scripts

| Script | Fonction | Invocation |
|--------|----------|-----------|
| [setup.sh](setup.sh) | Installation complète | `./setup.sh` |
| [scripts/init-lab.sh](scripts/init-lab.sh) | Crée l'infra | `make init` |
| [scripts/cleanup.sh](scripts/cleanup.sh) | Détruit l'infra | `make destroy` |

---

## Code Infrastructure

| Dossier | Contient |
|---------|----------|
| [lab/terraform/](lab/terraform/) | Infrastructure as Code Terraform |

---

## Par Cas d'Usage

### Je suis NOUVEAU
1. Lire [ONBOARDING.md](ONBOARDING.md)
2. Lancer `./setup.sh` + `make init`
3. Lire [README.md](README.md)

### Je veux CONTRIBUER
1. Lire [CONTRIBUTING.md](CONTRIBUTING.md)
2. Fork le repo
3. Créer une feature branch
4. Lire [ARCHITECTURE.md](ARCHITECTURE.md)

### J'ai un PROBLÈME
1. Chercher dans [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
2. Chercher dans [REQUIREMENTS.md](REQUIREMENTS.md)
3. Voir les logs

### Je veux COMPRENDRE
1. Lire [ARCHITECTURE.md](ARCHITECTURE.md)
2. Lire [MANIFEST.md](MANIFEST.md)
3. Explorer [lab/terraform/](lab/terraform/)

### Je veux DEPLOYER
1. Lire [REQUIREMENTS.md](REQUIREMENTS.md)
2. Lancer [setup.sh](setup.sh)
3. Utiliser [Makefile](Makefile)

---

## Commandes Principales

```bash
# Setup initial
./setup.sh              # Installation + configuration

# Infrastructure
make init               # Créer les 6 VMs + réseaux
make destroy            # Tout détruire
make check              # Vérifier l'état

# SSH
make ssh-client         # Accès wan-client
make ssh-gw             # Accès gateway
make ssh-cp             # Accès control-plane

# Utilitaires
make help               # Liste toutes les commandes
make vm-list            # Lister les VMs
make logs-libvirtd      # Voir les logs
```

---

## 📊 Statistiques Projet

- **16 fichiers essentiels** (zéro redondance)
- **~4000 lignes de documentation** professionnelle
- **40+ cibles Make** pour l'automatisation
- **6 VMs** prêtes à l'emploi
- **3 réseaux isolés** (WAN, DMZ, LAN)
- **100% plug & play** pour les collaborateurs

---

## 🎯 Checklists

### Pour un premier clone
- [ ] Lire [ONBOARDING.md](ONBOARDING.md)
- [ ] Lancer `./setup.sh`
- [ ] Lancer `make init`
- [ ] Lancer `make check`
- [ ] Lire [README.md](README.md)

### Avant de committer
- [ ] Code suit les standards ([CONTRIBUTING.md](CONTRIBUTING.md))
- [ ] Tests passent
- [ ] Documentation mise à jour
- [ ] [CHANGELOG.md](CHANGELOG.md) mis à jour
- [ ] Pas de secrets committé

### Avant une release
- [ ] Tous les tests passent
- [ ] Documentation à jour
- [ ] [CHANGELOG.md](CHANGELOG.md) complété
- [ ] Version bumpée (tag Git)

---

## 🔗 Liens Rapides

**Documentation** :  
[README.md](README.md) | [ONBOARDING.md](ONBOARDING.md) | [ARCHITECTURE.md](ARCHITECTURE.md)

**Guides** :  
[QUICKSTART.md](QUICKSTART.md) | [SETUP.md](SETUP.md) | [REQUIREMENTS.md](REQUIREMENTS.md)

**Contribution** :  
[CONTRIBUTING.md](CONTRIBUTING.md) | [CHANGELOG.md](CHANGELOG.md) | [MANIFEST.md](MANIFEST.md)

**Aide** :  
[TROUBLESHOOTING.md](TROUBLESHOOTING.md) | [LICENSE](LICENSE)

---

## 💡 Pro Tips

1. **Nouveaux ?** → Commencez par [ONBOARDING.md](ONBOARDING.md) (5 min)
2. **Perdu ?** → Ce fichier vous aide à naviguer
3. **En problème ?** → [TROUBLESHOOTING.md](TROUBLESHOOTING.md) a la réponse
4. **Prêt à coder ?** → [CONTRIBUTING.md](CONTRIBUTING.md) vous guide
5. **Besoin de référence ?** → [MANIFEST.md](MANIFEST.md) explique chaque fichier

---

**Dernière mise à jour** : 1 février 2026  
**Projet** : ZTNA Lab - Zero Trust Network Architecture  
**Status** : Production Ready ✅
