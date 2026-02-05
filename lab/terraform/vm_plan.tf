# Plan des VMs (pas encore de création)
# On garde un plan lisible avant d'automatiser les domaines libvirt.

locals {
  vm_plan = {
    "wan-client" = {
      networks = [var.wan_net_name]
      role     = "user"
    }
    "wan-attacker" = {
      networks = [var.wan_net_name]
      role     = "attacker"
    }
    "ztna-gw" = {
      networks = [var.wan_net_name, var.dmz_net_name]
      role     = "gateway"
    }
    "ztna-cp" = {
      networks = [var.dmz_net_name]
      role     = "control-plane"
    }
    "lan-app" = {
      networks = [var.lan_net_name]
      role     = "app"
    }
    "lan-admin" = {
      networks = [var.lan_net_name]
      role     = "admin"
    }
  }
}

# Étape suivante : transformer ce plan en ressources libvirt_domain
# après validation des bridges et des IPs statiques.
