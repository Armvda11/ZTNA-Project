#!/usr/bin/env bash
# =============================================================================
# demo/lib/banner.sh — Bannières ASCII et affichage grandes étapes
# =============================================================================
set -euo pipefail

source "$(dirname "${BASH_SOURCE[0]}")/colors.sh"

# Affiche la bannière de bienvenue du projet ZTNA
print_ztna_banner() {
    clear
    echo -e "${BLUE}${BOLD}"
    cat << 'EOF'
 ███████╗████████╗███╗   ██╗ █████╗
 ╚══███╔╝╚══██╔══╝████╗  ██║██╔══██╗
   ███╔╝    ██║   ██╔██╗ ██║███████║
  ███╔╝     ██║   ██║╚██╗██║██╔══██║
 ███████╗   ██║   ██║ ╚████║██║  ██║
 ╚══════╝   ╚═╝   ╚═╝  ╚═══╝╚═╝  ╚═╝
EOF
    echo -e "${NC}"
    echo -e "${WHITE}${BOLD}     Zero Trust Network Access — Démonstration Live${NC}"
    echo -e "${DIM}     Architecture : mTLS | OIDC | SSH Cert | Policy Engine${NC}"
    echo -e ""
}

# Affiche une bannière d'étape numérotée (grand format pour écran de présentation)
print_step_banner() {
    local step_num="$1"
    local step_title="$2"
    local step_desc="${3:-}"

    # Écriture atomique dans le fichier partagé pour le panneau NARRATEUR
    local shared_file="/tmp/ztna-demo/current_step"
    mkdir -p /tmp/ztna-demo
    {
        echo "STEP=${step_num}"
        echo "TITLE=${step_title}"
        echo "DESC=${step_desc}"
        echo "TIMESTAMP=$(date '+%H:%M:%S')"
    } > "$shared_file"

    echo -e ""
    echo -e "${CYAN}${BOLD}"
    echo -e "╔══════════════════════════════════════════════════════╗"
    printf  "║  ÉTAPE %-2s  %-43s║\n" "${step_num}" "${step_title}"
    if [[ -n "$step_desc" ]]; then
        # Tronquer à 43 chars si nécessaire
        printf  "║  %-51s║\n" "${step_desc:0:51}"
    fi
    echo -e "╚══════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

# Panneau NARRATEUR — suit /tmp/ztna-demo/current_step en continu
# À lancer dans le terminal dédié au narrateur
run_narrator() {
    local shared_file="/tmp/ztna-demo/current_step"
    mkdir -p /tmp/ztna-demo

    print_ztna_banner

    echo -e "${YELLOW}${BOLD}[ Panneau narrateur actif — en attente de la démo... ]${NC}"
    echo -e "${DIM}Les étapes s'afficheront ici automatiquement.${NC}"
    echo ""

    local last_ts=""
    while true; do
        if [[ -f "$shared_file" ]]; then
            local ts
            ts=$(grep '^TIMESTAMP=' "$shared_file" 2>/dev/null | cut -d= -f2 || true)
            if [[ "$ts" != "$last_ts" && -n "$ts" ]]; then
                last_ts="$ts"
                local step title desc
                step=$(grep '^STEP=' "$shared_file" | cut -d= -f2)
                title=$(grep '^TITLE=' "$shared_file" | cut -d= -f2)
                desc=$(grep '^DESC=' "$shared_file" | cut -d= -f2 || true)
                clear
                print_ztna_banner
                echo -e "${CYAN}${BOLD}"
                echo -e "  ╔══════════════════════════════════════════════════╗"
                printf  "  ║  ÉTAPE %-2s                                        ║\n" "${step}"
                echo -e "  ║                                                  ║"
                printf  "  ║  %-48s║\n" "${title}"
                if [[ -n "$desc" ]]; then
                    printf  "  ║                                                  ║\n"
                    printf  "  ║  %-48s║\n" "${desc:0:48}"
                fi
                echo -e "  ║                                                  ║"
                echo -e "  ╚══════════════════════════════════════════════════╝"
                echo -e "${NC}"
                echo -e "${DIM}  $(date '+%H:%M:%S')${NC}"
            fi
        fi
        sleep 0.5
    done
}

# Afficher l'architecture réseau
print_architecture() {
    echo -e "${BLUE}${BOLD}"
    cat << 'EOF'
  ┌─────────────────────────────────────────────────────────────────┐
  │                     TOPOLOGIE ZTNA LAB                          │
  │                                                                 │
  │  WAN 10.10.10.0/24          DMZ 10.10.20.0/24                  │
  │  ┌──────────────┐           ┌──────────────┐                   │
  │  │  wan-client  │──mTLS────▶│   ztna-gw    │──┐                │
  │  │  10.10.10.10 │──SSH─────▶│  10.10.10.20 │  │ LAN (isolé)    │
  │  └──────────────┘           │  10.10.20.20 │  │ 10.10.30.0/24  │
  │                             └──────┬───────┘  │                │
  │                                    │OIDC/JWT  ▼                │
  │                             ┌──────▼───────┐  ┌──────────────┐ │
  │                             │  ztna-cp     │  │  lan-app     │ │
  │                             │  10.10.20.30 │  │  10.10.30.10 │ │
  │                             │  + Keycloak  │  └──────────────┘ │
  │                             └──────────────┘                   │
  └─────────────────────────────────────────────────────────────────┘
EOF
    echo -e "${NC}"
}
