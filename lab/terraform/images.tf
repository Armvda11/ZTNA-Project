# Pool et image de base (Ubuntu cloud image)

resource "libvirt_pool" "ztna" {
  name = "ztna-lab"
  type = "dir"
  path = var.pool_path
}

resource "libvirt_volume" "base_image" {
  name   = "ubuntu-22.04.qcow2"
  pool   = libvirt_pool.ztna.name
  source = var.base_image_url
  format = "qcow2"
}
