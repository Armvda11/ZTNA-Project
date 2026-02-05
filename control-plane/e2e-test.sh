#!/bin/bash
###############################################################################
# ZTNA Control Plane - End-to-End VM Tests with Reports
#
# Usage:
#   ./e2e-test.sh                                  # Test HTTP
#   ./e2e-test.sh --https                          # Test HTTPS (with -k for self-signed)
#   ./e2e-test.sh --report json                    # JSON report
#   ./e2e-test.sh --report markdown                # Markdown report
#   ./e2e-test.sh --fix-known-hosts                # Clear SSH host keys
#   BASE_URL=https://10.10.20.30:8443 ./e2e-test.sh --https
###############################################################################

set -u

VM_CP="10.10.20.30"
VM_WAN="10.10.10.10"
VM_WAN_ATTACKER="10.10.10.11"
VM_GW_WAN="10.10.10.20"
VM_APP="10.10.30.10"
VM_ADMIN="10.10.30.11"
VM_USER="ztna"

PROTOCOL="http"
CURL_OPTS=""
BASE_URL_DEFAULT="http://${VM_CP}:8443"
BASE_URL="${BASE_URL:-$BASE_URL_DEFAULT}"

SSH_OPTS="-o StrictHostKeyChecking=accept-new -o ConnectTimeout=5"
FIX_KNOWN_HOSTS=false
REPORT_FORMAT=""
REPORT_FILE=""

# Parse arguments
while [ $# -gt 0 ]; do
    case "$1" in
        --https)
            PROTOCOL="https"
            CURL_OPTS="-k"
            if [[ "$BASE_URL" == http://* ]]; then
                BASE_URL="${BASE_URL//http/https}"
            fi
            shift
            ;;
        --fix-known-hosts)
            FIX_KNOWN_HOSTS=true
            shift
            ;;
        --report)
            shift
            if [ $# -gt 0 ]; then
                REPORT_FORMAT="$1"
                # Map format to extension
                EXT="$REPORT_FORMAT"
                [ "$REPORT_FORMAT" = "markdown" ] && EXT="md"
                REPORT_FILE="test-report-$(date +%s).${EXT}"
                shift
            else
                echo "Error: --report requires an argument (json or markdown)" >&2
                exit 1
            fi
            ;;
        *)
            echo "Unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

if [ "$FIX_KNOWN_HOSTS" = true ]; then
    ssh-keygen -f "$HOME/.ssh/known_hosts" -R "$VM_WAN" >/dev/null 2>&1 || true
    ssh-keygen -f "$HOME/.ssh/known_hosts" -R "$VM_WAN_ATTACKER" >/dev/null 2>&1 || true
    ssh-keygen -f "$HOME/.ssh/known_hosts" -R "$VM_GW_WAN" >/dev/null 2>&1 || true
    ssh-keygen -f "$HOME/.ssh/known_hosts" -R "$VM_CP" >/dev/null 2>&1 || true
fi

if ! command -v jq >/dev/null 2>&1; then
    echo "jq is required. Install it and re-run." >&2
    exit 1
fi

# Initialize reporting
declare -A TEST_RESULTS
TEST_RESULTS[ping_status]="unknown"
TEST_RESULTS[service_status]="unknown"
TEST_RESULTS[health_check]="unknown"
TEST_RESULTS[wan_health]="unknown"
TEST_RESULTS[login]="unknown"
TEST_RESULTS[cert_request]="unknown"
TEST_RESULTS[rate_limiting]="unknown"

declare -A TEST_DETAILS
TEST_DETAILS[start_time]="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
TEST_DETAILS[protocol]="$PROTOCOL"
TEST_DETAILS[base_url]="$BASE_URL"

cleanup() {
    rm -f /tmp/ztna_e2e_key /tmp/ztna_e2e_key.pub >/dev/null 2>&1 || true
    
    # Generate report if requested
    if [ -n "$REPORT_FORMAT" ] && [ -n "$REPORT_FILE" ]; then
        generate_report "$REPORT_FORMAT" "$REPORT_FILE"
    fi
}

trap cleanup EXIT

# Report generation functions
generate_report() {
    local format=$1
    local file=$2
    
    case "$format" in
        json)
            generate_json_report "$file" ;;
        markdown)
            generate_markdown_report "$file" ;;
    esac
}

