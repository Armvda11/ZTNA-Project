// Package domain définit les modèles de données partagés par la Gateway ZTNA.
package domain

// SubjectRef identifie un utilisateur authentifié via certificat mTLS.
type SubjectRef struct {
	// Sub est l'identifiant unique OIDC du sujet, extrait du certificat client.
	Sub string `json:"sub"`

	// Username est le nom d'utilisateur lisible.
	Username string `json:"username"`

	// Groups contient les groupes de l'utilisateur (si disponibles dans le cert).
	//
	// ⚠️  ATTENTION STALENESS : Les groupes proviennent du certificat mTLS
	// qui reflète l'état au moment de l'émission par le CP. Si les groupes
	// ont changé depuis, cette information est obsolète. Le CP re-vérifie
	// les groupes lors de la décision d'autorisation si nécessaire.
	Groups []string `json:"groups,omitempty"`
}

// ResourceRef identifie une ressource réseau cible.
type ResourceRef struct {
	Type string `json:"type"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Name string `json:"name,omitempty"`
}

// Validate vérifie qu'une ResourceRef est valide.
func (r *ResourceRef) Validate() error {
	if r.Host == "" {
		return ErrInvalidRequest
	}
	if r.Port < 1 || r.Port > 65535 {
		return ErrInvalidRequest
	}
	if r.Type == "" {
		return ErrInvalidRequest
	}
	return nil
}

// Validate vérifie qu'un SubjectRef est valide.
func (s *SubjectRef) Validate() error {
	if s.Sub == "" {
		return ErrNoIdentity
	}
	return nil
}
