# Setup Manuel ZTNA Lab

Ce guide couvre une installation manuelle sur Ubuntu quand `setup.sh` n'est pas utilise.

## 1. Installer paquets systeme

```bash
sudo apt update
sudo apt install -y \
  qemu-kvm qemu-system-x86 qemu-utils \
  libvirt-daemon libvirt-clients libvirt-daemon-system \
  cloud-utils cloud-initramfs-growroot \
  curl wget git make openssh-client \
  python3 python3-pip
```

## 2. Configurer libvirt

```bash
sudo usermod -aG libvirt,kvm $USER
newgrp libvirt
sudo systemctl enable --now libvirtd
```

Verifier:

```bash
virsh --connect qemu:///system list --all
```

## 3. Installer Terraform

Exemple binaire officiel:

```bash
cd /tmp
wget https://releases.hashicorp.com/terraform/1.14.3/terraform_1.14.3_linux_amd64.zip
unzip terraform_1.14.3_linux_amd64.zip
sudo mv terraform /usr/local/bin/
terraform version
```

## 4. Installer Go (si absent)

```bash
go version
# si absent, installer go >= 1.21
```

## 5. Preparer la cle SSH lab

```bash
mkdir -p ~/.ssh
chmod 700 ~/.ssh
[ -f ~/.ssh/id_ed25519 ] || ssh-keygen -t ed25519 -f ~/.ssh/id_ed25519 -N ""
cp ~/.ssh/id_ed25519.pub lab/terraform/ssh_public_key.pub
```

## 6. Verifier prerequis et lancer

```bash
make prereq
make quickstart
```

## 7. Verification post-install

```bash
make check
curl -sk https://10.10.20.30:8080/healthz
curl -sk https://10.10.20.30:8080/pki/ssh-ca/pubkey
curl -sk https://10.10.20.30:8080/pki/device-ca/cert
```

## 8. Commandes operationnelles

```bash
make up
make lab-start
make deploy
make deploy-gw
make logs-cp
make logs-gw
make destroy
```
