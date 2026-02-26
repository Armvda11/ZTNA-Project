#!/bin/bash
# Test E2E depuis wan-client vers ztna-cp
# Ce script simule un vrai utilisateur qui interagit avec le control plane

set -euo pipefail

# Configuration
CP_HOST="10.10.20.30"
CP_PORT="8080"
KC_HOST="10.10.20.30"
KC_PROTO="${KC_PROTO:-https}"  # https (default) or http (legacy fallback)
if [[ "${KC_PROTO}" == "https" ]]; then
  KC_PORT="8443"
else
  KC_PORT="8081"
fi
TIMEOUT=15

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║  ZTNA LAB - Test E2E depuis wan-client → ztna-cp             ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""
echo "Client: $(hostname) ($(hostname -I | awk '{print $1}'))"
echo "Target: Control Plane at ${CP_HOST}:${CP_PORT}"
echo ""

# Test 1: Connectivité réseau de base
echo -e "${YELLOW}[1/6] Network Connectivity${NC}"
echo "────────────────────────────────────────"
echo "Testing ping to control plane..."
if timeout 5 ping -c 3 -W 2 $CP_HOST > /dev/null 2>&1; then
    echo -e "  ${GREEN}✅ PASS${NC} - Can reach control plane at $CP_HOST"
    PING_STATS=$(ping -c 3 -W 2 $CP_HOST 2>&1 | tail -1)
    echo "  Stats: $PING_STATS"
else
    echo -e "  ${RED}❌ FAIL${NC} - Cannot reach control plane"
    echo "  This might be a routing issue between WAN and DMZ"
    exit 1
fi
echo ""

# Test 2: Vérifier les routes
echo -e "${YELLOW}[2/6] Network Routes${NC}"
echo "────────────────────────────────────────"
echo "Current routing table:"
ip route | grep -E "10.10.(20|30)" || true
echo ""
echo "Testing route to DMZ (10.10.20.0/24):"
if ip route get 10.10.20.30 > /dev/null 2>&1; then
    ROUTE_INFO=$(ip route get 10.10.20.30)
    echo -e "  ${GREEN}✅ PASS${NC} - Route exists"
    echo "  $ROUTE_INFO"
else
    echo -e "  ${RED}❌ FAIL${NC} - No route to DMZ"
    exit 1
fi
echo ""

