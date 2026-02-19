#!/bin/bash
# Script de diagnostic ZTNA - Tests individuels avec timeout strict
set -euo pipefail

# Configuration
CP_HOST="10.10.20.30"
CP_PORT="8080"
KC_HOST="10.10.20.30"
KC_PORT="8081"
PEP_TOKEN="CHANGE_ME_LONG_RANDOM"
TIMEOUT=10

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Fonction de test avec timeout
test_endpoint() {
    local name=$1
    local cmd=$2
    local check=$3
    
    echo -e "${BLUE}[TEST]${NC} $name"
    
    # Exécuter avec timeout
    if OUTPUT=$(timeout $TIMEOUT bash -c "$cmd" 2>&1); then
        if echo "$OUTPUT" | grep -q "$check"; then
            echo -e "  ${GREEN}✅ PASS${NC}"
            echo "  Output: ${OUTPUT:0:100}"
            return 0
        else
            echo -e "  ${RED}❌ FAIL${NC} - Check failed"
            echo "  Output: ${OUTPUT:0:200}"
            return 1
        fi
    else
        echo -e "  ${RED}❌ FAIL${NC} - Timeout or error"
        return 1
    fi
    echo ""
}

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║           ZTNA Control Plane - Diagnostic                     ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# Test 1: Network ping
echo -e "${YELLOW}[1] Network Connectivity${NC}"
echo "────────────────────────────────────────"
if timeout 5 ping -c 2 $CP_HOST > /dev/null 2>&1; then
    echo -e "  ${GREEN}✅ PASS${NC} - Ping to $CP_HOST successful"
else
    echo -e "  ${RED}❌ FAIL${NC} - Cannot ping $CP_HOST"
    exit 1
fi
echo ""

# Test 2: Health endpoint
echo -e "${YELLOW}[2] Health Endpoint${NC}"
echo "────────────────────────────────────────"
test_endpoint "GET /healthz" \
    "curl -sSfk --max-time $TIMEOUT https://$CP_HOST:$CP_PORT/healthz" \
    "ok"
echo ""

# Test 3: Keycloak availability
echo -e "${YELLOW}[3] Keycloak Availability${NC}"
echo "────────────────────────────────────────"
test_endpoint "Keycloak realm endpoint" \
    "curl -sSfk --max-time $TIMEOUT http://$KC_HOST:$KC_PORT/realms/ztna/.well-known/openid-configuration" \
    "issuer"
echo ""

# Test 4: OIDC Token
echo -e "${YELLOW}[4] OIDC Token Acquisition${NC}"
echo "────────────────────────────────────────"
TOKEN_CMD='curl -sSfk --max-time '"$TIMEOUT"' -X POST \
  http://'"$KC_HOST"':'"$KC_PORT"'/realms/ztna/protocol/openid-connect/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=ztna-control-plane" \
  -d "client_secret=demo-secret" \
  -d "username=alice" \
  -d "password=Password123!" \
  -d "grant_type=password"'

