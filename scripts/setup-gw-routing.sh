#!/usr/bin/env bash
# setup-gw-routing.sh — Règles iptables minimalistes sur ztna-gw (ZTNA strict)
#
# Philosophie Zero Trust :
#   FORWARD policy = DROP par défaut.
#   Seules les connexions strictement nécessaires à l'auth/enrollment sont ouvertes :
#     WAN → DMZ, TCP uniquement, src=wan-client, dst=ztna-cp :
#       · port 8081 : Keycloak  (token OIDC — étape 1 des deux flux)
#       · port 8080 : Control Plane (device-cert enrollment — étape 2 Flux2 / Flux1)
#   MASQUERADE uniquement WAN→DMZ (corrige l'asymétrie de routage TCP).
#   PAS de MASQUERADE WAN→LAN : la GW proxifie elle-même vers le LAN,
#     la source est déjà l'IP LAN de la GW — ajouter ce NAT transformerait
#     la GW en routeur généraliste et ouvrirait un vecteur d'attaque LAN.
#   Tout accès aux ressources LAN reste exclusivement via mTLS:4433 + PEP authorize.
#
# Usage (depuis le host KVM) :
#   ssh ztna@10.10.10.20 'bash -s' < scripts/setup-gw-routing.sh
#   ou : make setup-routing

set -euo pipefail

log()  { echo "[setup-gw-routing] $*"; }
warn() { echo "[setup-gw-routing] ⚠  $*"; }
die()  { echo "[ERREUR] $*" >&2; exit 1; }

# ── Variables (surchargeables via l'environnement) ───────────────────────────
WAN_IF="${WAN_IF:-ens3}"          # interface WAN de ztna-gw  (10.10.10.20)
DMZ_IF="${DMZ_IF:-ens4}"          # interface DMZ de ztna-gw  (10.10.20.20)
CLIENT_IP="${CLIENT_IP:-10.10.10.10}"  # wan-client (source autorisée)
CP_IP="${CP_IP:-10.10.20.30}"         # ztna-cp    (destination autorisée)
KC_PORT="${KC_PORT:-8081}"             # Keycloak OIDC
CP_PORT="${CP_PORT:-8080}"             # Control Plane API
WAN_CIDR="10.10.10.0/24"              # sous-réseau WAN (pour MASQUERADE)

log "Interfaces : WAN=${WAN_IF} DMZ=${DMZ_IF}"
log "Client autorisé  : ${CLIENT_IP}"
log "Destination CP   : ${CP_IP}  ports ${KC_PORT} (KC) ${CP_PORT} (CP)"

# ── ip_forward ───────────────────────────────────────────────────────────────
log "Vérification ip_forward..."
if [[ "$(cat /proc/sys/net/ipv4/ip_forward)" != "1" ]]; then
  sudo sysctl -w net.ipv4.ip_forward=1
  grep -q "net.ipv4.ip_forward=1" /etc/sysctl.conf 2>/dev/null \
    || echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf > /dev/null
fi
log "✓ ip_forward = 1"

# ── Politique par défaut FORWARD = DROP ──────────────────────────────────────
# Zero Trust : refuser tout ce qui n'est pas explicitement autorisé.
sudo iptables -P FORWARD DROP
log "✓ FORWARD policy = DROP"

# ── Règle 1 : connexions établies (retour des réponses) ──────────────────────
# Indispensable : les paquets de retour (SYN-ACK, ACK, données) doivent passer.
if ! sudo iptables -C FORWARD \
       -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null; then
  sudo iptables -I FORWARD 1 \
    -m state --state RELATED,ESTABLISHED -j ACCEPT
  log "✓ FORWARD ESTABLISHED,RELATED ajouté"
else
  log "✓ FORWARD ESTABLISHED,RELATED déjà présent"
fi

# ── Règle 2 : WAN→DMZ TCP:8081 (Keycloak — obtention du token OIDC) ─────────
if ! sudo iptables -C FORWARD \
       -i "$WAN_IF" -o "$DMZ_IF" \
       -s "$CLIENT_IP" -d "$CP_IP" \
       -p tcp --dport "$KC_PORT" \
       -m state --state NEW,ESTABLISHED -j ACCEPT 2>/dev/null; then
  sudo iptables -A FORWARD \
    -i "$WAN_IF" -o "$DMZ_IF" \
    -s "$CLIENT_IP" -d "$CP_IP" \
    -p tcp --dport "$KC_PORT" \
    -m state --state NEW,ESTABLISHED -j ACCEPT
  log "✓ FORWARD ${CLIENT_IP} → ${CP_IP}:${KC_PORT}/tcp (Keycloak) ajouté"
