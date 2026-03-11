#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CLIENT_IP="10.10.10.10"
SSH_KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
SSH_COMMON_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -i "$SSH_KEY")

AUTO_DB=false
for arg in "$@"; do
  case "$arg" in
    --auto-db) AUTO_DB=true ;;
  esac
done

TMP_BIN="/tmp/ztna-client-live-$$"
cleanup() {
  rm -f "$TMP_BIN"
}
trap cleanup EXIT

echo "━━━ CLIENT LIVE SHELL (wan-client) ━━━"
echo

echo "[1/2] Préparation binaire client ztna (si Go local disponible)..."
if command -v go >/dev/null 2>&1; then
  if (cd "$PROJECT_DIR/client" && go build -o "$TMP_BIN" ./cmd/ztna >/dev/null 2>&1); then
    scp "${SSH_COMMON_OPTS[@]}" "$TMP_BIN" "ztna@${CLIENT_IP}:/tmp/ztna-client-live" >/dev/null
    ssh "${SSH_COMMON_OPTS[@]}" "ztna@${CLIENT_IP}" \
      "mkdir -p ~/.local/bin && install -m 0755 /tmp/ztna-client-live ~/.local/bin/ztna && rm -f /tmp/ztna-client-live" >/dev/null
    echo "  ✓ ztna installé dans ~/.local/bin/ztna sur wan-client"
  else
    echo "  ! Build local ztna échoué — fallback remote (go run) si disponible"
  fi
else
  echo "  ! Go non trouvé localement — fallback remote (go run) si disponible"
fi

echo "[2/2] Connexion SSH interactive vers wan-client..."

if [[ "$AUTO_DB" == true ]]; then
  REMOTE_AUTO_DB=1
else
  REMOTE_AUTO_DB=0
fi

ssh "${SSH_COMMON_OPTS[@]}" "ztna@${CLIENT_IP}" "REMOTE_AUTO_DB=${REMOTE_AUTO_DB} bash -s" <<'REMOTE_EOF'
set -euo pipefail
export LANG=C.UTF-8
export LC_ALL=C.UTF-8

mkdir -p "$HOME/.local/bin"
RC_FILE="$HOME/.ztna-live-rc"

cat > "$RC_FILE" <<'RC_EOF'
export PATH="$HOME/.local/bin:$PATH"
export LANG=C.UTF-8
export LC_ALL=C.UTF-8

find_client_dir() {
  local candidates=(
    "$HOME/ztna/client"
    "$HOME/ztna/ztna/client"
    "$HOME/client"
  )
  local d
  for d in "${candidates[@]}"; do
    if [[ -f "$d/go.mod" ]]; then
      echo "$d"
      return 0
    fi
  done
  return 1
}

