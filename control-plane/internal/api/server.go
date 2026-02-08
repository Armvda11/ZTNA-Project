package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
	"golang.org/x/time/rate"

	"github.com/ztna/control-plane/internal/config"
	"github.com/ztna/control-plane/internal/logger"
	"github.com/ztna/control-plane/internal/sshca"
	"github.com/ztna/control-plane/internal/storage"
)

// Server represents the API server
type Server struct {
	config       *config.Config
	ca           *sshca.CA
	storage      *storage.Storage
	logger       *logger.Logger
	router       *mux.Router
	loginLimiter *ipRateLimiter
	apiLimiter   *ipRateLimiter
}

type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func newIPRateLimiter(requestsPerMinute, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rate:     rate.Limit(float64(requestsPerMinute) / 60.0),
		burst:    burst,
	}
}

func (l *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	limiter, exists := l.limiters[ip]
	if !exists {
		limiter = rate.NewLimiter(l.rate, l.burst)
		l.limiters[ip] = limiter
	}

	return limiter
}

// Claims represents JWT claims
type Claims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// NewServer creates a new API server
func NewServer(cfg *config.Config, ca *sshca.CA, store *storage.Storage, log *logger.Logger) *Server {
	s := &Server{
		config:  cfg,
		ca:      ca,
		storage: store,
		logger:  log,
		router:  mux.NewRouter(),
	}

	if cfg.Auth.RateLimit.Enabled {
		s.loginLimiter = newIPRateLimiter(cfg.Auth.RateLimit.RequestsPerMinute, cfg.Auth.RateLimit.Burst)
	}

	if cfg.API.RateLimit.Enabled {
		s.apiLimiter = newIPRateLimiter(cfg.API.RateLimit.RequestsPerMinute, cfg.API.RateLimit.Burst)
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures API routes
func (s *Server) setupRoutes() {
	// Public routes
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/api/v1/auth/login", s.handleLogin).Methods("POST")
	s.router.HandleFunc("/api/v1/auth/refresh", s.handleRefresh).Methods("POST")
	s.router.HandleFunc("/api/v1/ca/public-key", s.handleCAPublicKey).Methods("GET")

	// Protected routes (require JWT)
	api := s.router.PathPrefix("/api/v1").Subrouter()
	api.Use(s.authMiddleware)
	api.HandleFunc("/auth/logout", s.handleLogout).Methods("POST")
	api.HandleFunc("/certs/request", s.handleCertRequest).Methods("POST")

	admin := api.PathPrefix("/policies").Subrouter()
	admin.Use(s.requireAdmin)
	admin.HandleFunc("/versions", s.handleListPolicyVersions).Methods("GET")
	admin.HandleFunc("/versions", s.handleCreatePolicyVersion).Methods("POST")
	admin.HandleFunc("/versions/{id}/activate", s.handleActivatePolicyVersion).Methods("POST")
	admin.HandleFunc("/rules", s.handleListPolicyRules).Methods("GET")
	admin.HandleFunc("/rules", s.handleCreatePolicyRule).Methods("POST")
	admin.HandleFunc("/rules/{id}", s.handleUpdatePolicyRule).Methods("PUT")
	admin.HandleFunc("/rules/{id}", s.handleDeletePolicyRule).Methods("DELETE")

	adminUsers := api.PathPrefix("/users").Subrouter()
	adminUsers.Use(s.requireAdmin)
	adminUsers.HandleFunc("", s.handleListUsers).Methods("GET")
	adminUsers.HandleFunc("", s.handleCreateUser).Methods("POST")
	adminUsers.HandleFunc("/{id}", s.handleUpdateUser).Methods("PUT")
	adminUsers.HandleFunc("/{id}", s.handleDeleteUser).Methods("DELETE")

	api.HandleFunc("/policies/{resource}", s.handleCheckPolicy).Methods("GET")
	api.HandleFunc("/audit", s.handleAudit).Methods("GET")
}

// Router returns the configured router
func (s *Server) Router() http.Handler {
	return s.loggingMiddleware(s.corsMiddleware(s.router))
}

// Middleware: CORS
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !isOriginAllowed(origin, s.config.Server.CORS.AllowedOrigins) {
				s.respondError(w, http.StatusForbidden, "origin not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isOriginAllowed(origin string, allowed []string) bool {
	for _, candidate := range allowed {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(origin)) {
			return true
		}
	}
	return false
}

// Middleware: Logging
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Call the next handler
		next.ServeHTTP(w, r)

		// Log request
		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// Middleware: Authentication
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.respondError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			s.respondError(w, http.StatusUnauthorized, "invalid authorization header format")
			return
		}

		tokenString := parts[1]

		parser := jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithIssuer(s.config.Auth.Issuer),
			jwt.WithAudience(s.config.Auth.Audience),
		)

		// Parse and validate JWT
		token, err := parser.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(s.config.Auth.JWTSecret), nil
		})

		if err != nil || !token.Valid {
			s.respondError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		claims, ok := token.Claims.(*Claims)
		if !ok {
			s.respondError(w, http.StatusUnauthorized, "invalid token claims")
			return
		}

		if claims.ID == "" {
			s.respondError(w, http.StatusUnauthorized, "invalid token id")
			return
		}

		isRevoked, err := s.storage.IsTokenRevoked(claims.ID)
		if err != nil {
			s.respondError(w, http.StatusInternalServerError, "failed to validate token")
			return
		}
		if isRevoked {
			s.respondError(w, http.StatusUnauthorized, "token revoked")
			return
		}

		// Add identity to request context
		r.Header.Set("X-Username", claims.Username)
		r.Header.Set("X-Role", claims.Role)
		r.Header.Set("X-Token-ID", claims.ID)

		next.ServeHTTP(w, r)
	})
}