else
  log "✓ FORWARD →${KC_PORT} déjà présent"
fi

# ── Règle 3 : WAN→DMZ TCP:8080 (Control Plane — enrollment device cert) ──────
if ! sudo iptables -C FORWARD \
       -i "$WAN_IF" -o "$DMZ_IF" \
       -s "$CLIENT_IP" -d "$CP_IP" \
       -p tcp --dport "$CP_PORT" \
       -m state --state NEW,ESTABLISHED -j ACCEPT 2>/dev/null; then
  sudo iptables -A FORWARD \
    -i "$WAN_IF" -o "$DMZ_IF" \
    -s "$CLIENT_IP" -d "$CP_IP" \
    -p tcp --dport "$CP_PORT" \
    -m state --state NEW,ESTABLISHED -j ACCEPT
  log "✓ FORWARD ${CLIENT_IP} → ${CP_IP}:${CP_PORT}/tcp (Control Plane) ajouté"
else
  log "✓ FORWARD →${CP_PORT} déjà présent"
fi

# ── Règle 4 : MASQUERADE WAN→DMZ (corriger l'asymétrie de routage) ───────────
# Sans SNAT : ztna-cp répond avec dst=10.10.10.10 via sa GW par défaut (bridge KVM)
# sans repasser par ztna-gw → TCP SYN sans SYN-ACK → connexion impossible.
# Avec SNAT : ztna-cp voit src=10.10.20.20 (DMZ ztna-gw), retour garanti.
if ! sudo iptables -t nat -C POSTROUTING \
       -s "$WAN_CIDR" -o "$DMZ_IF" -j MASQUERADE 2>/dev/null; then
  sudo iptables -t nat -A POSTROUTING \
    -s "$WAN_CIDR" -o "$DMZ_IF" -j MASQUERADE
  log "✓ MASQUERADE ${WAN_CIDR}→${DMZ_IF} ajouté (SNAT WAN→DMZ)"
else
  log "✓ MASQUERADE WAN→DMZ déjà présent"
fi

# Pas de MASQUERADE WAN→LAN :
# - Flux2 : la GW établit elle-même la connexion TCP vers lan-app (src=IP LAN GW)
# - Ajouter ce NAT ouvrirait un vecteur d'accès direct WAN→LAN bypassant le PEP
log "✓ Pas de MASQUERADE WAN→LAN (inutile et contraire au principe Zero Trust)"

# ── Persistance ──────────────────────────────────────────────────────────────
if command -v netfilter-persistent >/dev/null 2>&1; then
  sudo netfilter-persistent save
  log "✓ Règles persistées (netfilter-persistent)"
elif command -v iptables-save >/dev/null 2>&1; then
  sudo mkdir -p /etc/iptables
  sudo iptables-save | sudo tee /etc/iptables/rules.v4 > /dev/null
  log "✓ Règles persistées (/etc/iptables/rules.v4)"
else
  warn "Pas d'outil de persistance — règles perdues au reboot."
  warn "Installer : sudo apt-get install -y iptables-persistent"
fi

# ── Résumé ───────────────────────────────────────────────────────────────────
log ""
log "=== Politique appliquée (ZTNA strict) ==="
log "  FORWARD default         : DROP"
log "  FORWARD autorisé        :"
log "    ESTABLISHED,RELATED                              (retour)"
log "    ${CLIENT_IP} → ${CP_IP}:${KC_PORT}/tcp          (Keycloak OIDC)"
log "    ${CLIENT_IP} → ${CP_IP}:${CP_PORT}/tcp          (Control Plane)"
log "  NAT POSTROUTING         :"
log "    MASQUERADE ${WAN_CIDR} → ${DMZ_IF}              (SNAT WAN→DMZ)"
log "  Accès ressources LAN    : mTLS:4433 + PEP uniquement"
log ""
echo "--- iptables FORWARD ---"
sudo iptables -L FORWARD -n --line-numbers -v 2>/dev/null || true
echo ""
echo "--- iptables NAT POSTROUTING ---"
sudo iptables -t nat -L POSTROUTING -n --line-numbers -v 2>/dev/null || true
log ""
log "✅ ztna-gw configuré (exposition minimale, Zero Trust)."
