#!/usr/bin/env bash
set -euo pipefail

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

ok()   { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()  { echo -e "${RED}[KO]${NC} $*"; }
info() { echo -e "${BLUE}[INFO]${NC} $*"; }

FAILS=0

require_cmd() {
  local label="$1"
  local cmd="$2"
  if command -v "${cmd}" >/dev/null 2>&1; then
    ok "${label} (${cmd})"
  else
    err "${label} requis mais introuvable: ${cmd}"
    FAILS=$((FAILS + 1))
  fi
}

optional_cmd() {
  local label="$1"
  local cmd="$2"
  if command -v "${cmd}" >/dev/null 2>&1; then
    ok "${label} (${cmd})"
  else
    warn "${label} optionnel absent: ${cmd}"
  fi
}

info "ZTNA Lab — vérification des prérequis (quickstart)"

if grep -E 'vmx|svm' /proc/cpuinfo >/dev/null 2>&1; then
  ok "Virtualisation CPU détectée (VT-x/AMD-V)"
else
  err "Virtualisation CPU absente (activer VT-x/AMD-V dans le BIOS)"
  FAILS=$((FAILS + 1))
fi

RAM_GB=$(free -g | awk 'NR==2 {print $2}')
if [ "${RAM_GB:-0}" -ge 16 ]; then
  ok "RAM ${RAM_GB} GB"
else
  warn "RAM ${RAM_GB:-0} GB (16 GB recommandés)"
fi

FREE_GB=$(df / | awk 'NR==2 {printf "%d", $4/1024/1024}')
if [ "${FREE_GB:-0}" -ge 100 ]; then
  ok "Espace disque ${FREE_GB} GB"
else
  err "Espace disque insuffisant: ${FREE_GB:-0} GB (100 GB minimum)"
  FAILS=$((FAILS + 1))
fi

require_cmd "Terraform" terraform
require_cmd "Libvirt CLI" virsh
require_cmd "SSH client" ssh
require_cmd "SCP" scp
require_cmd "Curl" curl
require_cmd "Go (build local CP/GW)" go

optional_cmd "OpenSSL (tests manuels)" openssl
optional_cmd "Python3 (scripts tests)" python3
optional_cmd "jq (debug JSON)" jq

if id -Gn | grep -q '\blibvirt\b'; then
  ok "Utilisateur dans le groupe libvirt"
else
  err "Utilisateur hors groupe libvirt (sudo usermod -aG libvirt,kvm \$USER puis newgrp libvirt)"
  FAILS=$((FAILS + 1))
fi

if systemctl is-active --quiet libvirtd 2>/dev/null; then
  ok "Service libvirtd actif"
else
  warn "libvirtd inactif (essayez: sudo systemctl start libvirtd)"
fi

if [ "${FAILS}" -gt 0 ]; then
  echo
  err "Pré-requis non satisfaits (${FAILS} erreur(s))."
  exit 1
fi

echo
ok "Pré-requis minimum satisfaits."
