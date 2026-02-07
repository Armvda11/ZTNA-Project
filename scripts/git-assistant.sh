#!/usr/bin/env bash
set -euo pipefail

if [[ -t 1 ]]; then
  C_RESET='\033[0m'
  C_RED='\033[0;31m'
  C_GREEN='\033[0;32m'
  C_YELLOW='\033[1;33m'
  C_BLUE='\033[0;34m'
  C_CYAN='\033[0;36m'
  C_BOLD='\033[1m'
else
  C_RESET=''
  C_RED=''
  C_GREEN=''
  C_YELLOW=''
  C_BLUE=''
  C_CYAN=''
  C_BOLD=''
fi

main_branch() {
  if [[ -n "${GIT_MAIN_BRANCH:-}" ]]; then
    printf '%s\n' "${GIT_MAIN_BRANCH}"
    return
  fi

  if git show-ref --verify --quiet refs/remotes/origin/main || git show-ref --verify --quiet refs/heads/main; then
    printf 'main\n'
    return
  fi

  if git show-ref --verify --quiet refs/remotes/origin/master || git show-ref --verify --quiet refs/heads/master; then
    printf 'master\n'
    return
  fi

  printf 'main\n'
}

log_info() {
  printf '%b[INFO]%b %s\n' "${C_BLUE}" "${C_RESET}" "$*"
}

log_ok() {
  printf '%b[OK]%b %s\n' "${C_GREEN}" "${C_RESET}" "$*"
}

log_warn() {
  printf '%b[WARN]%b %s\n' "${C_YELLOW}" "${C_RESET}" "$*"
}

log_error() {
  printf '%b[ERR]%b %s\n' "${C_RED}" "${C_RESET}" "$*"
}

run() {
  printf '%b$ %s%b\n' "${C_CYAN}" "$*" "${C_RESET}"
  "$@"
}

require_git_repo() {
  if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    log_error "Ce dossier n'est pas un repo Git."
    exit 1
  fi
}

current_branch() {
  git rev-parse --abbrev-ref HEAD
}

is_clean() {
  [[ -z "$(git status --porcelain)" ]]
}

slugify() {
  tr '[:upper:]' '[:lower:]' \
    | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-+/-/g'
}

sync_with_main() {
  local base branch
  base="$(main_branch)"
  branch="$(current_branch)"

  run git fetch origin --prune
  if [[ "${branch}" == "${base}" ]]; then
    run git pull --ff-only origin "${base}"
    log_ok "Branche ${base} à jour."
    return
  fi

  run git rebase "origin/${base}"
  log_ok "Branche ${branch} rebasée sur origin/${base}."
}

start_branch() {
  local type raw_name name branch base
  type="${1:-}"
  raw_name="${2:-}"
  base="$(main_branch)"

  if [[ -z "${type}" ]]; then
    printf 'Type (feat/fix/docs/refactor/chore/test/perf/hotfix): '
    read -r type
  fi

  if [[ -z "${raw_name}" ]]; then
    printf 'Nom court de la feature: '
    read -r raw_name
  fi

  name="$(printf '%s' "${raw_name}" | slugify)"
  type="$(printf '%s' "${type}" | slugify)"
  branch="${type}/${name}"

  if [[ -z "${name}" || -z "${type}" ]]; then
    log_error "Type ou nom invalide."
    exit 1
  fi

  if git show-ref --verify --quiet "refs/heads/${branch}"; then
    run git checkout "${branch}"
    log_warn "La branche existait déjà. Bascule dessus."
  else
    if is_clean; then
      run git fetch origin --prune
      run git checkout "${base}"
      run git pull --ff-only origin "${base}"
    else
      log_warn "Working tree non propre: création de branche avec changements en cours."
    fi
    run git checkout -b "${branch}"
    log_ok "Branche créée: ${branch}"
  fi
}

commit_easy() {
  local commit_type subject msg stage_answer
  commit_type="${1:-}"
  subject="${2:-}"

  if [[ -z "$(git status --porcelain)" ]]; then
    log_warn "Aucun changement à commit."
    return
  fi

  printf '%bChangements:%b\n' "${C_BOLD}" "${C_RESET}"
  git status --short
  printf '\n'
  if [[ -t 0 ]]; then
    printf 'Stager tous les fichiers (git add -A) ? [Y/n]: '
    read -r stage_answer
  elif [[ "${GIT_ASSISTANT_STAGE_ALL:-0}" == "1" ]]; then
    stage_answer="Y"
  else
    stage_answer="n"
  fi
  if [[ ! "${stage_answer:-Y}" =~ ^[Nn]$ ]]; then
    run git add -A
  fi

  if [[ -z "${commit_type}" ]]; then
    if [[ -t 0 ]]; then
      printf 'Type commit (feat/fix/docs/refactor/chore/test/perf): '
      read -r commit_type
    fi
  fi

  if [[ -z "${subject}" ]]; then
    if [[ -t 0 ]]; then
      printf 'Message court: '
      read -r subject
    fi
  fi

  commit_type="$(printf '%s' "${commit_type}" | slugify)"
  subject="$(printf '%s' "${subject}" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"

  if [[ -z "${commit_type}" || -z "${subject}" ]]; then
    log_error "Commit invalide. Type et message requis."
    exit 1
  fi

  msg="${commit_type}: ${subject}"
  run git commit -m "${msg}"
  log_ok "Commit créé: ${msg}"
}

