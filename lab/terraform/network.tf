# Bridges KVM (libvirt networks)

resource "libvirt_network" "wan" {
  name      = var.wan_net_name
  mode      = "nat"
  addresses = [var.wan_cidr]
  autostart = true

  dhcp {
    enabled = true
  }
}

resource "libvirt_network" "dmz" {
  name      = var.dmz_net_name
  mode      = "none"
  addresses = [var.dmz_cidr]
  autostart = true

  dhcp {
    enabled = false
  }
}

resource "libvirt_network" "lan" {
  name      = var.lan_net_name
  mode      = "none"
  addresses = [var.lan_cidr]
  autostart = true

  dhcp {
    enabled = false
  }
}
