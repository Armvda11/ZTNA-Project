# ZTNA Lab - Guide d'Installation Manuel

**Alternative** : Pour une installation automatique, utilisez `./setup.sh`

## Table des Matières
1. [Vérifier les Prérequis](#vérifier-les-prérequis)
2. [Installer les Dépendances Système](#installer-les-dépendances-système)
3. [Configurer KVM/libvirt](#configurer-kvmlibvirt)
4. [Installer Terraform](#installer-terraform)
5. [Initialiser le Lab](#initialiser-le-lab)
6. [Dépannage](#dépannage)

---

## Vérifier les Prérequis

### 1. Système d'Exploitation

```bash
# Vérifier la version Ubuntu
lsb_release -a

# Doit être 22.04 LTS ou 24.04 LTS
```

### 2. CPU et Virtualisation

```bash
# Vérifier la présence de VT-x (Intel) ou AMD-V (AMD)
grep -E 'vmx|svm' /proc/cpuinfo

# Doit afficher plusieurs lignes avec "vmx" ou "svm"
# Si rien n'apparaît, activez la virtualisation dans le BIOS
```

### 3. RAM et Disque

```bash
# Vérifier la RAM (minimum 16 GB recommandé)
free -h | grep Mem
# Résultat : 16Gi ou plus

# Vérifier l'espace disque (minimum 100 GB)
df -h /
# Résultat : 100G+ Available
```

---

## Installer les Dépendances Système

### 1. Mettre à Jour le Système

```bash
sudo apt update
sudo apt upgrade -y
```

### 2. Installer KVM et QEMU

```bash
sudo apt install -y \
    qemu-kvm \
    qemu-system-x86 \
    qemu-utils \
    libvirt-daemon \
    libvirt-clients \
    libvirt-daemon-system \
    virt-manager \
    apparmor \
    apparmor-utils
```

### 3. Installer les Outils Utilitaires

```bash
sudo apt install -y \
    git \
    curl \
    wget \
    make \
    ssh \
    net-tools \
    vim \
    cloud-utils \
    cloud-initramfs-growroot \
    python3 \
    python3-pip
```

### 4. Vérifier l'Installation KVM

```bash
# Vérifier que QEMU fonctionne
qemu-system-x86_64 --version

# Vérifier que libvirt fonctionne
virsh --version

# Doit afficher les versions sans erreur
```

---

## Configurer KVM/libvirt

### 1. Charger les Modules KVM

```bash
# Charger le module KVM
sudo modprobe kvm

# Charger le module spécifique à votre processeur
# Pour Intel
sudo modprobe kvm_intel

# Pour AMD
sudo modprobe kvm_amd

# Vérifier que les modules sont chargés
lsmod | grep kvm
# Doit afficher kvm et kvm_intel/kvm_amd
```

### 2. Ajouter l'Utilisateur aux Groupes

```bash
# Ajouter aux groupes libvirt et kvm
sudo usermod -aG libvirt,kvm $USER

# Vérifier
groups $USER
# Doit afficher ... libvirt kvm

# Important : Recharger les groupes dans la session actuelle
newgrp libvirt

# Ou se reconnecter complètement
# Puis vérifier que virsh fonctionne sans sudo
virsh list --all
```

### 3. Démarrer libvirtd

```bash
# Démarrer le service
sudo systemctl start libvirtd

# L'activer au démarrage
sudo systemctl enable libvirtd

# Vérifier le statut
sudo systemctl status libvirtd

# Doit afficher "active (running)"
```

### 4. Fixer les Permissions du Socket

```bash
# Le socket libvirt doit être accessible
sudo chmod 666 /var/run/libvirt/libvirt-sock

# Vérifier
ls -l /var/run/libvirt/libvirt-sock
# Doit afficher rw-rw-rw-
```

### 5. Configurer AppArmor

```bash
# Créer le répertoire de configuration local
sudo mkdir -p /etc/apparmor.d/local

# Ajouter le chemin ztna-lab
echo '  /var/lib/libvirt/images/ztna-lab/** rwk,' | \
    sudo tee /etc/apparmor.d/local/usr.sbin.libvirtd

# Recharger AppArmor
sudo systemctl reload apparmor

# Optionnel : Redémarrer libvirtd
sudo systemctl restart libvirtd
```

---

## Installer Terraform

### 1. Télécharger Terraform

```bash
# Version 1.14.3
cd /tmp
wget https://releases.hashicorp.com/terraform/1.14.3/terraform_1.14.3_linux_amd64.zip

# Extraire
unzip terraform_1.14.3_linux_amd64.zip

# Installer
sudo mv terraform /usr/local/bin/

# Vérifier
terraform version
# Doit afficher Terraform v1.14.3
```

### 2. Alternative : Installer via Snap

```bash
sudo snap install terraform --classic

# Vérifier
terraform version
```

---

## Initialiser le Lab

### 1. Cloner le Projet

```bash
# Clone le reposit
git clone https://github.com/your-org/ZTNA.git
cd ZTNA
```

### 2. Générer les Clés SSH

```bash
# Générer une clé SSH pour accéder aux VMs
mkdir -p ~/.ssh
chmod 700 ~/.ssh

ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -N "" -C "ztna-lab"

# Copier la clé publique pour les VMs
cp ~/.ssh/id_ed25519.pub lab/terraform/ssh_public_key.pub
```

### 3. Initialiser Terraform

```bash
# Aller dans le répertoire Terraform
cd lab/terraform

# Initialiser (télécharger les providers)
terraform init -upgrade

# Vérifier
terraform version
ls -la .terraform/
# Doit afficher les providers téléchargés
```

### 4. Créer l'Infrastructure

```bash
# Voir le plan (quels changements vont être appliqués)
terraform plan -var-file=terraform.tfvars

# Créer l'infrastructure
terraform apply -var-file=terraform.tfvars -auto-approve

# Attendre le démarrage des VMs (2-5 minutes)
```

### 5. Vérifier la Création

```bash
# Revenir au répertoire du projet
cd ../..

# Vérifier que les VMs sont créées
sudo chmod 666 /var/run/libvirt/libvirt-sock
virsh list --all

# Doit afficher 6 VMs

# Attendre que cloud-init finisse (30-60 secondes)
sleep 60

# Tester SSH
ssh -o StrictHostKeyChecking=no ztna@10.10.10.10 hostname

# Doit afficher "wan-client"
```

---

## Utiliser le Lab

### Commandes Terraform

```bash
# Voir le plan
cd lab/terraform
terraform plan -var-file=terraform.tfvars

# Appliquer les changements
terraform apply -var-file=terraform.tfvars -auto-approve

# Voir l'état
terraform show

# Détruire
terraform destroy -var-file=terraform.tfvars -auto-approve
```

### Commandes Make (Recommandé)

```bash
# Revenir au répertoire racine
cd ..

# Utiliser make
make init          # Créer l'infrastructure
make plan          # Voir le plan
make apply         # Appliquer
make destroy       # Détruire
make check         # Vérifier l'état
make ssh-client    # SSH vers client
```

### Vérifier l'Infrastructure

```bash
# Lister les VMs
virsh list --all

# Lister les réseaux
virsh net-list --all

# Voir les adresses IP
virsh net-dhcp-leases wan-net

# Tester la connectivité SSH
ssh -o StrictHostKeyChecking=no ztna@10.10.10.10 'hostname'
ssh -o StrictHostKeyChecking=no ztna@10.10.10.11 'hostname'
ssh -o StrictHostKeyChecking=no ztna@10.10.10.20 'hostname'
```

---

## Dépannage

### "Permission Denied" sur virsh

**Symptôme** :
```
error: failed to connect to the hypervisor
error: permission denied
```

**Solution** :
```bash
newgrp libvirt
# ou
sudo chmod 666 /var/run/libvirt/libvirt-sock
```

### "VT-x not detected"

**Symptôme** :
```
KVM not available
No VT-x or AMD-V
```

**Solution** :
1. Redémarrer l'ordinateur
2. Aller dans le BIOS/UEFI
3. Chercher "VT-x", "Intel Virtualization Technology", ou "AMD-V"
4. Activer l'option
5. Redémarrer

### "qemu-system-x86_64: Permission denied"

**Symptôme** :
```
Could not open '/var/lib/libvirt/images/ztna-lab/...': Permission denied
```

**Solution** :
```bash
# AppArmor bloque - configurer le chemin
echo '  /var/lib/libvirt/images/ztna-lab/** rwk,' | \
    sudo tee -a /etc/apparmor.d/local/usr.sbin.libvirtd

sudo systemctl reload apparmor
sudo systemctl restart libvirtd
```

### Les VMs ne démarrent pas

**Symptôme** :
```
VM state: "shut off"
```

**Solution** :
```bash
# Voir les logs
sudo journalctl -u libvirtd -n 50

# Ou voir les logs de la VM
sudo tail -f /var/log/libvirt/qemu/wan-client.log

# Vérifier la configuration
virsh dumpxml wan-client

# Essayer de démarrer
virsh start wan-client

# Accéder à la console
virsh console wan-client
```

### SSH ne marche pas

**Symptôme** :
```
ssh: connect to host 10.10.10.10 port 22: Connection refused
```

**Solution** :
```bash
# Attendre que cloud-init finisse (30-60 secondes)
sleep 60

# Vérifier que la VM tourne
virsh list --all | grep wan-client

# Accéder à la console et vérifier cloud-init
virsh console wan-client
# (Ctrl+] pour quitter)

# Dans la VM :
sudo cloud-init status
sudo tail -f /var/log/cloud-init.log
```

### Pas de connectivité réseau

**Symptôme** :
```
Pas de réseaux visibles dans virsh net-list
```

**Solution** :
```bash
# Redémarrer libvirt
sudo systemctl restart libvirtd

# Ou recréer manuellement
virsh net-define lab/terraform/networks.xml  # (si disponible)
virsh net-start wan-net
virsh net-autostart wan-net
```

---

## Étapes Suivantes

1. Vérifier que tout fonctionne
   ```bash
   make check
   ./scripts/check-lab.sh
   ```

2. Consulter la documentation
   ```bash
   cat README.md
   cat ARCHITECTURE.md
   ```

3. Développer le Control Plane et Gateway
   Voir `control-plane/` et `gateway/`

4. Tester l'infrastructure
   ```bash
   make test-network
   ```

---

**Version** : 1.0  
**Date** : 1 février 2026
