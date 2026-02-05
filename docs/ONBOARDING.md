# Onboarding - ZTNA Lab

**Bienvenue !** Vous êtes un nouveau collaborateur ? Vous êtes au bon endroit.  
Ce fichier vous guide en **< 5 minutes** pour avoir une infra opérationnelle.

---

## TL;DR (30 secondes)

```bash
# 1. Clone le repo
git clone https://github.com/your-org/ZTNA.git
cd ZTNA

# 2. Lance l'installation (prend ~15 min)
./setup.sh

# 3. Crée l'infrastructure (prend ~10 min)
make init

# 4. Vérifie que tout marche
make check

# 5. C'est bon !
```

**Résultat** : 6 VMs opérationnelles, SSH accessible, prêt à développer !

---

## Guides Rapides

### Je suis IMPATIENT
→ Voir **[QUICKSTART.md](QUICKSTART.md)** (4 étapes, 10-15 min)

### Je suis PERDU
→ Voir **[README.md](README.md)** (vue d'ensemble)

### J'ai un PROBLÈME
→ Voir **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** (40+ solutions)

### Je veux CONTRIBUER
→ Voir **[CONTRIBUTING.md](CONTRIBUTING.md)** (guide complet)

### Je veux COMPRENDRE l'ARCHITECTURE
→ Voir **[ARCHITECTURE.md](ARCHITECTURE.md)** (design détaillé)

---

## Checklist d'Onboarding

- [ ] **Cloner le repo**
  ```bash
  git clone https://github.com/your-org/ZTNA.git
  cd ZTNA
  ```

- [ ] **Vérifier les prérequis** (2 min)
  ```bash
  # Doit afficher des lignes vmx ou svm
  grep -E 'vmx|svm' /proc/cpuinfo
  
  # Doit afficher 16G+
  free -h | grep Mem
  
  # Doit afficher 100G+
  df -h / | grep -v Used
  ```

- [ ] **Lancer l'installation** (15 min)
  ```bash
  chmod +x setup.sh
  ./setup.sh
  # Attendre la fin (du café ☕)
  ```

- [ ] **Créer l'infrastructure** (10 min)
  ```bash
  make init
  # Attendre que toutes les VMs démarrent
  ```

- [ ] **Vérifier que tout marche**
  ```bash
  make check
  # Doit afficher: All 6 VMs running
  #              All networks active
  #              SSH connectivity OK
  ```

- [ ] **Accéder à une VM**
  ```bash
  make ssh-client
  # Ou manuellement
  ssh ztna@10.10.10.10
  ```

- [ ] **Lire la documentation**
  - [ ] [README.md](README.md) - Vue d'ensemble
  - [ ] [ARCHITECTURE.md](ARCHITECTURE.md) - Design
  - [ ] [CONTRIBUTING.md](CONTRIBUTING.md) - Contribution

- [ ] **Faire un premier commit** (optionnel)
  ```bash
  # Configurer Git
  git config user.name "Your Name"
  git config user.email "your@email.com"
  
  # Créer une branche
  git checkout -b feature/my-first-feature
  
  # Faire un changement, commit, push
  git add file.md
  git commit -m "feat: Add my first feature"
  git push -u origin feature/my-first-feature
  ```

---

## Commandes Essentielles

**Vous n'en retiendrez probablement que 3-4, et c'est normal !**

```bash
# Créer l'infra
make init

# Vérifier que tout marche
make check

# SSH vers une VM
make ssh-client      # Client WAN
make ssh-gw          # Gateway ZTNA
make ssh-cp          # Control Plane

# Voir tous les VMs
make vm-list

# Redémarrer
make vm-reboot

# Voir les logs
make logs-libvirtd

# Nettoyer
make destroy         # Détruit l'infra (attention!)
make clean-all       # Nettoie TOUT (attention!!!)

# Aide
make help
```

---

## Problèmes Courants

### "Permission denied"
```bash
# Ajouter votre user au groupe libvirt
newgrp libvirt

# Ou redémarrer la session
# (déconnexion + reconnexion)
```

### "VT-x not detected"
- Redémarrer l'ordinateur
- Aller dans le BIOS (F2 ou Del au démarrage)
- Activer "Virtualization Technology" ou "VT-x"
- Redémarrer

### "SSH connection refused"
```bash
# Attendre 60 secondes (cloud-init démarre les VMs)
sleep 60
make check
```

### setup.sh échoue
```bash
# Voir l'erreur complète
bash -x ./setup.sh 2>&1 | tail -50

# Ou faire l'installation manuelle
cat SETUP.md
```

**Plus de problèmes ?** → [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

---

## Prochaines Étapes

Après le setup initial, vous pouvez :

### 1. **Explorer l'infrastructure**
```bash
# Lister les VMs
virsh list --all

# Voir les réseaux
virsh net-list --all

# Accéder à une console
virsh console wan-client
# Ctrl+] pour quitter
```

### 2. **Tester la connectivité**
```bash
make ssh-client
ping 10.10.10.20 (gateway)
ping 10.10.30.10 (lan-app)
```

### 3. **Lire le design**
```bash
# Comprendre l'architecture
cat ARCHITECTURE.md

# Voir ce que fait chaque fichier
cat MANIFEST.md
```

### 4. **Contribuer**
```bash
# Créer une feature
git checkout -b feature/my-feature
# ... faire des changements ...
git commit -m "feat: Add my feature"
git push -u origin feature/my-feature

# Créer une PR sur GitHub
```

---

## Questions ?

1. **Vérifier la FAQ** → [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
2. **Lire la documentation** → [CONTRIBUTING.md](CONTRIBUTING.md)
3. **Chercher dans les logs** :
   ```bash
   sudo journalctl -u libvirtd -n 50
   ```
4. **Créer une issue** sur GitHub si aucune solution

---

## Résumé

| Étape | Commande | Durée | Résultat |
|-------|----------|-------|----------|
| 1 | `git clone ...` | 1 min | Code local |
| 2 | `./setup.sh` | 15 min | Outils installés |
| 3 | `make init` | 10 min | 6 VMs running |
| 4 | `make check` | 1 min | Vérification |
| **Total** | | **27 min** | **Infra prête** |

---

## Vous Êtes Prêt !

Maintenant, vous pouvez :
- Développer dans le Control Plane (`control-plane/`)
- Développer dans le Gateway (`gateway/`)
- Modifier l'infrastructure (`lab/terraform/`)
- Contribuer du code via des PRs

**Bienvenue dans l'équipe ZTNA Lab !**

---

**Besoin d'aide ?** Consultez [README.md](README.md)  
**Prêt à contribuer ?** Consultez [CONTRIBUTING.md](CONTRIBUTING.md)
