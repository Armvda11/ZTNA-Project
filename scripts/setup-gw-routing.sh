#!/usr/bin/env bash
# setup-gw-routing.sh — Règles iptables minimalistes sur ztna-gw (ZTNA strict)
#
# Philosophie Zero Trust :
#   FORWARD policy = DROP par défaut.
#   Seules les connexions strictement nécessaires à l'auth/enrollment sont ouvertes :
#     WAN → DMZ, TCP uniquement, src=wan-client, dst=ztna-cp :
#       · port 8081 : Keycloak HTTP  (token OIDC — legacy/fallback lab)
#       · port 8443 : Keycloak HTTPS (token OIDC — cible)
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
LAN_IF="${LAN_IF:-ens5}"          # interface LAN de ztna-gw  (10.10.30.20)
CLIENT_IP="${CLIENT_IP:-10.10.10.10}"  # wan-client (source autorisée)
WAN_NAT_IP="${WAN_NAT_IP:-10.10.10.1}" # passerelle libvirt WAN (NAT source vue sur GW)
CLIENT_SOURCES="${CLIENT_SOURCES:-${CLIENT_IP},${WAN_NAT_IP}}"
CP_IP="${CP_IP:-10.10.20.30}"         # ztna-cp    (destination autorisée)
KC_HTTP_PORT="${KC_HTTP_PORT:-8081}"   # Keycloak OIDC HTTP  (legacy/fallback)
KC_HTTPS_PORT="${KC_HTTPS_PORT:-8443}" # Keycloak OIDC HTTPS (cible)
CP_PORT="${CP_PORT:-8080}"             # Control Plane API
WAN_CIDR="10.10.10.0/24"              # sous-réseau WAN (pour MASQUERADE)
LAN_CIDR="${LAN_CIDR:-10.10.30.0/24}" # sous-réseau LAN (jamais accessible directement depuis WAN)

IFS=',' read -r -a SOURCE_IPS <<< "${CLIENT_SOURCES}"

log "Interfaces : WAN=${WAN_IF} DMZ=${DMZ_IF}"
log "Sources WAN autorisées vers DMZ : ${CLIENT_SOURCES}"
log "Destination CP   : ${CP_IP}  ports ${KC_HTTP_PORT}/${KC_HTTPS_PORT} (KC) ${CP_PORT} (CP)"

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

ensure_forward_allow() {
  local src_ip="$1"
  local dst_port="$2"
  local label="$3"
  if ! sudo iptables -C FORWARD \
         -i "$WAN_IF" -o "$DMZ_IF" \
         -s "$src_ip" -d "$CP_IP" \
         -p tcp --dport "$dst_port" \
         -m state --state NEW,ESTABLISHED -j ACCEPT 2>/dev/null; then
    sudo iptables -A FORWARD \
      -i "$WAN_IF" -o "$DMZ_IF" \
      -s "$src_ip" -d "$CP_IP" \
      -p tcp --dport "$dst_port" \
      -m state --state NEW,ESTABLISHED -j ACCEPT
    log "✓ FORWARD ${src_ip} → ${CP_IP}:${dst_port}/tcp (${label}) ajouté"
  else
    log "✓ FORWARD ${src_ip} → ${CP_IP}:${dst_port}/tcp (${label}) déjà présent"
  fi
}

# ── Règles 2/3/4 : WAN→DMZ TCP:8081, 8443 et 8080 pour chaque source autorisée ──
for src_ip in "${SOURCE_IPS[@]}"; do
  src_ip="$(echo "${src_ip}" | xargs)"
  [[ -z "${src_ip}" ]] && continue
  ensure_forward_allow "${src_ip}" "${KC_HTTP_PORT}"  "Keycloak HTTP"
  ensure_forward_allow "${src_ip}" "${KC_HTTPS_PORT}" "Keycloak HTTPS"
  ensure_forward_allow "${src_ip}" "${CP_PORT}" "Control Plane"
done

# ── Règle 4 : WAN→LAN DROP explicite + LOG (Zero Trust — jamais de bypass PEP) ──
# Cette règle est redondante avec la FORWARD policy DROP, mais elle est explicite :
# - elle log les tentatives d'accès direct WAN→LAN (visibilité SOC)
# - elle garantit qu'aucun paquet WAN ne peut atteindre le LAN sans passer par mTLS:4433
if ! sudo iptables -C FORWARD \
       -i "$WAN_IF" -d "$LAN_CIDR" \
       -m comment --comment "ZTNA: block WAN→LAN direct (bypass prevention)" \
       -j DROP 2>/dev/null; then
  # LOG d'abord (rate-limited pour éviter la saturation des logs)
  sudo iptables -A FORWARD \
    -i "$WAN_IF" -d "$LAN_CIDR" \
    -m limit --limit 10/min --limit-burst 20 \
    -j LOG --log-prefix "ZTNA-BLOCKED-WAN-LAN: " --log-level 4
  # Puis DROP explicite
  sudo iptables -A FORWARD \
    -i "$WAN_IF" -d "$LAN_CIDR" \
    -m comment --comment "ZTNA: block WAN→LAN direct (bypass prevention)" \
    -j DROP
  log "✓ FORWARD WAN→LAN BLOCKED explicitement (DROP + LOG) — Zero Trust"
else
  log "✓ FORWARD WAN→LAN DROP déjà présent"
fi

# ── Règle 5 : MASQUERADE WAN→DMZ (corriger l'asymétrie de routage) ───────────
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
for src_ip in "${SOURCE_IPS[@]}"; do
  src_ip="$(echo "${src_ip}" | xargs)"
  [[ -z "${src_ip}" ]] && continue
  log "    ${src_ip} → ${CP_IP}:${KC_HTTP_PORT}/tcp          (Keycloak HTTP)"
  log "    ${src_ip} → ${CP_IP}:${KC_HTTPS_PORT}/tcp          (Keycloak HTTPS)"
  log "    ${src_ip} → ${CP_IP}:${CP_PORT}/tcp          (Control Plane)"
done
log "  FORWARD explicitement bloqué :"
log "    WAN → ${LAN_CIDR} : DROP + LOG (aucun bypass PEP possible)"
log "  NAT POSTROUTING         :"
log "    MASQUERADE ${WAN_CIDR} → ${DMZ_IF}              (SNAT WAN→DMZ)"
log "    PAS de MASQUERADE WAN→LAN (la GW proxifie elle-même via mTLS:4433)"
log "  Accès ressources LAN    : mTLS:4433 + PEP authorize uniquement"
log ""
echo "--- iptables FORWARD ---"
sudo iptables -L FORWARD -n --line-numbers -v 2>/dev/null || true
echo ""
echo "--- iptables NAT POSTROUTING ---"
sudo iptables -t nat -L POSTROUTING -n --line-numbers -v 2>/dev/null || true
log ""
log "✅ ztna-gw configuré (exposition minimale, Zero Trust)."
