// Package protocol — handler.go
//
// Handler traite les connexions mTLS entrantes selon le protocole CONNECT.
// Pour chaque connexion, il :
//  1. Vérifie la révocation du certificat (CRL)
//  2. Lit la requête CONNECT du client
//  3. Extrait l'identité du sujet depuis le certificat TLS
//  4. Consulte le cache de décisions ou appelle le CP
//  5. Si allow : établit un proxy TCP avec enforced TTL
//  6. Envoie la télémétrie de session au CP (start/end)
//  7. Si deny : retourne une erreur au client et ferme la connexion
package protocol

import (
	"context"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"time"

	authorize "ztna-gateway/internal/infra/authz"
	decisioncache "ztna-gateway/internal/infra/cache"
	"ztna-gateway/internal/config"
	"ztna-gateway/internal/core/domain"
	"ztna-gateway/internal/infra/mtls"
	"ztna-gateway/internal/infra/proxy"
	resourceclient "ztna-gateway/internal/infra/resource"
	crl "ztna-gateway/internal/infra/revocation"
	"ztna-gateway/internal/infra/session"
	"ztna-gateway/internal/infra/telemetry"
)

// Handler traite les connexions mTLS entrantes.
type Handler struct {
	authz      *authorize.Client
	proxy      *proxy.TCPProxy
	sessions   *session.Manager
	log        *slog.Logger
	crlStore   *crl.Store
	cache      *decisioncache.Cache
	cacheTTL   time.Duration
	cpDownMode string // "deny" ou "cache_allow"
	telemetry  *telemetry.Client
	cfg        *config.Config // pour résoudre les routes (legacy)
	resources  *resourceclient.Client // résolution de ressource via CP
}

// NewHandler crée un nouveau handler de protocole CONNECT.
func NewHandler(authz *authorize.Client, proxy *proxy.TCPProxy, sessions *session.Manager, log *slog.Logger) *Handler {
	return &Handler{
		authz:      authz,
		proxy:      proxy,
		sessions:   sessions,
		log:        log,
		cpDownMode: "deny",
	}
}

// SetCRLStore configure le store de révocation pour vérification post-handshake.
func (h *Handler) SetCRLStore(store *crl.Store) {
	h.crlStore = store
}

// SetDecisionCache configure le cache de décisions d'autorisation.
func (h *Handler) SetDecisionCache(cache *decisioncache.Cache, ttl time.Duration) {
	h.cache = cache
	h.cacheTTL = ttl
}

// SetCPDownMode configure le comportement quand le CP est inaccessible.
func (h *Handler) SetCPDownMode(mode string) {
	h.cpDownMode = mode
}

// SetTelemetryClient configure le client de télémétrie de session.
func (h *Handler) SetTelemetryClient(tc *telemetry.Client) {
	h.telemetry = tc
}

// SetConfig attache la configuration pour la résolution de routes.
func (h *Handler) SetConfig(cfg *config.Config) {
	h.cfg = cfg
}

// SetResourceClient configure le client de résolution de ressource via le CP.
func (h *Handler) SetResourceClient(rc *resourceclient.Client) {
	h.resources = rc
}

