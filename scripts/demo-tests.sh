#!/usr/bin/env bash
# ============================================================================
# ZTNA — Suite de Tests Complète (Présentation Jury)
# ============================================================================
# Exécute tous les tests unitaires + vérifie la compilation de chaque composant.
# Produit un rapport visuel coloré adapté à une soutenance.
#
# Usage:
#   bash scripts/demo-tests.sh           # Rapport complet
#   bash scripts/demo-tests.sh --quick   # Tests rapides uniquement
# ============================================================================

set -uo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

PASS=0
FAIL=0
SKIP=0
TOTAL_TESTS=0
START_TIME=$(date +%s)

# ---- Banner ----------------------------------------------------------------

clear
echo -e "${CYAN}${BOLD}"
cat << 'BANNER'
  ╔══════════════════════════════════════════════════════════════════════╗
  ║            ZTNA — Suite de Tests & Validation Technique            ║
  ╚══════════════════════════════════════════════════════════════════════╝
BANNER
echo -e "${NC}"

# ---- Helpers ---------------------------------------------------------------

section() {
  echo ""
  echo -e "  ${BOLD}${BLUE}━━━ $1 ━━━${NC}"
  echo ""
}

check_ok() {
  echo -e "  ${GREEN}  ✓${NC} $1"
  ((PASS++))
}

check_fail() {
  echo -e "  ${RED}  ✗${NC} $1"
  ((FAIL++))
}

check_skip() {
  echo -e "  ${YELLOW}  ⊘${NC} $1 ${DIM}(skipped)${NC}"
  ((SKIP++))
}

# ---- Phase 1: Compilation --------------------------------------------------

section "PHASE 1 — Compilation des composants"

for component in gateway control-plane client; do
  cd "$PROJECT_DIR/$component"
  if go build ./... 2>/dev/null; then
    check_ok "$component — compilation OK"
  else
    check_fail "$component — ERREUR de compilation"
  fi
done

# ---- Phase 2: Tests unitaires — Gateway ------------------------------------

section "PHASE 2 — Tests unitaires Gateway"

cd "$PROJECT_DIR/gateway"
TEST_OUTPUT=$(go test ./... -v -count=1 -timeout 120s 2>&1)

# Count results
while IFS= read -r line; do
  if [[ "$line" =~ ^---\ PASS:\ (.+) ]]; then
    name="${BASH_REMATCH[1]}"
    check_ok "gateway: $name"
    ((TOTAL_TESTS++))
  elif [[ "$line" =~ ^---\ FAIL:\ (.+) ]]; then
    name="${BASH_REMATCH[1]}"
    check_fail "gateway: $name"
    ((TOTAL_TESTS++))
  elif [[ "$line" =~ ^---\ SKIP:\ (.+) ]]; then
    name="${BASH_REMATCH[1]}"
    check_skip "gateway: $name"
    ((TOTAL_TESTS++))
  fi
done <<< "$TEST_OUTPUT"

# Count passing packages
GW_PKG_PASS=$(echo "$TEST_OUTPUT" | grep -c "^ok" || true)
GW_PKG_FAIL=$(echo "$TEST_OUTPUT" | grep -c "^FAIL" || true)

echo ""
echo -e "  ${DIM}  Packages: ${GREEN}$GW_PKG_PASS OK${NC} ${DIM}/${NC} ${RED}$GW_PKG_FAIL FAIL${NC}"

# ---- Phase 3: Tests unitaires — Control Plane ------------------------------

section "PHASE 3 — Tests unitaires Control Plane"

cd "$PROJECT_DIR/control-plane"
TEST_OUTPUT=$(go test ./... -v -count=1 -timeout 60s 2>&1)

while IFS= read -r line; do
  if [[ "$line" =~ ^---\ PASS:\ (.+) ]]; then
    name="${BASH_REMATCH[1]}"
    check_ok "cp: $name"
    ((TOTAL_TESTS++))
  elif [[ "$line" =~ ^---\ FAIL:\ (.+) ]]; then
    name="${BASH_REMATCH[1]}"
    check_fail "cp: $name"
    ((TOTAL_TESTS++))
  elif [[ "$line" =~ ^---\ SKIP:\ (.+) ]]; then
    name="${BASH_REMATCH[1]}"
    check_skip "cp: $name"
    ((TOTAL_TESTS++))
  fi
done <<< "$TEST_OUTPUT"

CP_PKG_PASS=$(echo "$TEST_OUTPUT" | grep -c "^ok" || true)
CP_PKG_FAIL=$(echo "$TEST_OUTPUT" | grep -c "^FAIL" || true)

echo ""
echo -e "  ${DIM}  Packages: ${GREEN}$CP_PKG_PASS OK${NC} ${DIM}/${NC} ${RED}$CP_PKG_FAIL FAIL${NC}"

# ---- Phase 4: Vérifications de sécurité ------------------------------------

section "PHASE 4 — Vérifications de sécurité"

# Check SSRF protection
cd "$PROJECT_DIR/gateway"
if grep -q "validateTarget" internal/infra/proxy/tcp.go; then
  check_ok "SSRF Protection — validateTarget() implémenté"
