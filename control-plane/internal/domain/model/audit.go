package model

type AuditEvent struct {
	ID            int64  `json:"id"`
	Timestamp     string `json:"ts"`
	Subject       string `json:"subject"`
	Action        string `json:"action"`
	Resource      string `json:"resource"`
	ResourceName  string `json:"resource_name,omitempty"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
	PepID         string `json:"pep_id"`
	SourceIP      string `json:"src_ip"`
	PolicyVersion int64  `json:"policy_version"`
}
