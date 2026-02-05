#!/bin/bash
###############################################################################
# ZTNA Control Plane - Security Audit Script
# 
# Vérifie les vulnérabilités connues dans les dépendances Go
###############################################################################

set -e

# Couleurs
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[⚠]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1"
}

echo "╔════════════════════════════════════════════════════════╗"
echo "║   ZTNA Control Plane - Security Audit                 ║"
echo "╚════════════════════════════════════════════════════════╝"
echo ""

# Vérifier que nous sommes dans le bon répertoire
if [ ! -f "main.go" ]; then
    log_error "main.go not found. Run from control-plane directory."
    exit 1
fi

# 1. Vérifier les versions des dépendances critiques
log_info "Checking critical dependencies..."
echo ""

# golang.org/x/crypto
CRYPTO_VERSION=$(go list -m golang.org/x/crypto | awk '{print $2}')
log_info "golang.org/x/crypto: $CRYPTO_VERSION"

# Versions vulnérables connues
VULNERABLE_CRYPTO=("v0.18.0" "v0.17.0" "v0.16.0" "v0.15.0" "v0.14.0")
IS_VULNERABLE_CRYPTO=false

for vuln_ver in "${VULNERABLE_CRYPTO[@]}"; do
    if [ "$CRYPTO_VERSION" == "$vuln_ver" ]; then
        IS_VULNERABLE_CRYPTO=true
        break
    fi
done

if [ "$IS_VULNERABLE_CRYPTO" = true ]; then
    log_error "golang.org/x/crypto $CRYPTO_VERSION is VULNERABLE!"
    log_error "  CVE-2024-45337 (Critical 9.1) - Authorization bypass"
    log_error "  CVE-2025-22869 (High 7.5) - DoS attack"
    echo "  📋 Solution: go get golang.org/x/crypto@latest"
    FOUND_ISSUES=true
else
    log_success "golang.org/x/crypto $CRYPTO_VERSION is secure"
fi

echo ""

# github.com/golang-jwt/jwt/v5
JWT_VERSION=$(go list -m github.com/golang-jwt/jwt/v5 | awk '{print $2}')
log_info "github.com/golang-jwt/jwt/v5: $JWT_VERSION"

VULNERABLE_JWT=("v5.2.0" "v5.1.0" "v5.0.0")
IS_VULNERABLE_JWT=false

for vuln_ver in "${VULNERABLE_JWT[@]}"; do
    if [ "$JWT_VERSION" == "$vuln_ver" ]; then
        IS_VULNERABLE_JWT=true
        break
    fi
done

if [ "$IS_VULNERABLE_JWT" = true ]; then
    log_error "github.com/golang-jwt/jwt/v5 $JWT_VERSION is VULNERABLE!"
    log_error "  CVE-2025-30204 (High 7.5) - JWT memory exhaustion"
    echo "  📋 Solution: go get github.com/golang-jwt/jwt/v5@latest"
    FOUND_ISSUES=true
else
    log_success "github.com/golang-jwt/jwt/v5 $JWT_VERSION is secure"
fi

echo ""

# 2. Vérifier la configuration de sécurité
log_info "Checking security configuration..."
echo ""

# Vérifier jwt_secret
if grep -q "jwt_secret: \"change-this-in-production-use-env-var\"" config.yaml; then
    log_warn "JWT secret uses default value in config.yaml"
    log_warn "  Use environment variable ZTNA_JWT_SECRET instead"
    FOUND_WARNINGS=true
else
    log_success "JWT secret appears to be changed from default"
fi

# Vérifier TLS
if grep -q "enabled: false" config.yaml; then
    log_warn "TLS is DISABLED in config.yaml"
    log_warn "  Enable TLS in production for HTTPS"
    FOUND_WARNINGS=true
else
    log_success "TLS appears to be enabled"
fi

echo ""

# 3. Vérifier les permissions des fichiers sensibles
log_info "Checking file permissions..."
echo ""

# Vérifier si la VM est accessible
if ping -c 1 -W 1 10.10.20.30 &>/dev/null; then
    log_info "Checking CA key permissions on VM..."
    CA_PERMS=$(ssh -o ConnectTimeout=5 ztna@10.10.20.30 'stat -c %a /etc/ztna/ssh_ca 2>/dev/null' 2>/dev/null || echo "")
    
    if [ -n "$CA_PERMS" ]; then
        if [ "$CA_PERMS" != "600" ]; then
            log_error "CA private key has insecure permissions: $CA_PERMS (should be 600)"
            echo "  📋 Solution: ssh ztna@10.10.20.30 'sudo chmod 600 /etc/ztna/ssh_ca'"
            FOUND_ISSUES=true
        else
            log_success "CA private key has secure permissions (600)"
        fi
    else
        log_warn "Could not check CA key permissions (key may not exist yet)"
    fi
else
    log_warn "VM 10.10.20.30 not accessible, skipping remote checks"
fi

echo ""

# 4. Vérifier les dépendances indirectes
log_info "Checking for outdated dependencies..."
echo ""

OUTDATED=$(go list -u -m all 2>/dev/null | grep '\[' || true)
if [ -n "$OUTDATED" ]; then
    log_warn "Some dependencies have updates available:"
    echo "$OUTDATED"
    echo "  📋 Solution: go get -u all && go mod tidy"
    FOUND_WARNINGS=true
else
    log_success "All dependencies are up to date"
fi

echo ""

# 5. Exécuter go vet pour détecter les problèmes
log_info "Running go vet for security issues..."
echo ""

if go vet ./... 2>&1 | grep -q "vet:"; then
    log_error "go vet found potential issues"
    go vet ./...
    FOUND_ISSUES=true
else
    log_success "go vet found no issues"
fi

echo ""

# 6. Vérifier les secrets hardcodés (basic check)
log_info "Checking for hardcoded secrets..."
echo ""

SECRETS_FOUND=false

# Chercher des patterns de secrets
if grep -r -i "password.*=.*\".*\"" internal/ --include="*.go" | grep -v "test" | grep -v "example" &>/dev/null; then
    log_warn "Potential hardcoded passwords found in code"
    SECRETS_FOUND=true
fi

if grep -r "secret.*=.*\".*\"" internal/ --include="*.go" | grep -v "jwt_secret" | grep -v "test" &>/dev/null; then
    log_warn "Potential hardcoded secrets found in code"
    SECRETS_FOUND=true
fi

if [ "$SECRETS_FOUND" = false ]; then
    log_success "No obvious hardcoded secrets detected"
else
    log_warn "  Review code for hardcoded credentials"
    FOUND_WARNINGS=true
fi

echo ""

# Résumé final
echo "╔════════════════════════════════════════════════════════╗"
echo "║   Security Audit Summary                               ║"
echo "╚════════════════════════════════════════════════════════╝"
echo ""

if [ "${FOUND_ISSUES:-false}" = true ]; then
    log_error "CRITICAL ISSUES FOUND - Fix immediately!"
    exit 1
elif [ "${FOUND_WARNINGS:-false}" = true ]; then
    log_warn "Warnings found - Review recommended"
    exit 0
else
    log_success "No security issues detected"
    log_success "System appears to be secure"
    exit 0
fi
