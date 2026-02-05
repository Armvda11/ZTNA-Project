#cloud-config
hostname: ${hostname}
manage_etc_hosts: true

users:
  - name: ztna
    sudo: ["ALL=(ALL) NOPASSWD:ALL"]
    groups: ["sudo"]
    shell: /bin/bash
    ssh_authorized_keys:
      - ${ssh_public_key}

package_update: true
package_upgrade: true
packages:
  - qemu-guest-agent
  - curl
  - vim

runcmd:
  - systemctl enable qemu-guest-agent
  - systemctl start qemu-guest-agent