// Middleware: Admin authorization
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Role") != "admin" {
			s.respondError(w, http.StatusForbidden, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler: Health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"version": "0.1.0",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

// Handler: Login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.loginLimiter != nil {
		clientIP := parseClientIP(r.RemoteAddr)
		if !s.loginLimiter.getLimiter(clientIP).Allow() {
			s.respondError(w, http.StatusTooManyRequests, "too many login attempts")
			return
		}
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate credentials
	user, err := s.storage.ValidatePassword(req.Username, req.Password)
	if err != nil {
		s.logger.Warn("Login failed", "username", req.Username, "error", err)
		_ = s.storage.LogAudit(req.Username, "login", "", "failed", r.RemoteAddr, err.Error())
		s.respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	tokenString, refreshToken, expiresAt, refreshExpiresAt, err := s.issueTokens(user.Username, user.Role)
	if err != nil {
		s.logger.Error("Failed to issue tokens", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	_ = s.storage.LogAudit(user.Username, "login", "", "success", r.RemoteAddr, "")
	s.logger.Info("User logged in", "username", user.Username)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"token":              tokenString,
		"refresh_token":      refreshToken,
		"expires_at":         expiresAt.Unix(),
		"refresh_expires_at": refreshExpiresAt.Unix(),
		"username":           user.Username,
		"role":               user.Role,
	})
}

// Handler: Refresh token
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.RefreshToken == "" {
		s.respondError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	username, err := s.storage.ConsumeRefreshToken(req.RefreshToken)
	if err != nil {
		s.logger.Warn("Refresh token rejected", "error", err)
		s.respondError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	user, err := s.storage.GetUserByUsername(username)
	if err != nil {
		s.respondError(w, http.StatusUnauthorized, "invalid user")
		return
	}

	accessToken, refreshToken, expiresAt, refreshExpiresAt, err := s.issueTokens(user.Username, user.Role)
	if err != nil {
		s.logger.Error("Failed to issue refresh tokens", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	_ = s.storage.LogAudit(user.Username, "token_refresh", "", "success", r.RemoteAddr, "")

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"token":              accessToken,
		"refresh_token":      refreshToken,
		"expires_at":         expiresAt.Unix(),
		"refresh_expires_at": refreshExpiresAt.Unix(),
		"username":           user.Username,
		"role":               user.Role,
	})
}

// Handler: Logout / revoke token
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}

	_ = json.NewDecoder(r.Body).Decode(&req)

	username := r.Header.Get("X-Username")
	jti := r.Header.Get("X-Token-ID")

	if err := s.storage.RevokeToken(jti); err != nil {
		s.logger.Error("Failed to revoke token", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to revoke token")
		return
	}

	if req.RefreshToken != "" {
		if err := s.storage.RevokeRefreshToken(req.RefreshToken); err != nil {
			s.logger.Warn("Failed to revoke refresh token", "error", err)
		}
	}

	_ = s.storage.LogAudit(username, "logout", "", "success", r.RemoteAddr, "")

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "revoked",
	})
}

