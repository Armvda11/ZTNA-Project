package model

type DecisionEffect string

const (
	DecisionAllow DecisionEffect = "allow"
	DecisionDeny  DecisionEffect = "deny"
)

type Decision struct {
	Effect        DecisionEffect `json:"effect"`
	Reason        string         `json:"reason"`
	TTLSeconds    int            `json:"ttl_seconds"`
	PolicyVersion int64          `json:"policy_version"`
	DecisionID    string         `json:"decision_id"`
}
