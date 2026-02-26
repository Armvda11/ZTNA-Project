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
	pepID    string
}

// NewHandler crée un nouveau handler de protocole CONNECT.
func NewHandler(authz *authorize.Client, proxy *proxy.TCPProxy, sessions *session.Manager, crlStore *crl.Store, log *slog.Logger, pepID string) *Handler {
	return &Handler{
		authz:    authz,
		proxy:    proxy,
		sessions: sessions,
		crl:      crlStore,
		log:      log,
		pepID:    pepID,
	}
}

// HandleConnection implémente mtls.ConnectionHandler.
// Elle traite une connexion mTLS entrante complète : CONNECT → authorize → proxy.
//
// Flux complet :
//  1. Extraire l'identité (SubjectRef) + serial depuis le certificat client
//  2. Vérifier la CRL locale (révocation immédiate avant tout)
//  3. Lire et valider la requête CONNECT
//  4. Appeler le CP (authorize)
//  5. Si allow :
//     a. Créer un contexte TTL borné par decision.TTLSeconds
//     b. Enregistrer dans le session.Manager (avec cancel func)
//     c. Notifier le CP (POST /pep/sessions/start)
//     d. Démarrer une goroutine de poll "is-session-valid" toutes les 5s
//     e. Relayer le trafic TCP via proxy
//     f. Notifier le CP de la fin (POST /pep/sessions/end)
func (h *Handler) HandleConnection(conn net.Conn, clientCert *x509.Certificate) {
	defer conn.Close()
	baseCtx := context.Background()

	// 1. Extraire l'identité du sujet depuis le certificat
	subject := mtls.ExtractSubjectFromCert(clientCert, h.log)
	deviceSerial := crl.CertSerial(clientCert)
	srcIP, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
	h.log.Info("connexion mTLS reçue",
		"sub", subject.Sub,
		"username", subject.Username,
		"device_serial", deviceSerial,
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
			GatewayID: h.pepID,
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

	// 6. Autorisation accordée — créer un contexte borné par le TTL du CP
	ttl := time.Duration(decision.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute // valeur de sécurité si le CP n'envoie pas de TTL
	}
	proxyCtx, cancel := context.WithTimeout(baseCtx, ttl)
	defer cancel()

	// 6b. Enregistrer la session avec la cancel func (permet kill admin + KillRevoked CRL)
	sess := &session.Session{
		Sub:          subject.Sub,
		Username:     subject.Username,
		DeviceSerial: deviceSerial,
		ResourceType: req.Resource.Type,
		ResourceHost: req.Resource.Host,
		ResourcePort: req.Resource.Port,
		SourceIP:     srcIP,
		DecisionID:   decision.DecisionID,
	}
	sessionID, err := h.sessions.Register(sess, cancel)
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

	// 7b. Notifier le CP de l'ouverture de la session (télémétrie)
	h.authz.SessionStart(&authorize.SessionStartRequest{
		SessionID:       sessionID,
		DecisionID:      decision.DecisionID,
		SubjectSub:      subject.Sub,
		SubjectUsername: subject.Username,
		DeviceSerial:    deviceSerial,
		ResourceType:    req.Resource.Type,
		ResourceMatch:   fmt.Sprintf("%s:%s:%d", req.Resource.Type, req.Resource.Host, req.Resource.Port),
	})

	// 7c. Goroutine de poll : vérifie toutes les 5s si la session n'a pas été tuée côté CP
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-proxyCtx.Done():
				return
			case <-ticker.C:
				if !h.authz.IsSessionValid(sessionID) {
					h.log.Info("session tuée par admin CP, fermeture du proxy",
						"session_id", sessionID, "sub", subject.Sub)
					cancel()
					return
				}
			}
		}
	}()

	h.log.Info("session autorisée, démarrage du proxy",
		"sub", subject.Sub,
		"session_id", sessionID,
		"decision_id", decision.DecisionID,
		"ttl", ttl.String(),
		"target", fmt.Sprintf("%s:%d", req.Resource.Host, req.Resource.Port),
	)

	// 8. Relayer le trafic TCP vers la ressource cible
	proxyStarted := time.Now()
	proxyErr := h.proxy.Proxy(proxyCtx, conn, req.Resource.Host, req.Resource.Port)
	durationMs := time.Since(proxyStarted).Milliseconds()

	endReason := "eof"
	if proxyErr != nil {
		if proxyCtx.Err() == context.DeadlineExceeded {
			endReason = "ttl_expired"
		} else if proxyCtx.Err() == context.Canceled {
			endReason = "revoked"
		} else {
			endReason = "error"
		}
		h.log.Debug("proxy terminé", "session_id", sessionID, "reason", endReason, "error", proxyErr)
	}

	// 8b. Notifier le CP de la fermeture de la session
	h.authz.SessionEnd(&authorize.SessionEndRequest{
		SessionID:  sessionID,
		DurationMs: durationMs,
		EndReason:  endReason,
	})

	h.log.Info("session terminée",
		"session_id", sessionID,
		"sub", subject.Sub,
		"duration_ms", durationMs,
		"end_reason", endReason,
	)
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
