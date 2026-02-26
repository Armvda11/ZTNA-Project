#!/usr/bin/env bash
# =============================================================================
# demo/lib/logfmt.sh — Affichage formaté des logs JSON du Control Plane / Gateway
#
# Usage:
#   source demo/lib/logfmt.sh
#   stream_cp_logs          # fenêtre Control Plane
#   stream_gw_logs          # fenêtre Gateway
# =============================================================================

DEMO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_BASE="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=QUIET -i ${SSH_KEY}"

CP_IP="${CP_IP:-10.10.20.30}"
GW_IP="${GW_IP:-10.10.10.20}"

# ─── Formateur JSON → ligne lisible ──────────────────────────────────────────
# Entrée   : {"time":"...","level":"INFO","msg":"started","addr":"0.0.0.0:8080"}
# Sortie   : 11:56:43  INFO  started  addr=0.0.0.0:8080
JSON_FORMATTER='
import sys, json, datetime

RESET  = "\033[0m"
BOLD   = "\033[1m"
DIM    = "\033[2m"
CYAN   = "\033[0;36m"
GREEN  = "\033[0;32m"
YELLOW = "\033[1;33m"
RED    = "\033[0;31m"
BLUE   = "\033[0;34m"
MAG    = "\033[0;35m"

LEVEL_COLOR = {
    "DEBUG": DIM,
    "INFO":  CYAN,
    "WARN":  YELLOW,
    "WARNING": YELLOW,
    "ERROR": RED,
    "FATAL": f"{RED}{BOLD}",
}

KNOWN = {"time", "level", "msg", "message"}

def fmt_level(lvl):
    color = LEVEL_COLOR.get(lvl.upper(), "")
    return f"{color}{lvl.upper():<5}{RESET}"

def fmt_time(raw):
    try:
        # "2026-02-26T11:56:43.102Z" ou "2026-02-26T11:56:43+0000"
        raw = raw.replace("+0000", "Z").replace(" ", "T")
        dt = datetime.datetime.fromisoformat(raw.replace("Z", "+00:00"))
        return dt.strftime("%H:%M:%S")
    except Exception:
        return raw[:8]

def fmt_extras(d):
    parts = []
    for k, v in d.items():
        if k in KNOWN:
            continue
        val = str(v)
        parts.append(f"{DIM}{k}={RESET}{CYAN}{val}{RESET}")
    return "  " + "  ".join(parts) if parts else ""

for line in sys.stdin:
    line = line.rstrip()
    if not line:
        continue
    # Conserver les lignes non-JSON (systemd, etc.)
    if not line.lstrip().startswith("{"):
        # Aplatir les lignes systemd : "... systemd[1]: Started ..."
        if "systemd" in line:
            print(f"{DIM}{line}{RESET}")
        else:
            print(line)
        sys.stdout.flush()
        continue
    try:
        d = json.loads(line)
    except json.JSONDecodeError:
        print(line)
        sys.stdout.flush()
        continue

    t   = fmt_time(d.get("time", d.get("ts", "")))
    lvl = d.get("level", d.get("severity", "INFO"))
    msg = d.get("msg", d.get("message", line))
    extras = fmt_extras(d)

    print(f"  {DIM}{t}{RESET}  {fmt_level(lvl)}  {BOLD}{msg}{RESET}{extras}")
    sys.stdout.flush()
'

# ─── Bannière d'en-tête de fenêtre ────────────────────────────────────────────
_log_header() {
    local color="$1"
    local title="$2"
    clear
    echo -e "${color}\033[1m"
    echo -e "  ╔══════════════════════════════════════════════════╗"
    printf  "  ║  %-48s║\n" "$title"
    echo -e "  ╚══════════════════════════════════════════════════╝"
    echo -e "\033[0m"
    echo -e "\033[2m  TIME     LEVEL  MESSAGE\033[0m"
    echo -e "\033[2m  ──────   ─────  ─────────────────────────────────────────\033[0m"
    echo ""
}

# ─── Stream logs Control Plane ────────────────────────────────────────────────
stream_cp_logs() {
    _log_header "\033[0;35m" "CONTROL PLANE  ztna-cp  ${CP_IP}:8080"
    ssh $SSH_BASE ztna@${CP_IP} \
        'sudo journalctl -u ztna-cp -f --no-pager -o cat 2>/dev/null || \
         sudo journalctl -u ztna-cp -f --no-pager 2>/dev/null || \
         (echo "Service ztna-cp introuvable" && sleep 60)' \
    | python3 -c "$JSON_FORMATTER"
}

# ─── Stream logs Gateway ──────────────────────────────────────────────────────
stream_gw_logs() {
    _log_header "\033[0;34m" "GATEWAY  ztna-gw  ${GW_IP}:9443"
    ssh $SSH_BASE ztna@${GW_IP} \
        'sudo journalctl -u ztna-gateway -f --no-pager -o cat 2>/dev/null || \
         sudo journalctl -u ztna-gateway -f --no-pager 2>/dev/null || \
         (echo "Service ztna-gateway introuvable" && sleep 60)' \
    | python3 -c "$JSON_FORMATTER"
}
