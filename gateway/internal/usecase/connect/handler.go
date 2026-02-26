// Package protocol — handler.go
//
// Handler traite les connexions mTLS entrantes selon le protocole CONNECT.
// Pour chaque connexion, il :
//  1. Lit la requête CONNECT du client
//  2. Extrait l'identité du sujet depuis le certificat TLS
//  3. Appelle le Control Plane pour obtenir une décision d'autorisation
//  4. Si allow : établit le proxy TCP vers la ressource cible
//  5. Si deny : retourne une erreur au client et ferme la connexion
package protocol

import (
	"context"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"time"

	authorize "ztna-gateway/internal/infra/authz"
	"ztna-gateway/internal/infra/mtls"
	"ztna-gateway/internal/infra/proxy"
	crl "ztna-gateway/internal/infra/revocation"
	"ztna-gateway/internal/infra/session"
)

// Handler traite les connexions mTLS entrantes.
type Handler struct {
	authz    *authorize.Client
	proxy    *proxy.TCPProxy
	sessions *session.Manager
	crl      *crl.Store
	log      *slog.Logger
}

// NewHandler crée un nouveau handler de protocole CONNECT.
func NewHandler(authz *authorize.Client, proxy *proxy.TCPProxy, sessions *session.Manager, crlStore *crl.Store, log *slog.Logger) *Handler {
	return &Handler{
		authz:    authz,
		proxy:    proxy,
		sessions: sessions,
		crl:      crlStore,
		log:      log,
	}
}