// HandleConnection implémente mtls.ConnectionHandler.
// Traite une connexion mTLS entrante : CRL check → CONNECT → authorize → proxy.
func (h *Handler) HandleConnection(conn net.Conn, clientCert *x509.Certificate) {
	defer conn.Close()
	remoteAddr := conn.RemoteAddr().String()

	// 1. Extraire l'identité du sujet depuis le certificat
	subject := mtls.ExtractSubjectFromCert(clientCert, h.log)
	certSerial := clientCert.SerialNumber.String()

	h.log.Info("connexion mTLS reçue",
		"sub", subject.Sub,
		"username", subject.Username,
		"groups", subject.Groups,
		"remote_addr", remoteAddr,
		"cert_cn", clientCert.Subject.CommonName,
		"cert_serial", certSerial,
		"cert_not_after", clientCert.NotAfter.Format(time.RFC3339),
	)

	// 2. Vérifier la révocation du certificat (CRL check)
	if h.crlStore != nil && h.crlStore.IsRevoked(certSerial) {
		h.log.Warn("CERTIFICAT RÉVOQUÉ — connexion rejetée",
			"sub", subject.Sub,
			"cert_serial", certSerial,
			"remote_addr", remoteAddr,
		)
		h.sendDeny(conn, "certificat révoqué", "")
		return
	}

	// 3. Lire la requête CONNECT (framing length-prefixed JSON)
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var req ConnectRequest
	if err := ReadMessage(conn, &req); err != nil {
		h.log.Warn("erreur lecture requête CONNECT",
			"sub", subject.Sub,
			"remote_addr", remoteAddr,
			"error", err,
		)
		h.sendDeny(conn, "erreur de protocole: lecture requête", "")
		return
	}
	conn.SetReadDeadline(time.Time{}) // Reset deadline

	h.log.Info("requête CONNECT reçue",
		"sub", subject.Sub,
		"action", req.Action,
		"resource_type", req.Resource.Type,
		"resource_host", req.Resource.Host,
		"resource_port", req.Resource.Port,
	)

	// 4. Valider la requête
	if req.Action != "connect" {
		h.log.Warn("action invalide", "action", req.Action, "sub", subject.Sub)
		h.sendDeny(conn, fmt.Sprintf("action non supportée: %s", req.Action), "")
		return
	}
	// Name-based resolution required; host:port fallback is forbidden.
	if req.Resource.Name == "" && (req.Resource.Host == "" || req.Resource.Port <= 0 || req.Resource.Type == "") {
		h.log.Warn("requête CONNECT incomplète", "sub", subject.Sub, "resource", req.Resource)
		h.sendDeny(conn, "requête CONNECT invalide: resource name ou host/port/type manquant", "")
		return
	}

	// 5. Résoudre la ressource publiée : priorité au nom via le CP
	var targetHost string
	var targetPort int
	resourceType := req.Resource.Type
	resourceName := req.Resource.Name
	accessMode := ""

	if resourceName != "" && h.resources != nil {
		// Name-based resolution via Control Plane
		resolved, err := h.resources.GetResource(resourceName)
		if err != nil {
			h.log.Warn("résolution ressource échouée",
				"sub", subject.Sub,
				"resource_name", resourceName,
				"error", err,
			)
			h.sendDeny(conn, fmt.Sprintf("ressource inconnue: %s", resourceName), "")
			return
		}
		// Parse backend "host:port"
		bHost, bPortStr, err := net.SplitHostPort(resolved.Backend)
		if err != nil {
			h.log.Error("backend invalide pour ressource publiée", "resource", resourceName, "backend", resolved.Backend, "error", err)
			h.sendDeny(conn, "erreur interne: backend invalide", "")
			return
		}
		var bPort int
		if _, err := fmt.Sscanf(bPortStr, "%d", &bPort); err != nil || bPort <= 0 || bPort > 65535 {
			h.log.Error("port backend invalide", "resource", resourceName, "backend", resolved.Backend)
			h.sendDeny(conn, "erreur interne: port backend invalide", "")
			return
		}
		targetHost = bHost
		targetPort = bPort
		resourceType = resolved.Type
		accessMode = resolved.AccessMode
		// Override host/port for authorize call with resolved values.
		req.Resource.Host = bHost
		req.Resource.Port = bPort
		req.Resource.Type = resourceType
		h.log.Info("ressource résolue via CP",
			"resource_name", resourceName,
			"backend", resolved.Backend,
			"type", resourceType,
			"access_mode", accessMode,
		)
	} else if req.Resource.Host != "" && req.Resource.Port > 0 {
		// Legacy host:port — only if routes are configured (no raw fallback)
		canonical := fmt.Sprintf("%s:%s:%d", req.Resource.Type, req.Resource.Host, req.Resource.Port)
		if h.cfg != nil && len(h.cfg.Routes) > 0 {
			backend, found := h.cfg.ResolveRoute(req.Resource.Type, canonical)
			if !found {
				h.log.Warn("aucune route configurée pour cette ressource",
					"sub", subject.Sub,
					"canonical", canonical,
				)
				h.sendDeny(conn, "ressource non configurée sur cette gateway", "")
				return
			}
			bHost, bPortStr, err := net.SplitHostPort(backend)
			if err != nil {
				h.log.Error("route backend invalide", "backend", backend, "error", err)
				h.sendDeny(conn, "erreur interne: route backend invalide", "")
				return
			}
			var bPort int
			if _, err := fmt.Sscanf(bPortStr, "%d", &bPort); err != nil || bPort <= 0 || bPort > 65535 {
				h.log.Error("port backend invalide", "backend", backend, "port", bPortStr)
				h.sendDeny(conn, "erreur interne: port backend invalide", "")
				return
			}
			targetHost = bHost
			targetPort = bPort
			h.log.Info("route résolue (legacy)", "canonical", canonical, "backend", backend)
		} else {
			// NO FALLBACK: direct host:port without routes is forbidden.
			h.log.Warn("accès par host:port refusé — aucune route configurée et aucun nom de ressource fourni",
				"sub", subject.Sub,
				"host", req.Resource.Host,
				"port", req.Resource.Port,
			)
			h.sendDeny(conn, "accès refusé: utilisez un nom de ressource publié", "")
			return
		}
	} else {
		h.sendDeny(conn, "requête CONNECT invalide: nom de ressource requis", "")
		return
	}

	// 6. Consulter le cache de décisions ou appeler le CP (authorize)
	decision, err := h.resolveDecision(subject, req, remoteAddr)
	if err != nil {
		h.log.Error("erreur résolution autorisation",
			"sub", subject.Sub,
			"resource", fmt.Sprintf("%s:%d", req.Resource.Host, req.Resource.Port),
			"error", err,
		)
		h.sendDeny(conn, "erreur interne: control plane inaccessible", "")
		return
	}

	h.log.Info("décision d'autorisation résolue",
		"sub", subject.Sub,
		"effect", decision.Decision,
		"decision_id", decision.DecisionID,
		"reason", decision.Reason,
		"ttl_seconds", decision.TTLSeconds,
		"policy_version", decision.PolicyVersion,
	)

	// Si deny → répondre et fermer
	if decision.Decision != "allow" {
		reason := decision.Reason
		if reason == "" {
			reason = "accès refusé par la politique"
		}
		h.log.Warn("accès REFUSÉ",
			"sub", subject.Sub,
			"resource", fmt.Sprintf("%s://%s:%d", req.Resource.Type, req.Resource.Host, req.Resource.Port),
			"reason", reason,
			"decision_id", decision.DecisionID,
		)
		h.sendDeny(conn, reason, decision.DecisionID)
		return
	}

	// 7. Créer un contexte avec timeout TTL pour enforcer la durée maximale
	var ctx context.Context
	var cancel context.CancelFunc
	if decision.TTLSeconds > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(decision.TTLSeconds)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	// 8. Enregistrer la session avec CancelFunc pour admin kill + TTL GC
	sess := &session.Session{
		Sub:          subject.Sub,
		Username:     subject.Username,
		ResourceType: req.Resource.Type,
		ResourceName: resourceName,
		ResourceHost: req.Resource.Host,
		ResourcePort: req.Resource.Port,
		SourceIP:     remoteAddr,
		DecisionID:   decision.DecisionID,
		TTLSeconds:   decision.TTLSeconds,
		CancelFunc:   cancel,
		CertSerial:   certSerial,
	}
	sessionID, err := h.sessions.Register(sess)
	if err != nil {
		h.log.Error("erreur enregistrement session", "sub", subject.Sub, "error", err)
		h.sendDeny(conn, "limite de sessions atteinte", decision.DecisionID)
		return
	}

	h.log.Info("accès AUTORISÉ — session ouverte",
		"session_id", sessionID,
		"sub", subject.Sub,
		"resource", fmt.Sprintf("%s://%s:%d", req.Resource.Type, req.Resource.Host, req.Resource.Port),
		"decision_id", decision.DecisionID,
		"ttl_seconds", decision.TTLSeconds,
	)

	// 9. Notifier le CP du début de session (fire-and-forget)
	if h.telemetry != nil {
		h.telemetry.NotifyStart(ctx, telemetry.SessionStartRequest{
			SessionID:       sessionID,
			DecisionID:      decision.DecisionID,
			SubjectSub:      subject.Sub,
			SubjectUsername: subject.Username,
			ResourceType:    req.Resource.Type,
			ResourceName:    resourceName,
			ResourceMatch:   fmt.Sprintf("%s:%s:%d", req.Resource.Type, req.Resource.Host, req.Resource.Port),
		})
	}

	// 10. Envoyer la réponse allow au client
	resp := ConnectResponse{
		Decision:   "allow",
		DecisionID: decision.DecisionID,
		TTLSeconds: decision.TTLSeconds,
	}
	if err := WriteMessage(conn, resp); err != nil {
		h.log.Error("erreur envoi réponse allow", "session_id", sessionID, "error", err)
		h.sessions.Unregister(sessionID)
		return
	}

	// 11. Proxier le trafic (ctx transporte le timeout TTL)
	startTime := time.Now()
	result := h.proxy.Proxy(ctx, conn, targetHost, targetPort)

	// 12. Fin de session — métriques et cleanup
	duration := time.Since(startTime)
	endReason := result.EndReason

	h.sessions.SetEndStats(sessionID, result.BytesIn, result.BytesOut, endReason)
	h.sessions.Unregister(sessionID)

	// 13. Notifier le CP de la fin de session
	if h.telemetry != nil {
		h.telemetry.NotifyEnd(ctx, telemetry.SessionEndRequest{
			SessionID:  sessionID,
			BytesIn:    result.BytesIn,
			BytesOut:   result.BytesOut,
			DurationMs: duration.Milliseconds(),
			EndReason:  endReason,
		})
	}

	h.log.Info("session terminée",
		"session_id", sessionID,
		"sub", subject.Sub,
		"reason", endReason,
		"duration_ms", duration.Milliseconds(),
		"bytes_in", result.BytesIn,
		"bytes_out", result.BytesOut,
	)
}

