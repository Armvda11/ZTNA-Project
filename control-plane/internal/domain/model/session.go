package model

// Session représente une session TCP relayée par la gateway.
// Elle est créée au moment où la gateway ouvre le canal vers la ressource
// et complétée à la fermeture (EOF, timeout, révocation ou erreur réseau).
type Session struct {
	SessionID       string `json:"session_id"`
	DecisionID      string `json:"decision_id"`
	SubjectSub      string `json:"subject_sub"`
	SubjectUsername string `json:"subject_username"`
	DeviceSerial    string `json:"device_serial"`
	ResourceType    string `json:"resource_type"`
	ResourceName    string `json:"resource_name,omitempty"`
	ResourceMatch   string `json:"resource_match"`
	StartTime       string `json:"start_time"` // RFC3339 UTC
	EndTime         string `json:"end_time,omitempty"`
	BytesIn         int64  `json:"bytes_in"`
	BytesOut        int64  `json:"bytes_out"`
	DurationMs      int64  `json:"duration_ms"`
	// EndReason : "eof" | "error" | "cert_revoked" | "dial_error"
	EndReason string `json:"end_reason,omitempty"`
}
