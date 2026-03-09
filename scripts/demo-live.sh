#!/usr/bin/env bash
# ============================================================================
# ZTNA Live Demo — Multi-terminal dashboard (tmux)
# ============================================================================
# Displays a 4-pane dashboard showing:
#   ┌───────────────────┬───────────────────┐
#   │  Control Plane    │    Gateway         │
#   │  (logs -f)        │    (logs -f)       │
#   ├───────────────────┼───────────────────┤
#   │  Client / Test    │   Session Monitor  │
#   │  (interactive)    │   (watch status)   │
#   └───────────────────┴───────────────────┘
#
# Usage:
#   bash scripts/demo-live.sh              # lance le dashboard
#   bash scripts/demo-live.sh --test       # lance + exécute test-flux2 auto
#   bash scripts/demo-live.sh --detach     # lance en arrière-plan
#
# Pré-requis : tmux, ssh, VMs démarrées, CP + GW déployés
# ============================================================================

set -euo pipefail

# ---------- Configuration ---------------------------------------------------
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SESSION="ztna-demo"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"

CP_IP="10.10.20.30"
GW_IP="10.10.10.20"
CLIENT_IP="10.10.10.10"
APP_IP="10.10.30.10"

SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5"
SSH_CMD="ssh ${SSH_OPTS} -i ${SSH_KEY}"

# Colors for banner
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

AUTO_TEST=false
DETACH=false

for arg in "$@"; do
  case "$arg" in
    --test)   AUTO_TEST=true ;;
    --detach) DETACH=true ;;
    --help|-h)
      echo "Usage: $0 [--test] [--detach] [--help]"
      echo "  --test    Exécute automatiquement le test mTLS (flux2) dans le pane client"
      echo "  --detach  Lance la session tmux en arrière-plan"
      exit 0
      ;;
  esac
done

# ---------- Pre-checks ------------------------------------------------------

check_dep() {
  command -v "$1" >/dev/null 2>&1 || {
    echo -e "${RED}[✗]${NC} '$1' est requis mais non installé."
    exit 1
  }
}

check_dep tmux
check_dep ssh

# ---------- Kill existing session -------------------------------------------

tmux kill-session -t "$SESSION" 2>/dev/null || true

# ---------- Banner ----------------------------------------------------------

banner() {
  clear
  echo -e "${CYAN}${BOLD}"
  echo "  ╔═══════════════════════════════════════════════════════════╗"
  echo "  ║          ZTNA — Zero Trust Network Access                ║"
  echo "  ║              Live Demo Dashboard                         ║"
  echo "  ╠═══════════════════════════════════════════════════════════╣"
  echo "  ║  CP   : ${CP_IP}:8080/8443         mTLS + Policy Engine ║"
  echo "  ║  GW   : ${GW_IP}:8443              mTLS Reverse Proxy   ║"
  echo "  ║  CLI  : ${CLIENT_IP}                ZTNA Client          ║"
  echo "  ║  APP  : ${APP_IP}                LAN Resource            ║"
  echo "  ╠═══════════════════════════════════════════════════════════╣"
  echo "  ║  Ctrl-b + arrow  →  naviguer entre panes                ║"
  echo "  ║  Ctrl-b + z      →  zoom/unzoom un pane                 ║"
  echo "  ║  Ctrl-b + d      →  détacher la session                 ║"
  echo "  ║  tmux attach -t ztna-demo  →  réattacher                ║"
  echo "  ╚═══════════════════════════════════════════════════════════╝"
  echo -e "${NC}"
}

banner

# ---------- Build tmux session ----------------------------------------------

echo -e "${BLUE}[→]${NC} Création de la session tmux '${SESSION}'..."

# Create session with first pane (top-left: Control Plane logs)
tmux new-session -d -s "$SESSION" -n "dashboard" -x "$(tput cols)" -y "$(tput lines)"

# ---- Pane 0: Control Plane logs (top-left) ----
tmux send-keys -t "$SESSION:0.0" \
  "echo -e '${BOLD}${CYAN}━━━ CONTROL PLANE LOGS ━━━${NC}' && echo '→ ${CP_IP} — journalctl -u ztna-cp -f' && echo '' && ${SSH_CMD} ztna@${CP_IP} 'sudo journalctl -u ztna-cp -f --no-pager --output=short-iso 2>&1 || echo \"[!] CP unreachable or service not found\"'" C-m

# ---- Split horizontally: Pane 1 (bottom-left) ----
tmux split-window -t "$SESSION:0" -v

# ---- Pane 1: Client / Test panel (bottom-left) ----
if [ "$AUTO_TEST" = true ]; then
  tmux send-keys -t "$SESSION:0.1" \
    "echo -e '${BOLD}${GREEN}━━━ CLIENT — mTLS TUNNEL TEST ━━━${NC}' && echo '→ Exécution automatique test-flux2 (mTLS access)' && echo '' && sleep 3 && cd ${PROJECT_DIR} && make test-flux2 2>&1; echo '' && echo -e '${GREEN}[✓] Test terminé. Vous pouvez relancer: make test-flux2${NC}' && bash" C-m
