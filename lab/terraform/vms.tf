# VMs (création réelle)

locals {
  dns_default = "1.1.1.1"
}

# === WAN CLIENT ===
resource "libvirt_volume" "wan_client_disk" {
  name           = "wan-client.qcow2"
  pool           = libvirt_pool.ztna.name
  base_volume_id = libvirt_volume.base_image.id
  size           = var.vm_disk_size
}

resource "libvirt_cloudinit_disk" "wan_client_ci" {
  name           = "wan-client-ci.iso"
  pool           = libvirt_pool.ztna.name
  user_data      = templatefile("${path.module}/cloudinit/user-data.tpl", {
    hostname       = "wan-client"
    ssh_public_key = var.ssh_public_key
  })
  network_config = templatefile("${path.module}/cloudinit/network-config.tpl", {
    interfaces = [
      {
        name     = "ens3"
        ip_cidr  = "${var.wan_client_ip}/24"
        gateway  = var.wan_gateway
        dns      = local.dns_default
      }
    ]
  })
}

resource "libvirt_domain" "wan_client" {
  name   = "wan-client"
  memory = var.vm_small_memory
  vcpu   = var.vm_small_cpu

  cloudinit = libvirt_cloudinit_disk.wan_client_ci.id

  network_interface {
    network_id = libvirt_network.wan.id
  }

  disk {
    volume_id = libvirt_volume.wan_client_disk.id
  }
}

# === WAN ATTACKER ===
resource "libvirt_volume" "wan_attacker_disk" {
  name           = "wan-attacker.qcow2"
  pool           = libvirt_pool.ztna.name
  base_volume_id = libvirt_volume.base_image.id
  size           = var.vm_disk_size
}

resource "libvirt_cloudinit_disk" "wan_attacker_ci" {
  name           = "wan-attacker-ci.iso"
  pool           = libvirt_pool.ztna.name
  user_data      = templatefile("${path.module}/cloudinit/user-data.tpl", {
    hostname       = "wan-attacker"
    ssh_public_key = var.ssh_public_key
  })
  network_config = templatefile("${path.module}/cloudinit/network-config.tpl", {
    interfaces = [
      {
        name     = "ens3"
        ip_cidr  = "${var.wan_attacker_ip}/24"
        gateway  = var.wan_gateway
        dns      = local.dns_default
      }
    ]
  })
}

resource "libvirt_domain" "wan_attacker" {
  name   = "wan-attacker"
  memory = var.vm_small_memory
  vcpu   = var.vm_small_cpu

  cloudinit = libvirt_cloudinit_disk.wan_attacker_ci.id

  network_interface {
    network_id = libvirt_network.wan.id
  }

  disk {
    volume_id = libvirt_volume.wan_attacker_disk.id
  }
}

# === ZTNA GW ===
resource "libvirt_volume" "ztna_gw_disk" {
  name           = "ztna-gw.qcow2"
  pool           = libvirt_pool.ztna.name
  base_volume_id = libvirt_volume.base_image.id
  size           = var.vm_disk_size
}

resource "libvirt_cloudinit_disk" "ztna_gw_ci" {
  name           = "ztna-gw-ci.iso"
  pool           = libvirt_pool.ztna.name
  user_data      = templatefile("${path.module}/cloudinit/user-data.tpl", {
    hostname       = "ztna-gw"
    ssh_public_key = var.ssh_public_key
  })
  network_config = templatefile("${path.module}/cloudinit/network-config.tpl", {
    interfaces = [
      {
        name     = "ens3"
        ip_cidr  = "${var.ztna_gw_wan_ip}/24"
        gateway  = var.wan_gateway
        dns      = local.dns_default
      },
      {
        name     = "ens4"
        ip_cidr  = "${var.ztna_gw_dmz_ip}/24"
        gateway  = ""
        dns      = local.dns_default
      }
    ]
  })
}

resource "libvirt_domain" "ztna_gw" {
  name   = "ztna-gw"
  memory = var.vm_medium_memory
  vcpu   = var.vm_medium_cpu

  cloudinit = libvirt_cloudinit_disk.ztna_gw_ci.id

  network_interface {
    network_id = libvirt_network.wan.id
  }

  network_interface {
    network_id = libvirt_network.dmz.id
  }

  disk {
    volume_id = libvirt_volume.ztna_gw_disk.id
  }
}

