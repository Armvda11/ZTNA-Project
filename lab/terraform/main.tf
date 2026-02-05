terraform {
  required_version = ">= 1.5.0"
}

# Étape 1 (squelette) : Lab KVM/QEMU
# Ici, on ajoutera progressivement :
# - les bridges réseaux (wan-net, dmz-net, lan-net)
# - les définitions des VMs (wan-client, ztna-gw, etc.)
#
# Rien n’est créé pour le moment : c’est volontaire pour avancer pas à pas.

provider "libvirt" {
  uri = "qemu:///system"
}
