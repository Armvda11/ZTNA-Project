// Package authorize — types.go
//
// Types supplémentaires pour le module d'autorisation. Ces types sont
// séparés de client.go pour faciliter l'utilisation par d'autres packages
// sans importer les dépendances HTTP.
package authorize

// DecisionEffect représente le résultat d'une décision d'autorisation.
type DecisionEffect string

const (
	// DecisionAllow indique que l'accès est autorisé.
	DecisionAllow DecisionEffect = "allow"

	// DecisionDeny indique que l'accès est refusé.
	DecisionDeny DecisionEffect = "deny"
)

// IsAllow retourne true si la décision autorise l'accès.
func (r *AuthzResponse) IsAllow() bool {
	return r != nil && DecisionEffect(r.Decision) == DecisionAllow
}

// IsDeny retourne true si la décision refuse l'accès.
func (r *AuthzResponse) IsDeny() bool {
	return r == nil || DecisionEffect(r.Decision) == DecisionDeny
}
