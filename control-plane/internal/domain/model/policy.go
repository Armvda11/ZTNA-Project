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
}

type PolicySnapshot struct {
	Version PolicyVersion `json:"version"`
	Rules   []PolicyRule  `json:"rules"`
}
