#!/usr/bin/env bash
set -euo pipefail

echo "=== ZTNA Lab: verification des prerequis ==="

check_cmd() {
  local name="$1"
  local cmd="$2"
  if command -v "$cmd" >/dev/null 2>&1; then
    echo "OK  - $name"
  else
    echo "KO  - $name (commande '$cmd' introuvable)"
    return 1
  fi
}

# CPU virtualization
if grep -E 'vmx|svm' /proc/cpuinfo >/dev/null 2>&1; then
  echo "OK  - Virtualisation CPU"
else
  echo "KO  - Virtualisation CPU (active VT-x/AMD-V dans le BIOS)"
  exit 1
fi

# RAM
ram_gb=$(free -g | awk 'NR==2 {print $2}')
if [ "${ram_gb:-0}" -ge 16 ]; then
  echo "OK  - RAM ${ram_gb} GB"
else
  echo "WARN- RAM ${ram_gb:-0} GB (recommande 16 GB)"
fi

# Disk
free_gb=$(df / | awk 'NR==2 {printf "%d", $4/1024/1024}')
if [ "${free_gb:-0}" -ge 100 ]; then
  echo "OK  - Disque ${free_gb} GB libres"
else
  echo "KO  - Disque ${free_gb:-0} GB libres (min 100 GB)"
  exit 1
fi

check_cmd "Terraform" terraform
check_cmd "libvirt/virsh" virsh

# Groups
if id -Gn | grep -q '\blibvirt\b'; then
  echo "OK  - Groupe libvirt"
else
  echo "KO  - Groupe libvirt (ajouter l'utilisateur au groupe)"
  exit 1
fi

# libvirtd service
if systemctl is-active --quiet libvirtd; then
  echo "OK  - libvirtd actif"
else
  echo "KO  - libvirtd inactif (sudo systemctl start libvirtd)"
  exit 1
fi

echo "=== Prerequis OK ==="