# Test 3: Control plane health check
echo -e "${YELLOW}[3/6] Control Plane Health${NC}"
echo "────────────────────────────────────────"
echo "Testing HTTPS endpoint: https://${CP_HOST}:${CP_PORT}/healthz"
if HEALTH=$(timeout $TIMEOUT curl -sSfk --max-time 10 https://$CP_HOST:$CP_PORT/healthz 2>&1); then
    if echo "$HEALTH" | grep -q "ok"; then
        echo -e "  ${GREEN}✅ PASS${NC} - Control plane is healthy"
        echo "  Response: $HEALTH"
    else
        echo -e "  ${RED}❌ FAIL${NC} - Unexpected response"
        echo "  Response: $HEALTH"
        exit 1
    fi
else
    echo -e "  ${RED}❌ FAIL${NC} - Cannot connect to control plane HTTPS"
    echo "  Error: $HEALTH"
    exit 1
fi
echo ""

# Test 4: Obtenir un token OIDC (comme un vrai utilisateur)
echo -e "${YELLOW}[4/6] User Authentication (OIDC)${NC}"
echo "────────────────────────────────────────"
echo "Authenticating as user 'alice' via Keycloak..."

TOKEN_RESPONSE=$(timeout $TIMEOUT curl -sSfk --max-time 10 -X POST \
  ${KC_PROTO}://$KC_HOST:$KC_PORT/realms/ztna/protocol/openid-connect/token \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "client_id=ztna-control-plane" \
  -d "client_secret=demo-secret" \
  -d "username=alice" \
  -d "password=Password123!" \
  -d "grant_type=password" 2>&1)

TOKEN=$(echo "$TOKEN_RESPONSE" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)

if [ -n "$TOKEN" ] && [ ${#TOKEN} -gt 100 ]; then
    echo -e "  ${GREEN}✅ PASS${NC} - Successfully authenticated"
    echo "  Token length: ${#TOKEN} characters"
    echo "  Token preview: ${TOKEN:0:60}..."
    export TOKEN
else
    echo -e "  ${RED}❌ FAIL${NC} - Authentication failed"
    echo "  Response: ${TOKEN_RESPONSE:0:300}"
    exit 1
fi
echo ""

# Test 5: Vérifier l'identité (whoami)
echo -e "${YELLOW}[5/6] Identity Verification${NC}"
echo "────────────────────────────────────────"
echo "Calling GET /api/v1/whoami..."

WHOAMI=$(timeout $TIMEOUT curl -sSfk --max-time 10 \
  -H "Authorization: Bearer $TOKEN" \
  https://$CP_HOST:$CP_PORT/api/v1/whoami 2>&1)

if echo "$WHOAMI" | grep -q '"username"'; then
    echo -e "  ${GREEN}✅ PASS${NC} - Identity verified"
    echo "  Response: $WHOAMI"
    
    USERNAME=$(echo "$WHOAMI" | grep -o '"username":"[^"]*' | cut -d'"' -f4)
    GROUPS=$(echo "$WHOAMI" | grep -o '"groups":\[[^]]*\]' || echo "groups:[unknown]")
    echo ""
    echo "  Authenticated as: $USERNAME"
    echo "  Groups: $GROUPS"
else
    echo -e "  ${RED}❌ FAIL${NC} - Identity check failed"
    echo "  Response: ${WHOAMI:0:300}"
    exit 1
fi
echo ""

# Test 6: Demander un certificat SSH (cas d'usage principal)
echo -e "${YELLOW}[6/6] SSH Certificate Request${NC}"
echo "────────────────────────────────────────"
echo "Generating SSH key pair..."

# Générer une clé temporaire
KEYFILE="/tmp/wan_client_test_key_$$"
if timeout 5 ssh-keygen -t ed25519 -f "$KEYFILE" -N "" -q 2>/dev/null; then
    echo -e "  ${GREEN}✓${NC} Key pair generated"
    PUB=$(cat "${KEYFILE}.pub")
    echo "  Public key: ${PUB:0:70}..."
    echo ""
    
    echo "Requesting SSH certificate from control plane..."
    CERT_JSON=$(timeout $TIMEOUT curl -sSfk --max-time 10 -X POST \
      https://$CP_HOST:$CP_PORT/api/v1/credentials/ssh-cert \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"public_key\": \"$PUB\", \"ttl_seconds\": 3600}" 2>&1)
    
    if echo "$CERT_JSON" | grep -q '"certificate"'; then
        echo -e "  ${GREEN}✅ PASS${NC} - Certificate issued successfully"
        
        CERT=$(echo "$CERT_JSON" | grep -o '"certificate":"[^"]*' | cut -d'"' -f4)
        KEY_ID=$(echo "$CERT_JSON" | grep -o '"key_id":"[^"]*' | cut -d'"' -f4)
        PRINCIPALS=$(echo "$CERT_JSON" | grep -o '"principals":\[[^]]*\]' || echo "[]")
        
        echo ""
        echo "  Certificate type: $(echo $CERT | cut -d' ' -f1)"
        echo "  Certificate length: ${#CERT} chars"
        echo "  Key ID: $KEY_ID"
        echo "  Principals: $PRINCIPALS"
        
        # Sauvegarder le certificat pour inspection
        echo "$CERT" > "${KEYFILE}-cert.pub"
        echo ""
        echo "  Certificate saved to: ${KEYFILE}-cert.pub"
        echo ""
        echo "  Certificate details:"
        ssh-keygen -L -f "${KEYFILE}-cert.pub" 2>/dev/null | head -20
    else
        echo -e "  ${RED}❌ FAIL${NC} - Certificate request failed"
        echo "  Response: ${CERT_JSON:0:300}"
        rm -f "$KEYFILE" "${KEYFILE}.pub"
        exit 1
    fi
    
    # Cleanup
    echo ""
    echo "Cleaning up temporary files..."
    rm -f "$KEYFILE" "${KEYFILE}.pub" "${KEYFILE}-cert.pub"
    echo -e "  ${GREEN}✓${NC} Done"
else
    echo -e "  ${RED}❌ FAIL${NC} - Could not generate SSH key"
    exit 1
fi
echo ""

# Summary
echo "╔════════════════════════════════════════════════════════════════╗"
echo "║                    Test Summary                                ║"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""
echo -e "${GREEN}✅ ALL TESTS PASSED${NC}"
echo ""
echo "Verification Results:"
echo "  ✓ Network connectivity from wan-client to ztna-cp (DMZ)"
echo "  ✓ Control plane health endpoint responding"
echo "  ✓ User authentication via Keycloak OIDC"
echo "  ✓ Identity verification (whoami endpoint)"
echo "  ✓ SSH certificate issuance working"
echo ""
echo "Flow Verified:"
echo "  wan-client (10.10.10.10)"
echo "      ↓ HTTPS request"
echo "  ztna-cp (10.10.20.30:8080)"
echo "      ↓ OIDC validation"
echo "  Keycloak (10.10.20.30:$KC_PORT)"
echo "      ↓ JWT token"
echo "  Control Plane Auth ✓"
echo "      ↓ SSH Cert"
echo "  User receives certificate ✓"
echo ""
echo -e "${BLUE}The control plane (ztna-cp) is fully operational from client perspective!${NC}"
echo ""