# === ZTNA CP ===
resource "libvirt_volume" "ztna_cp_disk" {
  name           = "ztna-cp.qcow2"
  pool           = libvirt_pool.ztna.name
  base_volume_id = libvirt_volume.base_image.id
  size           = var.vm_disk_size
}

resource "libvirt_cloudinit_disk" "ztna_cp_ci" {
  name           = "ztna-cp-ci.iso"
  pool           = libvirt_pool.ztna.name
  user_data      = templatefile("${path.module}/cloudinit/user-data.tpl", {
    hostname       = "ztna-cp"
    ssh_public_key = var.ssh_public_key
  })
  network_config = templatefile("${path.module}/cloudinit/network-config.tpl", {
    interfaces = [
      {
        name     = "ens3"
        ip_cidr  = "${var.ztna_cp_ip}/24"
        gateway  = ""
        dns      = local.dns_default
      }
    ]
  })
}

resource "libvirt_domain" "ztna_cp" {
  name   = "ztna-cp"
  memory = var.vm_medium_memory
  vcpu   = var.vm_medium_cpu

  cloudinit = libvirt_cloudinit_disk.ztna_cp_ci.id

  network_interface {
    network_id = libvirt_network.dmz.id
  }

  disk {
    volume_id = libvirt_volume.ztna_cp_disk.id
  }
}

# === LAN APP ===
resource "libvirt_volume" "lan_app_disk" {
  name           = "lan-app.qcow2"
  pool           = libvirt_pool.ztna.name
  base_volume_id = libvirt_volume.base_image.id
  size           = var.vm_disk_size
}

resource "libvirt_cloudinit_disk" "lan_app_ci" {
  name           = "lan-app-ci.iso"
  pool           = libvirt_pool.ztna.name
  user_data      = templatefile("${path.module}/cloudinit/user-data.tpl", {
    hostname       = "lan-app"
    ssh_public_key = var.ssh_public_key
  })
  network_config = templatefile("${path.module}/cloudinit/network-config.tpl", {
    interfaces = [
      {
        name     = "ens3"
        ip_cidr  = "${var.lan_app_ip}/24"
        gateway  = ""
        dns      = local.dns_default
      }
    ]
  })
}

resource "libvirt_domain" "lan_app" {
  name   = "lan-app"
  memory = var.vm_small_memory
  vcpu   = var.vm_small_cpu

  cloudinit = libvirt_cloudinit_disk.lan_app_ci.id

  network_interface {
    network_id = libvirt_network.lan.id
  }

  disk {
    volume_id = libvirt_volume.lan_app_disk.id
  }
}

# === LAN ADMIN ===
resource "libvirt_volume" "lan_admin_disk" {
  name           = "lan-admin.qcow2"
  pool           = libvirt_pool.ztna.name
  base_volume_id = libvirt_volume.base_image.id
  size           = var.vm_disk_size
}

resource "libvirt_cloudinit_disk" "lan_admin_ci" {
  name           = "lan-admin-ci.iso"
  pool           = libvirt_pool.ztna.name
  user_data      = templatefile("${path.module}/cloudinit/user-data.tpl", {
    hostname       = "lan-admin"
    ssh_public_key = var.ssh_public_key
  })
  network_config = templatefile("${path.module}/cloudinit/network-config.tpl", {
    interfaces = [
      {
        name     = "ens3"
        ip_cidr  = "${var.lan_admin_ip}/24"
        gateway  = ""
        dns      = local.dns_default
      }
    ]
  })
}

resource "libvirt_domain" "lan_admin" {
  name   = "lan-admin"
  memory = var.vm_small_memory
  vcpu   = var.vm_small_cpu

  cloudinit = libvirt_cloudinit_disk.lan_admin_ci.id

  network_interface {
    network_id = libvirt_network.lan.id
  }

  disk {
    volume_id = libvirt_volume.lan_admin_disk.id
  }
}
