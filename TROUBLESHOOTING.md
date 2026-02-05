# ZTNA Lab - Guide de Dépannage

Bienvenue dans le guide complet de dépannage pour le lab ZTNA. Ce document couvre les problèmes les plus courants et leurs solutions.

## Table des Matières

- [Problèmes d'Installation](#problèmes-dinstallation)
- [Problèmes KVM/libvirt](#problèmes-kvmlibvirt)
- [Problèmes Terraform](#problèmes-terraform)
- [Problèmes de Connectivité](#problèmes-de-connectivité)
- [Problèmes de VMs](#problèmes-de-vms)
- [Problèmes de Performance](#problèmes-de-performance)
- [Obtenir de l'Aide](#obtenir-de-laide)

---

## Problèmes d'Installation

### Installation setup.sh échoue

**Symptômes** :
- Erreurs lors de `./setup.sh`
- Packages non trouvés
- Permissions refusées

**Solutions** :

1. **Vérifier l'OS**
   ```bash
   lsb_release -a
   # Doit afficher Ubuntu 22.04 LTS ou 24.04 LTS
   ```

2. **Mettre à jour le système**
   ```bash
   sudo apt update
   sudo apt upgrade -y
   ```

3. **Relancer le setup**
   ```bash
   # Utiliser le mode verbose
   bash -x ./setup.sh
   
   # Copier la dernière ligne d'erreur
   # Chercher la solution ci-dessous
   ```

4. **Installation manuelle**
   ```bash
   # Si setup.sh échoue, voir SETUP.md pour l'installation manuelle
   cat SETUP.md
   ```

---

### Permissions refusées

**Symptômes** :
```
Permission denied
sudo: unable to resolve host
```

**Solutions** :

1. **Vérifier les permissions sudo**
   ```bash
   # Vous devez avoir des droits sudo
   sudo whoami
   # Doit afficher "root"
   ```

2. **Ajouter l'utilisateur à sudoers** (si nécessaire)
   ```bash
   # Demander à un administrateur
   # ou effectuer les opérations avec sudo
   ```

3. **Espace disque insuffisant**
   ```bash
   df -h
   # Doit afficher 100+ GB disponible
   
   # Nettoyer
   sudo apt clean
   sudo apt autoclean
   ```

---

## Problèmes KVM/libvirt

### libvirtd ne démarre pas

**Symptômes** :
```
Failed to connect to hypervisor
error: failed to connect to the hypervisor
error: unable to connect to '/var/run/libvirt/libvirt-sock'
```

**Solutions** :

1. **Vérifier le statut**
   ```bash
   sudo systemctl status libvirtd
   sudo systemctl restart libvirtd
   ```

2. **Vérifier les logs**
   ```bash
   sudo journalctl -u libvirtd -n 50
   sudo tail -f /var/log/libvirt/libvirtd.log
   ```

3. **Fixer les permissions du socket**
   ```bash
   sudo chmod 666 /var/run/libvirt/libvirt-sock
   
   # Vérifier
   ls -l /var/run/libvirt/libvirt-sock
   # Doit afficher rw-rw-rw-
   ```

4. **Ajouter l'utilisateur au groupe libvirt**
   ```bash
   sudo usermod -aG libvirt,kvm $USER
   newgrp libvirt
   
   # Vérifier
   groups
   # Doit afficher libvirt et kvm
   ```

---

### VT-x/AMD-V non détectés

**Symptômes** :
```
KVM not available
No VT-x or AMD-V
error: kvm not available
```

**Solutions** :

1. **Vérifier que la virtualisation est activée**
   ```bash
   grep -E 'vmx|svm' /proc/cpuinfo
   # Doit afficher plusieurs lignes
   ```

2. **Si rien n'apparaît** :
   - Redémarrer l'ordinateur
   - Appuyer sur F2 ou Del au démarrage (voir le splash screen)
   - Chercher dans le BIOS les options :
     - "Virtualization Technology"
     - "VT-x" (Intel)
     - "AMD-V" (AMD)
     - "Intel Virtualization Technology"
   - Activer l'option
   - Redémarrer

3. **Charger les modules**
   ```bash
   # Intel
   sudo modprobe kvm_intel
   
   # AMD
   sudo modprobe kvm_amd
   
   # Vérifier
   lsmod | grep kvm
   ```

---

### AppArmor bloque les VMs

**Symptômes** :
```
Permission denied
Could not open '/var/lib/libvirt/images/ztna-lab/...': Permission denied
```

**Solutions** :

1. **Voir les logs AppArmor**
   ```bash
   sudo journalctl -S "10 minutes ago" | grep -i apparmor
   sudo dmesg | tail -20
   ```

2. **Ajouter le chemin à AppArmor**
   ```bash
   # Créer le fichier local
   echo '  /var/lib/libvirt/images/ztna-lab/** rwk,' | \
       sudo tee -a /etc/apparmor.d/local/usr.sbin.libvirtd
   
   # Recharger AppArmor
   sudo systemctl reload apparmor
   
   # Redémarrer libvirtd
   sudo systemctl restart libvirtd
   ```

3. **Alternative : désactiver AppArmor pour libvirtd** (moins sécurisé)
   ```bash
   sudo aa-disable /etc/apparmor.d/usr.sbin.libvirtd
   sudo systemctl restart libvirtd
   ```

---

## Problèmes Terraform

### terraform init échoue

**Symptômes** :
```
Error loading provider plugins
Provider version conflict
```

**Solutions** :

1. **Nettoyer et réinitialiser**
   ```bash
   cd lab/terraform
   rm -rf .terraform .terraform.lock.hcl
   terraform init -upgrade
   ```

2. **Vérifier la version de Terraform**
   ```bash
   terraform version
   # Doit afficher v1.14.3 ou compatible
   ```

3. **Vérifier la connexion internet**
   ```bash
   curl https://registry.terraform.io
   # Doit répondre
   ```

4. **Utiliser un mirror local** (si behind proxy)
   ```bash
   export TF_PLUGIN_CACHE_DIR="~/.terraform.d/plugin-cache"
   mkdir -p ~/.terraform.d/plugin-cache
   terraform init
   ```

---

### terraform plan échoue

**Symptômes** :
```
Error: error connecting to the hypervisor
Error: failed to connect to libvirt socket
```

**Solutions** :

1. **Vérifier la connexion à libvirt**
   ```bash
   virsh list --all
   # Doit fonctionner sans erreur
   ```

2. **Fixer le socket libvirt**
   ```bash
   sudo chmod 666 /var/run/libvirt/libvirt-sock
   newgrp libvirt
   ```

3. **Vérifier terraform.tfvars**
   ```bash
   cat terraform.tfvars
   # Vérifier les chemins et configurations
   ```

4. **Voir les logs détaillés**
   ```bash
   TF_LOG=debug terraform plan -var-file=terraform.tfvars
   ```

---

### terraform apply échoue sur une VM

**Symptômes** :
```
Error: error creating VM
Error: failed to download image
```

**Solutions** :

1. **Vérifier que les images existent**
   ```bash
   # Voir les images disponibles
   virsh vol-list default
   
   # Vérifier l'image Ubuntu
   ls -lh /var/lib/libvirt/images/jammy-server-cloudimg-amd64.img
   ```

2. **Télécharger l'image manuellement**
   ```bash
   # Créer le pool si absent
   virsh pool-create-as default dir --target /var/lib/libvirt/images
   
   # Télécharger l'image
   cd /var/lib/libvirt/images
   
   # Ubuntu 22.04
   wget https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img
   
   # Ubuntu 24.04
   wget https://cloud-images.ubuntu.com/noble/current/noble-server-cloudimg-amd64.img
   ```

3. **Redémarrer terraform**
   ```bash
   terraform apply -var-file=terraform.tfvars
   ```

---

## Problèmes de Connectivité

### SSH ne marche pas

**Symptômes** :
```
ssh: connect to host 10.10.10.10 port 22: Connection refused
ssh: connect to host 10.10.10.10 port 22: No route to host
```

**Solutions** :

1. **Vérifier que les VMs tournent**
   ```bash
   virsh list --all
   # Doit afficher toutes les VMs en "running"
   ```

2. **Attendre cloud-init**
   ```bash
   # cloud-init a besoin de 30-60 secondes
   sleep 60
   
   # Accéder à la console pour vérifier
   virsh console wan-client
   # (Ctrl+] pour quitter)
   
   # Dans la console, vérifier :
   sudo cloud-init status
   sudo tail -f /var/log/cloud-init-output.log
   ```

3. **Vérifier la clé SSH**
   ```bash
   # Vérifier que la clé existe
   ls -la ~/.ssh/id_ed25519
   
   # Vérifier que SSH peut lire la clé
   ssh-keygen -y -f ~/.ssh/id_ed25519
   ```

4. **Tester SSH avec verbosité**
   ```bash
   ssh -v -o StrictHostKeyChecking=no ztna@10.10.10.10 'hostname'
   
   # Ou avec plus de verbosité
   ssh -vvv -o StrictHostKeyChecking=no ztna@10.10.10.10 'hostname'
   ```

5. **Accéder à la console pour corriger**
   ```bash
   virsh console wan-client
   
   # Dans la console :
   sudo systemctl status ssh
   sudo tail -50 /var/log/auth.log
   ```

---

### Pas de connectivité réseau entre VMs

**Symptômes** :
```
ping: sendto: No route to host
Network unreachable
```

**Solutions** :

1. **Vérifier les réseaux**
   ```bash
   virsh net-list --all
   # Doit afficher : default, wan-net, dmz-net, lan-net
   ```

2. **Activer les réseaux si absent**
   ```bash
   # Pour chaque réseau absent
   virsh net-start wan-net
   virsh net-autostart wan-net
   ```

3. **Vérifier la configuration réseau dans les VMs**
   ```bash
   virsh console wan-client
   
   # Dans la console :
   ip addr show
   # Doit montrer : eth0 avec une IP
   
   ip route show
   # Doit montrer une route par défaut
   
   cat /etc/cloud/cloud.cfg.d/99-custom-networking.cfg
   ```

4. **Redémarrer réseau dans les VMs**
   ```bash
   virsh console wan-client
   
   # Dans la console :
   sudo systemctl restart networking
   # ou
   sudo netplan apply
   ```

5. **Vérifier les règles de firewall**
   ```bash
   virsh console wan-client
   
   # Dans la console :
   sudo ufw status
   sudo iptables -L -n
   ```

---

## Problèmes de VMs

### VMs ne démarrent pas

**Symptômes** :
```
VM state: "shut off"
virsh start wan-client
# error: Requested operation is not valid: domain is not running
```

**Solutions** :

1. **Voir les logs**
   ```bash
   virsh dumpxml wan-client | grep vcpu
   
   # Voir les logs d'erreur
   sudo tail -f /var/log/libvirt/qemu/wan-client.log
   ```

2. **Vérifier la configuration VM**
   ```bash
   virsh dominfo wan-client
   virsh dumpxml wan-client | less
   ```

3. **Essayer de démarrer avec logs**
   ```bash
   sudo libvirtd -l -d -v 2>&1 | tee /tmp/libvirtd.log &
   virsh start wan-client
   tail -f /tmp/libvirtd.log
   ```

4. **Accéder à la console pour voir l'erreur**
   ```bash
   virsh console wan-client
   # (Ctrl+] pour quitter)
   ```

5. **Recréer la VM**
   ```bash
   cd lab/terraform
   terraform taint libvirt_domain.wan-client
   terraform apply -var-file=terraform.tfvars
   ```

---

### VM consomme trop de CPU

**Symptômes** :
```
Une VM consomme 100% CPU
Le système ralentit
```

**Solutions** :

1. **Identifier la VM**
   ```bash
   top
   # Chercher qemu-system-x86_64 avec % CPU élevé
   
   # Ou
   ps aux | grep qemu
   ```

2. **Vérifier les processus dans la VM**
   ```bash
   virsh console wan-client
   
   # Dans la console :
   top
   ```

3. **Limiter les vCPU** (dans terraform.tfvars)
   ```hcl
   vcpu = 2  # au lieu de 4
   ```

4. **Redémarrer la VM**
   ```bash
   virsh reboot wan-client
   ```

---

### VM consomme trop de RAM

**Symptômes** :
```
Erreur : "not enough memory"
Le système ralentit
```

**Solutions** :

1. **Vérifier la RAM disponible**
   ```bash
   free -h
   ```

2. **Vérifier la RAM des VMs**
   ```bash
   virsh list --all
   virsh dominfo wan-client
   ```

3. **Réduire la RAM dans terraform.tfvars**
   ```hcl
   memory = 1024  # au lieu de 2048
   ```

4. **Vérifier les fuites mémoire dans les VMs**
   ```bash
   virsh console wan-client
   
   # Dans la console :
   free -h
   ps aux --sort=-%mem | head
   ```

---

## Problèmes de Performance

### Le lab est très lent

**Symptômes** :
- Tout est lent
- SSH répond lentement
- Création des VMs prend très longtemps

**Solutions** :

1. **Vérifier les ressources**
   ```bash
   # CPU
   nproc
   # Doit afficher 4 ou plus
   
   # RAM
   free -h | grep Mem
   # Doit afficher 16G ou plus
   
   # Disque
   df -h /
   # Doit afficher 100G+ avec 30%+ disponible
   ```

2. **Vérifier les processus**
   ```bash
   top
   # Chercher les processus qui consomment beaucoup
   ```

3. **Réduire le nombre de VMs**
   ```bash
   # Dans terraform.tfvars, réduire le nombre de vCPU/RAM par VM
   ```

4. **Vérifier le disque**
   ```bash
   # Voir l'utilisation disque
   df -h
   
   # Voir les fichiers volumineux
   du -sh /var/lib/libvirt/images/*
   ```

---

### Les VMs sont isolées du réseau hôte

**Symptômes** :
- Pas d'accès à internet depuis les VMs
- Pas de DNS
- Pas de temps synchronisé

**Solutions** :

1. **Vérifier la configuration de NAT**
   ```bash
   # Vérifier le réseau wan-net
   virsh net-dumpxml wan-net | grep -A5 nat
   
   # Doit contenir une section <nat>
   ```

2. **Vérifier les rules iptables**
   ```bash
   sudo iptables -t nat -L -n
   # Doit montrer des règles pour wan-net
   ```

3. **Redémarrer le réseau**
   ```bash
   virsh net-destroy wan-net
   virsh net-start wan-net
   ```

4. **Tester depuis une VM**
   ```bash
   virsh console wan-client
   
   # Dans la console :
   ping 8.8.8.8
   nslookup google.com
   ```

---

## Obtenir de l'Aide

### Collecter les informations de debug

```bash
# Créer un rapport de debug
./scripts/debug-info.sh > debug-report.txt

# Ou manuellement
mkdir -p debug-report
cd debug-report

# System info
uname -a > system.txt
lsb_release -a >> system.txt

# CPU
lscpu > cpu.txt
grep -E 'vmx|svm' /proc/cpuinfo > virtualization.txt

# RAM
free -h > memory.txt

# Disque
df -h > disk.txt
du -sh /var/lib/libvirt/images/* > vm-images.txt

# Réseau
virsh net-list --all > networks.txt
virsh net-dumpxml wan-net >> networks.txt

# VMs
virsh list --all > vms.txt

# Logs
sudo journalctl -u libvirtd -n 100 > libvirtd-logs.txt
sudo tail -100 /var/log/libvirt/qemu/wan-client.log >> vm-logs.txt

# Terraform
cat lab/terraform/terraform.tfvars >> terraform-config.txt

# Archiver
cd ..
tar -czf debug-report.tar.gz debug-report/
```

### Fichiers de logs importants

```bash
# libvirtd
sudo tail -f /var/log/libvirt/libvirtd.log

# VM spécifique
sudo tail -f /var/log/libvirt/qemu/wan-client.log

# Terraform
export TF_LOG=debug
terraform plan

# Système
sudo journalctl -u libvirtd
sudo dmesg

# SSH
ssh -vvv user@10.10.10.10
```

---

**Version** : 1.0  
**Dernière mise à jour** : 1 février 2026  
**Auteur** : ZTNA Lab Team
