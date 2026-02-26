#!/usr/bin/env bash
# =============================================================================
# demo/steps/05_ssh.sh — Connexion SSH via certificat (Flux 1)
#
# Le CP émet un certificat SSH signé par la SSH CA (Ed25519).
# Le client se connecte à lan-app via SSH avec jump sur ztna-gw.
# TrustedUserCAKeys est configurée sur les serveurs LAN → accès sans mot de passe.
# =============================================================================
set -euo pipefail
DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

print_step_banner "5" "SSH via CERTIFICAT — FLUX 1" "Client → CP (SSH-cert) → SSH jump GW → lan-app"

echo -e "${BOLD}Flux 1 — SSH Certificate Authority${NC}"
print_separator
print_kv "CA SSH"       "Ed25519 — hébergée sur le CP"
print_kv "TTL cert SSH" "15 minutes (sécurité maximale)"
print_kv "Jump host"    "ztna-gw (${GW_IP})"
print_kv "Cible"        "lan-app (${APP_IP}) — réseau LAN isolé"
print_kv "Auth SSH"     "Certificat signé — pas de mot de passe ni pubkey fixe"
echo -e ""

echo -e "${INFO}Émission du certificat SSH et connexion…${NC}"
print_separator

ssh_client bash <<ENDSSH
set -e
WORK_DIR="/tmp/ztna-demo"
mkdir -p "\$WORK_DIR"
CP_API="https://${CP_IP}:8080"

ACCESS_TOKEN=\$(cat "\$WORK_DIR/access_token.txt" 2>/dev/null || true)
if [[ -z "\$ACCESS_TOKEN" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Token manquant — exécutez d'abord l'étape 02 (login)"
    exit 1
fi

# 1. Générer une paire de clés SSH éphémère
KEY_FILE="\$WORK_DIR/ssh_ztna_demo"
CERT_FILE="\$KEY_FILE-cert.pub"
rm -f "\$KEY_FILE" "\$KEY_FILE.pub" "\$CERT_FILE"
ssh-keygen -t ed25519 -N "" -f "\$KEY_FILE" -C "demo-$(date +%s)" -q
echo -e "\033[0;32m[✓]\033[0m Paire de clés SSH Ed25519 éphémère générée"

# 2. Demander le certificat SSH au CP
PUBKEY=\$(cat "\$KEY_FILE.pub")
echo ""
echo -e "\033[0;36m[wan-client]\033[0m Demande de certificat SSH au CP…"

RESPONSE=\$(curl -sk --max-time 15 \\
    -X POST "\${CP_API}/api/v1/credentials/ssh-cert" \\
    -H "Authorization: Bearer \${ACCESS_TOKEN}" \\
    -H "Content-Type: application/json" \\
    -d "{\"public_key\":\"\$(echo \$PUBKEY | sed 's/\"/\\\"/g')\"}" 2>&1)

CERT=\$(echo "\$RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('certificate',''))" 2>/dev/null || true)

if [[ -z "\$CERT" ]]; then
    echo -e "\033[0;31m[✗]\033[0m Échec — réponse CP:"
    echo "\$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "\$RESPONSE"
    exit 1
fi

echo "\$CERT" > "\$CERT_FILE"
echo -e "\033[0;32m[✓]\033[0m Certificat SSH obtenu"

# Afficher les détails du certificat
echo ""
echo -e "    \033[1m--- Certificat SSH ---\033[0m"
ssh-keygen -L -f "\$CERT_FILE" 2>/dev/null | grep -E "Type:|Public key:|Signing CA:|Valid:|Principals:|Extensions:" | \
    while IFS= read -r line; do printf "    %s\n" "\$line"; done

# 3. Connexion SSH lan-app via jump host ztna-gw
echo ""
echo -e "\033[0;36m[wan-client]\033[0m Connexion SSH → ztna-gw → lan-app…"
chmod 600 "\$KEY_FILE" "\$CERT_FILE"

ssh -o StrictHostKeyChecking=no \\
    -o UserKnownHostsFile=/dev/null \\
    -o ConnectTimeout=10 \\
    -i "\$KEY_FILE" \\
    -i "\$CERT_FILE" \\
    -J "ztna@${GW_IP}" \\
    "ztna@${APP_IP}" \\
    'echo -e "\033[0;32m[lan-app]\033[0m Connexion SSH réussie!" && echo "  hostname: \$(hostname)" && echo "  user:     \$(id -un)" && echo "  uptime:   \$(uptime -p)"' 2>/dev/null

ENDSSH

echo -e ""
print_ok "Flux 1 complet — SSH via certificat éphémère (15 min)"
