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
