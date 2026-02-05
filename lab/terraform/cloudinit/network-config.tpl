version: 2
ethernets:
%{ for iface in interfaces ~}
  ${iface.name}:
    dhcp4: false
    addresses:
      - ${iface.ip_cidr}
%{ if iface.gateway != "" ~}
    gateway4: ${iface.gateway}
%{ endif ~}
    nameservers:
      addresses:
        - ${iface.dns}
%{ endfor ~}
