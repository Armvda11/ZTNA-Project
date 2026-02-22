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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"ztna-gateway/internal/config"
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
	cfg    *config.Config
	pepCli *pep.Client
	log    *slog.Logger
}

// New creates a gateway proxy server.
func New(cfg *config.Config, pepClient *pep.Client, log *slog.Logger) *Server {
	return &Server{cfg: cfg, pepCli: pepClient, log: log}
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
	subject := extractSubject(clientCert)

	s.log.Info("client connected",
		slog.String("remote", conn.RemoteAddr().String()),
		slog.String("username", subject.Username),
		slog.String("sub", subject.Sub),
	)

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
	pepReq := buildAuthorizeRequest(subject, req)
	decision, err := s.pepCli.Authorize(ctx, pepReq)
	if err != nil {
		s.log.Error("pep authorize failed", slog.Any("err", err))
		_ = writeResponse(conn, ConnectResponse{Allowed: false, Reason: "authorization check failed"})
		return
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
		return
	}
	defer backend.Close()

	proxyTCP(conn, backend)
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

func buildAuthorizeRequest(subject pep.SubjectDTO, req ConnectRequest) pep.AuthorizeRequest {
	resource := pep.ResourceDTO{Type: req.ResourceType}
	switch req.ResourceType {
	case "ssh":
		host, port := splitHostPort(req.ResourceMatch, "ssh:", 22)
		resource.SSH = &pep.SSHResource{Host: host, Port: port}
	case "http":
		host, port := splitHostPort(req.ResourceMatch, "http:", 80)
		resource.HTTP = &pep.HTTPResource{Host: host, Port: port}
	}
	return pep.AuthorizeRequest{Subject: subject, Action: req.Action, Resource: resource}
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

func proxyTCP(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	half := func(dst, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src) //nolint:errcheck
		if tc, ok := dst.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}

	go half(a, b)
	go half(b, a)
	wg.Wait()
}
