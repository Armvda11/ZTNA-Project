package tunnel

import (
	"net"
	"testing"
)

func TestWriteReadMessage_RoundTrip(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	req := ConnectRequest{
		ProtocolVersion: 1,
		Action:          "connect",
		Resource: ResourceRef{
			Type: "ssh",
			Host: "10.0.30.10",
			Port: 22,
			Name: "backend-ssh",
		},
		Context: ConnectContext{
			Timestamp: "2026-01-01T00:00:00Z",
		},
	}

	// Écrire dans une goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- WriteMessage(client, req)
	}()

	// Lire côté serveur
	var received ConnectRequest
	if err := ReadMessage(server, &received); err != nil {
		t.Fatalf("ReadMessage() error: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("WriteMessage() error: %v", err)
	}

	if received.Action != "connect" {
		t.Errorf("Action=%q, want connect", received.Action)
	}
	if received.Resource.Type != "ssh" {
		t.Errorf("Resource.Type=%q, want ssh", received.Resource.Type)
	}
	if received.Resource.Host != "10.0.30.10" {
		t.Errorf("Resource.Host=%q, want 10.0.30.10", received.Resource.Host)
	}
	if received.Resource.Port != 22 {
		t.Errorf("Resource.Port=%d, want 22", received.Resource.Port)
	}
	if received.ProtocolVersion != 1 {
		t.Errorf("ProtocolVersion=%d, want 1", received.ProtocolVersion)
	}
}

func TestWriteReadMessage_ConnectResponse(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	resp := ConnectResponse{
		Decision:   "allow",
		Reason:     "policy matched",
		DecisionID: "dec-abc-123",
		TTLSeconds: 3600,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- WriteMessage(server, resp)
	}()

	var received ConnectResponse
	if err := ReadMessage(client, &received); err != nil {
		t.Fatalf("ReadMessage() error: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("WriteMessage() error: %v", err)
	}

	if received.Decision != "allow" {
		t.Errorf("Decision=%q, want allow", received.Decision)
	}
	if received.DecisionID != "dec-abc-123" {
		t.Errorf("DecisionID=%q, want dec-abc-123", received.DecisionID)
	}
	if received.TTLSeconds != 3600 {
		t.Errorf("TTLSeconds=%d, want 3600", received.TTLSeconds)
	}
}

func TestReadMessage_EmptyPayload(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Écrire un header avec taille 0
	go func() {
		_, _ = client.Write([]byte{0, 0, 0, 0})
	}()

	var dest ConnectRequest
	err := ReadMessage(server, &dest)
	if err == nil {
		t.Fatal("ReadMessage() should fail on empty payload")
	}
}

func TestReadMessage_OversizedHeader(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Écrire un header indiquant >MaxMessageSize
	go func() {
		// 2 Mo (> 1 Mo max)
		_, _ = client.Write([]byte{0, 0x20, 0, 0})
	}()

	var dest ConnectRequest
	err := ReadMessage(server, &dest)
	if err == nil {
		t.Fatal("ReadMessage() should fail on oversized message")
	}
}

func TestParseResource_SSHUri(t *testing.T) {
	ref := ParseResource("ssh://10.0.30.10:22")
	if ref.Type != "ssh" {
		t.Errorf("Type=%q, want ssh", ref.Type)
	}
	if ref.Host != "10.0.30.10" {
		t.Errorf("Host=%q, want 10.0.30.10", ref.Host)
	}
	if ref.Port != 22 {
		t.Errorf("Port=%d, want 22", ref.Port)
	}
}

func TestParseResource_HostPort(t *testing.T) {
	ref := ParseResource("10.0.30.10:8080")
	if ref.Type != "tcp" {
		t.Errorf("Type=%q, want tcp", ref.Type)
	}
	if ref.Host != "10.0.30.10" {
		t.Errorf("Host=%q, want 10.0.30.10", ref.Host)
	}
	if ref.Port != 8080 {
		t.Errorf("Port=%d, want 8080", ref.Port)
	}
}

func TestParseResource_NameOnly(t *testing.T) {
	ref := ParseResource("backend-ssh")
	if ref.Name != "backend-ssh" {
		t.Errorf("Name=%q, want backend-ssh", ref.Name)
	}
	if ref.Type != "" {
		t.Errorf("Type=%q, want empty for name-only resource", ref.Type)
	}
}

func TestParseResource_HTTPUri(t *testing.T) {
	ref := ParseResource("http://webapp.internal:8443")
	if ref.Type != "http" {
		t.Errorf("Type=%q, want http", ref.Type)
	}
	if ref.Host != "webapp.internal" {
		t.Errorf("Host=%q, want webapp.internal", ref.Host)
	}
	if ref.Port != 8443 {
		t.Errorf("Port=%d, want 8443", ref.Port)
	}
}
