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
	"crypto/x509"
	"log/slog"
	"net"

	"ztna-gateway/internal/infra/authz"
	"ztna-gateway/internal/core/domain"
	"ztna-gateway/internal/infra/mtls"
	"ztna-gateway/internal/infra/proxy"
	"ztna-gateway/internal/infra/session"
)

// Handler traite les connexions mTLS entrantes.
type Handler struct {
	authz    *authorize.Client
	proxy    *proxy.TCPProxy
	sessions *session.Manager
	log      *slog.Logger
}

// NewHandler crée un nouveau handler de protocole CONNECT.
func NewHandler(authz *authorize.Client, proxy *proxy.TCPProxy, sessions *session.Manager, log *slog.Logger) *Handler {
	return &Handler{
		authz:    authz,
		proxy:    proxy,
		sessions: sessions,
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

	// 1. Extraire l'identité du sujet depuis le certificat
	subject := mtls.ExtractSubjectFromCert(clientCert, h.log)
	h.log.Info("connexion mTLS reçue",
		"sub", subject.Sub,
		"username", subject.Username,
		"remote_addr", conn.RemoteAddr().String(),
	)

	// 2. Lire la requête CONNECT
	// TODO: implémenter ReadMessage(conn, &req) avec le framing défini
	// var req ConnectRequest
	// if err := ReadMessage(conn, &req); err != nil { ... }

	// 3. Valider la requête
	// TODO: vérifier req.Action == "connect"
	// TODO: vérifier req.Resource.Host et req.Resource.Port non vides

	// 4. Construire la requête d'autorisation
	// TODO: créer authorize.AuthzRequest à partir de subject + req

	// 5. Appeler le Control Plane
	// TODO: decision, err := h.authz.Authorize(ctx, authzReq)

	// 6. Si deny → répondre et fermer
	// TODO: if decision.Effect == "deny" { WriteMessage(conn, denyResponse); return }

	// 7. Si allow → enregistrer session et proxier
	// TODO: sessionID := h.sessions.Register(subject, req.Resource)
	// TODO: WriteMessage(conn, allowResponse)
	// TODO: h.proxy.Proxy(ctx, conn, req.Resource.Host, req.Resource.Port)
	// TODO: h.sessions.Unregister(sessionID)

	h.log.Warn("TODO: HandleConnection non implémenté — connexion fermée")
}

// validateRequest vérifie que la ConnectRequest est complète et valide.
func validateRequest(req *ConnectRequest) *domain.SubjectRef {
	// TODO: vérifier les champs obligatoires
	// TODO: retourner une erreur typée si invalide
	_ = req
	return nil
}
