package domain

import (
	"encoding/json"
	"testing"
)

func TestResourceRef_JSON(t *testing.T) {
	resource := ResourceRef{
		Type: "tcp",
		Host: "backend-db.local",
		Port: 5432,
		Name: "postgres-prod",
	}

	// Test marshaling
	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Test unmarshaling
	var decoded ResourceRef
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Type != resource.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, resource.Type)
	}
	if decoded.Host != resource.Host {
		t.Errorf("Host = %q, want %q", decoded.Host, resource.Host)
	}
	if decoded.Port != resource.Port {
		t.Errorf("Port = %d, want %d", decoded.Port, resource.Port)
	}
	if decoded.Name != resource.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, resource.Name)
	}
}

func TestSubjectRef_JSON(t *testing.T) {
	subject := SubjectRef{
		Sub:      "oidc|user456",
		Username: "bob",
		Groups:   []string{"operators", "infra"},
	}

	data, err := json.Marshal(subject)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded SubjectRef
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Sub != subject.Sub {
		t.Errorf("Sub = %q, want %q", decoded.Sub, subject.Sub)
	}
	if decoded.Username != subject.Username {
		t.Errorf("Username = %q, want %q", decoded.Username, subject.Username)
	}
	if len(decoded.Groups) != len(subject.Groups) {
		t.Errorf("len(Groups) = %d, want %d", len(decoded.Groups), len(subject.Groups))
	}
}