publish_branch() {
  local branch upstream
  branch="$(current_branch)"
  if [[ "${branch}" == "$(main_branch)" ]]; then
    log_warn "Vous êtes sur la branche principale. Je pousse quand même."
  fi

  if upstream="$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null)"; then
    log_info "Upstream détecté: ${upstream}"
    run git push
  else
    run git push -u origin "${branch}"
  fi
  log_ok "Push terminé pour ${branch}."
}

finish_feature() {
  local base branch answer
  base="$(main_branch)"
  branch="$(current_branch)"

  if [[ "${branch}" == "${base}" ]]; then
    log_error "Finish doit être lancé sur une branche feature."
    exit 1
  fi

  if ! is_clean; then
    log_error "Working tree non propre. Commit/stash avant finish."
    exit 1
  fi

  log_info "Préparation de ${branch} pour PR propre."
  run git fetch origin --prune
  run git rebase "origin/${base}"
  run git push -u --force-with-lease origin "${branch}"
  log_ok "Branche feature mise à jour et poussée."

  if [[ -t 0 ]]; then
    printf 'Supprimer la branche locale + remote après merge PR ? [y/N]: '
    read -r answer
    if [[ "${answer:-N}" =~ ^[Yy]$ ]]; then
      log_info "Conservez cette commande pour plus tard:"
      printf '  %bgit branch -d %s && git push origin --delete %s%b\n' "${C_CYAN}" "${branch}" "${branch}" "${C_RESET}"
    fi
  fi
}

merge_to_main() {
  local base feature answer
  base="$(main_branch)"
  feature="$(current_branch)"

  if [[ "${feature}" == "${base}" ]]; then
    log_error "Vous êtes déjà sur ${base}."
    exit 1
  fi

  if ! is_clean; then
    log_error "Working tree non propre. Commit/stash avant merge."
    exit 1
  fi

  if [[ -t 0 ]]; then
    printf '%bAction sensible:%b merge direct vers %s puis push.\n' "${C_YELLOW}" "${C_RESET}" "${base}"
    printf 'Confirmer ? [y/N]: '
    read -r answer
    if [[ ! "${answer:-N}" =~ ^[Yy]$ ]]; then
      log_warn "Merge annulé."
      return
    fi
  else
    log_error "Merge direct nécessite un terminal interactif."
    exit 1
  fi

  run git fetch origin --prune
  run git checkout "${base}"
  run git pull --ff-only origin "${base}"
  run git merge --no-ff "${feature}" -m "merge(${feature}): into ${base}"
  run git push origin "${base}"
  run git checkout "${feature}"
  log_ok "Merge direct terminé. ${base} a été poussé."
}

show_agent_prompt() {
  local base branch
  base="$(main_branch)"
  branch="$(current_branch)"
  if [[ "${branch}" == "${base}" ]]; then
    cat <<EOF
Prompt agent:
Tu es mon agent Git. Sur le repo courant:
1) Crée une branche feature: ./scripts/git-assistant.sh start feat nom-feature
2) Vérifie le statut: ./scripts/git-assistant.sh status
3) Une fois le code prêt, lance: GIT_ASSISTANT_STAGE_ALL=1 ./scripts/git-assistant.sh commit feat "message"
4) Lance: ./scripts/git-assistant.sh publish
5) Prépare une PR de la branche feature vers ${base}.
EOF
    return
  fi

  cat <<EOF
Prompt agent:
Tu es mon agent Git. Sur le repo courant:
1) Vérifie que je suis sur ${branch}
2) Lance: ./scripts/git-assistant.sh sync
3) Lance: GIT_ASSISTANT_STAGE_ALL=1 ./scripts/git-assistant.sh commit
4) Lance: ./scripts/git-assistant.sh publish
5) Prépare une PR de ${branch} vers ${base} avec un résumé des commits.
EOF
}

