# ZTNA Testing Guide

Complete guide for testing the ZTNA Control Plane with various options and reportingformats.

## Quick Start

### 1. Standard HTTP Test
```bash
./e2e-test.sh
```
Tests the Control Plane on HTTP, verifies all VMs and endpoints.

### 2. HTTPS Test (Self-Signed)
```bash
./e2e-test.sh --https
```
Tests the Control Plane on HTTPS with `-k` (insecure) flag for self-signed certificates.

**To enable HTTPS in config.yaml:**
```yaml
server:
  tls:
    enabled: true
    cert: "/etc/ztna/tls/server.crt"
    key: "/etc/ztna/tls/server.key"
```

Then redeploy:
```bash
./deploy.sh  # Full rebuild + TLS setup
```

### 3. With Test Reports

#### JSON Report
```bash
./e2e-test.sh --report json
# Creates: test-report-<timestamp>.json
```

#### Markdown Report
```bash
./e2e-test.sh --report markdown
# Creates: test-report-<timestamp>.md
```

#### HTTPS + Reports
```bash
./e2e-test.sh --https --report json
./e2e-test.sh --https --report markdown
```

### 4. Custom Base URL
```bash
BASE_URL=https://10.10.20.30:9443 ./e2e-test.sh --https
```

### 5. Fix SSH Known Hosts
```bash
./e2e-test.sh --fix-known-hosts
```
Clears stale SSH keys from `~/.ssh/known_hosts` (useful when VMs are recreated).

## Deployment Scripts

### Full Deployment (Build + Deploy)
```bash
./deploy.sh
```
- Builds the Go binary
- Deploys binary and config to VM
- Restarts the systemd service
- Validates health check

**Use when:**
- First deployment
- Code changes made
- Major Go dependency updates

### Config-Only Deployment (Fast)
```bash
./deploy-config-only.sh
```
- Only uploads `config.yaml` to VM
- Restarts the systemd service
- No rebuild required

**Use when:**
- Updating TLS settings
- Changing rate limits
- Modifying policies
- Adjusting log levels
- Quick iteration during testing

**Example Workflow:**
```bash
# Edit config.yaml
vim config.yaml

# Fast deploy without rebuild
./deploy-config-only.sh

# Verify changes
./e2e-test.sh
```

## Report Formats

### JSON Report
Useful for CI/CD pipelines and automated processing.

```json
{
  "metadata": {
    "timestamp": "2026-02-05T00:30:00Z",
    "protocol": "http",
    "base_url": "http://10.10.20.30:8443",
    "hostname": "dev-box"
  },
  "results": {
    "ping": "pass",
    "service": "pass",
    "health_check": "pass",
    "wan_health": "pass",
    "login": "pass",
    "cert_request": "pass",
    "rate_limiting": "pass"
  }
}
```

**Archive reports:**
```bash
mkdir -p test-reports
./e2e-test.sh --report json
mv test-report-*.json test-reports/
```

### Markdown Report
Useful for human-readable archival and documentation.

```markdown
# ZTNA Control Plane - E2E Test Report

**Test Date:** 2026-02-05T00:30:00Z  
**Protocol:** http  
**Base URL:** http://10.10.20.30:8443  

## Test Results

| Component | Status |
|-----------|--------|
| Ping (All VMs) | pass |
| Service Status | pass |
| Health Check (Local) | pass |
| ... | ... |
```

**Build a test report library:**
```bash
for i in {1..5}; do
  ./e2e-test.sh --report markdown
  sleep 2
done
ls -lh test-report-*.md
```

## Testing Scenarios

### Scenario 1: Verify Security Features
```bash
# Test bcrypt password hashing
curl -X POST http://10.10.20.30:8443/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"alice123"}'

# Test JWT token validation
TOKEN=$(curl -s -X POST http://10.10.20.30:8443/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"alice123"}' | jq -r '.token')

curl -X GET http://10.10.20.30:8443/api/v1/policies/lan-app \
  -H "Authorization: Bearer $TOKEN"

# Invalid token should return 401
curl -I -X GET http://10.10.20.30:8443/api/v1/policies/lan-app \
  -H "Authorization: Bearer invalid-token"
```

