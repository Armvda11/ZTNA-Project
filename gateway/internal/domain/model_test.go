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

func TestResourceRef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		res     ResourceRef
		wantErr bool
	}{
		{
			name:    "valid resource",
			res:     ResourceRef{Type: "tcp", Host: "192.168.1.100", Port: 5432},
			wantErr: false,
		},
		{
			name:    "missing host",
			res:     ResourceRef{Type: "tcp", Port: 5432},
			wantErr: true,
		},
		{
			name:    "invalid port (negative)",
			res:     ResourceRef{Type: "tcp", Host: "192.168.1.100", Port: -1},
			wantErr: true,
		},
		{
			name:    "invalid port (too high)",
			res:     ResourceRef{Type: "tcp", Host: "192.168.1.100", Port: 99999},
			wantErr: true,
		},
		{
			name:    "missing type",
			res:     ResourceRef{Host: "192.168.1.100", Port: 5432},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.res.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubjectRef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		subject SubjectRef
		wantErr bool
	}{
		{
			name:    "valid subject",
			subject: SubjectRef{Sub: "oidc|user789", Username: "charlie"},
			wantErr: false,
		},
		{
			name:    "missing sub",
			subject: SubjectRef{Username: "charlie"},
			wantErr: true,
		},
		{
			name:    "valid with groups",
			subject: SubjectRef{Sub: "oidc|user789", Username: "charlie", Groups: []string{"backend"}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.subject.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
