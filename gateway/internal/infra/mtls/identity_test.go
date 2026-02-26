package mtls

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"log/slog"
	"net/url"
	"os"
	"testing"
)

func TestExtractSubjectFromCert_CommonName(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "auth0|user123",
		},
	}

	subject := ExtractSubjectFromCert(cert, log)

	if subject.Sub != "auth0|user123" {
		t.Errorf("Sub = %q, want %q", subject.Sub, "auth0|user123")
	}
	if subject.Username != "auth0|user123" {
		t.Errorf("Username = %q, want %q", subject.Username, "auth0|user123")
	}
}

func TestExtractSubjectFromCert_SANURI(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	oidcURI, _ := url.Parse("oidc:auth0|user456")
	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "alice",
		},
		URIs: []*url.URL{oidcURI},
	}

	subject := ExtractSubjectFromCert(cert, log)

	// SAN URI a la priorité
	if subject.Sub != "auth0|user456" {
		t.Errorf("Sub = %q, want %q", subject.Sub, "auth0|user456")
	}
	// Username devrait être le CN car différent du sub
	if subject.Username != "alice" {
		t.Errorf("Username = %q, want %q", subject.Username, "alice")
	}
}

func TestExtractSubjectFromCert_SANDNS(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "bob",
		},
		DNSNames: []string{"user789.oidc.local"},
	}

	subject := ExtractSubjectFromCert(cert, log)

	// SAN DNS a la priorité sur CN
	if subject.Sub != "user789.oidc.local" {
		t.Errorf("Sub = %q, want %q", subject.Sub, "user789.oidc.local")
	}
	if subject.Username != "bob" {
		t.Errorf("Username = %q, want %q", subject.Username, "bob")
	}
}

func TestExtractSubjectFromCert_Empty(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cert := &x509.Certificate{
		Subject: pkix.Name{},
	}

	subject := ExtractSubjectFromCert(cert, log)

	if subject.Sub != "" {
		t.Errorf("Sub = %q, want empty", subject.Sub)
	}
	if subject.Username != "" {
		t.Errorf("Username = %q, want empty", subject.Username)
	}
}