### Scenario 2: Test Rate Limiting
```bash
# Burst 12 logins - should get 429 responses
for i in {1..12}; do
  curl -s -o /dev/null -w "Request $i: %{http_code}\n" \
    -X POST http://10.10.20.30:8443/api/v1/auth/login \
    -H "Content-Type: application/json" \
    -d '{"username":"alice","password":"alice123"}'
done
```

### Scenario 3: HTTPS with Certificate Validation
```bash
# Generate self-signed cert (if not present)
mkdir -p /tmp/tls
openssl req -x509 -newkey rsa:4096 -keyout /tmp/tls/server.key \
  -out /tmp/tls/server.crt -days 365 -nodes -subj "/CN=10.10.20.30"

# Update config.yaml to point to cert/key
sed -i 's|enabled: false|enabled: true|' config.yaml
sed -i 's|/etc/ztna/tls/server.crt|/tmp/tls/server.crt|' config.yaml
sed -i 's|/etc/ztna/tls/server.key|/tmp/tls/server.key|' config.yaml

# Deploy and test
./deploy.sh
./e2e-test.sh --https --report json
```

### Scenario 4: Cross-Platform Testing
```bash
# Test from each VM
./e2e-test.sh                              # From localhost
ssh ztna@10.10.10.10 'curl ...'            # From wan-client
ssh ztna@10.10.30.10 'curl ...'            # From lan-app
ssh ztna@10.10.30.11 'curl ...'            # From lan-admin
```

## Troubleshooting

### "jq is required"
```bash
# Ubuntu/Debian
sudo apt-get install jq

# macOS
brew install jq

# RHEL/CentOS
sudo yum install jq
```

### "SSH host key changed"
```bash
./e2e-test.sh --fix-known-hosts
./e2e-test.sh
```

### HTTPS Certificate Errors
```bash
# View cert on VM
ssh ztna@10.10.20.30 'openssl x509 -in /etc/ztna/tls/server.crt -text'

# Test with curl insecure (as in e2e-test.sh)
curl -k https://10.10.20.30:8443/health
```

### Rate Limiting Not Working
```bash
# Check config is deployed
ssh ztna@10.10.20.30 'grep -A 3 "rate_limit:" /home/ztna/config.yaml'

# Redeploy config
./deploy-config-only.sh

# Verify service restarted
ssh ztna@10.10.20.30 'systemctl status ztna-cp'
```

## CI/CD Integration

### GitHub Actions Example
```yaml
- name: Run E2E Tests
  run: |
    cd control-plane
    ./e2e-test.sh --report json
    
- name: Archive Results
  uses: actions/upload-artifact@v3
  with:
    name: test-reports
    path: test-report-*.json
```

### GitLab CI Example
```yaml
test:e2e:
  script:
    - cd control-plane
    - ./e2e-test.sh --report json
    - ./e2e-test.sh --report markdown
  artifacts:
    paths:
      - test-report-*
    expire_in: 30 days
```

## Test Checklist

- [ ] HTTP endpoint responding
- [ ] HTTPS endpoint working (if enabled)
- [ ] JWT authentication working
- [ ] Policy validation correct
- [ ] Rate limiting active
- [ ] SSH certificates issued
- [ ] Audit logging capturing events
- [ ] Service restarts cleanly
- [ ] Config-only deploy works

## Performance Notes

- Full test suite: ~30-40 seconds
- With reports: +5 seconds
- Between tests: wait 2+ seconds for service to be ready
- Rate limit test: fast (sub-1 second)

## Logging

View test-related logs:
```bash
# From VM
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp -f'

# With timestamps
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp -n 50 --no-pager'

# JSON output
ssh ztna@10.10.20.30 'sudo journalctl -u ztna-cp -o json | jq .'
```

## Next Steps

1. **Production TLS:** Use real certificates (Let's Encrypt, self-signed validated)
2. **Monitoring:** Set up Prometheus endpoints and Grafana dashboards
3. **Load Testing:** Use `ab` or `wrk` to stress test endpoints
4. **Security Audit:** Run `./security-audit.sh` regularly
5. **Backup Testing:** Test backup/restore of audit logs and configuration