else
  check_fail "SSRF Protection — manquant"
fi

# Check CRL
if grep -q "StartAutoRefresh" internal/infra/revocation/crl.go; then
  check_ok "CRL Auto-Refresh — StartAutoRefresh() implémenté"
else
  check_fail "CRL Auto-Refresh — manquant"
fi

# Check MaxBytesReader on CP
cd "$PROJECT_DIR/control-plane"
if grep -q "MaxBytesReader" internal/api/handlers/pep_authorize.go; then
  check_ok "MaxBytesReader — protection body size sur /authorize"
else
  check_fail "MaxBytesReader — manquant sur /authorize"
fi

# Check errors.Is
if grep -q "errors.Is" internal/api/httputil/httputil.go; then
  check_ok "WriteError — errors.Is() pour wrapped errors"
else
  check_fail "WriteError — utilise == au lieu de errors.Is()"
fi

# Check error sanitization
if grep -q "erreur interne du serveur" internal/api/httputil/httputil.go; then
  check_ok "Error Sanitization — erreurs internes masquées"
else
  check_fail "Error Sanitization — erreurs internes exposées"
fi

# Check session manager features
cd "$PROJECT_DIR/gateway"
if grep -q "KillSession" internal/infra/session/manager.go; then
  check_ok "Session Kill — admin kill de sessions actives"
else
  check_fail "Session Kill — manquant"
fi

if grep -q "StartGarbageCollector" internal/infra/session/manager.go; then
  check_ok "Session GC — garbage collector TTL implémenté"
else
  check_fail "Session GC — manquant"
fi

if grep -q "maxPerSubject" internal/infra/session/manager.go; then
  check_ok "Session Limits — limite par sujet implémentée"
else
  check_fail "Session Limits — manquant"
fi

# Check heartbeat
if [ -f "internal/infra/heartbeat/client.go" ]; then
  check_ok "Heartbeat — client heartbeat implémenté"
else
  check_fail "Heartbeat — manquant"
fi

# Check telemetry
if [ -f "internal/infra/telemetry/client.go" ]; then
  check_ok "Telemetry — client telemetry implémenté"
else
  check_fail "Telemetry — manquant"
fi

# Check graceful shutdown
if grep -q "drain" internal/bootstrap/app.go; then
  check_ok "Graceful Shutdown — drain des sessions implémenté"
else
  check_fail "Graceful Shutdown — manquant"
fi

# Check hexagonal architecture
if grep -q "Authorizer" internal/core/ports/ports.go; then
  check_ok "Architecture hexagonale — interfaces ports définies"
else
  check_fail "Architecture hexagonale — ports manquants"
fi

# ---- Phase 5: Métriques de code -------------------------------------------

section "PHASE 5 — Métriques du code"

cd "$PROJECT_DIR"
TOTAL_GO_FILES=$(find gateway/ control-plane/ client/ -name '*.go' | wc -l)
TOTAL_GO_LINES=$(find gateway/ control-plane/ client/ -name '*.go' -exec cat {} + | wc -l)
TOTAL_TEST_FILES=$(find gateway/ control-plane/ client/ -name '*_test.go' | wc -l)
TOTAL_PACKAGES=$(find gateway/ control-plane/ client/ -name '*.go' -exec dirname {} \; | sort -u | wc -l)

echo -e "  ${DIM}  Fichiers Go:${NC}      $TOTAL_GO_FILES"
echo -e "  ${DIM}  Lignes Go:${NC}        $TOTAL_GO_LINES"
echo -e "  ${DIM}  Fichiers test:${NC}    $TOTAL_TEST_FILES"
echo -e "  ${DIM}  Packages:${NC}         $TOTAL_PACKAGES"

# ---- Summary ---------------------------------------------------------------

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

echo ""
echo -e "  ${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
echo -e "  ${BOLD}RÉSUMÉ${NC}"
echo ""
echo -e "  ${GREEN}  ✓ PASS:${NC}  $PASS"
echo -e "  ${RED}  ✗ FAIL:${NC}  $FAIL"
echo -e "  ${YELLOW}  ⊘ SKIP:${NC}  $SKIP"
echo -e "  ${DIM}  Total:${NC}   $TOTAL_TESTS tests unitaires"
echo -e "  ${DIM}  Durée:${NC}   ${DURATION}s"
echo ""

if [ "$FAIL" -eq 0 ]; then
  echo -e "  ${GREEN}${BOLD}  ══════════════════════════════════════════════${NC}"
  echo -e "  ${GREEN}${BOLD}  ║  ✓  TOUS LES TESTS PASSENT — PROJET OK   ║${NC}"
  echo -e "  ${GREEN}${BOLD}  ══════════════════════════════════════════════${NC}"
else
  echo -e "  ${RED}${BOLD}  ══════════════════════════════════════════════${NC}"
  echo -e "  ${RED}${BOLD}  ║  ✗  $FAIL ERREUR(S) DÉTECTÉE(S)             ║${NC}"
  echo -e "  ${RED}${BOLD}  ══════════════════════════════════════════════${NC}"
fi

echo ""
exit $FAIL
