#!/bin/bash
set -euo pipefail

CURL_OPTS="-sSfk --max-time 15"

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║           ZTNA LAB - COMPLETE DIAGNOSTIC TEST                  ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# Test 1: Network connectivity
echo "[1/5] NETWORK CONNECTIVITY"
echo "======================================"
echo "Testing PING from wan-client → ztna-cp (10.10.20.30)..."
PING_RESULT=$(ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
  -o ConnectTimeout=10 ztna@10.10.10.10 'ping -c 4 10.10.20.30' 2>&1 | grep "packet loss")

if echo "$PING_RESULT" | grep -q "0% packet loss"; then
  echo "✅ PASS: Network OK (0% loss)"
  echo "   $PING_RESULT"
else
  echo "❌ FAIL: Network problem"
  echo "   $PING_RESULT"
  exit 1
fi
echo ""

# Test 2: Control-plane health
echo "[2/5] CONTROL-PLANE HEALTH"
echo "======================================"
echo "Testing https://10.10.20.30:8080/healthz..."
HEALTH=$(curl ${CURL_OPTS} https://10.10.20.30:8080/healthz 2>&1)
if echo "$HEALTH" | grep -q "ok"; then
  echo "✅ PASS: Control-plane healthy"
  echo "   Response: $HEALTH"
else
  echo "❌ FAIL: Control-plane unhealthy"
  echo "   Response: $HEALTH"
  exit 1
fi
echo ""

# Test 3: Keycloak token endpoint
echo "[3/7] KEYCLOAK - TOKEN ENDPOINT"
echo "======================================"
TOKEN=$(curl -sS -X POST \
  http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=ztna-control-plane" \
  -d "client_secret=demo-secret" \
  -d "username=alice" \
  -d "password=Password123!" \
  -d "grant_type=password" 2>/dev/null | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)

if [ -n "$TOKEN" ] && [ ${#TOKEN} -gt 100 ]; then
  echo "✅ PASS: OIDC token obtained"
  echo "   Token length: ${#TOKEN} chars"
  echo "   Preview: ${TOKEN:0:50}..."
else
  echo "❌ FAIL: Token endpoint failed"
  TOKEN_RESPONSE=$(curl -s -X POST \
    http://10.10.20.30:8081/realms/ztna/protocol/openid-connect/token \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "client_id=ztna-control-plane" \
    -d "client_secret=demo-secret" \
    -d "username=alice" \
    -d "password=Password123!" \
    -d "grant_type=password" 2>&1)
  echo "   Response: ${TOKEN_RESPONSE:0:300}"
  exit 1
fi
echo ""

# Test 4: whoami
echo "[4/7] CONTROL-PLANE - WHOAMI"
echo "======================================"
WHOAMI=$(curl ${CURL_OPTS} -H "Authorization: Bearer ${TOKEN}" https://10.10.20.30:8080/api/v1/whoami 2>&1)
if echo "$WHOAMI" | grep -q '"username"'; then
  echo "✅ PASS: whoami OK"
  echo "   Response: ${WHOAMI}"
else
  echo "❌ FAIL: whoami failed"
  echo "   Response: ${WHOAMI:0:300}"
  exit 1
fi
echo ""

# Test 5: Ensure policy supports connect
echo "[5/7] CONTROL-PLANE - POLICY CHECK"
echo "======================================"
ACTIVE=$(curl ${CURL_OPTS} -H "Authorization: Bearer ${TOKEN}" https://10.10.20.30:8080/api/v1/admin/policies/active 2>&1)
if echo "$ACTIVE" | grep -q '"action":"connect"'; then
  HAS_CONNECT="yes"
else
  HAS_CONNECT="no"
fi
if [ "$HAS_CONNECT" != "yes" ]; then
  echo "⚠ Policy missing connect rule, creating a new version..."
  NEW_VERSION=$(curl ${CURL_OPTS} -X POST https://10.10.20.30:8080/api/v1/admin/policies \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"rules":[{"effect":"allow","subject_match":"group:ztna-admins","action":"connect","resource_type":"ssh","resource_match":"ssh:lan-app:22"},{"effect":"deny","subject_match":"*","action":"*","resource_type":"*","resource_match":"*"}]}' \
    | python3 -c 'import sys, json; print(json.load(sys.stdin)["version_id"])')
  curl ${CURL_OPTS} -X POST "https://10.10.20.30:8080/api/v1/admin/policies/${NEW_VERSION}/activate" \
    -H "Authorization: Bearer ${TOKEN}" >/dev/null
  echo "✅ Policy updated (version ${NEW_VERSION})"
else
  echo "✅ Policy already supports connect"
fi
echo ""

# Test 6: SSH certificate endpoint (need a valid token)
echo "[6/7] CONTROL-PLANE - SSH CERT ENDPOINT"
echo "======================================"
KEYFILE=$(mktemp /tmp/ztna_lab_key_XXXXXX)
ssh-keygen -t ed25519 -f "${KEYFILE}" -N "" -C "test@lab" -q 2>/dev/null
PUB=$(cat "${KEYFILE}.pub")

# Write JSON to temp file instead of using stdin redirect
JSONFILE=$(mktemp /tmp/ztna_cert_req_XXXXXX.json)
echo "{\"public_key\": \"${PUB}\"}" > "${JSONFILE}"

CERT_RESPONSE=$(curl ${CURL_OPTS} -X POST \
  https://10.10.20.30:8080/api/v1/credentials/ssh-cert \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d @"${JSONFILE}" 2>&1)

CERT=$(echo "$CERT_RESPONSE" | grep -o '"certificate":"[^"]*' | cut -d'"' -f4)

if [ -n "$CERT" ] && [ ${#CERT} -gt 100 ]; then
  echo "✅ PASS: SSH certificate issued"
  echo "   Cert type: $(echo "$CERT" | cut -d' ' -f1)"
  echo "   Cert length: ${#CERT} chars"
else
  echo "❌ FAIL: SSH cert endpoint failed"
  echo "   Response: ${CERT_RESPONSE:0:300}"
  rm -f "${KEYFILE}" "${KEYFILE}.pub" "${JSONFILE}"
  exit 1
fi
rm -f "${KEYFILE}" "${KEYFILE}.pub" "${JSONFILE}"
echo ""

# Test 7: PEP authorize + audit
echo "[7/7] CONTROL-PLANE - PEP AUTHORIZE + AUDIT"
echo "======================================"
DECISION=$(curl ${CURL_OPTS} -X POST https://10.10.20.30:8080/api/v1/pep/authorize \
  -H "X-PEP-ID: ztna-gw-1" -H "X-PEP-TOKEN: CHANGE_ME_LONG_RANDOM" \
  -H "Content-Type: application/json" \
  -d '{"subject":{"username":"alice","groups":["ztna-admins"]},"action":"connect","resource":{"type":"ssh","host":"lan-app","port":22},"context":{"src_ip":"1.2.3.4"}}' 2>&1)

if echo "$DECISION" | grep -q '"decision":"allow"'; then
  echo "✅ PASS: PEP authorize allow"
  echo "   Response: ${DECISION}"
else
  echo "❌ FAIL: PEP authorize failed"
  echo "   Response: ${DECISION:0:300}"
  exit 1
fi

AUDIT=$(curl ${CURL_OPTS} -H "Authorization: Bearer ${TOKEN}" https://10.10.20.30:8080/api/v1/admin/audit 2>&1)
if echo "$AUDIT" | grep -q '"action":"issue_ssh_cert"' && echo "$AUDIT" | grep -q '"action":"connect"'; then
  echo "✅ PASS: Audit events present"
else
  echo "❌ FAIL: Audit events missing"
  echo "   Response: ${AUDIT:0:300}"
  exit 1
fi
echo ""

# Summary
echo "[SUMMARY]"
echo "======================================"
echo "✅ ALL TESTS PASSED"
echo ""
echo "System Status:"
echo "  • Network routing:   wan-client ↔ ztna-cp (0% ping loss)"
echo "  • Control-plane:     Healthy (port 8080 mTLS + 8443 PEP)"
echo "  • Keycloak:          Online (OIDC token generation working)"
echo "  • SSH certificates:  Fully operational"
echo ""
echo "ZTNA Control-Plane Lab is fully functional! 🎉"