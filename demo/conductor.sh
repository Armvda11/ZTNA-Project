#!/usr/bin/env bash
# =============================================================================
# demo/conductor.sh — Orchestre les étapes de la démo ZTNA
#
# Usage:
#   ./demo/conductor.sh [OPTIONS]
#
# Options:
#   --mode auto|manual    Mode d'avancement (défaut: manual)
#   --delay N             Secondes entre étapes en mode auto (défaut: 6)
#   --from N              Commencer à l'étape N (défaut: 0)
#   --to N                S'arrêter à l'étape N (défaut: 7)
#   --user USER           Utilisateur OIDC (défaut: alice)
#   --pass PASS           Mot de passe OIDC (défaut: Password123!)
#   --skip N[,N...]       Sauter certaines étapes (ex: --skip 4,5)
#   -h, --help            Afficher cette aide
# =============================================================================
set -uo pipefail

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${DEMO_DIR}/lib/colors.sh"
source "${DEMO_DIR}/lib/banner.sh"
source "${DEMO_DIR}/lib/ssh.sh"

# ─── Défauts ─────────────────────────────────────────────────────────────────
MODE="manual"
AUTO_DELAY=6
FROM_STEP=0
TO_STEP=7
SKIP_STEPS=""
SHOW_HELP=false

# ─── Parser les arguments ────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --mode)     MODE="$2";       shift 2 ;;
        --delay)    AUTO_DELAY="$2"; shift 2 ;;
        --from)     FROM_STEP="$2";  shift 2 ;;
        --to)       TO_STEP="$2";    shift 2 ;;
        --user)     ZTNA_USER="$2";  shift 2 ;;
        --pass)     ZTNA_PASS="$2";  shift 2 ;;
        --skip)     SKIP_STEPS="$2"; shift 2 ;;
        -h|--help)  SHOW_HELP=true;  shift ;;
        *) echo "Option inconnue: $1" >&2; exit 1 ;;
    esac
done

export ZTNA_USER ZTNA_PASS CP_IP GW_IP CLIENT_IP APP_IP
export KC_URL CP_API KC_PROTO KC_PORT

# ─── Aide ────────────────────────────────────────────────────────────────────
if $SHOW_HELP; then
    grep '^#' "$0" | grep -v '^#!/' | sed 's/^# //' | sed 's/^#//'
    exit 0
fi

# ─── Tableau des étapes ───────────────────────────────────────────────────────
declare -A STEP_FILES=(
    [0]="${DEMO_DIR}/steps/00_welcome.sh"
    [1]="${DEMO_DIR}/steps/01_health.sh"
    [2]="${DEMO_DIR}/steps/02_login.sh"
    [3]="${DEMO_DIR}/steps/03_issuecert.sh"
    [4]="${DEMO_DIR}/steps/04_connect.sh"
    [5]="${DEMO_DIR}/steps/05_ssh.sh"
    [6]="${DEMO_DIR}/steps/06_revoke.sh"
    [7]="${DEMO_DIR}/steps/07_blocked.sh"
)

declare -A STEP_NAMES=(
    [0]="Bienvenue & Architecture"
    [1]="Health Check"
    [2]="Authentification OIDC"
    [3]="Émission Device-Cert (X.509)"
    [4]="Connexion mTLS — Flux 2"
    [5]="SSH via Certificat — Flux 1"
    [6]="Révocation du Cert (CRL)"
    [7]="Zero Trust en Action — DENY"
)

# ─── Aide contextuelle mode ───────────────────────────────────────────────────
show_progress_bar() {
    local current=$1
    local total=$2
    local bar_width=38
    local filled=$(( current * bar_width / total ))
    local empty=$(( bar_width - filled ))
    local bar_filled="" bar_empty=""
    for ((i=0; i<filled; i++)); do bar_filled+="█"; done
    for ((i=0; i<empty;  i++)); do bar_empty+="░";  done
    printf "\033[0;32m%s\033[2m%s\033[0m %d/%d\n" "$bar_filled" "$bar_empty" "$current" "$total"
}

# ─── is_skipped ───────────────────────────────────────────────────────────────
is_skipped() {
    local step=$1
    [[ -n "$SKIP_STEPS" ]] && echo "$SKIP_STEPS" | tr ',' '\n' | grep -q "^${step}$"
}

