# Requirements - ZTNA Lab

## Système d'Exploitation

- **Linux** : Ubuntu 22.04 LTS (testé et recommandé)
- **RAM** : Minimum 16 GB (8 GB pour 3 VMs, 8 GB pour le host)
- **CPU** : 8 cores minimum (support KVM/VT-x requis)
- **Disque** : 100 GB d'espace libre minimum

Vérifier le support de la virtualisation :
```bash
grep -E 'vmx|svm' /proc/cpuinfo  # Affiche les flags de virtualisation
```

---

## Dépendances Système

### Installation Automatique
```bash
curl -sSL https://raw.githubusercontent.com/your-repo/ZTNA/main/setup.sh | bash
```

### Installation Manuelle

#### 1. Virtualisation KVM/QEMU
```bash
sudo apt update
sudo apt install -y qemu-kvm qemu-system-x86 qemu-utils libvirt-daemon libvirt-clients virt-manager
```

#### 2. Terraform
```bash
wget https://releases.hashicorp.com/terraform/1.14.3/terraform_1.14.3_linux_amd64.zip
unzip terraform_1.14.3_linux_amd64.zip
sudo mv terraform /usr/local/bin/
terraform version
```

Ou via snap :
```bash
sudo snap install terraform --classic
```

#### 3. Dépendances Système Supplémentaires
```bash
sudo apt install -y git curl wget make ssh net-tools vim
```

#### 4. Configuration de l'Utilisateur
```bash
# Ajouter l'utilisateur aux groupes KVM/libvirt
sudo usermod -aG libvirt,kvm $USER
newgrp libvirt

# Vérifier
groups $USER  # Doit afficher : ... libvirt kvm
```

---

## Dépendances Go (Optionnel - pour développement)

### Installation Go 1.21+
```bash
# Télécharger la dernière version
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz

# Ajouter à PATH
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

go version  # Vérifier
```

### Packages Go nécessaires
```bash
go get github.com/golang/protobuf/proto
go get google.golang.org/grpc
go get github.com/urfave/cli/v2
```

---

## Dépendances Python (Optionnel - pour outils)

### Python 3.10+
```bash
sudo apt install -y python3 python3-pip python3-venv

python3 --version
pip3 --version
```

### Packages Python
```bash
pip3 install -r requirements.txt
```

Contenu de `requirements.txt` :
```
ansible>=2.12
pyyaml>=6.0
paramiko>=2.12
```

---

## Configuration libvirt/AppArmor

### Désactiver AppArmor pour libvirt (temporaire)
```bash
sudo ln -sf /etc/apparmor.d/usr.sbin.libvirtd /etc/apparmor.d/disable/
sudo apparmor_parser -R /etc/apparmor.d/usr.sbin.libvirtd 2>/dev/null || true
sudo systemctl restart libvirtd
```

### Ou l'activer avec le bon chemin (permanent)
```bash
echo '  /var/lib/libvirt/images/ztna-lab/** rwk,' | sudo tee /etc/apparmor.d/local/usr.sbin.libvirtd
sudo systemctl reload apparmor
```

---

## Vérification des Prérequis