// HandleConnection implémente mtls.ConnectionHandler.
// Elle traite une connexion mTLS entrante complète : CONNECT → authorize → proxy.
//
// Flux détaillé :
//  1. Extraire l'identité (SubjectRef) depuis le certificat client
//  2. Lire la requête CONNECT du client (framing length-prefixed JSON)
//  3. Valider la requête (action, resource type/host/port obligatoires)
//  4. Construire la requête d'autorisation pour le CP
//  5. Appeler authorize.Client.Authorize() avec les infos du sujet et de la ressource
//  6. Si deny :
//     - Envoyer une ConnectResponse{Decision: "deny", Reason: ...}
//     - Fermer la connexion
//     - Journaliser l'événement
//  7. Si allow :
//     - Enregistrer la session dans session.Manager
//     - Envoyer une ConnectResponse{Decision: "allow", TTLSeconds: ...}
//     - Appeler proxy.TCPProxy.Proxy() pour relayer le trafic
//     - À la fin du proxy : supprimer la session et journaliser
//
// TODO: Implémenter le traitement complet
// TODO: Ajouter des événements d'audit pour chaque décision
// TODO: Ajouter un cache de décisions (optionnel, avec TTL du CP)
// TODO: Gérer les timeouts de lecture de la requête CONNECT
// TODO: Enrichir le contexte avec src_ip du client (conn.RemoteAddr())
func (h *Handler) HandleConnection(conn net.Conn, clientCert *x509.Certificate) {
	defer conn.Close()
	ctx := context.Background()

	// 1. Extraire l'identité du sujet depuis le certificat
	subject := mtls.ExtractSubjectFromCert(clientCert, h.log)
	srcIP, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	h.log.Info("connexion mTLS reçue",
		"sub", subject.Sub,
		"username", subject.Username,
		"remote_addr", conn.RemoteAddr().String(),
	)

	// 1b. Vérifier si le certificat est révoqué (CRL locale)
	if h.crl != nil {
		serial := crl.CertSerial(clientCert)
		if h.crl.IsRevoked(serial) {
			h.log.Info("certificat révoqué refusé",
				"serial", serial,
				"sub", subject.Sub,
			)
			WriteMessage(conn, ConnectResponse{ //nolint:errcheck
				Decision: "deny",
				Reason:   "certificate revoked",
			})
			return
		}
	}

	// 2. Lire la requête CONNECT (timeout de lecture)
	conn.SetDeadline(time.Now().Add(30 * time.Second)) //nolint:errcheck
	var req ConnectRequest
	if err := ReadMessage(conn, &req); err != nil {
		h.log.Error("impossible de lire la requête CONNECT", "error", err, "sub", subject.Sub)
		return
	}
	conn.SetDeadline(time.Time{}) //nolint:errcheck — désactiver le deadline après lecture

	// 3. Valider la requête
	if req.Action != "connect" {
		h.log.Warn("action invalide", "action", req.Action, "sub", subject.Sub)
		WriteMessage(conn, ConnectResponse{Decision: "deny", Reason: fmt.Sprintf("action inconnue: %s", req.Action)}) //nolint:errcheck
		return
	}
	if req.Resource.Host == "" || req.Resource.Port == 0 {
		h.log.Warn("ressource incomplete dans le ConnectRequest", "sub", subject.Sub)
		WriteMessage(conn, ConnectResponse{Decision: "deny", Reason: "ressource incomplète"}) //nolint:errcheck
		return
	}

	h.log.Info("requête CONNECT reçue",
		"sub", subject.Sub,
		"resource_type", req.Resource.Type,
		"resource_host", req.Resource.Host,
		"resource_port", req.Resource.Port,
	)

	// 4. Construire et envoyer la requête d'autorisation au CP
	authzReq := &authorize.AuthzRequest{
		Subject: subject,
		Action:  "connect",
		Resource: authorize.ResourceRef{
			Type: req.Resource.Type,
			Host: req.Resource.Host,
			Port: req.Resource.Port,
		},
		Context: authorize.AuthzContext{
			SourceIP:  srcIP,
			GatewayID: "", // TODO: injecter le gateway_id depuis la config
		},
	}

	decision, err := h.authz.Authorize(authzReq)
	if err != nil {
		h.log.Error("erreur lors de l'appel d'autorisation", "error", err, "sub", subject.Sub)
		WriteMessage(conn, ConnectResponse{Decision: "deny", Reason: "erreur interne d'autorisation"}) //nolint:errcheck
		return
	}

	// 5. Traiter la décision
	if decision.Decision != "allow" {
		h.log.Info("connexion refusée par le CP",
			"sub", subject.Sub,
			"decision_id", decision.DecisionID,
			"reason", decision.Reason,
		)
		WriteMessage(conn, ConnectResponse{ //nolint:errcheck
			Decision:   "deny",
			Reason:     decision.Reason,
			DecisionID: decision.DecisionID,
		})
		return
	}

	// 6. Autorisation accordée : enregistrer la session
	sess := &session.Session{
		Sub:          subject.Sub,
		Username:     subject.Username,
		ResourceType: req.Resource.Type,
		ResourceHost: req.Resource.Host,
		ResourcePort: req.Resource.Port,
		SourceIP:     srcIP,
		DecisionID:   decision.DecisionID,
	}
	sessionID, err := h.sessions.Register(sess)
	if err != nil {
		h.log.Error("impossible d'enregistrer la session", "error", err, "sub", subject.Sub)
		WriteMessage(conn, ConnectResponse{Decision: "deny", Reason: "limite de sessions atteinte"}) //nolint:errcheck
		return
	}
	defer h.sessions.Unregister(sessionID)

	// 7. Envoyer la réponse "allow" au client
	if err := WriteMessage(conn, ConnectResponse{
		Decision:   "allow",
		DecisionID: decision.DecisionID,
		TTLSeconds: decision.TTLSeconds,
	}); err != nil {
		h.log.Error("impossible d'envoyer la réponse allow", "error", err, "sub", subject.Sub)
		return
	}

	h.log.Info("session autorisée, démarrage du proxy",
		"sub", subject.Sub,
		"session_id", sessionID,
		"decision_id", decision.DecisionID,
		"target", fmt.Sprintf("%s:%d", req.Resource.Host, req.Resource.Port),
	)

	// 8. Relayer le trafic TCP vers la ressource cible
	if err := h.proxy.Proxy(ctx, conn, req.Resource.Host, req.Resource.Port); err != nil {
		h.log.Debug("proxy terminé", "session_id", sessionID, "error", err)
	}

	h.log.Info("session terminée", "session_id", sessionID, "sub", subject.Sub)
}

// validateRequest vérifie que la ConnectRequest est complète et valide.
func validateRequest(req *ConnectRequest) error {
	if req.Action == "" {
		return fmt.Errorf("action manquante")
	}
	if req.Resource.Host == "" {
		return fmt.Errorf("resource.host manquant")
	}
	if req.Resource.Port <= 0 || req.Resource.Port > 65535 {
		return fmt.Errorf("resource.port invalide: %d", req.Resource.Port)
	}
	return nil
}
