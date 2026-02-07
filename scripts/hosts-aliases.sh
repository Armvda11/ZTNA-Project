#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-install}"
HOSTS_FILE="/etc/hosts"

START_MARKER="# >>> ZTNA LAB HOSTS >>>"
END_MARKER="# <<< ZTNA LAB HOSTS <<<"

BLOCK_CONTENT=$(cat <<'EOF'
# >>> ZTNA LAB HOSTS >>>
10.10.10.10 wan-client client
10.10.10.11 wan-attacker attacker
10.10.10.20 ztna-gw gw-wan
10.10.20.20 ztna-gw-dmz gw
10.10.20.30 ztna-cp cp control-plane
10.10.30.10 lan-app app
10.10.30.11 lan-admin admin
# <<< ZTNA LAB HOSTS <<<
EOF
)

log_info() { echo "[INFO] $*"; }
log_ok() { echo "[OK] $*"; }
log_warn() { echo "[WARN] $*"; }
log_err() { echo "[ERR] $*"; }

ensure_deps() {
  command -v awk >/dev/null 2>&1 || { log_err "awk introuvable"; exit 1; }
  command -v mktemp >/dev/null 2>&1 || { log_err "mktemp introuvable"; exit 1; }
}

write_hosts_with_block() {
  local tmp
  tmp="$(mktemp)"

  awk -v start="${START_MARKER}" -v end="${END_MARKER}" '
    $0 == start { skip = 1; next }
    $0 == end   { skip = 0; next }
    skip != 1   { print }
  ' "${HOSTS_FILE}" > "${tmp}"

  {
    echo ""
    echo "${BLOCK_CONTENT}"
  } >> "${tmp}"

  if [[ "${EUID}" -eq 0 ]]; then
    cat "${tmp}" > "${HOSTS_FILE}"
  else
    sudo cp "${tmp}" "${HOSTS_FILE}"
  fi

  rm -f "${tmp}"
}

remove_block_from_hosts() {
  local tmp
  tmp="$(mktemp)"

  awk -v start="${START_MARKER}" -v end="${END_MARKER}" '
    $0 == start { skip = 1; next }
    $0 == end   { skip = 0; next }
    skip != 1   { print }
  ' "${HOSTS_FILE}" > "${tmp}"

  if [[ "${EUID}" -eq 0 ]]; then
    cat "${tmp}" > "${HOSTS_FILE}"
  else
    sudo cp "${tmp}" "${HOSTS_FILE}"
  fi

  rm -f "${tmp}"
}

show_block() {
  if grep -qF "${START_MARKER}" "${HOSTS_FILE}"; then
    awk -v start="${START_MARKER}" -v end="${END_MARKER}" '
      $0 == start { print; inblock = 1; next }
      $0 == end   { print; inblock = 0; next }
      inblock == 1 { print }
    ' "${HOSTS_FILE}"
  else
    log_warn "Aucun alias ZTNA installé dans ${HOSTS_FILE}"
  fi
}

main() {
  ensure_deps

  case "${MODE}" in
    install)
      log_info "Installation des aliases ZTNA dans ${HOSTS_FILE}..."
      write_hosts_with_block
      log_ok "Aliases installés."
      show_block
      ;;
    remove)
      log_info "Suppression des aliases ZTNA de ${HOSTS_FILE}..."
      remove_block_from_hosts
      log_ok "Aliases supprimés."
      ;;
    show)
      show_block
      ;;
    *)
      log_err "Usage: $0 [install|remove|show]"
      exit 1
      ;;
  esac
}

main "$@"