### Script de Vérification
```bash
#!/bin/bash
set -e

echo "=== Vérification des prérequis ZTNA ==="

# 1. CPU - Virtualisation
echo -n "Virtualisation CPU : "
if grep -E 'vmx|svm' /proc/cpuinfo > /dev/null; then
    echo "✓ OK"
else
    echo "✗ FAIL - VT-x/AMD-V non activé"
    exit 1
fi

# 2. RAM
echo -n "RAM : "
RAM_GB=$(free -g | awk 'NR==2 {print $2}')
if [ "$RAM_GB" -ge 16 ]; then
    echo "✓ OK ($RAM_GB GB)"
else
    echo "⚠ WARN ($RAM_GB GB, recommandé 16 GB)"
fi

# 3. Disque
echo -n "Espace disque : "
DISK_GB=$(df / | awk 'NR==2 {printf "%.0f", $4/1024/1024}')
if [ "$DISK_GB" -ge 100 ]; then
    echo "✓ OK ($DISK_GB GB libre)"
else
    echo "✗ FAIL - Moins de 100 GB libres"
    exit 1
fi

# 4. KVM
echo -n "KVM : "
if modprobe -l | grep -q kvm; then
    echo "✓ OK"
else
    echo "✗ FAIL"
    exit 1
fi

# 5. Terraform
echo -n "Terraform : "
if command -v terraform &> /dev/null; then
    TF_VERSION=$(terraform version | grep Terraform | awk '{print $2}')
    echo "✓ OK ($TF_VERSION)"
else
    echo "✗ FAIL - Non installé"
    exit 1
fi

# 6. Groupes
echo -n "Groupes utilisateur : "
if id -Gn | grep -q 'libvirt'; then
    echo "✓ OK"
else
    echo "✗ FAIL - Utilisateur pas dans le groupe libvirt"
    exit 1
fi

# 7. Libvirt
echo -n "libvirtd : "
if systemctl is-active --quiet libvirtd; then
    echo "✓ OK"
else
    echo "✗ FAIL - Service non actif"
    exit 1
fi

echo -e "\n=== Tous les prérequis sont satisfaits ==="
```

Sauvegarder et exécuter :
```bash
chmod +x check-requirements.sh
./check-requirements.sh
```

---

## Résumé pour Installation Rapide

```bash
# 1. Cloner le repo
git clone https://github.com/your-org/ZTNA.git
cd ZTNA

# 2. Exécuter le setup (installation complète)
./setup.sh

# 3. Vérifier les prérequis
./scripts/check-requirements.sh

# 4. Lancer l'infrastructure
make init

# 5. Vérifier le lab
./scripts/check-lab.sh
```

---

## Dépannage Installation

### Erreur : "permission denied" sur libvirt
```bash
sudo chmod 666 /var/run/libvirt/libvirt-sock
sudo usermod -aG libvirt,kvm $USER
newgrp libvirt
```

### Erreur : "KVM not found"
```bash
# Vérifier BIOS/UEFI - VT-x doit être activé
grep -E 'vmx|svm' /proc/cpuinfo

# Charger les modules KVM
sudo modprobe kvm
sudo modprobe kvm_intel  # Pour Intel
# ou
sudo modprobe kvm_amd    # Pour AMD
```

### Erreur : "AppArmor permission denied"
```bash
# Désactiver temporairement
sudo systemctl stop apparmor
sudo systemctl disable apparmor

# Ou activer le chemin spécifique
echo '  /var/lib/libvirt/images/ztna-lab/** rwk,' | sudo tee -a /etc/apparmor.d/local/usr.sbin.libvirtd
sudo systemctl reload apparmor
```

---

## Version Minimale de Tous les Outils

| Outil | Version Min | Recommandée |
|-------|-------------|-------------|
| Ubuntu | 22.04 LTS | 24.04 LTS |
| QEMU | 7.0 | 8.0+ |
| libvirt | 8.0 | 10.0+ |
| Terraform | 1.0 | 1.14+ |
| Go | 1.18 | 1.21+ |
| Python | 3.8 | 3.11+ |

---

## Fichiers de Configuration Importants

Après installation, vérifier :

```bash
# Fichiers de config
~/.ssh/id_ed25519          # Clé SSH privée (généré par setup.sh)
~/.ssh/id_ed25519.pub      # Clé SSH publique
~/.terraform               # Cache Terraform

# Fichiers système
/etc/libvirt/libvirtd.conf
/etc/apparmor.d/usr.sbin.libvirtd
/var/lib/libvirt/images/ztna-lab/  # Images VMs
```

---

## Support et Aide

- Consulter [SETUP.md](SETUP.md) pour installation manuelle détaillée
- Voir [TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md) pour problèmes courants
- Lire [ARCHITECTURE.md](ARCHITECTURE.md) pour comprendre le design

---

**Dernière mise à jour** : 1 février 2026
