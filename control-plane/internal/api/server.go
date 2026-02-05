package api

import (
	"encoding/json"
	"net"
	"net/http"
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

	s.setupRoutes()
	return s
}

// setupRoutes configures API routes
func (s *Server) setupRoutes() {
	// Public routes
	s.router.HandleFunc("/health", s.handleHealth).Methods("GET")
	s.router.HandleFunc("/api/v1/auth/login", s.handleLogin).Methods("POST")
	s.router.HandleFunc("/api/v1/ca/public-key", s.handleCAPublicKey).Methods("GET")

	// Protected routes (require JWT)
	api := s.router.PathPrefix("/api/v1").Subrouter()
	api.Use(s.authMiddleware)
	api.HandleFunc("/certs/request", s.handleCertRequest).Methods("POST")
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
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
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

		// Parse and validate JWT
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
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

		// Add username to request context
		r.Header.Set("X-Username", claims.Username)

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

	// Generate JWT token
	expiresAt := time.Now().Add(15 * time.Minute)
	claims := &Claims{
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "ztna-cp",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.config.Auth.JWTSecret))
	if err != nil {
		s.logger.Error("Failed to sign token", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	_ = s.storage.LogAudit(user.Username, "login", "", "success", r.RemoteAddr, "")
	s.logger.Info("User logged in", "username", user.Username)

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"token":      tokenString,
		"expires_at": expiresAt.Unix(),
		"username":   user.Username,
	})
}

func parseClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// Handler: Certificate request
func (s *Server) handleCertRequest(w http.ResponseWriter, r *http.Request) {
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
	username := r.Header.Get("X-Username")
	vars := mux.Vars(r)
	resource := vars["resource"]

	// Check policy (simple rule-based for MVP)
	allowed := s.checkPolicy(username, resource)

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

// checkPolicy checks if user is allowed to access resource
func (s *Server) checkPolicy(username, resource string) bool {
	// Default deny
	if s.config.Policies.DefaultDeny {
		// Check rules
		for _, rule := range s.config.Policies.Rules {
			if rule.User == username || rule.User == "*" {
				for _, res := range rule.Resources {
					if res == resource || res == "*" {
						return rule.Allowed
					}
				}
			}
		}
		return false
	}
	return true
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
