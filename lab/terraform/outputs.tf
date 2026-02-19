output "networks" {
  value = {
    wan = libvirt_network.wan.name
    dmz = libvirt_network.dmz.name
    lan = libvirt_network.lan.name
  }
}

output "ip_plan" {
  value = {
    wan_client  = var.wan_client_ip
    ztna_gw_wan = var.ztna_gw_wan_ip
    ztna_gw_dmz = var.ztna_gw_dmz_ip
    ztna_gw_lan = var.ztna_gw_lan_ip
    ztna_cp     = var.ztna_cp_ip
    lan_app     = var.lan_app_ip
    lan_admin   = var.lan_admin_ip
  }
}