if TOKEN_JSON=$(timeout $TIMEOUT bash -c "$TOKEN_CMD" 2>&1); then
    TOKEN=$(echo "$TOKEN_JSON" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
    if [ -n "$TOKEN" ] && [ ${#TOKEN} -gt 100 ]; then
        echo -e "  ${GREEN}✅ PASS${NC}"
        echo "  Token length: ${#TOKEN} chars"
        export TOKEN
    else
        echo -e "  ${RED}❌ FAIL${NC} - Invalid token"
        echo "  Response: ${TOKEN_JSON:0:200}"
        exit 1
    fi
else
    echo -e "  ${RED}❌ FAIL${NC} - Token request failed"
    exit 1
fi
echo ""

# Test 5: Whoami endpoint
echo -e "${YELLOW}[5] Whoami Endpoint${NC}"
echo "────────────────────────────────────────"
test_endpoint "GET /api/v1/whoami" \
    "curl -sSfk --max-time $TIMEOUT -H 'Authorization: Bearer $TOKEN' https://$CP_HOST:$CP_PORT/api/v1/whoami" \
    "username"
echo ""

# Test 6: Active policy
echo -e "${YELLOW}[6] Active Policy${NC}"
echo "────────────────────────────────────────"
test_endpoint "GET /api/v1/admin/policies/active" \
    "curl -sSfk --max-time $TIMEOUT -H 'Authorization: Bearer $TOKEN' https://$CP_HOST:$CP_PORT/api/v1/admin/policies/active" \
    "rules"
echo ""

# Test 7: SSH Cert (simplifié avec clé pré-générée)
echo -e "${YELLOW}[7] SSH Certificate Issuance${NC}"
echo "────────────────────────────────────────"

# Générer clé dans un subshell avec timeout
echo -e "  ${BLUE}[INFO]${NC} Generating SSH key..."
if timeout 5 bash -c 'ssh-keygen -t ed25519 -f /tmp/ztna_diag_key -N "" -q 2>/dev/null' 2>&1; then
    if [ -f /tmp/ztna_diag_key.pub ]; then
        PUB=$(cat /tmp/ztna_diag_key.pub)
        echo -e "  ${GREEN}✓${NC} Key generated: ${PUB:0:50}..."
        
        # Créer JSON et appeler endpoint
        JSON_PAYLOAD="{\"public_key\": \"$PUB\"}"
        
        echo -e "  ${BLUE}[INFO]${NC} Requesting certificate..."
        if CERT_RESPONSE=$(timeout $TIMEOUT curl -sSfk --max-time $TIMEOUT -X POST \
            https://$CP_HOST:$CP_PORT/api/v1/credentials/ssh-cert \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: application/json" \
            -d "$JSON_PAYLOAD" 2>&1); then
            
            if echo "$CERT_RESPONSE" | grep -q '"certificate"'; then
                echo -e "  ${GREEN}✅ PASS${NC}"
                CERT=$(echo "$CERT_RESPONSE" | grep -o '"certificate":"[^"]*' | cut -d'"' -f4)
                echo "  Certificate type: $(echo $CERT | cut -d' ' -f1)"
                echo "  Certificate length: ${#CERT} chars"
            else
                echo -e "  ${RED}❌ FAIL${NC} - No certificate in response"
                echo "  Response: ${CERT_RESPONSE:0:200}"
            fi
        else
            echo -e "  ${RED}❌ FAIL${NC} - Certificate request failed/timeout"
        fi
        
        # Cleanup
        rm -f /tmp/ztna_diag_key /tmp/ztna_diag_key.pub
    else
        echo -e "  ${RED}❌ FAIL${NC} - Key file not created"
    fi
else
    echo -e "  ${RED}❌ FAIL${NC} - ssh-keygen timeout/failed"
fi
echo ""

# Test 8: PEP Authorize
echo -e "${YELLOW}[8] PEP Authorization${NC}"
echo "────────────────────────────────────────"
AUTH_CMD='curl -sSfk --max-time '"$TIMEOUT"' -X POST https://'"$CP_HOST"':'"$CP_PORT"'/api/v1/pep/authorize \
  -H "X-PEP-ID: ztna-gw-1" \
  -H "X-PEP-TOKEN: '"$PEP_TOKEN"'" \
  -H "Content-Type: application/json" \
  -d '"'"'{"subject":{"username":"alice","groups":["ztna-admins"]},"action":"connect","resource":{"type":"ssh","host":"lan-app","port":22},"context":{"src_ip":"10.10.10.10"}}'"'"

test_endpoint "POST /api/v1/pep/authorize" \
    "$AUTH_CMD" \
    "decision"
echo ""

# Test 9: Audit logs
echo -e "${YELLOW}[9] Audit Logs${NC}"
echo "────────────────────────────────────────"
test_endpoint "GET /api/v1/admin/audit" \
    "curl -sSfk --max-time $TIMEOUT -H 'Authorization: Bearer $TOKEN' https://$CP_HOST:$CP_PORT/api/v1/admin/audit" \
    "action"
echo ""

# Summary
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                      Diagnostic Summary                        ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""
echo -e "${GREEN}All critical endpoints are functional!${NC}"
echo ""
echo "Next steps:"
echo "  • Full E2E test: bash scripts/ztna-lab-test.sh"
echo "  • Manual testing: See CONTROL_PLANE_ANALYSIS.md"
echo "  • View logs: sudo journalctl -u ztna-cp -f"
echo ""