// Handler: List users (admin)
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	users, err := s.storage.ListUsers()
	if err != nil {
		s.logger.Error("Failed to list users", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"users": users,
	})
}

// Handler: Create user (admin)
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		s.respondError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "user"
	}
	if role != "user" && role != "admin" {
		s.respondError(w, http.StatusBadRequest, "invalid role")
		return
	}

	user, err := s.storage.CreateUserWithRole(req.Username, req.Password, req.Email, role)
	if err != nil {
		s.logger.Error("Failed to create user", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	_ = s.storage.LogAudit(r.Header.Get("X-Username"), "user_create", user.Username, "success", r.RemoteAddr, "")

	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"role":     user.Role,
			"email":    user.Email,
		},
	})
}

// Handler: Update user (admin)
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	vars := mux.Vars(r)
	userID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	var req struct {
		Email    *string `json:"email"`
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Role != nil {
		role := strings.TrimSpace(*req.Role)
		if role != "user" && role != "admin" {
			s.respondError(w, http.StatusBadRequest, "invalid role")
			return
		}
		req.Role = &role
	}

	if err := s.storage.UpdateUser(userID, storage.UserUpdateInput{
		Email:    req.Email,
		Role:     req.Role,
		Password: req.Password,
	}); err != nil {
		s.logger.Error("Failed to update user", "error", err)
		switch {
		case strings.Contains(err.Error(), "not found"):
			s.respondError(w, http.StatusNotFound, "user not found")
		case strings.Contains(err.Error(), "no fields"):
			s.respondError(w, http.StatusBadRequest, "no fields to update")
		default:
			s.respondError(w, http.StatusInternalServerError, "failed to update user")
		}
		return
	}

	_ = s.storage.LogAudit(r.Header.Get("X-Username"), "user_update", fmt.Sprintf("%d", userID), "success", r.RemoteAddr, "")

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
		"id":     userID,
	})
}

// Handler: Delete user (admin)
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	vars := mux.Vars(r)
	userID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := s.storage.DeleteUser(userID); err != nil {
		s.logger.Error("Failed to delete user", "error", err)
		if strings.Contains(err.Error(), "not found") {
			s.respondError(w, http.StatusNotFound, "user not found")
			return
		}
		s.respondError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	_ = s.storage.LogAudit(r.Header.Get("X-Username"), "user_delete", fmt.Sprintf("%d", userID), "success", r.RemoteAddr, "")

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "deleted",
		"id":     userID,
	})
}

func parseClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func (s *Server) enforceAPIRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if s.apiLimiter == nil {
		return true
	}
	clientIP := parseClientIP(r.RemoteAddr)
	if !s.apiLimiter.getLimiter(clientIP).Allow() {
		s.respondError(w, http.StatusTooManyRequests, "too many requests")
		return false
	}
	return true
}

