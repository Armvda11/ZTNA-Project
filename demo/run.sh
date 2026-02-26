#!/usr/bin/env bash
# =============================================================================
# demo/run.sh — Lance la démo ZTNA en ouvrant 5 fenêtres GNOME Terminal
#               positionnées précisément sur l'écran.
#
# Usage:
#   ./demo/run.sh [--mode auto|manual] [--delay N] [--from N] [--to N]
#                 [--skip N,...] [--geometry WxH] [--no-narrator]
# =============================================================================
set -euo pipefail

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/ssh.sh"

# ─── Valeurs par défaut ───────────────────────────────────────────────────────
MODE="manual"
AUTO_DELAY=6
FROM_STEP=0
TO_STEP=7
SKIP_STEPS=""
SHOW_NARRATOR=true
SCREEN_W="${SCREEN_W:-1920}"
SCREEN_H="${SCREEN_H:-1080}"
FONT_SIZE="${FONT_SIZE:-12}"

# ─── Parser les arguments ────────────────────────────────────────────────────
CONDUCTOR_ARGS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --mode)          MODE="$2";              CONDUCTOR_ARGS+=(--mode "$2");  shift 2 ;;
        --delay)         AUTO_DELAY="$2";        CONDUCTOR_ARGS+=(--delay "$2"); shift 2 ;;
        --from)          FROM_STEP="$2";         CONDUCTOR_ARGS+=(--from "$2");  shift 2 ;;
        --to)            TO_STEP="$2";           CONDUCTOR_ARGS+=(--to "$2");    shift 2 ;;
        --skip)          SKIP_STEPS="$2";        CONDUCTOR_ARGS+=(--skip "$2");  shift 2 ;;
        --user)          ZTNA_USER="$2";         CONDUCTOR_ARGS+=(--user "$2");  shift 2 ;;
        --pass)          ZTNA_PASS="$2";         CONDUCTOR_ARGS+=(--pass "$2");  shift 2 ;;
        --no-narrator)   SHOW_NARRATOR=false;    shift ;;
        --font-size)     FONT_SIZE="$2";         shift 2 ;;
        *) echo "Option inconnue: $1" >&2; exit 1 ;;
    esac
done

# ─── Vérification des prérequis ───────────────────────────────────────────────
echo -e "${BOLD}Vérification des prérequis de la démo…${NC}"

MISSING=false
for cmd in ssh curl python3; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        print_err "Commande manquante : ${cmd}"
        MISSING=true
    fi
done

# Détection automatique du terminal émulateur
TERM_EMU=""
for t in gnome-terminal konsole xfce4-terminal tilix alacritty kitty xterm; do
    if command -v "$t" >/dev/null 2>&1; then
        TERM_EMU="$t"
        break
    fi
done

if [[ -z "$TERM_EMU" ]]; then
    print_err "Aucun émulateur de terminal trouvé (gnome-terminal, konsole, xterm…)"
    MISSING=true
else
    print_ok "Terminal détecté : ${TERM_EMU}"
fi

# wmctrl ou xdotool pour le positionnement des fenêtres
POSITIONING_TOOL=""
if command -v wmctrl >/dev/null 2>&1; then
    POSITIONING_TOOL="wmctrl"
elif command -v xdotool >/dev/null 2>&1; then
    POSITIONING_TOOL="xdotool"
else
    print_warn "wmctrl et xdotool absents — positionnement auto désactivé"
    print_warn "Installez avec : sudo apt install wmctrl"
fi

$MISSING && { print_err "Installez les prérequis manquants et relancez."; exit 1; }

print_ok "Prérequis vérifiés"
echo -e ""

# ─── Préparer le répertoire partagé ──────────────────────────────────────────
mkdir -p /tmp/ztna-demo

# ─── Géométries des fenêtres (pour un écran 1920×1080) ────────────────────────
# Ajuste via SCREEN_W/SCREEN_H si différent
#
# Disposition :
# ┌────────────────────────────────┬───────────────┬───────────────┐  ← y=0
# │  CONDUCTEUR (CLI ztna)         │ CP logs       │ GW logs       │
# │  x=0,  y=0,  w=960, h=600     │ x=960,y=0     │ x=1440,y=0    │
# │                                │ w=480,h=540   │ w=480,h=540   │
# ├────────────────────────────────┼───────────────┴───────────────┤  ← y=540
# │  wan-client (SSH interactif)   │ NARRATEUR (étapes)            │
# │  x=0,  y=600, w=960, h=480    │ x=960,y=540,  w=960, h=540    │
# └────────────────────────────────┴───────────────────────────────┘

HALF_W=$(( SCREEN_W / 2 ))
THIRD_W=$(( SCREEN_W / 3 ))
TOP_H=$(( SCREEN_H * 56 / 100 ))
BOT_H=$(( SCREEN_H - TOP_H ))
RIGHT_W=$(( SCREEN_W - HALF_W ))
MINI_W=$(( SCREEN_W - HALF_W ))

GEO_CONDUCTOR="${HALF_W}x${TOP_H}+0+0"
GEO_CP_LOGS="${THIRD_W}x${TOP_H}+${HALF_W}+0"
GEO_GW_LOGS="${THIRD_W}x${TOP_H}+$(( HALF_W + THIRD_W ))+0"
GEO_CLIENT="${HALF_W}x${BOT_H}+0+${TOP_H}"
GEO_NARRATOR="${RIGHT_W}x${BOT_H}+${HALF_W}+${TOP_H}"

