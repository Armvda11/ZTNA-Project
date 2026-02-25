#!/usr/bin/env bash
# Vérifie le flux PEP register + heartbeat strict côté control-plane.
#
# Usage:
#   PEP_ID=ztna-gw-01 PEP_TOKEN=ztna-lab-pep-secret-2026 bash scripts/test-pep-register-heartbeat.sh

set -euo pipefail

CP_URL="${CP_URL:-https://10.10.20.30:8080}"
PEP_ID="${PEP_ID:-ztna-gw-01}"
PEP_TOKEN="${PEP_TOKEN:-ztna-lab-pep-secret-2026}"
GW_VERSION="${GW_VERSION:-dev-test}"
FINGERPRINT="${FINGERPRINT:-local-test-fp}"

log() { echo "[$(date +%H:%M:%S)] $*"; }
die() { echo "[ERROR] $*" >&2; exit 1; }

REGISTER_RESP=$(curl -sk --max-time 10 \
  -H "X-PEP-ID: ${PEP_ID}" \
  -H "X-PEP-TOKEN: ${PEP_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"gateway_id\":\"${PEP_ID}\",\"name\":\"${PEP_ID}\",\"version\":\"${GW_VERSION}\",\"fingerprint\":\"${FINGERPRINT}\"}" \
  "${CP_URL}/api/v1/pep/register")

if ! echo "${REGISTER_RESP}" | grep -q '"status":"registered"'; then
  die "register échoué: ${REGISTER_RESP}"
fi
log "✓ register OK: ${REGISTER_RESP}"

HEARTBEAT_RESP=$(curl -sk --max-time 10 \
  -H "X-PEP-ID: ${PEP_ID}" \
  -H "X-PEP-TOKEN: ${PEP_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"version\":\"${GW_VERSION}\"}" \
  "${CP_URL}/api/v1/pep/heartbeat")

if ! echo "${HEARTBEAT_RESP}" | grep -q '"status":"registered"'; then
  die "heartbeat non strict/ko: ${HEARTBEAT_RESP}"
fi
log "✓ heartbeat OK: ${HEARTBEAT_RESP}"

log "✅ Flux PEP register + heartbeat validé"