// resolveDecision consulte le cache puis le CP pour obtenir une décision.
func (h *Handler) resolveDecision(subject domain.SubjectRef, req ConnectRequest, remoteAddr string) (*authorize.AuthzResponse, error) {
	// Construire la clé de cache
	cacheKey := fmt.Sprintf("%s|%s|%s|%s|%d", subject.Sub, req.Action, req.Resource.Type, req.Resource.Host, req.Resource.Port)

	// Consulter le cache
	if h.cache != nil {
		if entry, ok := h.cache.Get(cacheKey, time.Now()); ok {
			h.log.Debug("décision servie depuis le cache", "sub", subject.Sub, "cache_key", cacheKey)
			return &authorize.AuthzResponse{
				Decision:      entry.Decision,
				Reason:        entry.Reason,
				PolicyVersion: entry.PolicyVersion,
				DecisionID:    "cached",
			}, nil
		}
	}

	// Appeler le CP
	authzReq := &authorize.AuthzRequest{
		Subject: subject,
		Action:  req.Action,
		Resource: authorize.ResourceRef{
			Type: req.Resource.Type,
			Name: req.Resource.Name,
			Host: req.Resource.Host,
			Port: req.Resource.Port,
		},
		Context: authorize.AuthzContext{
			SourceIP:  remoteAddr,
			GatewayID: h.authz.GatewayID(),
		},
	}

	decision, err := h.authz.Authorize(authzReq)
	if err != nil {
		// CP inaccessible — appliquer cp_down_mode
		if h.cpDownMode == "cache_allow" && h.cache != nil {
			// Vérifier si on a une ancienne décision dans le cache (même expirée)
			h.log.Warn("CP inaccessible — mode cache_allow, recherche dans le cache",
				"sub", subject.Sub,
				"error", err,
			)
			// En mode cache_allow, on pourrait servir une décision "allow" par défaut
			// pour les sujets déjà autorisés. Ici on laisse passer l'erreur.
		}
		return nil, err
	}

	// Mettre en cache la décision
	if h.cache != nil && decision.TTLSeconds > 0 {
		ttl := h.cacheTTL
		if ttl <= 0 {
			ttl = time.Duration(decision.TTLSeconds) * time.Second
		}
		h.cache.Put(cacheKey, decisioncache.DecisionEntry{
			Decision:      decision.Decision,
			Reason:        decision.Reason,
			PolicyVersion: decision.PolicyVersion,
		}, ttl)
	}

	return decision, nil
}

// sendDeny envoie une réponse deny au client.
func (h *Handler) sendDeny(conn net.Conn, reason, decisionID string) {
	resp := ConnectResponse{
		Decision:   "deny",
		Reason:     reason,
		DecisionID: decisionID,
	}
	if err := WriteMessage(conn, resp); err != nil {
		h.log.Debug("erreur envoi réponse deny", "error", err)
	}
}
