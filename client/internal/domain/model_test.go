package domain

import (
	"encoding/json"
	"testing"
)

func TestResourceRef_JSON(t *testing.T) {
	resource := ResourceRef{
		Type: "ssh",
		Host: "backend.example.com",
		Port: 22,
		Name: "backend-server",
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
		Sub:      "auth0|123456",
		Username: "alice",
		Groups:   []string{"admins", "developers"},
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

func TestConnectRequest_JSON(t *testing.T) {
	req := ConnectRequest{
		Action: "connect",
		Resource: ResourceRef{
			Type: "tcp",
			Host: "10.0.1.50",
			Port: 8080,
		},
		Context: RequestContext{
			SourceIP: "192.168.1.100",
			DeviceInfo: map[string]string{
				"os": "linux",
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded ConnectRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Action != req.Action {
		t.Errorf("Action = %q, want %q", decoded.Action, req.Action)
	}
	if decoded.Resource.Host != req.Resource.Host {
		t.Errorf("Resource.Host = %q, want %q", decoded.Resource.Host, req.Resource.Host)
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
			res:     ResourceRef{Type: "ssh", Host: "10.10.20.40", Port: 22},
			wantErr: false,
		},
		{
			name:    "missing host",
			res:     ResourceRef{Type: "ssh", Port: 22},
			wantErr: true,
		},
		{
			name:    "invalid port (0)",
			res:     ResourceRef{Type: "ssh", Host: "10.10.20.40", Port: 0},
			wantErr: true,
		},
		{
			name:    "invalid port (too high)",
			res:     ResourceRef{Type: "ssh", Host: "10.10.20.40", Port: 70000},
			wantErr: true,
		},
		{
			name:    "missing type",
			res:     ResourceRef{Host: "10.10.20.40", Port: 22},
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
			subject: SubjectRef{Sub: "user-123", Username: "alice"},
			wantErr: false,
		},
		{
			name:    "missing sub",
			subject: SubjectRef{Username: "alice"},
			wantErr: true,
		},
		{
			name:    "valid with groups",
			subject: SubjectRef{Sub: "user-123", Username: "alice", Groups: []string{"admin", "users"}},
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