generate_json_report() {
    local file=$1
    local end_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    
    cat > "$file" << 'EOFJ'
{
  "metadata": {
    "timestamp": "START_TIME",
    "end_time": "END_TIME",
    "protocol": "PROTOCOL",
    "base_url": "BASE_URL",
    "hostname": "HOSTNAME",
    "test_suite": "ztna-e2e"
  },
  "results": {
    "ping": "PING_STATUS",
    "service": "SERVICE_STATUS",
    "health_check": "HEALTH_STATUS",
    "wan_health": "WAN_STATUS",
    "login": "LOGIN_STATUS",
    "cert_request": "CERT_STATUS",
    "rate_limiting": "RATE_STATUS"
  },
  "next_steps": {
    "view_logs": "ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp -n 50'",
    "deploy_config": "./deploy-config-only.sh",
    "full_rebuild": "./deploy.sh"
  }
}
EOFJ
    
    # Replace placeholders
    sed -i "s|START_TIME|${TEST_DETAILS[start_time]}|g" "$file"
    sed -i "s|END_TIME|$end_time|g" "$file"
    sed -i "s|PROTOCOL|${TEST_DETAILS[protocol]}|g" "$file"
    sed -i "s|BASE_URL|${TEST_DETAILS[base_url]}|g" "$file"
    sed -i "s|HOSTNAME|$(hostname)|g" "$file"
    sed -i "s|PING_STATUS|${TEST_RESULTS[ping_status]}|g" "$file"
    sed -i "s|SERVICE_STATUS|${TEST_RESULTS[service_status]}|g" "$file"
    sed -i "s|HEALTH_STATUS|${TEST_RESULTS[health_check]}|g" "$file"
    sed -i "s|WAN_STATUS|${TEST_RESULTS[wan_health]}|g" "$file"
    sed -i "s|LOGIN_STATUS|${TEST_RESULTS[login]}|g" "$file"
    sed -i "s|CERT_STATUS|${TEST_RESULTS[cert_request]}|g" "$file"
    sed -i "s|RATE_STATUS|${TEST_RESULTS[rate_limiting]}|g" "$file"
    
    echo "✓ Report saved: $file"
}