show_status() {
  local base branch
  base="$(main_branch)"
  branch="$(current_branch)"

  printf '%b╔════════════════════════════════════════════════════╗%b\n' "${C_BLUE}" "${C_RESET}"
  printf '%b║                Git Assistant - Status              ║%b\n' "${C_BLUE}" "${C_RESET}"
  printf '%b╚════════════════════════════════════════════════════╝%b\n' "${C_BLUE}" "${C_RESET}"
  printf '%bBranche:%b %s\n' "${C_BOLD}" "${C_RESET}" "${branch}"
  printf '%bMain:%b    %s\n' "${C_BOLD}" "${C_RESET}" "${base}"
  printf '\n'
  git status -sb
}

on_open() {
  local base branch answer
  base="$(main_branch)"
  branch="$(current_branch)"

  log_info "Ouverture workspace détectée. Vérification Git rapide..."
  run git fetch origin --prune

  if [[ "${branch}" == "${base}" ]]; then
    if is_clean; then
      run git pull --ff-only origin "${base}"
      log_ok "${base} est à jour."
      if [[ -t 0 ]]; then
        printf 'Créer une nouvelle branche maintenant ? [y/N]: '
        read -r answer
        if [[ "${answer:-N}" =~ ^[Yy]$ ]]; then
          start_branch
        fi
      fi
    else
      log_warn "Vous êtes sur ${base} avec des changements locaux."
    fi
  else
    log_info "Vous êtes sur ${branch}. Lancez sync si besoin."
  fi
}

show_help() {
  cat <<'EOF'
Usage:
  ./scripts/git-assistant.sh menu
  ./scripts/git-assistant.sh on-open
  ./scripts/git-assistant.sh status
  ./scripts/git-assistant.sh start [type] [nom]
  ./scripts/git-assistant.sh sync
  ./scripts/git-assistant.sh commit [type] [message]
  ./scripts/git-assistant.sh publish
  ./scripts/git-assistant.sh finish
  ./scripts/git-assistant.sh merge
  ./scripts/git-assistant.sh agent
  ./scripts/git-assistant.sh help
EOF
}

menu() {
  local choice
  run_menu_action() {
    if ! "$@"; then
      log_warn "Action interrompue. Le menu continue."
    fi
  }

  while true; do
    printf '\n%b╔════════════════════════════════════════════════════╗%b\n' "${C_BLUE}" "${C_RESET}"
    printf '%b║              ZTNA Git Assistant Menu               ║%b\n' "${C_BLUE}" "${C_RESET}"
    printf '%b╚════════════════════════════════════════════════════╝%b\n' "${C_BLUE}" "${C_RESET}"
    printf '  %b1)%b Status\n' "${C_CYAN}" "${C_RESET}"
    printf '  %b2)%b Créer/Changer branche feature\n' "${C_CYAN}" "${C_RESET}"
    printf '  %b3)%b Sync avec main (rebase)\n' "${C_CYAN}" "${C_RESET}"
    printf '  %b4)%b Commit rapide\n' "${C_CYAN}" "${C_RESET}"
    printf '  %b5)%b Push (publish)\n' "${C_CYAN}" "${C_RESET}"
    printf '  %b6)%b Finish feature (prêt PR)\n' "${C_CYAN}" "${C_RESET}"
    printf '  %b7)%b Merge direct vers main\n' "${C_CYAN}" "${C_RESET}"
    printf '  %b8)%b Prompt agent\n' "${C_CYAN}" "${C_RESET}"
    printf '  %b9)%b Quitter\n' "${C_CYAN}" "${C_RESET}"
    printf 'Choix: '
    read -r choice

    case "${choice}" in
      1) run_menu_action show_status ;;
      2) run_menu_action start_branch ;;
      3) run_menu_action sync_with_main ;;
      4) run_menu_action commit_easy ;;
      5) run_menu_action publish_branch ;;
      6) run_menu_action finish_feature ;;
      7) run_menu_action merge_to_main ;;
      8) run_menu_action show_agent_prompt ;;
      9) break ;;
      *) log_warn "Choix invalide." ;;
    esac
  done
}

main() {
  local cmd
  cmd="${1:-menu}"
  require_git_repo

  case "${cmd}" in
    menu) menu ;;
    on-open) on_open ;;
    status) show_status ;;
    start) shift; start_branch "${1:-}" "${2:-}" ;;
    sync) sync_with_main ;;
    commit) shift; commit_easy "${1:-}" "${2:-}" ;;
    publish) publish_branch ;;
    finish) finish_feature ;;
    merge) merge_to_main ;;
    agent) show_agent_prompt ;;
    help|-h|--help) show_help ;;
    *)
      log_error "Commande inconnue: ${cmd}"
      show_help
      exit 1
      ;;
  esac
}

main "$@"
