package model

type PolicyVersion struct {
	ID        int64  `json:"id" yaml:"id"`
	Active    bool   `json:"active" yaml:"active"`
	CreatedAt string `json:"created_at" yaml:"created_at"`
	CreatedBy string `json:"created_by" yaml:"created_by"`
}

type PolicyRule struct {
	ID            int64  `json:"id" yaml:"id"`
	VersionID     int64  `json:"version_id" yaml:"version_id"`
	Effect        string `json:"effect" yaml:"effect"`
	SubjectMatch  string `json:"subject_match" yaml:"subject_match"`
	Action        string `json:"action" yaml:"action"`
	ResourceType  string `json:"resource_type" yaml:"resource_type"`
	ResourceMatch string `json:"resource_match" yaml:"resource_match"`
	CreatedAt     string `json:"created_at" yaml:"created_at"`

	// Conditions contextuelles optionnelles (politique context-aware)
	AllowedHours       string `json:"allowed_hours,omitempty" yaml:"allowed_hours,omitempty"`             // ex: "08:00-18:00" (vide = pas de restriction)
	RequiredDeviceTrust string `json:"required_device_trust,omitempty" yaml:"required_device_trust,omitempty"` // ex: "high" — contexte.device_trust doit être >= (vide = pas de restriction)
}

type PolicySnapshot struct {
	Version PolicyVersion `json:"version"`
	Rules   []PolicyRule  `json:"rules"`
}