# ─── Fonction : ouvrir un terminal (multi-emulateur) ────────────────────────
open_terminal() {
    local title="$1"
    local geometry="$2"   # format WxH+X+Y
    local cmd="${3:-bash}"

    # Parser la géométrie
    local w h x y rest
    w=${geometry%%x*}
    rest=${geometry#*x}
    h=${rest%%+*}
    rest=${rest#*+}
    x=${rest%%+*}
    y=${rest#*+}

    case "$TERM_EMU" in
        gnome-terminal)
            gnome-terminal \
                --title="ZTNA: ${title}" \
                --geometry="${geometry}" \
                -- bash -c "${cmd}; exec bash" &
            ;;
        konsole)
            konsole \
                --title "ZTNA: ${title}" \
                --geometry "${w}x${h}+${x}+${y}" \
                -e bash -c "${cmd}; exec bash" &
            ;;
        xfce4-terminal)
            xfce4-terminal \
                --title="ZTNA: ${title}" \
                --geometry="${geometry}" \
                -e "bash -c '${cmd}; exec bash'" &
            ;;
        tilix)
            tilix \
                --title="ZTNA: ${title}" \
                -e "bash -c '${cmd}; exec bash'" &
            ;;
        alacritty)
            alacritty \
                --title "ZTNA: ${title}" \
                -e bash -c "${cmd}; exec bash" &
            ;;
        kitty)
            kitty \
                --title "ZTNA: ${title}" \
                bash -c "${cmd}; exec bash" &
            ;;
        xterm)
            xterm \
                -title "ZTNA: ${title}" \
                -geometry "${geometry}" \
                -e bash -c "${cmd}; exec bash" &
            ;;
    esac

    # Petit délai pour que la fenêtre s'ouvre
    sleep 0.8

    # Positionnement précis via wmctrl (fiable sur X11 pour tous les terminaux)
    if [[ "$POSITIONING_TOOL" == "wmctrl" ]]; then
        sleep 0.5
        wmctrl -r "ZTNA: ${title}" -e "0,${x},${y},${w},${h}" 2>/dev/null || true
    elif [[ "$POSITIONING_TOOL" == "xdotool" ]]; then
        sleep 0.5
        local wid
        wid=$(xdotool search --name "ZTNA: ${title}" 2>/dev/null | tail -1)
        if [[ -n "$wid" ]]; then
            xdotool windowmove "$wid" "$x" "$y" 2>/dev/null || true
            xdotool windowsize "$wid" "$w" "$h" 2>/dev/null || true
        fi
    fi
}

# ─── Ouvrir les 5 terminaux ───────────────────────────────────────────────────
echo -e "${BOLD}Ouverture des terminaux ZTNA Demo…${NC}"

# 1. Logs Control Plane (en premier pour avoir le temps de démarrer)
echo -e "${CYAN}  [1/5]${NC} Logs Control Plane…"
open_terminal \
    "Control Plane — logs" \
    "${GEO_CP_LOGS}" \
    "source '${DEMO_DIR}/lib/logfmt.sh'; stream_cp_logs"

# 2. Logs Gateway
echo -e "${CYAN}  [2/5]${NC} Logs Gateway…"
open_terminal \
    "Gateway — logs" \
    "${GEO_GW_LOGS}" \
    "source '${DEMO_DIR}/lib/logfmt.sh'; stream_gw_logs"

# 3. Terminal wan-client (SSH interactif)
echo -e "${CYAN}  [3/5]${NC} Terminal wan-client…"
open_terminal \
    "wan-client — SSH" \
    "${GEO_CLIENT}" \
    "echo -e '\033[0;33m\033[1m[ WAN-CLIENT — Terminal SSH interactif ]\033[0m'; \
     echo -e '\033[2mConnexion SSH vers wan-client (${CLIENT_IP})...\033[0m'; \
     ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=QUIET -i ${SSH_KEY} ztna@${CLIENT_IP}"

# 4. Panneau narrateur (si activé)
if $SHOW_NARRATOR; then
    echo -e "${CYAN}  [4/5]${NC} Panneau narrateur…"
    open_terminal \
        "Narrateur — Étapes" \
        "${GEO_NARRATOR}" \
        "bash -c 'source ${DEMO_DIR}/lib/banner.sh && run_narrator'"
fi

# 5. Conducteur principal (dernière fenêtre — au premier plan)
echo -e "${CYAN}  [5/5]${NC} Conducteur principal…"
sleep 0.5
open_terminal \
    "Conducteur — Demo ZTNA" \
    "${GEO_CONDUCTOR}" \
    "bash '${DEMO_DIR}/conductor.sh' ${CONDUCTOR_ARGS[*]:-}"

echo -e ""
print_ok "5 terminaux ouverts — la démo est en cours"
echo -e ""
echo -e "${DIM}Pour rejouer : make demo-reset && make demo${NC}"
echo -e "${DIM}Mode auto    : make demo-auto${NC}"
echo -e "${DIM}Depuis étape : make demo FROM=3${NC}"
