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
%{ if length(lookup(iface, "routes", [])) > 0 ~}
    routes:
%{ for route in iface.routes ~}
      - to: ${route.to}
        via: ${route.via}
%{ endfor ~}
%{ endif ~}
    nameservers:
      addresses:
        - ${iface.dns}
%{ endfor ~}
