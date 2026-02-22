package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"ztna-gateway/internal/pep"
)

// ─── routeMatch ──────────────────────────────────────────────────────────────

func TestRouteMatch_Exact(t *testing.T) {
	if !routeMatch("ssh:lan-app:22", "ssh:lan-app:22") {
		t.Error("exact match doit retourner true")
	}
}

func TestRouteMatch_CaseInsensitive(t *testing.T) {
	if !routeMatch("SSH:lan-app:22", "ssh:lan-app:22") {
		t.Error("comparaison doit être insensible à la casse")
	}
}

func TestRouteMatch_Wildcard(t *testing.T) {
	if !routeMatch("ssh:*", "ssh:lan-app:22") {
		t.Error("wildcard * doit matcher le préfixe")
	}
}

func TestRouteMatch_GlobalWildcard(t *testing.T) {
	if !routeMatch("*", "ssh:lan-app:22") {
		t.Error("wildcard seul doit tout matcher")
	}
}

func TestRouteMatch_NoMatch(t *testing.T) {
	if routeMatch("http:lan-app:80", "ssh:lan-app:22") {
		t.Error("types différents ne doivent pas matcher")
	}
}

// ─── extractSubject ───────────────────────────────────────────────────────────

func TestExtractSubject_Fields(t *testing.T) {
	cert := makeCert(t, pkix.Name{
		CommonName:   "alice",
		SerialNumber: "sub-abc-123",
		Organization: []string{"ztna-admins", "dev"},
	})

	subj := extractSubject(cert)

	if subj.Username != "alice" {
		t.Errorf("Username: got %q, want %q", subj.Username, "alice")
	}
	if subj.Sub != "sub-abc-123" {
		t.Errorf("Sub: got %q, want %q", subj.Sub, "sub-abc-123")
	}
	// L'ordre des champs Organization peut varier après parsing X.509.
	groupSet := make(map[string]bool, len(subj.Groups))
	for _, g := range subj.Groups {
		groupSet[g] = true
	}
	if len(subj.Groups) != 2 || !groupSet["ztna-admins"] || !groupSet["dev"] {
		t.Errorf("Groups: got %v, want [ztna-admins dev]", subj.Groups)
	}
}

// ─── splitHostPort ────────────────────────────────────────────────────────────

func TestSplitHostPort_WithPort(t *testing.T) {
	host, port := splitHostPort("ssh:lan-app:22", "ssh:", 22)
	if host != "lan-app" || port != 22 {
		t.Errorf("got %s:%d, want lan-app:22", host, port)
	}
}

func TestSplitHostPort_DefaultPort(t *testing.T) {
	host, port := splitHostPort("http:lan-app", "http:", 80)
	if host != "lan-app" || port != 80 {
		t.Errorf("got %s:%d, want lan-app:80", host, port)
	}
}

// ─── buildAuthorizeRequest ────────────────────────────────────────────────────

func TestBuildAuthorizeRequest_SSH(t *testing.T) {
	subj := pep.SubjectDTO{Username: "alice", Sub: "uid1", Groups: []string{"ztna-admins"}}
	req := ConnectRequest{ResourceType: "ssh", ResourceMatch: "ssh:lan-app:22", Action: "connect"}

	ar := buildAuthorizeRequest(subj, req)

	if ar.Action != "connect" {
		t.Errorf("Action: %q", ar.Action)
	}
	if ar.Resource.SSH == nil {
		t.Fatal("SSH resource doit être non-nil")
	}
	if ar.Resource.SSH.Host != "lan-app" || ar.Resource.SSH.Port != 22 {
		t.Errorf("SSH: got %s:%d", ar.Resource.SSH.Host, ar.Resource.SSH.Port)
	}
}

func TestBuildAuthorizeRequest_HTTP(t *testing.T) {
	subj := pep.SubjectDTO{Username: "alice"}
	req := ConnectRequest{ResourceType: "http", ResourceMatch: "http:lan-app:80", Action: "connect"}

	ar := buildAuthorizeRequest(subj, req)

	if ar.Resource.HTTP == nil {
		t.Fatal("HTTP resource doit être non-nil")
	}
	if ar.Resource.HTTP.Port != 80 {
		t.Errorf("HTTP port: %d", ar.Resource.HTTP.Port)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func makeCert(t *testing.T, subject pkix.Name) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	templ := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, templ, templ, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}