func generateRandomToken(byteLen int) (string, error) {
	buf := make([]byte, byteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Server) issueTokens(username, role string) (string, string, time.Time, time.Time, error) {
	tokenTTL, err := s.config.Auth.TokenExpiryDuration()
	if err != nil {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf("invalid token expiry: %w", err)
	}
	refreshTTL, err := s.config.Auth.RefreshTokenExpiryDuration()
	if err != nil {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf("invalid refresh token expiry: %w", err)
	}

	expiresAt := time.Now().Add(tokenTTL)
	refreshExpiresAt := time.Now().Add(refreshTTL)

	jti, err := generateRandomToken(32)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}

	claims := &Claims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    s.config.Auth.Issuer,
			Audience:  []string{s.config.Auth.Audience},
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString([]byte(s.config.Auth.JWTSecret))
	if err != nil {
		return "", "", time.Time{}, time.Time{}, fmt.Errorf("failed to sign token: %w", err)
	}

	refreshToken, err := generateRandomToken(48)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}

	if err := s.storage.StoreRefreshToken(username, refreshToken, refreshExpiresAt); err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}

	return accessToken, refreshToken, expiresAt, refreshExpiresAt, nil
}

// Handler: Certificate request
func (s *Server) handleCertRequest(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	username := r.Header.Get("X-Username")

	var req struct {
		PublicKey string `json:"public_key"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.PublicKey == "" {
		s.respondError(w, http.StatusBadRequest, "public_key is required")
		return
	}

	// Issue SSH certificate
	certReq := sshca.CertRequest{
		Username:   username,
		PublicKey:  req.PublicKey,
		ValidUntil: s.ca.DefaultValidUntil(),
	}

	cert, err := s.ca.IssueCertificate(certReq)
	if err != nil {
		s.logger.Error("Failed to issue certificate", "username", username, "error", err)
		_ = s.storage.LogAudit(username, "cert_request", "", "failed", r.RemoteAddr, err.Error())
		s.respondError(w, http.StatusInternalServerError, "failed to issue certificate")
		return
	}

	_ = s.storage.LogAudit(username, "cert_request", "", "success", r.RemoteAddr, "")

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"certificate":   cert,
		"valid_until":   certReq.ValidUntil.Unix(),
		"ca_public_key": s.ca.PublicKey(),
	})
}

// Handler: Check policy
func (s *Server) handleCheckPolicy(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	username := r.Header.Get("X-Username")
	role := r.Header.Get("X-Role")
	vars := mux.Vars(r)
	resource := vars["resource"]

	// Check policy (simple rule-based for MVP)
	allowed := s.checkPolicy(username, role, resource)

	_ = s.storage.LogAudit(username, "policy_check", resource, map[bool]string{true: "allowed", false: "denied"}[allowed], r.RemoteAddr, "")

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"username": username,
		"resource": resource,
		"allowed":  allowed,
	})
}

// Handler: Get CA public key
func (s *Server) handleCAPublicKey(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"public_key":  s.ca.PublicKey(),
		"fingerprint": s.ca.Fingerprint(),
	})
}

// Handler: Audit logs
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	logs, err := s.storage.GetAuditLogs(100)
	if err != nil {
		s.logger.Error("Failed to get audit logs", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to retrieve audit logs")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"logs": logs,
	})
}

// Handler: List policy versions
func (s *Server) handleListPolicyVersions(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	versions, err := s.storage.ListPolicyVersions()
	if err != nil {
		s.logger.Error("Failed to list policy versions", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list policy versions")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"versions": versions,
	})
}

// Handler: Create policy version
func (s *Server) handleCreatePolicyVersion(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	var req struct {
		Description string                    `json:"description"`
		DefaultDeny bool                      `json:"default_deny"`
		Rules       []storage.PolicyRuleInput `json:"rules"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	versionID, err := s.storage.CreatePolicyVersion(req.Description, req.DefaultDeny)
	if err != nil {
		s.logger.Error("Failed to create policy version", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create policy version")
		return
	}

	for _, rule := range req.Rules {
		if err := validatePolicyRule(rule); err != nil {
			s.respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := s.storage.CreatePolicyRule(versionID, rule); err != nil {
			s.logger.Error("Failed to create policy rule", "error", err)
			s.respondError(w, http.StatusInternalServerError, "failed to create policy rule")
			return
		}
	}

	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"version_id": versionID,
	})
}

// Handler: Activate policy version
func (s *Server) handleActivatePolicyVersion(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	vars := mux.Vars(r)
	versionID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid version id")
		return
	}

	if err := s.storage.ActivatePolicyVersion(versionID); err != nil {
		s.logger.Error("Failed to activate policy version", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to activate policy version")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "active",
		"version_id": versionID,
	})
}