else
  tmux send-keys -t "$SESSION:0.1" \
    "echo -e '${BOLD}${GREEN}━━━ CLIENT TERMINAL ━━━${NC}' && echo '→ Commandes utiles:' && echo '  make test-flux2          # Test mTLS tunnel complet' && echo '  make test-flux1-auto     # Test SSH cert access' && echo '  make test-crl-routing    # Test CRL + sessions + routing' && echo '  make test-cp-gw-lab      # Suite complète' && echo '  make ssh-client          # SSH vers wan-client' && echo '' && cd ${PROJECT_DIR} && bash" C-m
fi

# ---- Split pane 0 vertically: Pane 2 (top-right) ----
tmux select-pane -t "$SESSION:0.0"
tmux split-window -t "$SESSION:0.0" -h

# ---- Pane 2: Gateway logs (top-right) ----
tmux send-keys -t "$SESSION:0.2" \
  "echo -e '${BOLD}${YELLOW}━━━ GATEWAY LOGS ━━━${NC}' && echo '→ ${GW_IP} — journalctl -u ztna-gateway -f' && echo '' && ${SSH_CMD} ztna@${GW_IP} 'sudo journalctl -u ztna-gateway -f --no-pager --output=short-iso 2>&1 || echo \"[!] Gateway unreachable or service not found\"'" C-m

# ---- Split pane 1 vertically: Pane 3 (bottom-right) ----
tmux select-pane -t "$SESSION:0.1"
tmux split-window -t "$SESSION:0.1" -h

# ---- Pane 3: Session monitor / status (bottom-right) ----
tmux send-keys -t "$SESSION:0.3" \
  "echo -e '${BOLD}${RED}━━━ ZTNA STATUS MONITOR ━━━${NC}' && echo '' && cd ${PROJECT_DIR} && watch -n 5 -c 'echo \"=== $(date +%H:%M:%S) — ZTNA Health ===\"; echo \"\"; echo \"Control Plane:\"; curl -sfk --max-time 2 https://${CP_IP}:8080/healthz && echo \" ✓ UP\" || echo \" ✗ DOWN\"; echo \"\"; echo \"Gateway service:\"; ssh ${SSH_OPTS} -i ${SSH_KEY} ztna@${GW_IP} \"systemctl is-active ztna-gateway 2>/dev/null && echo \\\" ✓ active\\\" || echo \\\" ✗ inactive\\\"\" 2>/dev/null || echo \" ✗ unreachable\"; echo \"\"; echo \"SSH connectivity:\"; for h in ${CLIENT_IP} ${GW_IP} ${CP_IP}; do ssh ${SSH_OPTS} -i ${SSH_KEY} ztna@\$h \"hostname\" 2>/dev/null && echo \" ✓ \$h\" || echo \" ✗ \$h\"; done; echo \"\"; echo \"Active sessions (GW):\"; ssh ${SSH_OPTS} -i ${SSH_KEY} ztna@${GW_IP} \"ss -tnp | grep -E \\\":8443|:443\\\" | head -5\" 2>/dev/null || echo \" (unavailable)\"'" C-m

# ---- Layout & focus ----
tmux select-layout -t "$SESSION:0" tiled
# Give slightly more height to top row (logs)
tmux resize-pane -t "$SESSION:0.0" -U 5 2>/dev/null || true

# Focus on client pane (bottom-left)
tmux select-pane -t "$SESSION:0.1"

# ---- Pane titles ----
tmux set-option -t "$SESSION" pane-border-status top 2>/dev/null || true
tmux set-option -t "$SESSION" pane-border-format \
  " #{?pane_active,#[fg=green bold],#[fg=white]} #T " 2>/dev/null || true

tmux select-pane -t "$SESSION:0.0" -T "Control Plane Logs"
tmux select-pane -t "$SESSION:0.1" -T "Client Terminal"
tmux select-pane -t "$SESSION:0.2" -T "Gateway Logs"
tmux select-pane -t "$SESSION:0.3" -T "Status Monitor"

# ---- Status bar ----
tmux set-option -t "$SESSION" status-style "bg=colour235,fg=colour250" 2>/dev/null || true
tmux set-option -t "$SESSION" status-left \
  "#[fg=colour16,bg=colour39,bold] ZTNA DEMO #[default] " 2>/dev/null || true
tmux set-option -t "$SESSION" status-right \
  "#[fg=colour250] %H:%M  #[fg=colour39]CP:${CP_IP} GW:${GW_IP} " 2>/dev/null || true

# ---------- Attach or print instructions ------------------------------------

echo -e "${GREEN}[✓]${NC} Session tmux '${SESSION}' créée avec 4 panes."

if [ "$DETACH" = true ]; then
  echo -e "${BLUE}[→]${NC} Pour rejoindre : ${BOLD}tmux attach -t ${SESSION}${NC}"
else
  echo -e "${BLUE}[→]${NC} Attachement dans 2 secondes..."
  sleep 2
  tmux attach-session -t "$SESSION"
fi
