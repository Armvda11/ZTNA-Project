# Réseaux du lab (CIDR)
variable "wan_cidr" {
  type    = string
  default = "10.10.10.0/24"
}

variable "dmz_cidr" {
  type    = string
  default = "10.10.20.0/24"
}

variable "lan_cidr" {
  type    = string
  default = "10.10.30.0/24"
}

# Noms des réseaux
variable "wan_net_name" {
  type    = string
  default = "wan-net"
}

variable "dmz_net_name" {
  type    = string
  default = "dmz-net"
}

variable "lan_net_name" {
  type    = string
  default = "lan-net"
}

# Chemins / images
variable "pool_path" {
  type    = string
  default = "/var/lib/libvirt/images/ztna-lab"
}

variable "base_image_url" {
  type    = string
  default = "https://cloud-images.ubuntu.com/jammy/current/jammy-server-cloudimg-amd64.img"
}

# SSH
variable "ssh_public_key" {
  type        = string
  description = "Clé publique SSH pour cloud-init"
}

# Tailles des VMs
variable "vm_disk_size" {
  type    = number
  default = 10737418240 # 10GB
}

variable "vm_small_memory" {
  type    = number
  default = 1024
}

variable "vm_medium_memory" {
  type    = number
  default = 2048
}

variable "vm_small_cpu" {
  type    = number
  default = 1
}

variable "vm_medium_cpu" {
  type    = number
  default = 2
}

# Plan IP - Architecture simplifiée
variable "wan_gateway" {
  type    = string
  default = "10.10.10.1"
}

variable "dmz_gateway" {
  type    = string
  default = "10.10.20.1"
}

variable "wan_client_ip" {
  type    = string
  default = "10.10.10.10"
}

variable "ztna_gw_wan_ip" {
  type    = string
  default = "10.10.10.20"
}

variable "ztna_gw_dmz_ip" {
  type    = string
  default = "10.10.20.20"
}

variable "ztna_gw_lan_ip" {
  type    = string
  default = "10.10.30.20"
}

variable "ztna_cp_ip" {
  type    = string
  default = "10.10.20.30"
}

variable "lan_app_ip" {
  type    = string
  default = "10.10.30.10"
}

variable "lan_admin_ip" {
  type    = string
  default = "10.10.30.11"
}