generate_markdown_report() {
    local file=$1
    local end_time="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    
    cat > "$file" << 'EOFMD'
# ZTNA Control Plane - E2E Test Report

EOFMD
    
    cat >> "$file" << EOF
**Test Date:** ${TEST_DETAILS[start_time]} to $end_time  
**Protocol:** ${TEST_DETAILS[protocol]}  
**Base URL:** ${TEST_DETAILS[base_url]}  
**Host:** $(hostname)  

## Test Results

| Component | Status |
|-----------|--------|
| Ping (All VMs) | ${TEST_RESULTS[ping_status]} |
| Service Status | ${TEST_RESULTS[service_status]} |
| Health Check (Local) | ${TEST_RESULTS[health_check]} |
| Health Check (WAN Client) | ${TEST_RESULTS[wan_health]} |
| Authentication | ${TEST_RESULTS[login]} |
| SSL Certificate Request | ${TEST_RESULTS[cert_request]} |
| Rate Limiting | ${TEST_RESULTS[rate_limiting]} |

## Next Steps

- View logs: \`ssh ztna@${VM_CP} 'sudo journalctl -u ztna-cp -n 50'\`
- Deploy config: \`./deploy-config-only.sh\`
- Full rebuild: \`./deploy.sh\`

EOF
    echo "✓ Report saved: $file"
}

echo "== VM reachability =="
PING_OK=0
for ip in $VM_WAN $VM_WAN_ATTACKER $VM_GW_WAN $VM_CP $VM_APP $VM_ADMIN; do
    if ping -c 1 -W 1 "$ip" >/dev/null 2>&1; then
        echo "OK ping $ip"
        ((PING_OK++))
    else
        echo "FAIL ping $ip"
    fi
done

if [ $PING_OK -eq 6 ]; then
    TEST_RESULTS[ping_status]="pass"
else
    TEST_RESULTS[ping_status]="fail"
fi

echo ""

echo "== Control Plane service =="
if ssh $SSH_OPTS ztna@${VM_CP} 'systemctl is-active ztna-cp.service'; then
    echo "OK"
    TEST_RESULTS[service_status]="pass"
else
    echo "FAIL"
    TEST_RESULTS[service_status]="fail"
fi
echo ""

echo "== Control Plane status =="
if ssh $SSH_OPTS ztna@${VM_CP} 'systemctl status ztna-cp.service --no-pager | head -20'; then
    echo "OK"
else
    echo "FAIL"
fi
echo ""

echo "== Health check (host: ${PROTOCOL}) =="
if curl $CURL_OPTS -s ${BASE_URL}/health | jq .; then
    echo "OK"
    TEST_RESULTS[health_check]="pass"
else
    echo "FAIL"
    TEST_RESULTS[health_check]="fail"
fi
echo ""

echo "== Health check (wan-client: ${PROTOCOL}) =="
WAN_HEALTH_OUTPUT=""
if WAN_HEALTH_OUTPUT=$(ssh $SSH_OPTS ztna@${VM_WAN} "curl $CURL_OPTS -s ${BASE_URL}/health" 2>&1); then
    echo "$WAN_HEALTH_OUTPUT"
    echo "OK"
    TEST_RESULTS[wan_health]="pass"
else
    echo "FAIL (Gateway not yet implemented - v0.2.0)"
    echo "Details: $WAN_HEALTH_OUTPUT"
    
    # Check if failure is due to network unreachability (expected for missing gateway)
    if echo "$WAN_HEALTH_OUTPUT" | grep -qiE "connection refused|connection timed out|no route|unreachable"; then
        echo "Note: This is expected - Gateway (PEP) not yet implemented in v0.1.0"
        TEST_RESULTS[wan_health]="skip"
    else
        echo "Hint: run with --fix-known-hosts if the SSH host key changed."
        TEST_RESULTS[wan_health]="fail"
    fi
fi
echo ""
echo "== Login + policy checks =="
TOKEN=$(curl $CURL_OPTS -s -X POST ${BASE_URL}/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"alice123"}' | jq -r '.token' 2>/dev/null)

if [ -z "$TOKEN" ] || [ "$TOKEN" = "null" ]; then
    echo "FAIL: login failed"
    TEST_RESULTS[login]="fail"
else
    echo "Token OK"
    TEST_RESULTS[login]="pass"
    curl $CURL_OPTS -s -X GET ${BASE_URL}/api/v1/policies/lan-app -H "Authorization: Bearer $TOKEN" | jq .
    curl $CURL_OPTS -s -X GET ${BASE_URL}/api/v1/policies/lan-admin -H "Authorization: Bearer $TOKEN" | jq .

    BAD_TOKEN="${TOKEN}x"
    HTTP_STATUS=$(curl $CURL_OPTS -s -o /dev/null -w "%{http_code}" -X GET ${BASE_URL}/api/v1/policies/lan-app -H "Authorization: Bearer $BAD_TOKEN")
    echo "Invalid token status: $HTTP_STATUS"
fi

echo ""

echo "== SSH certificate request =="
ssh-keygen -t ed25519 -f /tmp/ztna_e2e_key -N "" -C "ztna-e2e" >/dev/null 2>&1
PUBKEY=$(cat /tmp/ztna_e2e_key.pub)
CERT=$(curl $CURL_OPTS -s -X POST ${BASE_URL}/api/v1/certs/request \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"public_key\":\"$PUBKEY\"}" | jq -r '.certificate' 2>/dev/null)
if [ -z "$CERT" ] || [ "$CERT" = "null" ]; then
    echo "FAIL: certificate request failed"
    TEST_RESULTS[cert_request]="fail"
else
    echo "Certificate OK"
    TEST_RESULTS[cert_request]="pass"
fi

echo ""

echo "== Audit logs (latest 5) =="
curl $CURL_OPTS -s -X GET ${BASE_URL}/api/v1/audit -H "Authorization: Bearer $TOKEN" | jq '.logs[0:5]'

echo ""

echo "== Rate limit check (login burst) =="
COUNT_429=$(seq 1 12 | xargs -I{} sh -c "curl $CURL_OPTS -s -o /dev/null -w '%{http_code}\n' -X POST ${BASE_URL}/api/v1/auth/login -H 'Content-Type: application/json' -d '{\"username\":\"alice\",\"password\":\"alice123\"}'" | awk '$1==429{c++} END{print c+0}')
echo "429 responses: $COUNT_429"

RATE_LIMIT_ENABLED="unknown"
if ssh $SSH_OPTS ztna@${VM_CP} 'grep -q "rate_limit:" /home/ztna/config.yaml'; then
    if ssh $SSH_OPTS ztna@${VM_CP} 'grep -q "enabled: true" /home/ztna/config.yaml'; then
        RATE_LIMIT_ENABLED="true"
    else
        RATE_LIMIT_ENABLED="false"
    fi
fi

if [ "$RATE_LIMIT_ENABLED" = "true" ] && [ "$COUNT_429" -eq 0 ]; then
    echo "FAIL: rate limiting appears disabled at runtime"
    echo "Hint: deploy updated config.yaml and restart service"
    TEST_RESULTS[rate_limiting]="fail"
elif [ "$COUNT_429" -gt 0 ]; then
    echo "PASS: rate limiting is working"
    TEST_RESULTS[rate_limiting]="pass"
else
    TEST_RESULTS[rate_limiting]="unknown"
fi

echo ""
echo "=== Test Summary ==="
FAILED=0
for test in "${!TEST_RESULTS[@]}"; do
    if [ "${TEST_RESULTS[$test]}" != "pass" ]; then
        FAILED=1
    fi
    printf "  %-25s %s\n" "${test}:" "${TEST_RESULTS[$test]}"
done

if [ "$FAILED" -ne 0 ]; then
    echo ""
    echo "E2E tests finished with issues"
    if [ -n "$REPORT_FILE" ]; then
        echo "Report generated: $REPORT_FILE"
    fi
    exit 1
fi

echo ""
echo "E2E tests finished successfully"
if [ -n "$REPORT_FILE" ]; then
    echo "Report generated: $REPORT_FILE"
fi