// Handler: List policy rules for active version
func (s *Server) handleListPolicyRules(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	active, err := s.storage.GetActivePolicyVersion()
	if err != nil {
		s.logger.Error("Failed to get active policy version", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to get active policy version")
		return
	}

	rules, err := s.storage.ListPolicyRules(active.ID)
	if err != nil {
		s.logger.Error("Failed to list policy rules", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to list policy rules")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"version_id": active.ID,
		"rules":      rules,
	})
}

// Handler: Create policy rule in active version
func (s *Server) handleCreatePolicyRule(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	active, err := s.storage.GetActivePolicyVersion()
	if err != nil {
		s.logger.Error("Failed to get active policy version", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to get active policy version")
		return
	}

	var req storage.PolicyRuleInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validatePolicyRule(req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	rule, err := s.storage.CreatePolicyRule(active.ID, req)
	if err != nil {
		s.logger.Error("Failed to create policy rule", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to create policy rule")
		return
	}

	s.respondJSON(w, http.StatusCreated, map[string]interface{}{
		"rule": rule,
	})
}

// Handler: Update policy rule
func (s *Server) handleUpdatePolicyRule(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	vars := mux.Vars(r)
	ruleID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	var req storage.PolicyRuleInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validatePolicyRule(req); err != nil {
		s.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.storage.UpdatePolicyRule(ruleID, req); err != nil {
		s.logger.Error("Failed to update policy rule", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to update policy rule")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "updated",
		"rule_id": ruleID,
	})
}

// Handler: Delete policy rule
func (s *Server) handleDeletePolicyRule(w http.ResponseWriter, r *http.Request) {
	if !s.enforceAPIRateLimit(w, r) {
		return
	}

	vars := mux.Vars(r)
	ruleID, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid rule id")
		return
	}

	if err := s.storage.DeletePolicyRule(ruleID); err != nil {
		s.logger.Error("Failed to delete policy rule", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to delete policy rule")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "deleted",
		"rule_id": ruleID,
	})
}

// checkPolicy checks if user is allowed to access resource

func validatePolicyRule(rule storage.PolicyRuleInput) error {
	if rule.SubjectType == "" || rule.Subject == "" || rule.Resource == "" {
		return fmt.Errorf("subject_type, subject, and resource are required")
	}

	switch rule.SubjectType {
	case "user", "role", "*":
		return nil
	default:
		return fmt.Errorf("invalid subject_type: %s", rule.SubjectType)
	}
}

// checkPolicy checks if user is allowed to access resource using DB-backed rules.
func (s *Server) checkPolicy(username, role, resource string) bool {
	version, err := s.storage.GetActivePolicyVersion()
	if err != nil {
		s.logger.Error("No active policy version", "error", err)
		return false
	}

	rules, err := s.storage.ListPolicyRules(version.ID)
	if err != nil {
		s.logger.Error("Failed to load policy rules", "error", err)
		return false
	}

	for _, rule := range rules {
		if !matchResource(rule.Resource, resource) {
			continue
		}

		switch rule.SubjectType {
		case "user":
			if matchSubject(rule.Subject, username) {
				return rule.Allowed
			}
		case "role":
			if matchSubject(rule.Subject, role) {
				return rule.Allowed
			}
		case "*":
			return rule.Allowed
		}
	}

	return !version.DefaultDeny
}

func matchResource(ruleResource, resource string) bool {
	return ruleResource == "*" || ruleResource == resource
}

func matchSubject(ruleSubject, subject string) bool {
	return ruleSubject == "*" || ruleSubject == subject
}

// respondJSON writes JSON response
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes error response
func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]interface{}{
		"error": message,
	})
}
