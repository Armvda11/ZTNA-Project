# Bridges KVM (libvirt networks) - Architecture simplifiée

resource "libvirt_network" "wan" {
  name      = var.wan_net_name
  mode      = "nat"
  addresses = [var.wan_cidr]
  autostart = true

  dhcp {
    enabled = false  # IPs statiques via cloud-init
  }
}

resource "libvirt_network" "dmz" {
  name      = var.dmz_net_name
  mode      = "nat"  # Accessible depuis PC pour déploiement facile
  addresses = [var.dmz_cidr]
  autostart = true

  dhcp {
    enabled = false
  }
}

resource "libvirt_network" "lan" {
  name      = var.lan_net_name
  mode      = "none"  # Isolé, accès via ztna-gw seulement
  addresses = [var.lan_cidr]
  autostart = true

  dhcp {
    enabled = false
  }
}