resolve_cfg_path() {
  local client_dir="$1"
  local cfg_input="${2:-config.lab.yaml}"

  if [[ "$cfg_input" == /* ]] && [[ -f "$cfg_input" ]]; then
    echo "$cfg_input"
    return 0
  fi

  if [[ -f "$client_dir/$cfg_input" ]]; then
    echo "$client_dir/$cfg_input"
    return 0
  fi

  if [[ -f "$client_dir/config.lab.yaml" ]]; then
    echo "$client_dir/config.lab.yaml"
    return 0
  fi

  return 1
}

ztna_cmd() {
  if command -v ztna >/dev/null 2>&1; then
    ztna "$@"
    return
  fi

  local client_dir
  client_dir=$(find_client_dir || true)

  if [[ -n "$client_dir" ]] && [[ -x "$client_dir/ztna" ]]; then
    "$client_dir/ztna" "$@"
    return
  fi

  if [[ -n "$client_dir" ]] && command -v go >/dev/null 2>&1; then
    (cd "$client_dir" && go run ./cmd/ztna "$@")
    return
  fi

  echo "[ERREUR] ztna indisponible (ni binaire, ni fallback go run)."
  echo "         Vérifiez que le repo est présent sous ~/ztna et/ou Go installé."
  return 127
}

ensure_psql() {
  if command -v psql >/dev/null 2>&1; then
    return 0
  fi

  echo "[INFO] Préparation client PostgreSQL..."
  if command -v apt-get >/dev/null 2>&1; then
    if ! sudo -n true >/dev/null 2>&1; then
      echo "[ERREUR] sudo sans mot de passe requis pour installer psql automatiquement."
      return 1
    fi
    if ! (DEBIAN_FRONTEND=noninteractive sudo -n apt-get update -qq >/dev/null 2>&1 && \
      DEBIAN_FRONTEND=noninteractive sudo -n apt-get install -y -qq postgresql-client >/dev/null 2>&1); then
      echo "[ERREUR] Installation automatique de psql échouée."
      return 1
    fi
    return 0
  fi

  echo "[ERREUR] apt-get introuvable: installez manuellement le client PostgreSQL."
  return 1
}

ztna-db-live() {
  local cfg_in="${ZTNA_CFG:-config.lab.yaml}"
  local addr="${ZTNA_LOCAL_ADDR:-127.0.0.1:15432}"
  local user="${ZTNA_DB_USER:-alice}"
  local db="${ZTNA_DB_NAME:-appdb}"

  ensure_psql || return 1

  local workdir
  workdir=$(find_client_dir || true)
  if [[ -z "$workdir" ]]; then
    echo "[ERREUR] Répertoire client introuvable (go.mod non trouvé)."
    return 1
  fi

  local cfg
  cfg=$(resolve_cfg_path "$workdir" "$cfg_in" || true)
  if [[ -z "$cfg" ]]; then
    echo "[ERREUR] Fichier de configuration introuvable: $cfg_in"
    return 1
  fi

  local port
  port="${addr##*:}"

  (
    cd "$workdir"
    ztna_cmd -config "$cfg" access pg-staging "$addr"
  ) >/tmp/ztna-db-live.log 2>&1 &
  local tunnel_pid=$!

  local ready=0
  local i
  for i in $(seq 1 60); do
    if (echo >"/dev/tcp/127.0.0.1/${port}") >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 0.2
  done

  if [[ "$ready" != "1" ]]; then
    echo "[ERREUR] Tunnel non prêt sur 127.0.0.1:${port}."
    tail -n 20 /tmp/ztna-db-live.log 2>/dev/null || true
    kill "$tunnel_pid" >/dev/null 2>&1 || true
    wait "$tunnel_pid" 2>/dev/null || true
    return 1
  fi

  psql -h 127.0.0.1 -p "$port" -U "$user" -d "$db"

  kill "$tunnel_pid" >/dev/null 2>&1 || true
  wait "$tunnel_pid" 2>/dev/null || true
}

client_dir_for_prompt=$(find_client_dir || true)
if [[ -n "$client_dir_for_prompt" ]]; then
  cd "$client_dir_for_prompt"
fi

echo
echo "Session live prête (même terminal)."
echo "Commande recommandée :"
echo "  ztna-db-live"
echo
echo "Mode manuel :"
echo "  ztna_cmd -config config.lab.yaml access pg-staging 127.0.0.1:15432 &"
echo "  psql -h 127.0.0.1 -p 15432 -U alice -d appdb"
echo
echo "Pour terminer : Ctrl+C (tunnel), puis exit"
echo

if [[ "${REMOTE_AUTO_DB:-0}" == "1" ]]; then
  echo "[AUTO] Ouverture session SQL..."
  ztna-db-live || true
  echo
  echo "[AUTO] Session SQL terminée. Shell interactif disponible."
  echo
fi
RC_EOF

exec bash --rcfile "$RC_FILE" -i
REMOTE_EOF

ssh -tt "${SSH_COMMON_OPTS[@]}" "ztna@${CLIENT_IP}" "REMOTE_AUTO_DB=${REMOTE_AUTO_DB} bash --rcfile ~/.ztna-live-rc -i"

echo

echo "[INFO] Session client fermée."
