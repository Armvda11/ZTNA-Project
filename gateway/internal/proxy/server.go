// Package proxy implements the ZTNA gateway mTLS listener and TCP proxy.
//
// Protocol (application layer over mTLS):
//  1. Client opens TLS connection presenting a Device CA-signed certificate.
//  2. Client sends a newline-terminated JSON connect request:
//     {"resource_type":"ssh","resource_match":"ssh:lan-app:22","action":"connect"}
//  3. Gateway validates the cert, extracts the ZTNA subject, resolves the route,
//     and calls the CP PEP to authorize the access.
//  4. Gateway replies with a newline-terminated JSON result:
//     {"allowed":true,"decision_id":"dec-...","reason":"rule:1"}
//  5. If allowed: raw TCP bidirectional proxy to the backend begins.
package proxy

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ztna-gateway/internal/config"
	"ztna-gateway/internal/crl"
	"ztna-gateway/internal/decisioncache"
	"ztna-gateway/internal/pep"
)

// ConnectRequest is the JSON payload sent by the ZTNA client.
type ConnectRequest struct {
	ResourceType  string `json:"resource_type"`
	ResourceMatch string `json:"resource_match"`
	Action        string `json:"action"`
}

// ConnectResponse is the JSON payload sent back to the client.
type ConnectResponse struct {
	Allowed    bool   `json:"allowed"`
	DecisionID string `json:"decision_id,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Server is the mTLS gateway server.
type Server struct {
	cfg           *config.Config
	pepCli        *pep.Client
	log           *slog.Logger
	crlCache      *crl.CRLCache        // nil = révocation non vérifiée (fail-open)
	registry      *sessionRegistry     // index serial → conns actives
	decisionCache *decisioncache.Cache // nil = cache désactivé
}

// New creates a gateway proxy server.
// crlCache peut être nil (pas d'enforcement CRL) ou un *crl.CRLCache actif.
func New(cfg *config.Config, pepClient *pep.Client, crlCache *crl.CRLCache, log *slog.Logger) *Server {
	var cache *decisioncache.Cache
	if cfg.DecisionCacheMaxKeys > 0 {
		cache = decisioncache.New(cfg.DecisionCacheMaxKeys)
	}
	return &Server{
		cfg:           cfg,
		pepCli:        pepClient,
		log:           log,
		crlCache:      crlCache,
		registry:      newSessionRegistry(),
		decisionCache: cache,
	}
}

// KillRevoked ferme toutes les connexions actives dont le serial est révoqué
// selon le cache CRL fourni. Appelé après chaque refresh CRL réussi.
func (s *Server) KillRevoked(cache *crl.CRLCache) {
	s.registry.mu.RLock()
	serials := make([]string, 0, len(s.registry.bySerial))
	for serial := range s.registry.bySerial {
		serials = append(serials, serial)
	}
	s.registry.mu.RUnlock()

	for _, serial := range serials {
		if cache.IsRevokedHex(serial) {
			s.log.Warn("killing active sessions: cert revoked",
				slog.String("serial", serial))
			s.registry.closeAll(serial)
		}
	}
}

// ── SessionRegistry ─────────────────────────────────────────────────────────

// sessionRegistry indexe les connexions TCP actives par serial de certificat
// pour permettre la coupure immédiate lors d'une révocation.
type sessionRegistry struct {
	mu       sync.RWMutex
	bySerial map[string][]net.Conn
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{bySerial: make(map[string][]net.Conn)}
}

func (r *sessionRegistry) register(serial string, conn net.Conn) {
	r.mu.Lock()
	r.bySerial[serial] = append(r.bySerial[serial], conn)
	r.mu.Unlock()
}

func (r *sessionRegistry) unregister(serial string, conn net.Conn) {
	r.mu.Lock()
	conns := r.bySerial[serial]
	newConns := conns[:0]
	for _, c := range conns {
		if c != conn {
			newConns = append(newConns, c)
		}
	}
	if len(newConns) == 0 {
		delete(r.bySerial, serial)
	} else {
		r.bySerial[serial] = newConns
	}
	r.mu.Unlock()
}

func (r *sessionRegistry) closeAll(serial string) {
	r.mu.Lock()
	conns := r.bySerial[serial]
	for _, c := range conns {
		_ = c.Close()
	}
	r.mu.Unlock()
}

// ListenAndServe starts the mTLS listener and blocks until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, tlsConfig *tls.Config) error {
	ln, err := tls.Listen("tcp", s.cfg.ListenAddr, tlsConfig)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.cfg.ListenAddr, err)
	}
	defer ln.Close()

	s.log.Info("ztna gateway listening", slog.String("addr", s.cfg.ListenAddr))

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			s.log.Warn("accept error", slog.Any("err", err))
			continue
		}
		go s.handleConn(ctx, conn)
	}
}

// handleConn traite une connexion mTLS entrante du début au bout.
// Séquence :
//  1. Compléter le handshake TLS (15 s timeout).
//  2. Vérifier la présence du certificat client (mTLS obligatoire).
//  3. Extraire le sujet ZTNA depuis le cert X.509 (CN, O, SerialNumber).
//  4. Lire le ConnectRequest JSON depuis la connexion (15 s timeout).
//  5. Résoudre la route : resource_match → adresse TCP de la cible.
//  6. Appeler le CP PEP pour obtenir allow/deny.
//  7. Envoyer ConnectResponse JSON au client.
//  8. Si allow : ouvrir la connexion TCP vers la cible et proxifier
//     le trafic bidirectionnel avec proxyTCP.
func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return
	}

	// Étape 1 : forcer le handshake mTLS avec timeout court pour éviter les
	// connexions lentes / scans de port qui bloqueraient des goroutines.
	_ = tlsConn.SetDeadline(time.Now().Add(15 * time.Second))
	if err := tlsConn.Handshake(); err != nil {
		s.log.Warn("TLS handshake failed", slog.Any("err", err))
		return
	}
	_ = tlsConn.SetDeadline(time.Time{})

	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		s.log.Warn("no client certificate", slog.String("remote", conn.RemoteAddr().String()))
		_ = writeResponse(conn, ConnectResponse{Allowed: false, Reason: "client certificate required"})
		return
	}

	clientCert := state.PeerCertificates[0]

	// Post-handshake CRL check : le handshake TLS valide la cryptographie,
	// ici on vérifie que le serial n'a pas été révoqué administrativement.
	if s.crlCache != nil && s.crlCache.IsRevoked(clientCert.SerialNumber) {
		s.log.Warn("rejected: certificate revoked",
			slog.String("serial", certSerialHex(clientCert)),
			slog.String("remote", conn.RemoteAddr().String()),
		)
		_ = writeResponse(conn, ConnectResponse{Allowed: false, Reason: "certificate revoked"})
		return
	}

	// Enregistrer la connexion dans le registry par serial pour permettre
	// la coupure immédiate si le cert est révoqué pendant la session.
	hexSerial := certSerialHex(clientCert)
	s.registry.register(hexSerial, conn)
	defer s.registry.unregister(hexSerial, conn)

	subject := extractSubject(clientCert)

	s.log.Info("client connected",
		slog.String("remote", conn.RemoteAddr().String()),
		slog.String("username", subject.Username),
		slog.String("sub", subject.Sub),
	)

	// Expect a single JSON frame (newline-terminated by clients used in this repo).
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	var req ConnectRequest
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		s.log.Warn("read connect request failed", slog.Any("err", err))
		_ = writeResponse(conn, ConnectResponse{Allowed: false, Reason: "invalid connect request"})
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	if req.Action == "" {
		req.Action = "connect"
	}

	target, err := s.resolveRoute(req.ResourceType, req.ResourceMatch)
	if err != nil {
		s.log.Warn("no route", slog.String("resource", req.ResourceMatch))
		_ = writeResponse(conn, ConnectResponse{Allowed: false, Reason: "no route for this resource"})
		return
	}

	// Étape 6 : appel synchrone au CP PEP. On transmet le sujet extrait du cert
	// X.509 et la ressource demandée. Le CP évalue les politiques et renvoie
	// effect = "allow" | "deny" avec une raison et un TTL de cache.
	decisionKey := decisioncache.Key(subject, req.Action, req.ResourceType, req.ResourceMatch)
	var decision pep.AuthorizeResponse
	if s.decisionCache != nil {
		if cached, ok := s.decisionCache.Get(decisionKey, time.Now()); ok {
			decision = cached
			s.log.Debug("authorize cache hit", slog.String("decision_id", decision.DecisionID))
		}
	}
	if decision.DecisionID == "" {
		pepReq := buildAuthorizeRequest(
			subject,
			req,
			remoteIP(conn.RemoteAddr()),
			s.cfg.GatewayID,
			hexSerial,
		)
		decision, err = s.pepCli.Authorize(ctx, pepReq)
		if err != nil {
			// Deterministic fallback policy when CP is unavailable.
			if s.cfg.CPDownMode == "cache_allow" && s.decisionCache != nil {
				if cached, ok := s.decisionCache.Get(decisionKey, time.Now()); ok {
					decision = cached
					s.log.Warn("authorize fallback to cache_allow", slog.String("decision_id", decision.DecisionID))
				} else {
					s.log.Error("pep authorize failed (cache miss)", slog.Any("err", err))
					_ = writeResponse(conn, ConnectResponse{Allowed: false, Reason: "authorization check failed"})
					return
				}
			} else {
				s.log.Error("pep authorize failed", slog.Any("err", err))
				_ = writeResponse(conn, ConnectResponse{Allowed: false, Reason: "authorization check failed"})
				return
			}
		} else if s.decisionCache != nil {
			ttl := time.Duration(decision.TTLSeconds) * time.Second
			if ttl <= 0 {
				ttl = s.cfg.DecisionCacheTTL
			}
			s.decisionCache.InvalidateOnPolicyChange(decision.PolicyVersion)
			s.decisionCache.Put(decisionKey, decision, ttl)
		}
	}

	if decision.Effect != "allow" {
		s.log.Info("access denied",
			slog.String("username", subject.Username),
			slog.String("reason", decision.Reason),
		)
		_ = writeResponse(conn, ConnectResponse{Allowed: false, Reason: decision.Reason})
		return
	}

	s.log.Info("access allowed",
		slog.String("username", subject.Username),
		slog.String("target", target),
		slog.String("decision_id", decision.DecisionID),
	)

	if err := writeResponse(conn, ConnectResponse{
		Allowed:    true,
		DecisionID: decision.DecisionID,
		Reason:     decision.Reason,
	}); err != nil {
		return
	}

	backend, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		s.log.Error("dial backend", slog.String("target", target), slog.Any("err", err))
		_ = s.pepCli.SessionEnd(ctx, pep.SessionEndRequest{
			SessionID: "", // non démarré
			EndReason: "dial_error",
		})
		return
	}
	defer backend.Close()

	// Démarrer la télémétrie de session.
	sessionID := newSessionID()
	startTime := time.Now()
	if err := s.pepCli.SessionStart(ctx, pep.SessionStartRequest{
		SessionID:       sessionID,
		DecisionID:      decision.DecisionID,
		SubjectSub:      subject.Sub,
		SubjectUsername: subject.Username,
		DeviceSerial:    hexSerial,
		ResourceType:    req.ResourceType,
		ResourceMatch:   req.ResourceMatch,
	}); err != nil {
		s.log.Warn("session_start failed (non-fatal)", slog.Any("err", err))
	}

	bytesOut, bytesIn, endReason := proxyTCPInstrumented(conn, backend)

	// Fin de session : envoyer la télémétrie au CP.
	if err := s.pepCli.SessionEnd(ctx, pep.SessionEndRequest{
		SessionID:  sessionID,
		BytesIn:    bytesIn,
		BytesOut:   bytesOut,
		DurationMs: time.Since(startTime).Milliseconds(),
		EndReason:  endReason,
	}); err != nil {
		s.log.Warn("session_end failed (non-fatal)", slog.Any("err", err))
	}

	s.log.Info("session closed",
		slog.String("session_id", sessionID),
		slog.String("username", subject.Username),
		slog.Int64("bytes_out", bytesOut),
		slog.Int64("bytes_in", bytesIn),
		slog.String("reason", endReason),
	)
}

func (s *Server) resolveRoute(resourceType, resourceMatch string) (string, error) {
	for _, r := range s.cfg.Routes {
		if strings.EqualFold(r.ResourceType, resourceType) &&
			routeMatch(r.ResourceMatch, resourceMatch) {
			return r.Target, nil
		}
	}
	return "", fmt.Errorf("no route: %s / %s", resourceType, resourceMatch)
}

func routeMatch(pattern, candidate string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(candidate, strings.TrimSuffix(pattern, "*"))
	}
	return strings.EqualFold(pattern, candidate)
}

// extractSubject reconstructs a ZTNA Subject from the X.509 client certificate.
// Convention set by CP Device CA: CN=username, SerialNumber=sub, O=groups.
func extractSubject(cert *x509.Certificate) pep.SubjectDTO {
	return pep.SubjectDTO{
		Username: cert.Subject.CommonName,
		Sub:      cert.Subject.SerialNumber,
		Groups:   cert.Subject.Organization,
	}
}

func buildAuthorizeRequest(
	subject pep.SubjectDTO,
	req ConnectRequest,
	srcIP string,
	gatewayID string,
	sessionHint string,
) pep.AuthorizeRequest {
	resource := pep.ResourceDTO{Type: req.ResourceType}
	switch req.ResourceType {
	case "ssh":
		host, port := splitHostPort(req.ResourceMatch, "ssh:", 22)
		resource.SSH = &pep.SSHResource{Host: host, Port: port}
	case "http":
		host, port := splitHostPort(req.ResourceMatch, "http:", 80)
		resource.HTTP = &pep.HTTPResource{Host: host, Port: port}
	}
	return pep.AuthorizeRequest{
		Subject:  subject,
		Action:   req.Action,
		Resource: resource,
		Context: pep.AuthorizeContext{
			SrcIP:       srcIP,
			GatewayID:   gatewayID,
			SessionHint: sessionHint,
		},
	}
}

func splitHostPort(resourceMatch, prefix string, defaultPort int) (string, int) {
	s := strings.TrimPrefix(resourceMatch, prefix)
	parts := strings.Split(s, ":")
	if len(parts) == 2 {
		var port int
		fmt.Sscanf(parts[1], "%d", &port)
		if port == 0 {
			port = defaultPort
		}
		return parts[0], port
	}
	return s, defaultPort
}

func writeResponse(w io.Writer, resp ConnectResponse) error {
	return json.NewEncoder(w).Encode(resp)
}

// proxyTCPInstrumented relaie le trafic TCP bidirectionnel entre client et backend.
// Retourne (bytesClientVersBackend, bytesBBackendVersClient, raisonFin).
// raisonFin vaut "eof" (fin normale) ou "error" (erreur réseau).
func proxyTCPInstrumented(client, backend net.Conn) (bytesOut, bytesIn int64, reason string) {
	var (
		wg        sync.WaitGroup
		rawOut    int64
		rawIn     int64
		once      sync.Once
		endReason string
	)
	setReason := func(r string) { once.Do(func() { endReason = r }) }

	wg.Add(2)
	go func() {
		defer wg.Done()
		n, err := io.Copy(backend, client)
		atomic.AddInt64(&rawOut, n)
		if err != nil {
			setReason("error")
		} else {
			setReason("eof")
		}
		if tc, ok := backend.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		n, err := io.Copy(client, backend)
		atomic.AddInt64(&rawIn, n)
		if err != nil {
			setReason("error")
		} else {
			setReason("eof")
		}
		if tc, ok := client.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	wg.Wait()
	if endReason == "" {
		endReason = "eof"
	}
	return rawOut, rawIn, endReason
}

// newSessionID génère un UUID v4 aléatoire sans dépendance externe.
func newSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// certSerialHex retourne le SerialNumber d'un certificat X.509 en hex minuscule.
func certSerialHex(cert *x509.Certificate) string {
	if cert.SerialNumber == nil {
		return "00"
	}
	b := cert.SerialNumber.Bytes()
	if len(b) == 0 {
		return "00"
	}
	return hex.EncodeToString(b)
}

// bigIntFromHex est utilisé pour debug / tests.
func bigIntFromHex(s string) *big.Int {
	n := new(big.Int)
	n.SetString(s, 16)
	return n
}

func remoteIP(addr net.Addr) string {
	if addr == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}