# ─── Attente entre étapes ────────────────────────────────────────────────────
wait_or_pause() {
    local step_num=$1
    local step_name=$2
    local is_last=$3

    if $is_last; then
        return 0
    fi

    echo -e ""
    print_separator
    if [[ "$MODE" == "auto" ]]; then
        echo -e "${DIM}Mode auto — prochaine étape dans ${AUTO_DELAY}s… (Ctrl+C pour arrêter)${NC}"
        sleep "$AUTO_DELAY"
    else
        echo -e "${CYAN}${BOLD}Appuyez sur [Entrée] pour continuer vers la prochaine étape…${NC}"
        echo -e "${DIM}  (ou tapez 'skip' pour sauter, 'quit' pour terminer)${NC}"
        read -r user_input </dev/tty || true
        case "${user_input,,}" in
            quit|q|exit) echo -e "${YELLOW}Démo interrompue.${NC}"; exit 0 ;;
            skip|s)      return 1 ;;
        esac
    fi
    return 0
}

# ─── Boucle principale ───────────────────────────────────────────────────────
print_ztna_banner

echo -e "${BOLD}Démo ZTNA — Configuration${NC}"
print_separator
print_kv "Mode"         "$MODE"
print_kv "Étapes"       "${FROM_STEP} → ${TO_STEP}"
print_kv "Utilisateur"  "$ZTNA_USER"
print_kv "CP"           "$CP_IP"
print_kv "Gateway"      "$GW_IP"
print_kv "Client"       "$CLIENT_IP"
[[ -n "$SKIP_STEPS" ]] && print_kv "Étapes sautées" "$SKIP_STEPS"
echo -e ""

# Préparer les répertoires partagés
mkdir -p /tmp/ztna-demo
echo "$CP_API"   > /tmp/ztna-demo/cp_api.txt
echo "$GW_IP:9443" > /tmp/ztna-demo/gw_addr.txt

TOTAL_STEPS=$(( TO_STEP - FROM_STEP + 1 ))
CURRENT=0

for step_num in $(seq "$FROM_STEP" "$TO_STEP"); do
    CURRENT=$(( CURRENT + 1 ))
    STEP_FILE="${STEP_FILES[$step_num]:-}"

    # Sauter si demandé
    if is_skipped "$step_num"; then
        echo -e "${DIM}[SKIP] Étape ${step_num} — ${STEP_NAMES[$step_num]:-?}${NC}"
        continue
    fi

    # Vérifier que le fichier existe
    if [[ -z "$STEP_FILE" || ! -f "$STEP_FILE" ]]; then
        print_warn "Fichier d'étape introuvable: ${STEP_FILE:-?} — étape ignorée"
        continue
    fi

    # Barre de progression
    echo -e ""
    printf "${DIM}Progression : ${NC}"
    show_progress_bar "$CURRENT" "$TOTAL_STEPS"
    echo -e "${MAGENTA}${BOLD}▶ Étape ${step_num}/7 — ${STEP_NAMES[$step_num]:-?}${NC}"
    echo -e ""

    # Exécuter l'étape
    EXIT_CODE=0
    bash "$STEP_FILE" || EXIT_CODE=$?
    if [[ $EXIT_CODE -ne 0 ]]; then
        print_err "Étape ${step_num} terminée avec une erreur (code ${EXIT_CODE})"
        if [[ "$MODE" != "auto" ]]; then
            echo -e "${YELLOW}Continuer quand même ? [Entrée=oui / 'quit'=arrêter]${NC}"
            read -r user_input </dev/tty || true
            [[ "${user_input,,}" == "quit" ]] && exit 1
        fi
    fi

    # Pause entre étapes
    IS_LAST=false
    [[ $step_num -eq $TO_STEP ]] && IS_LAST=true

    if ! wait_or_pause "$step_num" "${STEP_NAMES[$step_num]:-?}" $IS_LAST; then
        echo -e "${DIM}Étape suivante sautée.${NC}"
    fi
done

echo -e ""
print_ztna_banner
echo -e "${GREEN}${BOLD}  ✅  Démonstration terminée avec succès !${NC}"
echo -e ""
echo -e "${DIM}  Replay : make demo-reset && make demo${NC}"
