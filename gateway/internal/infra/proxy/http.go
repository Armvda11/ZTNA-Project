// Package proxy — http.go
//
// Stub pour le proxy HTTP Layer 7 (évolution future).
// Actuellement, seul le proxy TCP (Layer 4) est implémenté.
//
// Le proxy HTTP permettrait :
//   - Inspection des en-têtes HTTP (Host, Path, Method)
//   - Politiques d'accès basées sur l'URL (pas seulement host:port)
//   - Injection de headers d'identité (X-Forwarded-User, etc.)
//   - Terminaison TLS vers la ressource backend (re-chiffrement)
//   - Logging des requêtes HTTP individuelles (audit L7)
//   - Rate limiting par requête HTTP (pas seulement par connexion)
//
// TODO: Implémenter le proxy L7 quand les cas d'usage le justifient
// TODO: Évaluer si httputil.ReverseProxy suffit ou si un proxy custom est nécessaire
// TODO: Gérer les WebSockets et les connexions longues (streaming)
package proxy

// HTTPProxy est un placeholder pour le proxy HTTP Layer 7 futur.
type HTTPProxy struct {
	// TODO: ajouter les champs nécessaires (config, logger, etc.)
}

// TODO: Méthodes à implémenter :
//
//   func NewHTTPProxy(cfg *config.Config, log *slog.Logger) *HTTPProxy
//
//   // ProxyHTTP relaie une requête HTTP avec inspection L7.
//   func (p *HTTPProxy) ProxyHTTP(ctx context.Context, clientConn net.Conn,
//       targetHost string, targetPort int) error
//
//   // InjectIdentityHeaders ajoute les headers d'identité de l'utilisateur
//   // à la requête HTTP avant de la transmettre au backend.
//   func (p *HTTPProxy) InjectIdentityHeaders(req *http.Request, subject SubjectRef)
